package sub

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/service"
	"github.com/alireza0/s-ui/util"

	"gopkg.in/yaml.v3"
)

type ClashService struct {
	service.SettingService
	JsonService
	LinkService
}

type clashSelectorGroupConfig struct {
	Name            string
	DefaultOutbound string
}

const (
	clashNodeSelectorTag         = "节点选择"
	clashAutoSelectorTag         = "自动选择"
	clashGlobalDirectSelectorTag = "全球直连"
	clashGlobalBlockSelectorTag  = "全球拦截"
	clashFinalSelectorTag        = "漏网之鱼"
	clashGlobalSelectorTag       = "GLOBAL"
	defaultLatencyURL            = "http://www.gstatic.com/generate_204"
	defaultLatencyInterval       = 180
	defaultLatencyTolerance      = 50
)

var legacyClashSelectorTagAliases = map[string]string{
	"🚀 节点选择":           clashNodeSelectorTag,
	"🚀节点选择":            clashNodeSelectorTag,
	"\\U0001F680 节点选择": clashNodeSelectorTag,
	"\\U0001F680节点选择":  clashNodeSelectorTag,
	"🎈 自动选择":           clashAutoSelectorTag,
	"🎈自动选择":            clashAutoSelectorTag,
	"\\U0001F388 自动选择": clashAutoSelectorTag,
	"\\U0001F388自动选择":  clashAutoSelectorTag,
	"🎯 全球直连":           clashGlobalDirectSelectorTag,
	"🎯全球直连":            clashGlobalDirectSelectorTag,
	"\\U0001F3AF 全球直连": clashGlobalDirectSelectorTag,
	"\\U0001F3AF全球直连":  clashGlobalDirectSelectorTag,
	"🛑 全球拦截":           clashGlobalBlockSelectorTag,
	"🛑全球拦截":            clashGlobalBlockSelectorTag,
	"\\U0001F6D1 全球拦截": clashGlobalBlockSelectorTag,
	"\\U0001F6D1全球拦截":  clashGlobalBlockSelectorTag,
	"🐟 漏网之鱼":           clashFinalSelectorTag,
	"🐟漏网之鱼":            clashFinalSelectorTag,
	"\\U0001F41F 漏网之鱼": clashFinalSelectorTag,
	"\\U0001F41F漏网之鱼":  clashFinalSelectorTag,
}

func normalizeLegacyClashSelectorTag(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if normalized, exists := legacyClashSelectorTagAliases[trimmed]; exists {
		return normalized
	}
	return trimmed
}

func replaceLegacyClashSelectorTagsInString(raw string) string {
	replaced := raw
	for legacy, normalized := range legacyClashSelectorTagAliases {
		replaced = strings.ReplaceAll(replaced, legacy, normalized)
	}
	return replaced
}

func sanitizeLegacyClashSelectorValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		normalized := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			normalized[key] = sanitizeLegacyClashSelectorValue(item)
		}
		return normalized
	case []interface{}:
		normalized := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			normalized = append(normalized, sanitizeLegacyClashSelectorValue(item))
		}
		return normalized
	case string:
		return replaceLegacyClashSelectorTagsInString(typed)
	default:
		return typed
	}
}

const basicClashConfig = `mixed-port: 7890
allow-lan: false
mode: rule
log-level: info
external-controller: 127.0.0.1:9090
tun:
  enable: true
  stack: system
  auto-route: true
  auto-detect-interface: true
  dns-hijack:
    - any:53
dns:
  enable: true
  ipv6: false
  enhanced-mode: fake-ip
  fake-ip-range: 198.18.0.1/15
  default-nameserver:
    - udp://223.5.5.5
    - udp://223.6.6.6
  nameserver:
    - "udp://8.8.8.8#\u8282\u70b9\u9009\u62e9"
    - "tcp://8.8.8.8#\u8282\u70b9\u9009\u62e9"
  fallback:
    - "udp://8.8.4.4#\u8282\u70b9\u9009\u62e9"
    - "tcp://8.8.4.4#\u8282\u70b9\u9009\u62e9"
  proxy-server-nameserver:
    - udp://223.5.5.5
    - udp://223.6.6.6
  fake-ip-filter:
    - "*.lan"
    - localhost
    - "*.local"
rules:
  - GEOIP,Private,DIRECT
  - MATCH,节点选择
`

func (s *ClashService) GetClash(subId string) (*string, []string, error) {
	client, inDatas, err := s.getData(subId)
	if err != nil {
		return nil, nil, err
	}

	return s.buildClashSubscription(client, inDatas, false)
}

func (s *ClashService) GetMihomoClash(subId string) (*string, []string, error) {
	client, inDatas, err := loadMihomoSubscriptionData(subId)
	if err != nil {
		return nil, nil, err
	}

	return s.buildClashSubscription(client, inDatas, true)
}

func normalizeMihomoClashSubscriptionOutbounds(outbounds *[]map[string]interface{}) {
	if outbounds == nil {
		return
	}
	for i := range *outbounds {
		outbound := &(*outbounds)[i]
		outType := strings.TrimSpace(firstString((*outbound)["type"]))
		switch outType {
		case "tuic":
			delete(*outbound, "fast_open")
			delete(*outbound, "network")
		case "hysteria2":
			delete(*outbound, "fast_open")
		}
	}
}

func (s *ClashService) buildClashSubscription(client *model.Client, inDatas []*model.Inbound, isMihomo bool) (*string, []string, error) {
	namespace := "default"
	if isMihomo {
		namespace = "mihomo"
	}

	outbounds, outTags, err := s.getOutboundsForNamespace(client.Name, client.Config, inDatas, namespace)
	if err != nil {
		return nil, nil, err
	}
	if isMihomo {
		normalizeMihomoClashSubscriptionOutbounds(outbounds)
	}

	links := s.LinkService.GetLinks(&client.Links, "external", "")
	tagNumEnable := 0
	if len(links) > 1 {
		tagNumEnable = 1
	}
	for index, link := range links {
		json, tag, err := util.GetOutbound(link, (index+1)*tagNumEnable)
		if err == nil && len(tag) > 0 {
			*outbounds = append(*outbounds, *json)
			*outTags = append(*outTags, tag)
		}
	}

	// Keep behavior aligned with JSON subscription:
	// apply client-level serverIp override to all real proxy outbounds.
	s.JsonService.overrideServerIP(outbounds, client.ServerIp)

	// 读取并处理 clash 配置
	othersStr, latencyUrl, latencyInterval, latencyTolerance, selectorGroups, err := s.getClashConfigClean()
	if err != nil || len(othersStr) == 0 {
		othersStr = basicClashConfig
		latencyUrl = defaultLatencyURL
		latencyInterval = defaultLatencyInterval
		latencyTolerance = defaultLatencyTolerance
		selectorGroups = nil
	}

	result, err := s.convertToClashMeta(outbounds, latencyUrl, latencyInterval, latencyTolerance, selectorGroups, isMihomo)
	if err != nil {
		return nil, nil, err
	}
	resultStr := othersStr + "\n" + string(result)

	updateInterval, _ := s.SettingService.GetSubUpdates()
	headers := util.GetHeaders(client, updateInterval)

	return &resultStr, headers, nil
}

// getClashConfigClean 读取 subClashExt，清理 _uiConfig，提取延迟测试参数和命名策略组。
func (s *ClashService) getClashConfigClean() (string, string, int, int, []clashSelectorGroupConfig, error) {
	subClashExt, err := s.SettingService.GetSubClashExt()
	if err != nil {
		return "", "", 0, 0, nil, err
	}

	if len(subClashExt) == 0 {
		return "", "", 0, 0, nil, nil
	}

	// 默认值
	latencyUrl := defaultLatencyURL
	latencyInterval := defaultLatencyInterval
	latencyTolerance := defaultLatencyTolerance
	var selectorGroups []clashSelectorGroupConfig

	// 解析 YAML
	var clashConfig map[string]interface{}
	err = yaml.Unmarshal([]byte(subClashExt), &clashConfig)
	if err != nil {
		// 解析失败，返回原始字符串
		return replaceLegacyClashSelectorTagsInString(subClashExt), latencyUrl, latencyInterval, latencyTolerance, nil, nil
	}
	if normalizedConfig, ok := sanitizeLegacyClashSelectorValue(clashConfig).(map[string]interface{}); ok && normalizedConfig != nil {
		clashConfig = normalizedConfig
	}

	// 从 _uiConfig 提取延迟测试参数
	if uiConfig, ok := clashConfig["_uiConfig"].(map[string]interface{}); ok {
		selectorGroups = parseClashSelectorGroupsFromUI(uiConfig)
		if u, ok := uiConfig["latencyTestUrl"].(string); ok && u != "" {
			latencyUrl = u
		}
		if i, ok := uiConfig["latencyTestInterval"].(string); ok && i != "" {
			// mihomo latency-test interval: only accepts seconds with "s" suffix.
			parsed := parseMihomoLatencyIntervalSeconds(i)
			if parsed > 0 {
				latencyInterval = parsed
			}
		}
		if t, ok := uiConfig["latencyTolerance"].(string); ok && t != "" {
			val := 0
			fmt.Sscanf(t, "%d", &val)
			if val > 0 {
				latencyTolerance = val
			}
		}
		// 兼容数字类型
		if t, ok := uiConfig["latencyTolerance"].(int); ok && t > 0 {
			latencyTolerance = t
		}
	}

	// 删除 _uiConfig（不应出现在最终订阅输出中）
	delete(clashConfig, "_uiConfig")
	if normalized, ok := normalizeNumericTypesForYAML(clashConfig).(map[string]interface{}); ok && normalized != nil {
		clashConfig = normalized
	}

	// 重新序列化为 YAML
	cleanYaml, err := yaml.Marshal(clashConfig)
	if err != nil {
		return subClashExt, latencyUrl, latencyInterval, latencyTolerance, selectorGroups, nil
	}

	return string(cleanYaml), latencyUrl, latencyInterval, latencyTolerance, selectorGroups, nil
}

// parseMihomoLatencyIntervalSeconds parses latency-test interval for mihomo.
// Supported format: "<positive_integer>s", e.g. "30s", "300s".
func parseMihomoLatencyIntervalSeconds(s string) int {
	raw := strings.TrimSpace(strings.ToLower(s))
	if raw == "" || !strings.HasSuffix(raw, "s") {
		return 0
	}

	num := strings.TrimSpace(strings.TrimSuffix(raw, "s"))
	if num == "" {
		return 0
	}

	val, err := strconv.Atoi(num)
	if err != nil || val <= 0 {
		return 0
	}
	return val
}

// parseClashInterval 解析间隔字符串，返回秒数
// 支持格式：纯数字（秒）, "300s", "5m", "1h"
func parseClashInterval(s string) int {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return 0
	}

	// 尝试纯数字
	val := 0
	n, _ := fmt.Sscanf(s, "%d", &val)
	if n > 0 {
		unit := strings.TrimLeft(s, "0123456789 ")
		unit = strings.TrimSpace(unit)
		switch strings.ToLower(unit) {
		case "h":
			return val * 3600
		case "m":
			return val * 60
		case "s", "":
			return val
		case "d":
			return val * 86400
		}
	}
	return 0
}

func parseClashSelectorGroupsFromUI(uiConfig map[string]interface{}) []clashSelectorGroupConfig {
	if uiConfig == nil {
		return nil
	}

	reserved := map[string]struct{}{
		"Proxy":                      {},
		"Auto":                       {},
		"Global-Proxy":               {},
		"Global-Direct":              {},
		clashNodeSelectorTag:         {},
		clashAutoSelectorTag:         {},
		clashGlobalDirectSelectorTag: {},
		clashGlobalBlockSelectorTag:  {},
		clashFinalSelectorTag:        {},
		clashGlobalSelectorTag:       {},
		"DIRECT":                     {},
		"REJECT":                     {},
	}

	seen := make(map[string]struct{})
	groups := make([]clashSelectorGroupConfig, 0)
	addGroup := func(name string, defaultOutbound string) {
		normalizedName := normalizeLegacyClashSelectorTag(name)
		if normalizedName == "" {
			return
		}
		if _, exists := reserved[normalizedName]; exists {
			return
		}
		if _, exists := seen[normalizedName]; exists {
			return
		}
		seen[normalizedName] = struct{}{}
		groups = append(groups, clashSelectorGroupConfig{
			Name:            normalizedName,
			DefaultOutbound: normalizeClashSelectorDefaultOutbound(defaultOutbound),
		})
	}

	if rawGroups, ok := uiConfig["clashSelectorGroups"].([]interface{}); ok {
		for _, raw := range rawGroups {
			groupMap, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			name, _ := groupMap["name"].(string)
			if strings.TrimSpace(name) == "" {
				if fallback, ok := groupMap["tag"].(string); ok {
					name = fallback
				}
			}

			defaultOutbound, _ := groupMap["defaultOutbound"].(string)
			if strings.TrimSpace(defaultOutbound) == "" {
				if fallback, ok := groupMap["default_outbound"].(string); ok {
					defaultOutbound = fallback
				}
			}
			if strings.TrimSpace(defaultOutbound) == "" {
				if fallback, ok := groupMap["default"].(string); ok {
					defaultOutbound = fallback
				}
			}

			addGroup(name, defaultOutbound)
		}
	}

	// Backward compatibility: infer named selector groups from rule rows.
	if len(groups) == 0 {
		if rawRows, ok := uiConfig["clashRuleRows"].([]interface{}); ok {
			for _, raw := range rawRows {
				rowMap, ok := raw.(map[string]interface{})
				if !ok {
					continue
				}
				name, _ := rowMap["name"].(string)
				if strings.TrimSpace(name) == "" {
					if fallback, ok := rowMap["selectorName"].(string); ok {
						name = fallback
					}
				}
				addGroup(name, "Proxy")
			}
		}
	}

	return groups
}

func normalizeClashSelectorDefaultOutbound(outbound string) string {
	normalized := normalizeLegacyClashSelectorTag(outbound)
	if normalized == "" {
		return clashNodeSelectorTag
	}
	switch strings.ToLower(normalized) {
	case "proxy":
		return clashNodeSelectorTag
	case "auto":
		return clashAutoSelectorTag
	case "direct", "global-direct":
		return clashGlobalDirectSelectorTag
	case "global-proxy":
		return clashGlobalSelectorTag
	case "reject", "block":
		return clashNodeSelectorTag
	default:
		return normalized
	}
}

func buildNamedClashProxyGroups(groups []clashSelectorGroupConfig, nodeTags []string) []map[string]interface{} {
	if len(groups) == 0 {
		return nil
	}

	result := make([]map[string]interface{}, 0, len(groups))
	for _, group := range groups {
		name := strings.TrimSpace(group.Name)
		if name == "" {
			continue
		}
		result = append(result, map[string]interface{}{
			"name":    name,
			"type":    "select",
			"proxies": buildNamedClashProxyOptions(group.DefaultOutbound, nodeTags),
		})
	}

	return result
}

func buildClashSelectorOptions(base []string, nodeTags []string) []string {
	result := make([]string, 0, len(base)+len(nodeTags))
	seen := make(map[string]struct{}, len(base)+len(nodeTags))
	add := func(tag string) {
		normalized := normalizeLegacyClashSelectorTag(tag)
		if normalized == "" {
			return
		}
		if _, exists := seen[normalized]; exists {
			return
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}

	for _, tag := range base {
		add(tag)
	}
	for _, tag := range nodeTags {
		add(tag)
	}

	return result
}

func buildFixedMihomoProxyGroups(nodeTags []string, latencyUrl string, latencyInterval int, latencyTolerance int) []map[string]interface{} {
	return []map[string]interface{}{
		{
			"name":    clashNodeSelectorTag,
			"type":    "select",
			"proxies": buildClashSelectorOptions([]string{clashAutoSelectorTag}, nodeTags),
		},
		{
			"name":      clashAutoSelectorTag,
			"type":      "url-test",
			"proxies":   nodeTags,
			"url":       latencyUrl,
			"interval":  latencyInterval,
			"tolerance": latencyTolerance,
		},
		{
			"name":    clashGlobalDirectSelectorTag,
			"type":    "select",
			"proxies": buildClashSelectorOptions([]string{"DIRECT", "REJECT"}, nodeTags),
		},
		{
			"name":    clashGlobalBlockSelectorTag,
			"type":    "select",
			"proxies": buildClashSelectorOptions([]string{"REJECT", "DIRECT"}, nodeTags),
		},
		{
			"name":    clashFinalSelectorTag,
			"type":    "select",
			"proxies": buildClashSelectorOptions([]string{clashNodeSelectorTag, clashGlobalDirectSelectorTag}, nodeTags),
		},
		{
			"name": clashGlobalSelectorTag,
			"type": "select",
			"proxies": buildClashSelectorOptions([]string{
				clashNodeSelectorTag,
				clashAutoSelectorTag,
				clashGlobalDirectSelectorTag,
				clashGlobalBlockSelectorTag,
				clashFinalSelectorTag,
			}, nodeTags),
		},
	}
}

func buildNamedClashProxyOptions(defaultOutbound string, nodeTags []string) []string {
	base := []string{
		clashNodeSelectorTag,
		clashAutoSelectorTag,
		clashGlobalDirectSelectorTag,
		clashGlobalBlockSelectorTag,
		clashFinalSelectorTag,
	}

	result := make([]string, 0, len(base)+len(nodeTags)+1)
	seen := make(map[string]struct{}, len(base)+len(nodeTags)+1)
	add := func(tag string) {
		normalized := normalizeLegacyClashSelectorTag(tag)
		if normalized == "" {
			return
		}
		if _, exists := seen[normalized]; exists {
			return
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}

	add(normalizeClashSelectorDefaultOutbound(defaultOutbound))
	for _, tag := range base {
		add(tag)
	}
	for _, tag := range nodeTags {
		add(tag)
	}

	return result
}

func (s *ClashService) ConvertToClashMeta(outbounds *[]map[string]interface{}, latencyUrl string, latencyInterval int, latencyTolerance int, selectorGroups []clashSelectorGroupConfig) ([]byte, error) {
	return s.convertToClashMeta(outbounds, latencyUrl, latencyInterval, latencyTolerance, selectorGroups, false)
}

func (s *ClashService) convertToClashMeta(outbounds *[]map[string]interface{}, latencyUrl string, latencyInterval int, latencyTolerance int, selectorGroups []clashSelectorGroupConfig, isMihomo bool) ([]byte, error) {
	proxies := make([]interface{}, 0, len(*outbounds))
	proxyTags := make([]string, 0, len(*outbounds))
	outboundByTag := make(map[string]map[string]interface{}, len(*outbounds))
	for _, outbound := range *outbounds {
		tag, _ := outbound["tag"].(string)
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		outboundByTag[tag] = outbound
	}

	for _, obMap := range *outbounds {
		outType, _ := obMap["type"].(string)
		if !util.SupportsMihomoSubscriptionOutboundType(outType) {
			continue
		}

		tag, _ := obMap["tag"].(string)
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}

		if outType == "shadowsocks" {
			detourTag, _ := obMap["detour"].(string)
			detourTag = strings.TrimSpace(detourTag)
			if detourTag != "" {
				if detourOutbound, exists := outboundByTag[detourTag]; exists {
					if detourType, _ := detourOutbound["type"].(string); detourType == "shadowtls" {
						if proxy, ok := buildClashShadowTLSSSProxy(obMap, detourOutbound, isMihomo); ok {
							proxies = append(proxies, proxy)
							proxyTags = append(proxyTags, tag)
							continue
						}
					}
				}
			}
		}

		server, _ := obMap["server"].(string)
		server = strings.TrimSpace(server)
		serverPort, ok := toInt(obMap["server_port"])
		if outType == "ssh" && (!ok || serverPort <= 0) {
			serverPort = 22
			ok = true
		}
		portRange := strings.TrimSpace(firstString(obMap["port_range"]))
		if server == "" {
			continue
		}
		if outType == "mieru" {
			if portRange == "" && (!ok || serverPort <= 0) {
				continue
			}
		} else if !ok || serverPort <= 0 {
			continue
		}

		proxy := map[string]interface{}{
			"name":   tag,
			"type":   outType,
			"server": server,
		}
		if outType == "mieru" && portRange != "" {
			proxy["port-range"] = portRange
		} else {
			proxy["port"] = serverPort
		}

		switch outType {
		case "vmess":
			if uuid, ok := obMap["uuid"].(string); ok && uuid != "" {
				proxy["uuid"] = uuid
			}
			if alterID, ok := toInt(obMap["alter_id"]); ok {
				proxy["alterId"] = alterID
			} else {
				proxy["alterId"] = 0
			}
			if cipher, ok := obMap["security"].(string); ok && cipher != "" {
				proxy["cipher"] = cipher
			} else {
				proxy["cipher"] = "auto"
			}
			if packetEncoding, ok := obMap["packet_encoding"].(string); ok && packetEncoding != "" && packetEncoding != "none" {
				proxy["packet-encoding"] = packetEncoding
			}
			if globalPadding, ok := toBool(obMap["global_padding"]); ok {
				proxy["global-padding"] = globalPadding
			}
			if authenticatedLength, ok := toBool(obMap["authenticated_length"]); ok {
				proxy["authenticated-length"] = authenticatedLength
			}
			if network, ok := obMap["network"].(string); ok && network != "" && network != "tcp" {
				proxy["udp"] = true
			}
		case "vless":
			if uuid, ok := obMap["uuid"].(string); ok && uuid != "" {
				proxy["uuid"] = uuid
			}
			if flow, ok := obMap["flow"].(string); ok && flow != "" {
				proxy["flow"] = flow
			}
			if packetEncoding, ok := obMap["packet_encoding"].(string); ok && packetEncoding != "" && packetEncoding != "none" {
				proxy["packet-encoding"] = packetEncoding
			}
			if encryption, ok := obMap["encryption"].(string); ok && encryption != "" {
				proxy["encryption"] = encryption
			}
			if network, ok := obMap["network"].(string); ok && network != "" && network != "tcp" {
				proxy["udp"] = true
			}
		case "trojan":
			if password, ok := obMap["password"].(string); ok && password != "" {
				proxy["password"] = password
			}
			if network, ok := obMap["network"].(string); ok && network != "" && network != "tcp" {
				proxy["udp"] = true
			}
			if ssOpts, ok := obMap["ss_opts"].(map[string]interface{}); ok {
				mapped := map[string]interface{}{}
				if enabled, ok := toBool(ssOpts["enabled"]); ok {
					mapped["enabled"] = enabled
				}
				if method, ok := ssOpts["method"].(string); ok && method != "" {
					mapped["method"] = method
				}
				if password, ok := ssOpts["password"].(string); ok && password != "" {
					mapped["password"] = password
				}
				if len(mapped) > 0 {
					proxy["ss-opts"] = mapped
				}
			}
		case "tuic":
			token, _ := obMap["token"].(string)
			token = strings.TrimSpace(token)
			if token != "" {
				proxy["token"] = token
			} else {
				if uuid, ok := obMap["uuid"].(string); ok && uuid != "" {
					proxy["uuid"] = uuid
				}
				if password, ok := obMap["password"].(string); ok && password != "" {
					proxy["password"] = password
				}
			}
			if cc, ok := obMap["congestion_control"].(string); ok && cc != "" {
				proxy["congestion-controller"] = cc
			}
			if relayMode, ok := obMap["udp_relay_mode"].(string); ok && relayMode != "" {
				proxy["udp-relay-mode"] = relayMode
			}
			if reduceRTT, ok := toBool(obMap["zero_rtt_handshake"]); ok {
				proxy["reduce-rtt"] = reduceRTT
			}
			if timeoutMS, ok := durationToMilliseconds(obMap["request_timeout"]); ok {
				proxy["request-timeout"] = timeoutMS
			} else if timeoutMS, ok := durationToMilliseconds(obMap["auth_timeout"]); ok {
				proxy["request-timeout"] = timeoutMS
			}
			if heartbeatMS, ok := durationToMilliseconds(obMap["heartbeat"]); ok {
				proxy["heartbeat-interval"] = heartbeatMS
			}
			if maxOpenStreams, ok := toInt(obMap["max_open_streams"]); ok && maxOpenStreams > 0 {
				proxy["max-open-streams"] = maxOpenStreams
			}
			if maxUDPPacket, ok := toInt(obMap["max_udp_relay_packet_size"]); ok && maxUDPPacket > 0 {
				proxy["max-udp-relay-packet-size"] = maxUDPPacket
			}
			if cwnd, ok := toInt(obMap["cwnd"]); ok && cwnd > 0 {
				proxy["cwnd"] = cwnd
			}
			if ip, ok := obMap["ip"].(string); ok && ip != "" {
				proxy["ip"] = ip
			}
			if udpOverStream, ok := toBool(obMap["udp_over_stream"]); ok && udpOverStream {
				proxy["udp-over-stream"] = true
			}
			if udpOverStreamVersion, ok := toInt(obMap["udp_over_stream_version"]); ok && udpOverStreamVersion > 0 {
				proxy["udp-over-stream-version"] = udpOverStreamVersion
			}
			if disableMTUDiscovery, ok := toBool(obMap["disable_mtu_discovery"]); ok && disableMTUDiscovery {
				proxy["disable-mtu-discovery"] = true
			}
			if maxDatagramFrameSize, ok := toInt(obMap["max_datagram_frame_size"]); ok && maxDatagramFrameSize > 0 {
				proxy["max-datagram-frame-size"] = maxDatagramFrameSize
			}
			if clashMihomoFastOpenEnabled(obMap, outType) {
				proxy["fast-open"] = true
			}
		case "socks", "socks5":
			proxy["type"] = "socks5"
			if username, ok := obMap["username"].(string); ok && username != "" {
				proxy["username"] = username
			}
			if password, ok := obMap["password"].(string); ok && password != "" {
				proxy["password"] = password
			}
			if network, ok := obMap["network"].(string); ok && network != "" && network != "tcp" {
				proxy["udp"] = true
			}
		case "http":
			if username, ok := obMap["username"].(string); ok && username != "" {
				proxy["username"] = username
			}
			if password, ok := obMap["password"].(string); ok && password != "" {
				proxy["password"] = password
			}
			if headers := normalizeHeaders(obMap["headers"]); len(headers) > 0 {
				proxy["headers"] = headers
			}
		case "snell":
			if psk, ok := obMap["psk"].(string); ok && strings.TrimSpace(psk) != "" {
				proxy["psk"] = strings.TrimSpace(psk)
			}
			if version, ok := toInt(obMap["version"]); ok && version > 0 {
				proxy["version"] = version
			}
			if reuse, ok := toBool(obMap["reuse"]); ok {
				proxy["reuse"] = reuse
			}
			if obfsOpts, ok := obMap["obfs_opts"].(map[string]interface{}); ok && obfsOpts != nil {
				mode := strings.TrimSpace(firstString(obfsOpts["mode"]))
				if mode != "" {
					host := strings.TrimSpace(firstString(obfsOpts["host"]))
					if host == "" {
						host = "www.bing.com"
					}
					proxy["obfs-opts"] = map[string]interface{}{
						"mode": mode,
						"host": host,
					}
				}
			}
		case "shadowsocks":
			proxy["type"] = "ss"
			if method, ok := obMap["method"].(string); ok && method != "" {
				proxy["cipher"] = method
			}
			if password, ok := obMap["password"].(string); ok && password != "" {
				proxy["password"] = password
			}
			if network, ok := obMap["network"].(string); ok && network != "" && network != "tcp" {
				proxy["udp"] = true
			}
			switch udpOverTCP := obMap["udp_over_tcp"].(type) {
			case bool:
				if udpOverTCP {
					proxy["udp-over-tcp"] = true
				}
			case map[string]interface{}:
				if enabled, ok := toBool(udpOverTCP["enabled"]); ok && enabled {
					proxy["udp-over-tcp"] = true
					if version, ok := toInt(udpOverTCP["version"]); ok && version > 0 {
						proxy["udp-over-tcp-version"] = version
					}
				}
			}
			if plugin, ok := obMap["plugin"].(string); ok && plugin != "" {
				proxy["plugin"] = plugin
			}
			if pluginOpts, ok := obMap["plugin_opts"]; ok && pluginOpts != nil {
				proxy["plugin-opts"] = pluginOpts
			}
		case "hysteria":
			if auth, ok := obMap["auth_str"].(string); ok && auth != "" {
				proxy["auth-str"] = auth
			}
			if obfs, ok := obMap["obfs"].(string); ok && obfs != "" {
				proxy["obfs"] = obfs
			}
			if protocol, ok := obMap["protocol"].(string); ok && protocol != "" {
				proxy["protocol"] = protocol
			}
			if up, ok := toInt(obMap["up_mbps"]); ok && up > 0 {
				proxy["up"] = up
			}
			if down, ok := toInt(obMap["down_mbps"]); ok && down > 0 {
				proxy["down"] = down
			}
			if recvWindowConn, ok := toInt(obMap["recv_window_conn"]); ok && recvWindowConn > 0 {
				proxy["recv-window-conn"] = recvWindowConn
			}
			if recvWindow, ok := toInt(obMap["recv_window"]); ok && recvWindow > 0 {
				proxy["recv-window"] = recvWindow
			}
			if disableMTUDiscovery, ok := toBool(obMap["disable_mtu_discovery"]); ok && disableMTUDiscovery {
				proxy["disable-mtu-discovery"] = true
			}
			if clashMihomoFastOpenEnabled(obMap, outType) {
				proxy["fast-open"] = true
			}
			if ports := normalizeServerPorts(obMap["server_ports"]); len(ports) > 0 {
				proxy["ports"] = strings.Join(ports, ",")
			}
		case "hysteria2":
			if password, ok := obMap["password"].(string); ok && password != "" {
				proxy["password"] = password
			}
			if up, ok := toInt(obMap["up_mbps"]); ok && up > 0 {
				proxy["up"] = up
			}
			if down, ok := toInt(obMap["down_mbps"]); ok && down > 0 {
				proxy["down"] = down
			}
			if obfs, ok := obMap["obfs"].(map[string]interface{}); ok {
				if obfsType, ok := obfs["type"].(string); ok && obfsType != "" {
					proxy["obfs"] = obfsType
				}
				if obfsPassword, ok := obfs["password"].(string); ok && obfsPassword != "" {
					proxy["obfs-password"] = obfsPassword
				}
			}
			if ports := normalizeServerPorts(obMap["server_ports"]); len(ports) > 0 {
				proxy["ports"] = strings.Join(ports, ",")
			}
			if hopInterval, ok := buildMihomoHopInterval(obMap["hop_interval"], obMap["hop_interval_max"]); ok {
				proxy["hop-interval"] = hopInterval
			}
			if mihomoHy2, ok := obMap["mihomo_hy2"].(map[string]interface{}); ok {
				if v, ok := toInt(mihomoHy2["initial_stream_receive_window"]); ok && v > 0 {
					proxy["initial-stream-receive-window"] = v
				}
				if v, ok := toInt(mihomoHy2["max_stream_receive_window"]); ok && v > 0 {
					proxy["max-stream-receive-window"] = v
				}
				if v, ok := toInt(mihomoHy2["initial_connection_receive_window"]); ok && v > 0 {
					proxy["initial-connection-receive-window"] = v
				}
				if v, ok := toInt(mihomoHy2["max_connection_receive_window"]); ok && v > 0 {
					proxy["max-connection-receive-window"] = v
				}
			}
			if clashMihomoFastOpenEnabled(obMap, outType) {
				proxy["fast-open"] = true
			}
		case "anytls":
			if password, ok := obMap["password"].(string); ok && password != "" {
				proxy["password"] = password
			}
			if checkInterval, ok := durationToSeconds(obMap["idle_session_check_interval"]); ok && checkInterval > 0 {
				proxy["idle-session-check-interval"] = checkInterval
			}
			if timeout, ok := durationToSeconds(obMap["idle_session_timeout"]); ok && timeout > 0 {
				proxy["idle-session-timeout"] = timeout
			}
			if minIdle, ok := toInt(obMap["min_idle_session"]); ok && minIdle >= 0 {
				proxy["min-idle-session"] = minIdle
			}
		case "mieru":
			if username, ok := obMap["username"].(string); ok && strings.TrimSpace(username) != "" {
				proxy["username"] = strings.TrimSpace(username)
			}
			if password, ok := obMap["password"].(string); ok && strings.TrimSpace(password) != "" {
				proxy["password"] = strings.TrimSpace(password)
			}
			proxy["transport"] = util.NormalizeMieruTransport(firstString(obMap["transport"]))
			if udp, ok := toBool(obMap["udp"]); ok && udp {
				proxy["udp"] = true
			}
			if value := strings.TrimSpace(firstString(obMap["multiplexing"])); value != "" {
				proxy["multiplexing"] = util.NormalizeMieruMultiplexing(value)
			}
			if value := strings.TrimSpace(firstString(obMap["handshake_mode"])); value != "" {
				proxy["handshake-mode"] = util.NormalizeMieruHandshakeMode(value)
			}
		case "sudoku":
			if key := util.NormalizeSudokuKeyValue(obMap["key"]); key != "" {
				proxy["key"] = key
			}
			proxy["aead-method"] = util.NormalizeSudokuAEADMethod(firstString(obMap["aead_method"]))
			if value, ok := toInt(obMap["padding_min"]); ok && value > 0 {
				proxy["padding-min"] = value
			}
			if value, ok := toInt(obMap["padding_max"]); ok && value > 0 {
				proxy["padding-max"] = value
			}
			customTable := util.NormalizeSudokuCustomTable(obMap["custom_table"])
			customTables := util.NormalizeSudokuCustomTables(obMap["custom_tables"])
			proxy["table-type"] = util.NormalizeSudokuTableTypeForCustom(
				firstString(obMap["table_type"]),
				customTable != "" || len(customTables) > 0,
			)
			if customTable != "" {
				proxy["custom-table"] = customTable
			}
			if len(customTables) > 0 {
				proxy["custom-tables"] = customTables
			}
			if value, ok := toBool(obMap["enable_pure_downlink"]); ok {
				proxy["enable-pure-downlink"] = value
			} else {
				proxy["enable-pure-downlink"] = false
			}
			if httpmask := buildSudokuSubscriptionHTTPMask(obMap["httpmask"]); len(httpmask) > 0 {
				proxy["httpmask"] = httpmask
			}
		case "trusttunnel":
			util.ApplyTrustTunnelCredentials(proxy, obMap)
			util.ApplyTrustTunnelReuseOptions(proxy, obMap)
			if udp, ok := util.ResolveTrustTunnelUDP(obMap); ok {
				proxy["udp"] = udp
			}
			if quic, ok := toBool(obMap["quic"]); ok && quic {
				proxy["quic"] = true
			}
			if healthCheck, ok := toBool(obMap["health-check"]); ok {
				proxy["health-check"] = healthCheck
			} else if healthCheck, ok := toBool(obMap["health_check"]); ok {
				proxy["health-check"] = healthCheck
			}
			if value := strings.TrimSpace(firstString(obMap["congestion-controller"])); value != "" {
				proxy["congestion-controller"] = value
			} else if value := strings.TrimSpace(firstString(obMap["congestion_controller"])); value != "" {
				proxy["congestion-controller"] = value
			} else if value := strings.TrimSpace(firstString(obMap["congestion_control"])); value != "" {
				proxy["congestion-controller"] = value
			}
		case "ssh":
			username := strings.TrimSpace(firstString(obMap["username"]))
			if username == "" {
				username = strings.TrimSpace(firstString(obMap["user"]))
			}
			if username != "" {
				proxy["username"] = username
			}
			if password := strings.TrimSpace(firstString(obMap["password"])); password != "" {
				proxy["password"] = password
			}
			privateKey := strings.TrimSpace(firstString(obMap["private_key"]))
			if privateKey == "" {
				privateKey = strings.TrimSpace(firstString(obMap["private-key"]))
			}
			if privateKey != "" {
				proxy["private-key"] = privateKey
			}
			privateKeyPassphrase := strings.TrimSpace(firstString(obMap["private_key_passphrase"]))
			if privateKeyPassphrase == "" {
				privateKeyPassphrase = strings.TrimSpace(firstString(obMap["private-key-passphrase"]))
			}
			if privateKeyPassphrase != "" {
				proxy["private-key-passphrase"] = privateKeyPassphrase
			}
			hostKey := toStringSlice(obMap["host_key"])
			if len(hostKey) == 0 {
				hostKey = toStringSlice(obMap["host-key"])
			}
			if len(hostKey) > 0 {
				proxy["host-key"] = hostKey
			}
			hostKeyAlgorithms := toStringSlice(obMap["host_key_algorithms"])
			if len(hostKeyAlgorithms) == 0 {
				hostKeyAlgorithms = toStringSlice(obMap["host-key-algorithms"])
			}
			if len(hostKeyAlgorithms) > 0 {
				proxy["host-key-algorithms"] = hostKeyAlgorithms
			}
		default:
			continue
		}

		applyClashProxyCommonFields(proxy, obMap)

		tlsMap, tlsEnabled := extractEnabledTLS(obMap["tls"])
		if tlsEnabled {
			proxy["tls"] = true
			useMihomoFingerprint, _ := toBool(tlsMap["mihomo_use_fingerprint"])
			includeServerFingerprint := true
			if include, ok := toBool(tlsMap["include_server_fingerprint"]); ok && !include {
				includeServerFingerprint = false
			}

			if alpn := toStringSlice(tlsMap["alpn"]); len(alpn) > 0 {
				proxy["alpn"] = alpn
			}
			fingerprint := ""
			if includeServerFingerprint {
				if useMihomoFingerprint {
					if fp, ok := tlsMap["fingerprint"].(string); ok && strings.TrimSpace(fp) != "" {
						fingerprint = strings.TrimSpace(fp)
					}
					if fingerprint == "" {
						if derived, ok := deriveCertificateFingerprint(tlsMap); ok {
							fingerprint = derived
						}
					}
				} else if fp, ok := tlsMap["fingerprint"].(string); ok && strings.TrimSpace(fp) != "" {
					fingerprint = strings.TrimSpace(fp)
				}
			}
			if fingerprint != "" {
				proxy["fingerprint"] = fingerprint
			} else {
				delete(proxy, "fingerprint")
			}
			if sni, ok := tlsMap["server_name"].(string); ok && sni != "" {
				if outType == "vmess" || outType == "vless" {
					proxy["servername"] = sni
				} else {
					proxy["sni"] = sni
				}
			}
			if insecure, ok := toBool(tlsMap["insecure"]); ok && insecure && !useMihomoFingerprint {
				proxy["skip-cert-verify"] = true
			}
			if utls, ok := tlsMap["utls"].(map[string]interface{}); ok {
				enabled, hasEnabled := toBool(utls["enabled"])
				if !hasEnabled || enabled {
					if fp, ok := utls["fingerprint"].(string); ok && strings.TrimSpace(fp) != "" {
						proxy["client-fingerprint"] = strings.TrimSpace(fp)
					}
				}
			}
			if disableSNI, ok := toBool(tlsMap["disable_sni"]); ok && disableSNI {
				proxy["disable-sni"] = true
			}
			if outType != "anytls" && outType != "trusttunnel" {
				if reality, ok := tlsMap["reality"].(map[string]interface{}); ok {
					if realityEnabled, ok := toBool(reality["enabled"]); ok && realityEnabled {
						realityOpts := map[string]interface{}{}
						if pbk, ok := reality["public_key"].(string); ok && pbk != "" {
							realityOpts["public-key"] = pbk
						}
						if sid, ok := reality["short_id"].(string); ok && sid != "" {
							realityOpts["short-id"] = sid
						}
						if supportPQ, ok := toBool(reality["support_x25519mlkem768"]); ok && supportPQ {
							realityOpts["support-x25519mlkem768"] = true
						}
						if len(realityOpts) > 0 {
							proxy["reality-opts"] = realityOpts
						}
					}
				}
			}
			if outType != "trusttunnel" {
				if ech, ok := tlsMap["ech"].(map[string]interface{}); ok {
					echEnabled, _ := toBool(ech["enabled"])
					echConfig := flattenECHConfig(ech["config"])
					echQueryServerName, _ := ech["query_server_name"].(string)
					if echEnabled || echConfig != "" || echQueryServerName != "" {
						echOpts := map[string]interface{}{"enable": true}
						if echConfig != "" {
							echOpts["config"] = echConfig
						}
						if echQueryServerName != "" {
							echOpts["query-server-name"] = echQueryServerName
						}
						proxy["ech-opts"] = echOpts
					}
				}
			}
		}

		if transport, ok := obMap["transport"].(map[string]interface{}); ok {
			transportType := strings.ToLower(strings.TrimSpace(firstString(transport["type"])))
			switch transportType {
			case "http", "h2":
				path := firstString(transport["path"])
				headers := normalizeHeaders(transport["headers"])
				hosts := normalizeTransportHostValues(transport["host"], headers)
				method := strings.TrimSpace(firstString(transport["method"]))
				useH2 := transportType == "h2" || (transportType == "http" && tlsEnabled && method == "")
				if useH2 {
					proxy["network"] = "h2"
					h2Opts := map[string]interface{}{}
					if len(hosts) > 0 {
						h2Opts["host"] = hosts
					}
					if path != "" {
						h2Opts["path"] = path
					}
					if len(h2Opts) > 0 {
						proxy["h2-opts"] = h2Opts
					}
				} else {
					proxy["network"] = "http"
					httpOpts := map[string]interface{}{}
					if method != "" {
						httpOpts["method"] = method
					}
					if path != "" {
						httpOpts["path"] = []string{path}
					}
					if len(hosts) > 0 {
						if headers == nil {
							headers = map[string]interface{}{}
						}
						if _, hasHost := headers["Host"]; !hasHost {
							headers["Host"] = hosts
						}
					}
					if len(headers) > 0 {
						httpOpts["headers"] = headers
					}
					if len(httpOpts) > 0 {
						proxy["http-opts"] = httpOpts
					}
				}
			case "ws", "httpupgrade":
				proxy["network"] = "ws"
				wsOpts := map[string]interface{}{}
				if path, ok := transport["path"].(string); ok && path != "" {
					wsOpts["path"] = path
				}
				headers := normalizeHeaders(transport["headers"])
				if transportType == "httpupgrade" {
					if host, ok := transport["host"].(string); ok && host != "" {
						if headers == nil {
							headers = map[string]interface{}{}
						}
						if _, exists := headers["Host"]; !exists {
							headers["Host"] = host
						}
					}
					wsOpts["v2ray-http-upgrade"] = true
				}
				if enabled, ok := toBool(transport["v2ray_http_upgrade"]); ok && enabled {
					wsOpts["v2ray-http-upgrade"] = true
				}
				if enabled, ok := toBool(transport["v2ray_http_upgrade_fast_open"]); ok && enabled {
					wsOpts["v2ray-http-upgrade-fast-open"] = true
				}
				if len(headers) > 0 {
					wsOpts["headers"] = headers
				}
				if maxEarlyData, ok := toInt(transport["max_early_data"]); ok && maxEarlyData > 0 {
					wsOpts["max-early-data"] = maxEarlyData
				}
				if earlyDataHeaderName, ok := transport["early_data_header_name"].(string); ok && earlyDataHeaderName != "" {
					wsOpts["early-data-header-name"] = earlyDataHeaderName
				}
				if len(wsOpts) > 0 {
					proxy["ws-opts"] = wsOpts
				}
			case "grpc":
				proxy["network"] = "grpc"
				grpcOpts := map[string]interface{}{}
				if serviceName, ok := transport["service_name"].(string); ok && serviceName != "" {
					grpcOpts["grpc-service-name"] = serviceName
				}
				if userAgent, ok := transport["grpc_user_agent"].(string); ok && userAgent != "" {
					grpcOpts["grpc-user-agent"] = userAgent
				}
				if pingInterval, ok := toInt(transport["ping_interval"]); ok && pingInterval > 0 {
					grpcOpts["ping-interval"] = pingInterval
				}
				if maxConnections, ok := toInt(transport["max_connections"]); ok && maxConnections > 0 {
					grpcOpts["max-connections"] = maxConnections
				}
				if minStreams, ok := toInt(transport["min_streams"]); ok && minStreams >= 0 {
					grpcOpts["min-streams"] = minStreams
				}
				if maxStreams, ok := toInt(transport["max_streams"]); ok && maxStreams >= 0 {
					grpcOpts["max-streams"] = maxStreams
				}
				if len(grpcOpts) > 0 {
					proxy["grpc-opts"] = grpcOpts
				}
			case "xhttp":
				if !strings.EqualFold(strings.TrimSpace(outType), "vless") {
					break
				}
				proxy["network"] = "xhttp"
				if xhttpOpts := buildMihomoXHTTPOpts(transport); len(xhttpOpts) > 0 {
					proxy["xhttp-opts"] = xhttpOpts
				}
			}
		}

		proxies = append(proxies, proxy)
		proxyTags = append(proxyTags, tag)
	}

	proxyGroups := buildFixedMihomoProxyGroups(proxyTags, latencyUrl, latencyInterval, latencyTolerance)
	proxyGroups = append(proxyGroups, buildNamedClashProxyGroups(selectorGroups, proxyTags)...)

	output := map[string]interface{}{
		"proxies":      proxies,
		"proxy-groups": proxyGroups,
	}
	if normalized, ok := normalizeNumericTypesForYAML(output).(map[string]interface{}); ok && normalized != nil {
		output = normalized
	}
	util.ApplySudokuCustomTablesFlowYAML(output)
	raw, err := yaml.Marshal(output)
	if err != nil {
		return nil, err
	}
	return util.CompactSudokuCustomTablesFlowYAML(raw), nil
}

func buildClashShadowTLSSSProxy(ssOutbound map[string]interface{}, shadowTLSOutbound map[string]interface{}, isMihomo bool) (map[string]interface{}, bool) {
	tag, _ := ssOutbound["tag"].(string)
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return nil, false
	}

	server, _ := shadowTLSOutbound["server"].(string)
	server = strings.TrimSpace(server)
	if server == "" {
		fallbackServer, _ := ssOutbound["server"].(string)
		server = strings.TrimSpace(fallbackServer)
	}
	if server == "" {
		return nil, false
	}

	serverPort, ok := toInt(shadowTLSOutbound["server_port"])
	if !ok || serverPort <= 0 {
		serverPort, ok = toInt(ssOutbound["server_port"])
		if !ok || serverPort <= 0 {
			return nil, false
		}
	}

	proxy := map[string]interface{}{
		"name":   tag,
		"type":   "ss",
		"server": server,
		"port":   serverPort,
		"plugin": "shadow-tls",
	}

	if method, ok := ssOutbound["method"].(string); ok && strings.TrimSpace(method) != "" {
		proxy["cipher"] = strings.TrimSpace(method)
	}
	if password, ok := ssOutbound["password"].(string); ok && password != "" {
		proxy["password"] = password
	}
	if network, ok := ssOutbound["network"].(string); ok && network != "" && network != "tcp" {
		proxy["udp"] = true
	}
	switch udpOverTCP := ssOutbound["udp_over_tcp"].(type) {
	case bool:
		if udpOverTCP {
			proxy["udp-over-tcp"] = true
		}
	case map[string]interface{}:
		if enabled, ok := toBool(udpOverTCP["enabled"]); ok && enabled {
			proxy["udp-over-tcp"] = true
			if version, ok := toInt(udpOverTCP["version"]); ok && version > 0 {
				proxy["udp-over-tcp-version"] = version
			}
		}
	}

	pluginOpts := map[string]interface{}{}
	host := util.DeriveShadowTLSPluginHost(shadowTLSOutbound)
	if host == "" {
		host = server
	}
	pluginOpts["host"] = host

	if password, ok := shadowTLSOutbound["password"].(string); ok && password != "" {
		pluginOpts["password"] = password
	}
	if version, ok := toInt(shadowTLSOutbound["version"]); ok && version > 0 {
		pluginOpts["version"] = version
	}

	if tlsMap, ok := shadowTLSOutbound["tls"].(map[string]interface{}); ok && tlsMap != nil {
		includeServerFingerprint := true
		if include, ok := toBool(tlsMap["include_server_fingerprint"]); ok && !include {
			includeServerFingerprint = false
		}
		if utls, ok := tlsMap["utls"].(map[string]interface{}); ok && utls != nil {
			enabled, hasEnabled := toBool(utls["enabled"])
			if !hasEnabled || enabled {
				if fp, ok := utls["fingerprint"].(string); ok && strings.TrimSpace(fp) != "" {
					proxy["client-fingerprint"] = strings.TrimSpace(fp)
				}
			}
		}
		if !isMihomo {
			if includeServerFingerprint {
				if fp := strings.TrimSpace(firstString(tlsMap["fingerprint"])); fp != "" {
					pluginOpts["fingerprint"] = fp
				}
			}
			if insecure, ok := toBool(tlsMap["insecure"]); ok && insecure {
				pluginOpts["skip-cert-verify"] = true
			}
			if alpn := toStringSlice(tlsMap["alpn"]); len(alpn) > 0 {
				pluginOpts["alpn"] = alpn
			}
		}
	}

	applyClashProxyCommonFields(proxy, ssOutbound)
	proxy["plugin-opts"] = pluginOpts
	return proxy, true
}

func applyClashProxyCommonFields(proxy map[string]interface{}, obMap map[string]interface{}) {
	if proxy == nil || obMap == nil {
		return
	}

	if udp, ok := toBool(resolveMihomoCommonValue(obMap, "udp")); ok {
		proxy["udp"] = udp
	}
	if ipVersion := strings.TrimSpace(firstString(resolveMihomoCommonValue(obMap, "ip_version"))); ipVersion != "" {
		proxy["ip-version"] = ipVersion
	}
	if routingMark, ok := toInt(resolveMihomoCommonValue(obMap, "routing_mark")); ok {
		proxy["routing-mark"] = routingMark
	}
	if tcpFastOpen, ok := toBool(resolveMihomoCommonValue(obMap, "tcp_fast_open")); ok {
		proxy["tfo"] = tcpFastOpen
	}
	if tcpMultiPath, ok := toBool(resolveMihomoCommonValue(obMap, "tcp_multi_path")); ok {
		proxy["mptcp"] = tcpMultiPath
	}
	protocol := strings.TrimSpace(firstString(obMap["type"]))
	if util.SupportsMihomoBBRProfileProtocol(protocol) {
		if profile, ok := resolveMihomoCommonBBRProfile(obMap); ok {
			proxy["bbr-profile"] = profile
		}
	}

	mux, ok := resolveMihomoSMuxSource(obMap)
	if !ok || mux == nil {
		return
	}
	if enabled, ok := toBool(mux["enabled"]); !ok || !enabled {
		return
	}

	smux := map[string]interface{}{"enabled": true}
	if protocol, ok := mux["protocol"].(string); ok && protocol != "" {
		smux["protocol"] = protocol
	}
	if maxConnections, ok := toInt(mux["max_connections"]); ok {
		smux["max-connections"] = maxConnections
	}
	if minStreams, ok := toInt(mux["min_streams"]); ok {
		smux["min-streams"] = minStreams
	}
	if maxStreams, ok := toInt(mux["max_streams"]); ok {
		smux["max-streams"] = maxStreams
	}
	statistic := false
	if value, ok := toBool(mux["statistic"]); ok {
		statistic = value
	}
	smux["statistic"] = statistic

	onlyTCP := false
	if value, ok := toBool(mux["only_tcp"]); ok {
		onlyTCP = value
	} else if value, ok := toBool(mux["only-tcp"]); ok {
		onlyTCP = value
	}
	smux["only-tcp"] = onlyTCP
	if padding, ok := toBool(mux["padding"]); ok {
		smux["padding"] = padding
	}
	if brutal, ok := mux["brutal"].(map[string]interface{}); ok {
		if brutalEnabled, ok := toBool(brutal["enabled"]); ok && brutalEnabled {
			brutalOpts := map[string]interface{}{"enabled": true}
			if upMbps, ok := toInt(brutal["up_mbps"]); ok {
				brutalOpts["up"] = upMbps
			}
			if downMbps, ok := toInt(brutal["down_mbps"]); ok {
				brutalOpts["down"] = downMbps
			}
			smux["brutal-opts"] = brutalOpts
		}
	}
	proxy["smux"] = smux
}

func resolveMihomoCommonBBRProfile(obMap map[string]interface{}) (string, bool) {
	if profile, ok := util.NormalizeMihomoBBRProfile(resolveMihomoCommonValue(obMap, "bbr_profile")); ok {
		return profile, true
	}
	if profile, ok := util.NormalizeMihomoBBRProfile(resolveMihomoCommonValue(obMap, "bbr-profile")); ok {
		return profile, true
	}
	return "", false
}

func resolveMihomoCommonValue(obMap map[string]interface{}, key string) interface{} {
	if obMap == nil {
		return nil
	}

	if common, ok := resolveMihomoCommonSource(obMap); ok {
		if value, exists := common[key]; exists {
			return value
		}
	}

	return obMap[key]
}

func resolveMihomoCommonSource(obMap map[string]interface{}) (map[string]interface{}, bool) {
	if obMap == nil {
		return nil, false
	}

	common, ok := obMap["mihomo_common"].(map[string]interface{})
	if !ok || common == nil {
		return nil, false
	}

	return common, true
}

func resolveMihomoSMuxSource(obMap map[string]interface{}) (map[string]interface{}, bool) {
	if common, ok := resolveMihomoCommonSource(obMap); ok {
		if smux, ok := common["smux"].(map[string]interface{}); ok && smux != nil {
			return smux, true
		}
		if smux, ok := common["mux"].(map[string]interface{}); ok && smux != nil {
			return smux, true
		}
	}

	mux, ok := obMap["multiplex"].(map[string]interface{})
	if !ok || mux == nil {
		return nil, false
	}
	return mux, true
}

func resolveTLSFingerprint(tlsMap map[string]interface{}, useMihomoFingerprint bool) (string, bool) {
	if tlsMap == nil {
		return "", false
	}

	if fp, ok := tlsMap["fingerprint"].(string); ok && strings.TrimSpace(fp) != "" {
		return strings.TrimSpace(fp), true
	}
	if useMihomoFingerprint {
		if derived, ok := deriveCertificateFingerprint(tlsMap); ok {
			return derived, true
		}
	}

	return "", false
}

func decodeJSONMapUseNumber(raw []byte) (map[string]interface{}, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()

	var out map[string]interface{}
	if err := decoder.Decode(&out); err != nil {
		return nil, err
	}
	if out == nil {
		return nil, fmt.Errorf("decoded map is nil")
	}
	return out, nil
}

func normalizeProxyForYAML(proxy map[string]interface{}) map[string]interface{} {
	if proxy == nil {
		return nil
	}
	normalized, ok := normalizeNumericTypesForYAML(proxy).(map[string]interface{})
	if !ok || normalized == nil {
		return proxy
	}
	return normalized
}

func normalizeNumericTypesForYAML(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			out[key] = normalizeNumericTypesForYAML(item)
		}
		return out
	case []interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, normalizeNumericTypesForYAML(item))
		}
		return out
	case json.Number:
		if intVal, err := typed.Int64(); err == nil {
			return intVal
		}
		if floatVal, err := typed.Float64(); err == nil {
			return normalizeFloatForYAML(floatVal)
		}
		return typed.String()
	case float64:
		return normalizeFloatForYAML(typed)
	case float32:
		return normalizeFloatForYAML(float64(typed))
	default:
		return typed
	}
}

func normalizeFloatForYAML(value float64) interface{} {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return value
	}
	if value == math.Trunc(value) && value >= math.MinInt64 && value <= math.MaxInt64 {
		return int64(value)
	}
	return value
}

func normalizeTransportHostValues(rawHost interface{}, headers map[string]interface{}) []string {
	hosts := toStringSlice(rawHost)
	if len(hosts) > 0 {
		return hosts
	}
	return readHeaderValuesByName(headers, "Host")
}

func readHeaderValuesByName(headers map[string]interface{}, headerName string) []string {
	if len(headers) == 0 {
		return nil
	}

	lowerName := strings.ToLower(strings.TrimSpace(headerName))
	if lowerName == "" {
		return nil
	}

	for key, value := range headers {
		if strings.ToLower(strings.TrimSpace(key)) != lowerName {
			continue
		}
		return toStringSlice(value)
	}

	return nil
}

func readStringLikeValue(raw interface{}) string {
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value)
	case int:
		return strconv.Itoa(value)
	case int32:
		return strconv.FormatInt(int64(value), 10)
	case int64:
		return strconv.FormatInt(value, 10)
	case float32:
		return strconv.FormatFloat(float64(value), 'f', -1, 32)
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	default:
		return ""
	}
}

func assignStringLikeOption(dst map[string]interface{}, key string, raw interface{}) {
	if dst == nil {
		return
	}
	if value := readStringLikeValue(raw); value != "" {
		dst[key] = value
	}
}

func buildMihomoXHTTPReuseSettings(raw interface{}) map[string]interface{} {
	reuse, ok := raw.(map[string]interface{})
	if !ok || reuse == nil {
		return nil
	}

	out := map[string]interface{}{}
	assignStringLikeOption(out, "max-connections", reuse["max_connections"])
	assignStringLikeOption(out, "max-concurrency", reuse["max_concurrency"])
	assignStringLikeOption(out, "c-max-reuse-times", reuse["c_max_reuse_times"])
	assignStringLikeOption(out, "h-max-request-times", reuse["h_max_request_times"])
	assignStringLikeOption(out, "h-max-reusable-secs", reuse["h_max_reusable_secs"])

	if len(out) == 0 {
		return nil
	}
	return out
}

func buildMihomoXHTTPDownloadSettings(raw interface{}) map[string]interface{} {
	download, ok := raw.(map[string]interface{})
	if !ok || download == nil {
		return nil
	}

	out := map[string]interface{}{}
	if path := strings.TrimSpace(firstString(download["path"])); path != "" {
		out["path"] = path
	}
	if host := strings.TrimSpace(firstString(download["host"])); host != "" {
		out["host"] = host
	}
	if headers := normalizeHeaders(download["headers"]); len(headers) > 0 {
		out["headers"] = headers
	}
	if noGRPCHeader, ok := toBool(download["no_grpc_header"]); ok {
		out["no-grpc-header"] = noGRPCHeader
	}
	if padding := strings.TrimSpace(firstString(download["x_padding_bytes"])); padding != "" {
		out["x-padding-bytes"] = padding
	}
	if postBytes, ok := toInt(download["sc_max_each_post_bytes"]); ok && postBytes > 0 {
		out["sc-max-each-post-bytes"] = postBytes
	}
	if reuse := buildMihomoXHTTPReuseSettings(download["reuse_settings"]); len(reuse) > 0 {
		out["reuse-settings"] = reuse
	}

	if server := strings.TrimSpace(firstString(download["server"])); server != "" {
		out["server"] = server
	}
	if port, ok := toInt(download["port"]); ok && port > 0 {
		out["port"] = port
	}
	if tls, ok := toBool(download["tls"]); ok {
		out["tls"] = tls
	}
	if alpn := toStringSlice(download["alpn"]); len(alpn) > 0 {
		out["alpn"] = alpn
	}
	if echOpts, ok := download["ech_opts"].(map[string]interface{}); ok && len(echOpts) > 0 {
		out["ech-opts"] = echOpts
	}
	if realityOpts, ok := download["reality_opts"].(map[string]interface{}); ok && len(realityOpts) > 0 {
		out["reality-opts"] = realityOpts
	}
	if skipCertVerify, ok := toBool(download["skip_cert_verify"]); ok {
		out["skip-cert-verify"] = skipCertVerify
	}
	if fingerprint := strings.TrimSpace(firstString(download["fingerprint"])); fingerprint != "" {
		out["fingerprint"] = fingerprint
	}
	if certificate := strings.TrimSpace(firstString(download["certificate"])); certificate != "" {
		out["certificate"] = certificate
	}
	if privateKey := strings.TrimSpace(firstString(download["private_key"])); privateKey != "" {
		out["private-key"] = privateKey
	}
	if serverName := strings.TrimSpace(firstString(download["servername"])); serverName != "" {
		out["servername"] = serverName
	}
	if clientFingerprint := strings.TrimSpace(firstString(download["client_fingerprint"])); clientFingerprint != "" {
		out["client-fingerprint"] = clientFingerprint
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

func buildMihomoXHTTPOpts(transport map[string]interface{}) map[string]interface{} {
	if transport == nil {
		return nil
	}

	xhttpOpts := map[string]interface{}{}
	if path := strings.TrimSpace(firstString(transport["path"])); path != "" {
		xhttpOpts["path"] = path
	}
	if host := strings.TrimSpace(firstString(transport["host"])); host != "" {
		xhttpOpts["host"] = host
	}
	if mode := strings.TrimSpace(firstString(transport["mode"])); mode != "" {
		xhttpOpts["mode"] = mode
	}
	if headers := normalizeHeaders(transport["headers"]); len(headers) > 0 {
		xhttpOpts["headers"] = headers
	}
	if noGRPCHeader, ok := toBool(transport["no_grpc_header"]); ok {
		xhttpOpts["no-grpc-header"] = noGRPCHeader
	}
	if padding := strings.TrimSpace(firstString(transport["x_padding_bytes"])); padding != "" {
		xhttpOpts["x-padding-bytes"] = padding
	}
	if postBytes, ok := toInt(transport["sc_max_each_post_bytes"]); ok && postBytes > 0 {
		xhttpOpts["sc-max-each-post-bytes"] = postBytes
	}
	if reuse := buildMihomoXHTTPReuseSettings(transport["reuse_settings"]); len(reuse) > 0 {
		xhttpOpts["reuse-settings"] = reuse
	}
	if download := buildMihomoXHTTPDownloadSettings(transport["download_settings"]); len(download) > 0 {
		xhttpOpts["download-settings"] = download
	}

	if len(xhttpOpts) == 0 {
		return nil
	}
	return xhttpOpts
}

func toInt(raw interface{}) (int, bool) {
	switch value := raw.(type) {
	case int:
		return value, true
	case int32:
		return int(value), true
	case int64:
		return int(value), true
	case float32:
		return int(value), true
	case float64:
		return int(value), true
	case string:
		value = strings.TrimSpace(value)
		if value == "" {
			return 0, false
		}
		num := 0
		if _, err := fmt.Sscanf(value, "%d", &num); err == nil {
			return num, true
		}
	}
	return 0, false
}

func clashMihomoFastOpenEnabled(obMap map[string]interface{}, protocol string) bool {
	defaultEnabled := defaultClashMihomoFastOpenEnabled(protocol)
	if obMap == nil {
		return defaultEnabled
	}
	if fastOpen, ok := toBool(obMap["mihomo_fast_open"]); ok {
		return fastOpen
	}
	if fastOpen, ok := toBool(obMap["fast_open"]); ok {
		return fastOpen
	}
	return defaultEnabled
}

func defaultClashMihomoFastOpenEnabled(protocol string) bool {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "hysteria2", "tuic":
		return false
	default:
		return true
	}
}

func toBool(raw interface{}) (bool, bool) {
	switch value := raw.(type) {
	case bool:
		return value, true
	case int:
		return value != 0, true
	case int32:
		return value != 0, true
	case int64:
		return value != 0, true
	case float32:
		return value != 0, true
	case float64:
		return value != 0, true
	case string:
		value = strings.TrimSpace(strings.ToLower(value))
		if value == "" {
			return false, false
		}
		if value == "1" {
			return true, true
		}
		if value == "0" {
			return false, true
		}
		parsed, err := strconv.ParseBool(value)
		if err == nil {
			return parsed, true
		}
	}
	return false, false
}

func extractEnabledTLS(raw interface{}) (map[string]interface{}, bool) {
	tlsMap, ok := raw.(map[string]interface{})
	if !ok || tlsMap == nil {
		return nil, false
	}
	if enabled, hasEnabled := toBool(tlsMap["enabled"]); hasEnabled {
		return tlsMap, enabled
	}
	return tlsMap, len(tlsMap) > 0
}

func toStringSlice(raw interface{}) []string {
	switch value := raw.(type) {
	case []string:
		result := make([]string, 0, len(value))
		for _, item := range value {
			item = strings.TrimSpace(item)
			if item != "" {
				result = append(result, item)
			}
		}
		return result
	case []interface{}:
		result := make([]string, 0, len(value))
		for _, item := range value {
			itemStr, ok := item.(string)
			if !ok {
				continue
			}
			itemStr = strings.TrimSpace(itemStr)
			if itemStr != "" {
				result = append(result, itemStr)
			}
		}
		return result
	case string:
		value = strings.TrimSpace(value)
		if value == "" {
			return nil
		}
		return []string{value}
	default:
		return nil
	}
}

func firstString(raw interface{}) string {
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value)
	case []string:
		for _, item := range value {
			item = strings.TrimSpace(item)
			if item != "" {
				return item
			}
		}
	case []interface{}:
		for _, item := range value {
			itemStr, ok := item.(string)
			if !ok {
				continue
			}
			itemStr = strings.TrimSpace(itemStr)
			if itemStr != "" {
				return itemStr
			}
		}
	}
	return ""
}

func normalizeHeaders(raw interface{}) map[string]interface{} {
	switch value := raw.(type) {
	case map[string]interface{}:
		headers := make(map[string]interface{})
		for key, item := range value {
			key = strings.TrimSpace(key)
			if key == "" || item == nil {
				continue
			}
			switch itemValue := item.(type) {
			case []interface{}:
				normalized := make([]string, 0, len(itemValue))
				for _, child := range itemValue {
					childStr, ok := child.(string)
					if !ok {
						continue
					}
					childStr = strings.TrimSpace(childStr)
					if childStr != "" {
						normalized = append(normalized, childStr)
					}
				}
				if len(normalized) > 0 {
					headers[key] = normalized
				}
			case []string:
				normalized := make([]string, 0, len(itemValue))
				for _, child := range itemValue {
					child = strings.TrimSpace(child)
					if child != "" {
						normalized = append(normalized, child)
					}
				}
				if len(normalized) > 0 {
					headers[key] = normalized
				}
			case string:
				itemValue = strings.TrimSpace(itemValue)
				if itemValue != "" {
					headers[key] = itemValue
				}
			}
		}
		return headers
	case map[string]string:
		headers := make(map[string]interface{})
		for key, item := range value {
			key = strings.TrimSpace(key)
			item = strings.TrimSpace(item)
			if key != "" && item != "" {
				headers[key] = item
			}
		}
		return headers
	default:
		return nil
	}
}

func normalizeServerPorts(raw interface{}) []string {
	ports := toStringSlice(raw)
	if len(ports) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(ports))
	for _, port := range ports {
		port = strings.ReplaceAll(port, ":", "-")
		port = strings.TrimSpace(port)
		if port != "" {
			normalized = append(normalized, port)
		}
	}
	return normalized
}

func durationToSeconds(raw interface{}) (int, bool) {
	switch value := raw.(type) {
	case int:
		return value, value > 0
	case int32:
		return int(value), value > 0
	case int64:
		return int(value), value > 0
	case float32:
		return int(value), value > 0
	case float64:
		return int(value), value > 0
	case string:
		value = strings.TrimSpace(strings.ToLower(value))
		if value == "" {
			return 0, false
		}
		seconds := parseClashInterval(value)
		return seconds, seconds > 0
	default:
		return 0, false
	}
}

func buildMihomoHopInterval(raw interface{}, rawMax interface{}) (interface{}, bool) {
	if lower, upper, ok := parseMihomoHopIntervalRange(raw); ok {
		if upper > lower {
			return fmt.Sprintf("%d-%d", lower, upper), true
		}
		return lower, true
	}

	lower, lowerOK := durationToSeconds(raw)
	upper, upperOK := durationToSeconds(rawMax)
	switch {
	case lowerOK && upperOK:
		if lower > upper {
			lower, upper = upper, lower
		}
		if upper > lower {
			return fmt.Sprintf("%d-%d", lower, upper), true
		}
		return lower, true
	case lowerOK && lower > 0:
		return lower, true
	case upperOK && upper > 0:
		return upper, true
	default:
		return nil, false
	}
}

func parseMihomoHopIntervalRange(raw interface{}) (int, int, bool) {
	value, ok := raw.(string)
	if !ok {
		return 0, 0, false
	}

	input := strings.TrimSpace(strings.ToLower(value))
	if input == "" {
		return 0, 0, false
	}

	delimiter := ""
	switch {
	case strings.Contains(input, "-"):
		delimiter = "-"
	case strings.Contains(input, ":"):
		delimiter = ":"
	default:
		return 0, 0, false
	}

	parts := strings.SplitN(input, delimiter, 2)
	if len(parts) != 2 {
		return 0, 0, false
	}

	left := strings.TrimSpace(parts[0])
	right := strings.TrimSpace(parts[1])
	if left == "" || right == "" {
		return 0, 0, false
	}

	rightSeconds := parseClashInterval(right)
	if rightSeconds <= 0 {
		return 0, 0, false
	}

	leftSeconds := parseClashInterval(left)
	if leftSeconds <= 0 {
		unit := strings.TrimSpace(strings.TrimLeft(right, "0123456789 "))
		if unit != "" {
			leftSeconds = parseClashInterval(left + unit)
		}
	}
	if leftSeconds <= 0 {
		return 0, 0, false
	}

	if leftSeconds > rightSeconds {
		leftSeconds, rightSeconds = rightSeconds, leftSeconds
	}
	return leftSeconds, rightSeconds, true
}

func durationToMilliseconds(raw interface{}) (int, bool) {
	switch value := raw.(type) {
	case int:
		return value, value > 0
	case int32:
		return int(value), value > 0
	case int64:
		return int(value), value > 0
	case float32:
		return int(value), value > 0
	case float64:
		return int(value), value > 0
	case string:
		value = strings.TrimSpace(strings.ToLower(value))
		if value == "" {
			return 0, false
		}
		if strings.HasSuffix(value, "ms") {
			num := strings.TrimSpace(strings.TrimSuffix(value, "ms"))
			ms, err := strconv.Atoi(num)
			return ms, err == nil && ms > 0
		}
		seconds := parseClashInterval(value)
		if seconds <= 0 {
			return 0, false
		}
		return seconds * 1000, true
	default:
		return 0, false
	}
}

func normalizePEM(raw interface{}) (string, bool) {
	switch value := raw.(type) {
	case string:
		value = strings.TrimSpace(value)
		if value != "" {
			return value, true
		}
	case []string:
		lines := make([]string, 0, len(value))
		for _, line := range value {
			line = strings.TrimRight(line, "\r")
			if strings.TrimSpace(line) != "" {
				lines = append(lines, line)
			}
		}
		if len(lines) > 0 {
			return strings.Join(lines, "\n"), true
		}
	case []interface{}:
		lines := make([]string, 0, len(value))
		for _, lineRaw := range value {
			line, ok := lineRaw.(string)
			if !ok {
				continue
			}
			line = strings.TrimRight(line, "\r")
			if strings.TrimSpace(line) != "" {
				lines = append(lines, line)
			}
		}
		if len(lines) > 0 {
			return strings.Join(lines, "\n"), true
		}
	}
	return "", false
}

func flattenECHConfig(raw interface{}) string {
	lines := toStringSlice(raw)
	if len(lines) == 0 {
		return ""
	}

	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "-----BEGIN ") || strings.HasPrefix(trimmed, "-----END ") {
			continue
		}
		filtered = append(filtered, trimmed)
	}
	if len(filtered) == 0 {
		return strings.TrimSpace(strings.Join(lines, ""))
	}
	return strings.Join(filtered, "")
}

func deriveCertificateFingerprint(tlsMap map[string]interface{}) (string, bool) {
	if fingerprint, ok := calculateCertificateFingerprint(tlsMap["certificate"]); ok {
		return fingerprint, true
	}
	if fingerprint, ok := calculateCertificateFingerprint(tlsMap["certificate_path"]); ok {
		return fingerprint, true
	}
	return "", false
}

func buildSudokuSubscriptionHTTPMask(raw interface{}) map[string]interface{} {
	httpmaskRaw, _ := raw.(map[string]interface{})
	if httpmaskRaw == nil {
		httpmaskRaw = map[string]interface{}{}
	}

	httpmask := map[string]interface{}{
		"disable":   false,
		"mode":      "legacy",
		"tls":       true,
		"multiplex": "off",
	}
	if value, ok := toBool(httpmaskRaw["disable"]); ok {
		httpmask["disable"] = value
	}
	if value := util.NormalizeSudokuHTTPMaskMode(firstString(httpmaskRaw["mode"])); value != "" {
		httpmask["mode"] = value
	}
	if value, ok := toBool(httpmaskRaw["tls"]); ok {
		httpmask["tls"] = value
	}
	if host := util.NormalizeSudokuStringValue(httpmaskRaw["mask-host"]); host != "" {
		httpmask["mask-host"] = host
	} else if host := util.NormalizeSudokuStringValue(httpmaskRaw["host"]); host != "" {
		httpmask["mask-host"] = host
	}
	if pathRoot := util.NormalizeSudokuStringValue(httpmaskRaw["path-root"]); pathRoot != "" {
		httpmask["path-root"] = pathRoot
	} else if pathRoot := util.NormalizeSudokuStringValue(httpmaskRaw["path_root"]); pathRoot != "" {
		httpmask["path-root"] = pathRoot
	}
	if value := util.NormalizeSudokuHTTPMaskMultiplex(firstString(httpmaskRaw["multiplex"])); value != "" {
		httpmask["multiplex"] = value
	}
	return httpmask
}

func calculateCertificateFingerprint(raw interface{}) (string, bool) {
	pemText, ok := normalizePEM(raw)
	if !ok {
		return "", false
	}

	certBytes := []byte(pemText)
	if !strings.Contains(pemText, "BEGIN CERTIFICATE") {
		fileBytes, err := os.ReadFile(strings.TrimSpace(pemText))
		if err != nil {
			return "", false
		}
		certBytes = fileBytes
	}

	rest := certBytes
	for len(rest) > 0 {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}

		sum := sha256.Sum256(block.Bytes)
		hexStr := strings.ToUpper(hex.EncodeToString(sum[:]))
		parts := make([]string, 0, len(hexStr)/2)
		for i := 0; i < len(hexStr); i += 2 {
			parts = append(parts, hexStr[i:i+2])
		}
		return strings.Join(parts, ":"), true
	}
	return "", false
}
