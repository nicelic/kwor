package sub

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/service"
	"github.com/alireza0/s-ui/util"
)

const defaultJson = `
{
  "inbounds": [
    {
      "type": "tun",
      "address": [
        "172.19.0.1/30",
        "fdfe:dcba:9876::1/126"
      ],
      "mtu": 1500,
      "auto_route": true,
      "strict_route": true,
      "endpoint_independent_nat": false,
      "stack": "mixed",
      "exclude_package": []
    },
    {
      "type": "mixed",
      "listen": "127.0.0.1",
      "listen_port": 2080,
      "users": []
    }
  ]
}
`

type JsonService struct {
	service.SettingService
	LinkService
}

const (
	nodeSelectorTag         = "节点选择"
	autoSelectorTag         = "自动选择"
	globalDirectSelectorTag = "全球直连"
	globalBlockSelectorTag  = "全球拦截"
	finalSelectorTag        = "漏网之鱼"
	globalSelectorTag       = "GLOBAL"
	managedSubHTTPClientTag = "rules-download"
)

var legacySingboxSelectorTagAliases = map[string]string{
	"🚀 节点选择":           nodeSelectorTag,
	"🚀节点选择":            nodeSelectorTag,
	"\\U0001F680 节点选择": nodeSelectorTag,
	"\\U0001F680节点选择":  nodeSelectorTag,
	"🎈 自动选择":           autoSelectorTag,
	"🎈自动选择":            autoSelectorTag,
	"\\U0001F388 自动选择": autoSelectorTag,
	"\\U0001F388自动选择":  autoSelectorTag,
	"🎯 全球直连":           globalDirectSelectorTag,
	"🎯全球直连":            globalDirectSelectorTag,
	"\\U0001F3AF 全球直连": globalDirectSelectorTag,
	"\\U0001F3AF全球直连":  globalDirectSelectorTag,
	"🛑 全球拦截":           globalBlockSelectorTag,
	"🛑全球拦截":            globalBlockSelectorTag,
	"\\U0001F6D1 全球拦截": globalBlockSelectorTag,
	"\\U0001F6D1全球拦截":  globalBlockSelectorTag,
	"🐟 漏网之鱼":           finalSelectorTag,
	"🐟漏网之鱼":            finalSelectorTag,
	"\\U0001F41F 漏网之鱼": finalSelectorTag,
	"\\U0001F41F漏网之鱼":  finalSelectorTag,
}

func normalizeLegacySingboxSelectorTag(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if normalized, exists := legacySingboxSelectorTagAliases[trimmed]; exists {
		return normalized
	}
	return trimmed
}

type selectorGroupConfig struct {
	Tag             string
	DefaultOutbound string
}

func (j *JsonService) GetJson(subId string, format string) (*string, []string, error) {
	client, inDatas, err := j.getData(subId)
	if err != nil {
		return nil, nil, err
	}

	return j.buildJSONSubscription(client, inDatas)
}

func (j *JsonService) GetMihomoJson(subId string, format string) (*string, []string, error) {
	client, inDatas, err := loadMihomoSubscriptionData(subId)
	if err != nil {
		return nil, nil, err
	}

	return j.buildJSONSubscription(client, inDatas)
}

func (j *JsonService) buildJSONSubscription(client *model.Client, inDatas []*model.Inbound) (*string, []string, error) {
	var jsonConfig map[string]interface{}

	outbounds, outTags, err := j.getOutbounds(client.Name, client.Config, inDatas)
	if err != nil {
		return nil, nil, err
	}

	links := j.LinkService.GetLinks(&client.Links, "external", "")
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
	*outbounds, *outTags = util.FilterTaggedSubscriptionOutbounds(
		*outbounds,
		*outTags,
		util.SupportsSingboxSubscriptionOutboundType,
	)
	for i := range *outbounds {
		util.SanitizeSingboxSubscriptionOutbound((*outbounds)[i])
	}

	// Comment cleaned to avoid mojibake.
	latencyUrl := "http://www.gstatic.com/generate_204"
	latencyInterval := "10m"
	latencyTolerance := 50
	var extJson map[string]interface{}
	othersStr2, _ := j.SettingService.GetSubJsonExt()
	if len(othersStr2) > 0 {
		if err := json.Unmarshal([]byte(othersStr2), &extJson); err == nil {
			if u, ok := extJson["latency_test_url"].(string); ok && u != "" {
				latencyUrl = u
			}
			if i, ok := extJson["latency_test_interval"].(string); ok && i != "" {
				if normalized, ok := normalizeSingboxLatencyInterval(i); ok {
					latencyInterval = normalized
				}
			}
			if t, ok := extJson["latency_tolerance"].(float64); ok && t > 0 {
				latencyTolerance = int(t)
			}
		}
	}
	selectorGroups := parseSelectorGroupsFromExt(extJson)

	// Comment cleaned to avoid mojibake.
	stripMihomoFields(outbounds)

	j.addDefaultOutbounds(outbounds, outTags, latencyUrl, latencyInterval, latencyTolerance, selectorGroups)

	// Comment cleaned to avoid mojibake.
	j.overrideServerIP(outbounds, client.ServerIp)

	err = json.Unmarshal([]byte(defaultJson), &jsonConfig)
	if err != nil {
		return nil, nil, err
	}

	jsonConfig["outbounds"] = outbounds

	// Extract certificate store configured in TLS.
	tlsStore := j.extractTlsStore(inDatas)
	legacyTlsStore := extractAndStripTlsStoreFromOutbounds(outbounds)
	if tlsStore == "" {
		tlsStore = legacyTlsStore
	}
	tlsStore = j.SettingService.ResolveSubscriptionTLSStore(tlsStore)

	// Add other objects from settings
	j.addOthers(&jsonConfig)
	applyCertificateStore(&jsonConfig, tlsStore)

	result, _ := json.MarshalIndent(jsonConfig, "", "  ")
	resultStr := string(result)

	updateInterval, _ := j.SettingService.GetSubUpdates()
	headers := util.GetHeaders(client, updateInterval)

	return &resultStr, headers, nil
}

func normalizeSingboxLatencyInterval(raw string) (string, bool) {
	value := strings.TrimSpace(strings.ToLower(raw))
	if len(value) < 2 {
		return "", false
	}

	unit := value[len(value)-1]
	if unit != 's' && unit != 'm' && unit != 'h' && unit != 'd' {
		return "", false
	}

	numberPart := strings.TrimSpace(value[:len(value)-1])
	if numberPart == "" {
		return "", false
	}

	var interval int
	if _, err := fmt.Sscanf(numberPart, "%d", &interval); err != nil || interval <= 0 {
		return "", false
	}

	return fmt.Sprintf("%d%c", interval, unit), true
}

func (j *JsonService) getData(subId string) (*model.Client, []*model.Inbound, error) {
	db := database.GetDB()
	client := &model.Client{}
	err := db.Model(model.Client{}).Where("enable = true and name = ?", subId).First(client).Error
	if err != nil {
		return nil, nil, err
	}
	var clientInbounds []uint
	err = json.Unmarshal(client.Inbounds, &clientInbounds)
	if err != nil {
		return nil, nil, err
	}
	var inbounds []*model.Inbound
	err = db.Model(model.Inbound{}).Preload("Tls").Where("id in ?", clientInbounds).Find(&inbounds).Error
	if err != nil {
		return nil, nil, err
	}
	inbounds = util.OrderBaseInboundPtrsByIDs(clientInbounds, inbounds)
	return client, inbounds, nil
}

func (j *JsonService) overrideServerIP(outbounds *[]map[string]interface{}, serverIp string) {
	serverHost := util.NormalizeSubscriptionServerHost(serverIp)
	if serverHost == "" {
		return
	}
	for i := range *outbounds {
		outbound := &(*outbounds)[i]
		outType, _ := (*outbound)["type"].(string)
		// Skip virtual/helper outbounds.
		if outType == "selector" || outType == "urltest" || outType == "direct" || outType == "block" || outType == "dns" {
			continue
		}
		if _, ok := (*outbound)["server"]; ok {
			(*outbound)["server"] = serverHost
		}
	}
}

// extractTlsStore reads certificate store from inbound-linked TLS client config.
func (j *JsonService) extractTlsStore(inbounds []*model.Inbound) string {
	for _, inData := range inbounds {
		if inData.TlsId > 0 && inData.Tls != nil && len(inData.Tls.Client) > 0 {
			var tlsClient map[string]interface{}
			if err := json.Unmarshal(inData.Tls.Client, &tlsClient); err == nil {
				if store, ok := tlsClient["tls_store"].(string); ok && store != "" {
					return store
				}
				if store, ok := tlsClient["store"].(string); ok && store != "" {
					return store
				}
			}
		}
	}
	return ""
}

func extractAndStripTlsStoreFromOutbounds(outbounds *[]map[string]interface{}) string {
	var tlsStore string
	for i := range *outbounds {
		outbound := &(*outbounds)[i]
		tlsRaw, ok := (*outbound)["tls"]
		if !ok {
			continue
		}
		tlsMap, ok := tlsRaw.(map[string]interface{})
		if !ok {
			continue
		}
		if store, ok := tlsMap["tls_store"].(string); ok && store != "" && tlsStore == "" {
			tlsStore = store
		}
		if store, ok := tlsMap["store"].(string); ok && store != "" && tlsStore == "" {
			tlsStore = store
		}
		delete(tlsMap, "tls_store")
		delete(tlsMap, "store")
	}
	return tlsStore
}

func applyCertificateStore(jsonConfig *map[string]interface{}, tlsStore string) {
	if tlsStore == "" {
		return
	}

	certificate := map[string]interface{}{}
	if existing, ok := (*jsonConfig)["certificate"].(map[string]interface{}); ok && existing != nil {
		for k, v := range existing {
			certificate[k] = v
		}
	}
	certificate["store"] = tlsStore
	(*jsonConfig)["certificate"] = certificate
}

func (j *JsonService) getOutbounds(clientName string, clientConfig json.RawMessage, inbounds []*model.Inbound) (*[]map[string]interface{}, *[]string, error) {
	return j.getOutboundsForNamespace(clientName, clientConfig, inbounds, "default")
}

func (j *JsonService) getOutboundsForNamespace(clientName string, clientConfig json.RawMessage, inbounds []*model.Inbound, namespace string) (*[]map[string]interface{}, *[]string, error) {
	var outbounds []map[string]interface{}
	var configs map[string]interface{}
	var outTags []string

	err := json.Unmarshal(clientConfig, &configs)
	if err != nil {
		return nil, nil, err
	}
	for _, inData := range inbounds {
		if len(inData.OutJson) < 5 {
			continue
		}
		var outbound map[string]interface{}
		err = json.Unmarshal(inData.OutJson, &outbound)
		if err != nil {
			return nil, nil, err
		}
		refreshSubscriptionOutboundTLS(outbound, inData.Tls)
		protocol, _ := outbound["type"].(string)
		if protocol == "trusttunnel" {
			util.SanitizeTrustTunnelOutbound(outbound)
		}
		if protocol == "naive" {
			normalizeNaiveSubscriptionOutbound(outbound)
		}
		if protocol == "hysteria2" {
			util.SanitizeOptionalNetworkField(outbound)
		}

		// ShadowTLS: 生成 shadowsocks + shadowtls 两个出站
		if protocol == "shadowtls" {
			ssOutbound, stlsOutbound := j.buildShadowTLSOutbounds(outbound, configs, inData)
			if ssOutbound != nil && stlsOutbound != nil {
				var addrs []map[string]interface{}
				json.Unmarshal(inData.Addrs, &addrs)
				tag, _ := ssOutbound["tag"].(string)
				stlsTag, _ := stlsOutbound["tag"].(string)
				if len(addrs) == 0 {
					outTags = append(outTags, tag)
					outbounds = append(outbounds, ssOutbound)
					outbounds = append(outbounds, stlsOutbound)
				} else {
					for index, addr := range addrs {
						// Clone shadowsocks outbound
						newSsOut := make(map[string]interface{}, len(ssOutbound))
						for k, v := range ssOutbound {
							newSsOut[k] = v
						}
						// Clone shadowtls outbound
						newStlsOut := make(map[string]interface{}, len(stlsOutbound))
						for k, v := range stlsOutbound {
							newStlsOut[k] = v
						}
						// 更新 server/port
						newStlsOut["server"], _ = addr["server"].(string)
						port, _ := addr["server_port"].(float64)
						newStlsOut["server_port"] = int(port)

						// Override TLS
						if addrTls, ok := addr["tls"].(map[string]interface{}); ok {
							outTls, _ := newStlsOut["tls"].(map[string]interface{})
							if outTls == nil {
								outTls = make(map[string]interface{})
							}
							for key, value := range addrTls {
								outTls[key] = value
							}
							newStlsOut["tls"] = outTls
						}

						remark, _ := addr["remark"].(string)
						newTag := fmt.Sprintf("%d.%s%s", index+1, tag, remark)
						newStlsTag := fmt.Sprintf("%d.%s%s", index+1, stlsTag, remark)
						newSsOut["tag"] = newTag
						newSsOut["detour"] = newStlsTag
						newStlsOut["tag"] = newStlsTag
						outTags = append(outTags, newTag)
						outbounds = append(outbounds, newSsOut)
						outbounds = append(outbounds, newStlsOut)
					}
				}
			}
			continue
		}

		// Shadowsocks
		if protocol == "shadowsocks" {
			var inbOptions map[string]interface{}
			err = json.Unmarshal(inData.Options, &inbOptions)
			if err != nil {
				return nil, nil, err
			}
			if inbPass, ok := inbOptions["password"].(string); ok && inbPass != "" {
				outbound["password"] = inbPass
			}
		} else { // Other protocols
			config, _ := configs[protocol].(map[string]interface{})
			for key, value := range config {
				if shouldSkipSubscriptionClientConfigKey(namespace, protocol, key, inData.TlsId != 0) || (protocol == "trusttunnel" && (key == "uuid" || key == "network")) || (protocol == "sudoku" && key == "uuid") {
					continue
				}
				outbound[key] = value
			}
			if namespace == "mihomo" && !util.ShouldSkipMihomoOutboundClientConfigKey(protocol, "username", inData.TlsId != 0) {
				if strings.TrimSpace(firstString(outbound["username"])) == "" {
					if username := strings.TrimSpace(firstString(config["username"])); username != "" {
						outbound["username"] = username
					} else if legacyName := strings.TrimSpace(firstString(config["name"])); legacyName != "" {
						outbound["username"] = legacyName
					}
				}
			}
			if protocol == "sudoku" {
				applySubscriptionSudokuConfig(outbound, config, inData.Options)
			}
			if protocol == "trusttunnel" {
				util.ApplyTrustTunnelCredentials(outbound, config, clientName)
				util.SanitizeTrustTunnelOutbound(outbound)
			}
		}
		if protocol == "hysteria" {
			util.ApplyHysteriaInboundQUICToOutbound(outbound, inData.Options)
		}

		outboundVariants := []map[string]interface{}{outbound}
		if protocol == "mieru" {
			outboundVariants = expandMieruSubscriptionOutbounds(outbound)
		}

		var addrs []map[string]interface{}
		err = json.Unmarshal(inData.Addrs, &addrs)
		if err != nil {
			return nil, nil, err
		}
		tag, _ := outbound["tag"].(string)
		if len(addrs) == 0 {
			for _, variant := range outboundVariants {
				variantTag, _ := variant["tag"].(string)
				if protocol == "mixed" {
					variant["tag"] = variantTag
					j.pushMixed(&outbounds, &outTags, variant)
					continue
				}
				outTags = append(outTags, variantTag)
				outbounds = append(outbounds, variant)
			}
		} else {
			for index, addr := range addrs {
				for _, variant := range outboundVariants {
					newOut := make(map[string]interface{}, len(variant))
					for key, value := range variant {
						newOut[key] = value
					}
					newOut["server"], _ = addr["server"].(string)
					port, _ := addr["server_port"].(float64)
					if protocol != "mieru" || strings.TrimSpace(firstString(newOut["port_range"])) == "" {
						newOut["server_port"] = int(port)
					}

					if addrTls, ok := addr["tls"].(map[string]interface{}); ok {
						outTls, _ := newOut["tls"].(map[string]interface{})
						if outTls == nil {
							outTls = make(map[string]interface{})
						}
						for key, value := range addrTls {
							outTls[key] = value
						}
						newOut["tls"] = outTls
					}

					remark, _ := addr["remark"].(string)
					variantTag, _ := variant["tag"].(string)
					if variantTag == "" {
						variantTag = tag
					}
					newTag := fmt.Sprintf("%d.%s%s", index+1, variantTag, remark)
					newOut["tag"] = newTag
					if protocol == "mixed" {
						j.pushMixed(&outbounds, &outTags, newOut)
					} else {
						outTags = append(outTags, newTag)
						outbounds = append(outbounds, newOut)
					}
				}
			}
		}
	}
	return &outbounds, &outTags, nil
}

func shouldSkipSubscriptionClientConfigKey(namespace string, protocol string, key string, hasTLS bool) bool {
	if namespace == "mihomo" {
		return util.ShouldSkipMihomoOutboundClientConfigKey(protocol, key, hasTLS)
	}
	return util.ShouldSkipSingboxOutboundClientConfigKey(protocol, key, hasTLS)
}

func applySubscriptionSudokuConfig(outbound map[string]interface{}, config map[string]interface{}, inboundOptions json.RawMessage) {
	if outbound == nil {
		return
	}

	key := strings.TrimSpace(util.NormalizeSudokuKeyValue(config["uuid"]))
	if key == "" {
		key = subscriptionSudokuKeyFromInboundOptions(inboundOptions)
	}
	if key != "" {
		outbound["key"] = key
	}
	delete(outbound, "uuid")
}

func subscriptionSudokuKeyFromInboundOptions(options json.RawMessage) string {
	if len(options) == 0 {
		return ""
	}

	payload := map[string]interface{}{}
	if err := json.Unmarshal(options, &payload); err != nil {
		return ""
	}

	return strings.TrimSpace(util.NormalizeSudokuKeyValue(payload["key"]))
}

func (j *JsonService) addDefaultOutbounds(outbounds *[]map[string]interface{}, outTags *[]string, latencyUrl string, latencyInterval string, latencyTolerance int, selectorGroups []selectorGroupConfig) {
	customSelectors := buildNamedSelectorOutbounds(selectorGroups, *outTags)
	outbound := []map[string]interface{}{
		{
			"outbounds":                   append([]string{autoSelectorTag}, *outTags...),
			"tag":                         nodeSelectorTag,
			"type":                        "selector",
			"interrupt_exist_connections": true,
		},
		{
			"tag":                         autoSelectorTag,
			"type":                        "urltest",
			"outbounds":                   outTags,
			"url":                         latencyUrl,
			"interval":                    latencyInterval,
			"tolerance":                   latencyTolerance,
			"interrupt_exist_connections": true,
		},
		{
			"outbounds":                   append([]string{"direct", "block"}, *outTags...),
			"tag":                         globalDirectSelectorTag,
			"type":                        "selector",
			"interrupt_exist_connections": true,
		},
		{
			"outbounds":                   append([]string{"block", "direct"}, *outTags...),
			"tag":                         globalBlockSelectorTag,
			"type":                        "selector",
			"interrupt_exist_connections": true,
		},
		{
			"outbounds":                   append([]string{nodeSelectorTag, globalDirectSelectorTag}, *outTags...),
			"tag":                         finalSelectorTag,
			"type":                        "selector",
			"interrupt_exist_connections": true,
		},
		{
			"outbounds":                   append([]string{nodeSelectorTag, autoSelectorTag, globalDirectSelectorTag, globalBlockSelectorTag, finalSelectorTag}, *outTags...),
			"tag":                         globalSelectorTag,
			"type":                        "selector",
			"interrupt_exist_connections": true,
		},
	}
	outbound = append(outbound, customSelectors...)
	outbound = append(outbound,
		map[string]interface{}{"type": "direct", "tag": "direct"},
		map[string]interface{}{"type": "block", "tag": "block"},
	)
	*outbounds = append(outbound, *outbounds...)
}

func (j *JsonService) addOthers(jsonConfig *map[string]interface{}) error {
	route := map[string]interface{}{
		"auto_detect_interface":   true,
		"default_domain_resolver": "proxy-dns",
		"final":                   finalSelectorTag,
		"rules":                   []interface{}{},
	}

	othersStr, err := j.SettingService.GetSubJsonExt()
	if err != nil {
		return err
	}
	if len(othersStr) == 0 {
		(*jsonConfig)["route"] = route
		return nil
	}
	var othersJson map[string]interface{}
	err = json.Unmarshal([]byte(othersStr), &othersJson)
	if err != nil {
		return err
	}
	if _, ok := othersJson["log"]; ok {
		(*jsonConfig)["log"] = othersJson["log"]
	}
	if dns, ok := othersJson["dns"]; ok {
		(*jsonConfig)["dns"] = normalizeDnsDetours(reorderManagedCustomDnsRules(removeDeprecatedDnsClashModeRules(dns)))
	}
	if _, ok := othersJson["inbounds"]; ok {
		(*jsonConfig)["inbounds"] = othersJson["inbounds"]
	}
	if experimental, ok := othersJson["experimental"]; ok {
		(*jsonConfig)["experimental"] = normalizeExperimentalClashAPIDetour(experimental)
	}
	if _, ok := othersJson["certificate"]; ok {
		(*jsonConfig)["certificate"] = othersJson["certificate"]
	}
	if httpClients, ok := buildSubHTTPClients(othersJson); ok {
		(*jsonConfig)["http_clients"] = httpClients
		route["default_http_client"] = managedSubHTTPClientTag
	}
	if _, ok := othersJson["rule_set"]; ok {
		route["rule_set"] = normalizeRuleSetDownloadDetours(othersJson["rule_set"])
	}
	if settingRules, ok := othersJson["rules"].([]interface{}); ok {
		route["rules"] = normalizeRouteRules(settingRules)
	}
	// Remove front-end-only UI state from generated config.
	delete(othersJson, "_uiConfig")
	if routeFinal, ok := othersJson["route_final"].(string); ok {
		route["final"] = normalizeRouteFinalOutbound(routeFinal)
	}
	if defaultDomainResolver, ok := othersJson["default_domain_resolver"].(string); ok {
		route["default_domain_resolver"] = defaultDomainResolver
	}
	(*jsonConfig)["route"] = route

	return nil
}

func normalizeRouteFinalOutbound(routeFinal string) string {
	normalized := normalizeLegacySingboxSelectorTag(routeFinal)
	switch normalized {
	case nodeSelectorTag, autoSelectorTag, globalDirectSelectorTag, globalBlockSelectorTag, finalSelectorTag:
		return normalized
	case globalSelectorTag:
		return globalSelectorTag
	}

	switch strings.ToLower(strings.TrimSpace(normalized)) {
	case "", "proxy":
		// Keep legacy "proxy" final behavior for backward compatibility.
		return finalSelectorTag
	case "auto":
		return autoSelectorTag
	case "direct", "global-direct":
		return globalDirectSelectorTag
	case "global-proxy", "global":
		return globalSelectorTag
	case "global-block", "block", "reject", "reject-drop":
		return globalBlockSelectorTag
	case "final":
		return finalSelectorTag
	default:
		return normalized
	}
}

func normalizeRuleSetDownloadDetours(ruleSet interface{}) interface{} {
	ruleSetList, ok := ruleSet.([]interface{})
	if !ok {
		return ruleSet
	}

	for _, raw := range ruleSetList {
		ruleMap, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		detour, _ := ruleMap["download_detour"].(string)
		if detour == "" {
			continue
		}
		normalizedDetour := normalizeDetourTag(detour)
		delete(ruleMap, "download_detour")
		ruleMap["http_client"] = map[string]interface{}{
			"detour": normalizedDetour,
		}
	}

	return ruleSetList
}

func normalizeDnsDetours(dns interface{}) interface{} {
	dnsMap, ok := dns.(map[string]interface{})
	if !ok {
		return dns
	}

	servers, ok := dnsMap["servers"].([]interface{})
	if !ok || len(servers) == 0 {
		return dnsMap
	}

	for _, rawServer := range servers {
		serverMap, ok := rawServer.(map[string]interface{})
		if !ok {
			continue
		}
		detour, _ := serverMap["detour"].(string)
		if detour == "" {
			continue
		}
		serverMap["detour"] = normalizeDetourTag(detour)
	}

	return dnsMap
}

func normalizeExperimentalClashAPIDetour(experimental interface{}) interface{} {
	experimentalMap, ok := experimental.(map[string]interface{})
	if !ok || experimentalMap == nil {
		return experimental
	}

	clashAPI, ok := experimentalMap["clash_api"].(map[string]interface{})
	if !ok || clashAPI == nil {
		return experimentalMap
	}

	rawDetour, ok := clashAPI["external_ui_download_detour"].(string)
	if !ok {
		return experimentalMap
	}

	detour := strings.TrimSpace(rawDetour)
	if detour == "" {
		delete(clashAPI, "external_ui_download_detour")
		return experimentalMap
	}

	// Convert when possible; keep original value when conversion is not possible.
	clashAPI["external_ui_download_detour"] = normalizeDetourTag(detour)
	return experimentalMap
}

func normalizeRouteOutboundTag(outbound string) string {
	normalized := normalizeLegacySingboxSelectorTag(outbound)
	switch strings.ToLower(normalized) {
	case "proxy":
		return nodeSelectorTag
	case "auto":
		return autoSelectorTag
	case "direct", "global-direct":
		return globalDirectSelectorTag
	case "global-proxy":
		return globalSelectorTag
	default:
		return normalized
	}
}

func buildSubHTTPClients(extJson map[string]interface{}) ([]map[string]interface{}, bool) {
	if extJson == nil {
		return nil, false
	}

	rawDetour, ok := extJson["update_method"].(string)
	if !ok {
		return nil, false
	}

	detour := normalizeDetourTag(rawDetour)
	if strings.TrimSpace(detour) == "" {
		return nil, false
	}

	return []map[string]interface{}{
		{
			"tag":    managedSubHTTPClientTag,
			"detour": detour,
		},
	}, true
}

func normalizeDetourTag(detour string) string {
	normalized := normalizeLegacySingboxSelectorTag(detour)
	switch normalized {
	case nodeSelectorTag, autoSelectorTag, globalDirectSelectorTag, globalBlockSelectorTag, finalSelectorTag:
		return normalized
	case globalSelectorTag:
		return globalSelectorTag
	}
	switch strings.ToLower(strings.TrimSpace(normalized)) {
	case "proxy":
		return nodeSelectorTag
	case "auto":
		return autoSelectorTag
	case "direct", "global-direct":
		return globalDirectSelectorTag
	case "global-proxy", "global":
		return globalSelectorTag
	case "global-block", "block", "reject", "reject-drop":
		return globalBlockSelectorTag
	case "final":
		return finalSelectorTag
	default:
		return normalized
	}
}

func parseSelectorGroupsFromExt(extJson map[string]interface{}) []selectorGroupConfig {
	if extJson == nil {
		return nil
	}

	rawGroups, ok := extJson["selector_groups"].([]interface{})
	if !ok || len(rawGroups) == 0 {
		return nil
	}

	reserved := map[string]struct{}{
		nodeSelectorTag:         {},
		autoSelectorTag:         {},
		globalDirectSelectorTag: {},
		globalBlockSelectorTag:  {},
		finalSelectorTag:        {},
		globalSelectorTag:       {},
		"direct":                {},
		"block":                 {},
	}

	seen := make(map[string]struct{}, len(rawGroups))
	groups := make([]selectorGroupConfig, 0, len(rawGroups))
	for _, raw := range rawGroups {
		groupMap, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}

		tag, _ := groupMap["tag"].(string)
		if strings.TrimSpace(tag) == "" {
			if fallback, ok := groupMap["name"].(string); ok {
				tag = fallback
			}
		}
		tag = normalizeLegacySingboxSelectorTag(tag)
		if tag == "" {
			continue
		}
		if _, exists := reserved[tag]; exists {
			continue
		}
		if _, exists := seen[tag]; exists {
			continue
		}
		seen[tag] = struct{}{}

		defaultOutbound, _ := groupMap["default_outbound"].(string)
		if strings.TrimSpace(defaultOutbound) == "" {
			if fallback, ok := groupMap["default"].(string); ok {
				defaultOutbound = fallback
			}
		}
		defaultOutbound = normalizeRouteOutboundTag(defaultOutbound)
		if defaultOutbound == "" || strings.EqualFold(defaultOutbound, "reject") || strings.EqualFold(defaultOutbound, "block") {
			defaultOutbound = nodeSelectorTag
		}

		groups = append(groups, selectorGroupConfig{
			Tag:             tag,
			DefaultOutbound: defaultOutbound,
		})
	}

	return groups
}

func buildNamedSelectorOutbounds(groups []selectorGroupConfig, nodeTags []string) []map[string]interface{} {
	if len(groups) == 0 {
		return nil
	}

	result := make([]map[string]interface{}, 0, len(groups))
	for _, group := range groups {
		if strings.TrimSpace(group.Tag) == "" {
			continue
		}
		result = append(result, map[string]interface{}{
			"type":                        "selector",
			"tag":                         group.Tag,
			"outbounds":                   buildNamedSelectorOptions(group.DefaultOutbound, nodeTags),
			"interrupt_exist_connections": true,
		})
	}
	return result
}

func buildNamedSelectorOptions(defaultOutbound string, nodeTags []string) []string {
	base := []string{
		nodeSelectorTag,
		autoSelectorTag,
		globalDirectSelectorTag,
		globalBlockSelectorTag,
		finalSelectorTag,
	}

	result := make([]string, 0, len(base)+len(nodeTags)+1)
	seen := make(map[string]struct{}, len(base)+len(nodeTags)+1)
	add := func(tag string) {
		normalized := strings.TrimSpace(tag)
		if normalized == "" {
			return
		}
		if _, exists := seen[normalized]; exists {
			return
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}

	add(defaultOutbound)
	for _, tag := range base {
		add(tag)
	}
	for _, tag := range nodeTags {
		add(tag)
	}

	return result
}

// stripMihomoFields removes mihomo-only outbound fields that are invalid in sing-box.
func stripMihomoFields(outbounds *[]map[string]interface{}) {
	for i := range *outbounds {
		// Keep a defensive sanitize step here so callers that only invoke
		// stripMihomoFields still produce sing-box-safe transport blocks.
		util.SanitizeSingboxSubscriptionOutbound((*outbounds)[i])
		delete((*outbounds)[i], "mihomo_common")
		delete((*outbounds)[i], "mihomo_hy2")
		delete((*outbounds)[i], "mihomo_fast_open")
		delete((*outbounds)[i], "fast_open")
		if tlsMap, ok := (*outbounds)[i]["tls"].(map[string]interface{}); ok {
			delete(tlsMap, "mihomo_use_fingerprint")
			delete(tlsMap, "fingerprint")
			delete(tlsMap, "include_server_certificate")
			delete(tlsMap, "include_server_fingerprint")
		}
	}
}

func normalizeNaiveSubscriptionOutbound(outbound map[string]interface{}) {
	if outbound == nil {
		return
	}
	delete(outbound, "network")
	if tlsMap, ok := outbound["tls"].(map[string]interface{}); ok && tlsMap != nil {
		delete(tlsMap, "alpn")
		delete(tlsMap, "utls")
	}
	if value, ok := outbound["quic_congestion_control"].(string); ok && strings.TrimSpace(value) == "" {
		delete(outbound, "quic_congestion_control")
	}
}

func removeDeprecatedDnsClashModeRules(dns interface{}) interface{} {
	dnsMap, ok := dns.(map[string]interface{})
	if !ok {
		return dns
	}

	rules, ok := dnsMap["rules"].([]interface{})
	if !ok || len(rules) == 0 {
		return dnsMap
	}

	filteredRules := make([]interface{}, 0, len(rules))
	for _, rawRule := range rules {
		ruleMap, ok := rawRule.(map[string]interface{})
		if !ok {
			filteredRules = append(filteredRules, rawRule)
			continue
		}

		action, _ := ruleMap["action"].(string)
		clashMode, _ := ruleMap["clash_mode"].(string)
		server, _ := ruleMap["server"].(string)
		if strings.EqualFold(action, "route") &&
			((strings.EqualFold(clashMode, "global") && server == "proxy-dns") ||
				(strings.EqualFold(clashMode, "direct") && server == "direct-dns")) {
			continue
		}

		filteredRules = append(filteredRules, ruleMap)
	}

	dnsMap["rules"] = filteredRules
	return dnsMap
}

func hasNonEmptyStringArrayValue(value interface{}) bool {
	switch list := value.(type) {
	case []interface{}:
		for _, item := range list {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				return true
			}
		}
	case []string:
		for _, item := range list {
			if strings.TrimSpace(item) != "" {
				return true
			}
		}
	}
	return false
}

func isManagedCustomDomainMatcherDnsRule(rule map[string]interface{}) bool {
	if _, hasRuleSet := rule["rule_set"]; hasRuleSet {
		return false
	}
	if _, hasQueryType := rule["query_type"]; hasQueryType {
		return false
	}

	action, _ := rule["action"].(string)
	action = strings.ToLower(strings.TrimSpace(action))
	if action != "reject" && action != "route" {
		return false
	}
	if action == "route" {
		server, _ := rule["server"].(string)
		if strings.TrimSpace(server) == "" {
			return false
		}
	}

	matcherCount := 0
	for _, key := range []string{"domain", "domain_suffix", "domain_keyword", "domain_regex"} {
		if hasNonEmptyStringArrayValue(rule[key]) {
			matcherCount++
		}
	}
	return matcherCount == 1
}

func isRuleSetDnsRouteRule(rule map[string]interface{}) bool {
	action, _ := rule["action"].(string)
	if !strings.EqualFold(strings.TrimSpace(action), "route") {
		return false
	}
	_, hasRuleSet := rule["rule_set"]
	return hasRuleSet
}

func isQueryTypeDnsRouteRule(rule map[string]interface{}) bool {
	action, _ := rule["action"].(string)
	if !strings.EqualFold(strings.TrimSpace(action), "route") {
		return false
	}
	_, hasQueryType := rule["query_type"]
	return hasQueryType
}

func reorderManagedCustomDnsRules(dns interface{}) interface{} {
	dnsMap, ok := dns.(map[string]interface{})
	if !ok {
		return dns
	}

	rules, ok := dnsMap["rules"].([]interface{})
	if !ok || len(rules) == 0 {
		return dnsMap
	}

	customMatcherRules := make([]interface{}, 0, len(rules))
	managedRouteRules := make([]interface{}, 0, len(rules))
	otherRules := make([]interface{}, 0, len(rules))

	for _, rawRule := range rules {
		ruleMap, ok := rawRule.(map[string]interface{})
		if !ok {
			otherRules = append(otherRules, rawRule)
			continue
		}

		if isManagedCustomDomainMatcherDnsRule(ruleMap) {
			customMatcherRules = append(customMatcherRules, ruleMap)
			continue
		}
		if isRuleSetDnsRouteRule(ruleMap) || isQueryTypeDnsRouteRule(ruleMap) {
			managedRouteRules = append(managedRouteRules, ruleMap)
			continue
		}
		otherRules = append(otherRules, ruleMap)
	}

	dnsMap["rules"] = append(append(customMatcherRules, managedRouteRules...), otherRules...)
	return dnsMap
}

func normalizeRouteRules(rules []interface{}) []interface{} {
	normalized := make([]interface{}, 0, len(rules))
	for _, rawRule := range rules {
		ruleMap, ok := rawRule.(map[string]interface{})
		if !ok {
			normalized = append(normalized, rawRule)
			continue
		}
		normalized = append(normalized, normalizeRouteRuleMap(ruleMap))
	}
	return normalized
}

func normalizeRouteRuleMap(rule map[string]interface{}) map[string]interface{} {
	action, _ := rule["action"].(string)
	outbound, _ := rule["outbound"].(string)
	clashMode, _ := rule["clash_mode"].(string)

	// Migrate legacy reject/block outbound to rule action (sing-box new format).
	// Old: { "action": "route", "outbound": "reject" } or { "action": "route", "outbound": "block" }
	// New: { "action": "reject" }
	if strings.EqualFold(action, "route") && (outbound == "reject" || outbound == "block") {
		rule["action"] = "reject"
		delete(rule, "outbound")
		action = "reject"
		outbound = ""
	}

	if strings.EqualFold(action, "route") {
		switch {
		case strings.EqualFold(clashMode, "global"):
			rule["outbound"] = globalSelectorTag
		case strings.EqualFold(clashMode, "direct"):
			if strings.TrimSpace(outbound) == "" {
				rule["outbound"] = globalDirectSelectorTag
			} else {
				rule["outbound"] = normalizeRouteOutboundTag(outbound)
			}
		default:
			normalized := normalizeRouteOutboundTag(outbound)
			if normalized != "" {
				rule["outbound"] = normalized
			}
		}
	}

	if nested, ok := rule["rules"].([]interface{}); ok {
		rule["rules"] = normalizeRouteRules(nested)
	}

	return rule
}

func (j *JsonService) pushMixed(outbounds *[]map[string]interface{}, outTags *[]string, out map[string]interface{}) {
	socksOut := make(map[string]interface{}, 1)
	httpOut := make(map[string]interface{}, 1)
	for key, value := range out {
		socksOut[key] = value
		httpOut[key] = value
	}
	socksTag := fmt.Sprintf("%s-socks", out["tag"])
	httpTag := fmt.Sprintf("%s-http", out["tag"])
	socksOut["type"] = "socks"
	httpOut["type"] = "http"
	socksOut["tag"] = socksTag
	httpOut["tag"] = httpTag
	*outbounds = append(*outbounds, socksOut, httpOut)
	*outTags = append(*outTags, socksTag, httpTag)
}

func expandMieruSubscriptionOutbounds(outbound map[string]interface{}) []map[string]interface{} {
	bindings := util.NormalizeMieruOutboundBindings(outbound)
	if len(bindings) == 0 {
		return []map[string]interface{}{outbound}
	}

	baseTag, _ := outbound["tag"].(string)
	result := make([]map[string]interface{}, 0, len(bindings))
	for index, binding := range bindings {
		variant := make(map[string]interface{}, len(outbound))
		for key, value := range outbound {
			variant[key] = value
		}
		delete(variant, "port_bindings")
		delete(variant, "port_range")
		if port, ok := util.MieruPrimaryPortFromBinding(binding); ok {
			variant["server_port"] = port
		}
		if strings.Contains(binding, "-") {
			variant["port_range"] = binding
		}
		if len(bindings) > 1 && strings.TrimSpace(baseTag) != "" {
			variant["tag"] = fmt.Sprintf("%d.%s", index+1, baseTag)
		}
		result = append(result, variant)
	}
	return result
}

// buildShadowTLSOutbounds builds two outbounds from out_json for ShadowTLS: shadowsocks + shadowtls.
// 按照图b格式:
//
//	shadowsocks: type, tag, method, password, detour, udp_over_tcp, multiplex
//	shadowtls: type, tag(-out), server, server_port, version, password, tls
func (j *JsonService) buildShadowTLSOutbounds(outJson map[string]interface{}, configs map[string]interface{}, inData *model.Inbound) (map[string]interface{}, map[string]interface{}) {
	return util.BuildShadowTLSClientPair(outJson, configs, inData.Options)
}

func (j *JsonService) buildShadowTLSOutboundsLegacy(outJson map[string]interface{}, configs map[string]interface{}, inData *model.Inbound) (map[string]interface{}, map[string]interface{}) {
	tag, _ := outJson["tag"].(string)
	if tag == "" {
		return nil, nil
	}

	// Read ss_config.
	ssConfig, hasSsConfig := outJson["ss_config"].(map[string]interface{})

	// Read password from the user's ShadowTLS settings.
	stlsConfig, _ := configs["shadowtls"].(map[string]interface{})
	stlsPassword, _ := stlsConfig["password"].(string)

	// 构建 shadowtls 出站
	stlsTag := tag + "-out"
	stlsOutbound := map[string]interface{}{
		"type":        "shadowtls",
		"tag":         stlsTag,
		"server":      outJson["server"],
		"server_port": outJson["server_port"],
		"version":     outJson["version"],
		"password":    stlsPassword,
	}

	// 复制 TLS 配置
	if tls, ok := outJson["tls"]; ok {
		stlsOutbound["tls"] = tls
	}

	if !hasSsConfig || ssConfig == nil {
		// Comment cleaned to avoid mojibake.
		return nil, stlsOutbound
	}

	// 构建 shadowsocks 出站
	ssOutbound := map[string]interface{}{
		"type":   "shadowsocks",
		"tag":    tag,
		"detour": stlsTag,
	}

	if method, ok := ssConfig["method"]; ok {
		ssOutbound["method"] = method
	}
	if network, ok := ssConfig["network"]; ok && network != nil && network != "" {
		ssOutbound["network"] = network
	}
	if password, ok := ssConfig["password"]; ok {
		ssOutbound["password"] = password
	}
	// udp_over_tcp
	if udpOverTcp, ok := ssConfig["udp_over_tcp"]; ok {
		ssOutbound["udp_over_tcp"] = udpOverTcp
	}
	// multiplex
	if multiplex, ok := ssConfig["multiplex"]; ok {
		ssOutbound["multiplex"] = multiplex
	}

	return ssOutbound, stlsOutbound
}
