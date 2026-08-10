package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/alireza0/s-ui/util/common"

	"gopkg.in/yaml.v3"
)

const SubscriptionExtensionMaxBytes = 4 * 1024 * 1024

const SubscriptionJSONBaseConfig = `{
  "inbounds": [
    {
      "type": "tun",
      "address": ["172.19.0.1/30", "fdfe:dcba:9876::1/126"],
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
}`

const SubscriptionClashBaseConfig = `mixed-port: 7890
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
  use-system-hosts: false
  enhanced-mode: fake-ip
  fake-ip-range: 198.18.0.1/15
  default-nameserver:
    - udp://223.5.5.5
    - udp://223.6.6.6
  nameserver:
    - "udp://8.8.8.8#节点选择"
    - "tcp://8.8.8.8#节点选择"
  fallback:
    - "udp://8.8.4.4#节点选择"
    - "tcp://8.8.4.4#节点选择"
  proxy-server-nameserver:
    - udp://223.5.5.5
    - udp://223.6.6.6
  fake-ip-filter:
    - "*.lan"
    - localhost
    - "*.local"
sniffer:
  enable: true
  force-dns-mapping: true
  parse-pure-ip: true
  sniff:
    HTTP:
      ports:
        - 1-65535
    TLS:
      ports:
        - 1-65535
    QUIC:
      ports:
        - 1-65535
  override-destination: true
rules:
  - AND,((NETWORK,UDP),(DST-PORT,80)),REJECT
  - AND,((NETWORK,UDP),(DST-PORT,443)),REJECT
  - AND,((NETWORK,UDP),(DST-PORT,2443)),REJECT
  - AND,((NETWORK,UDP),(DST-PORT,4443)),REJECT
  - AND,((NETWORK,UDP),(DST-PORT,6443)),REJECT
  - AND,((NETWORK,UDP),(DST-PORT,8080)),REJECT
  - AND,((NETWORK,UDP),(DST-PORT,8081)),REJECT
  - AND,((NETWORK,UDP),(DST-PORT,8443)),REJECT
  - GEOIP,Private,DIRECT
  - MATCH,节点选择
`

const canonicalSubJSONExtension = `{
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
  ],
  "route_final": "节点选择",
  "latency_test_url": "http://www.gstatic.com/generate_204",
  "latency_test_interval": "10m",
  "latency_tolerance": 50,
  "_uiConfig": {
    "ruleSetSource": "karingx_github",
    "ruleRows": [],
    "dnsRouteRows": [],
    "updateMethod": "节点选择",
    "updateInterval": "1d",
    "routeFinal": "节点选择",
    "latencyTestUrl": "http://www.gstatic.com/generate_204",
    "latencyTestInterval": "10m",
    "latencyTolerance": "50",
    "enableSniff": false,
    "enableHijackDns": false,
    "enableRejectQuic": false,
    "enableReject443Udp": false
  }
}`

const canonicalSubClashExtension = SubscriptionClashBaseConfig + `_uiConfig:
  ruleSetSource: metacubex_cdn
  clashRuleRows: []
  clashDnsPolicyRows: []
  clashDnsSuffixRows: []
  noResolveGlobal: true
  updateMethod: 节点选择
  updateInterval: 1d
  routeFinal: 节点选择
  latencyTestUrl: http://www.gstatic.com/generate_204
  latencyTestInterval: 180s
  latencyTolerance: "50"
  enableSniff: true
  snifferOverrideDestination: true
  snifferForceDnsMapping: true
  snifferParsePureIp: true
  enableRejectQuic: true
  rejectUdpPortsInput: ""
`

func CanonicalSubJSONExtension() string {
	return canonicalSubJSONExtension
}

func CanonicalSubClashExtension() string {
	return canonicalSubClashExtension
}

func NormalizeSubscriptionExtension(key string, raw string) (string, error) {
	if !utf8.ValidString(raw) {
		return "", common.NewError("订阅扩展必须是有效的 UTF-8 文本")
	}
	if len([]byte(raw)) > SubscriptionExtensionMaxBytes {
		return "", common.NewErrorf("%s 超过 %d 字节限制", key, SubscriptionExtensionMaxBytes)
	}
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}

	switch key {
	case "subJsonExt":
		return normalizeSubJSONExtension(raw)
	case "subClashExt":
		return normalizeSubClashExtension(raw)
	default:
		return raw, nil
	}
}

func ParseSubJSONExtension(raw string) (map[string]interface{}, error) {
	if strings.TrimSpace(raw) == "" {
		raw = CanonicalSubJSONExtension()
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var value interface{}
	if err := decoder.Decode(&value); err != nil {
		return nil, common.NewErrorf("JSON 订阅扩展解析失败: %v", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, err
	}
	root, ok := value.(map[string]interface{})
	if !ok || root == nil {
		return nil, common.NewError("JSON 订阅扩展顶层必须是对象")
	}
	return root, nil
}

func ParseSubClashExtension(raw string) (map[string]interface{}, error) {
	if strings.TrimSpace(raw) == "" {
		raw = CanonicalSubClashExtension()
	}

	decoder := yaml.NewDecoder(strings.NewReader(raw))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return nil, common.NewErrorf("Clash 订阅扩展解析失败: %v", err)
	}
	if err := rejectDuplicateYAMLKeys(&document, "$"); err != nil {
		return nil, err
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, common.NewError("Clash 订阅扩展只能包含一个 YAML 文档")
		}
		return nil, common.NewErrorf("Clash 订阅扩展解析失败: %v", err)
	}

	root := yamlDocumentRoot(&document)
	if root == nil || root.Kind != yaml.MappingNode {
		return nil, common.NewError("Clash 订阅扩展顶层必须是映射对象")
	}
	value := map[string]interface{}{}
	if err := root.Decode(&value); err != nil {
		return nil, common.NewErrorf("Clash 订阅扩展解析失败: %v", err)
	}
	return value, nil
}

func ValidateSubJSONExtension(root map[string]interface{}) error {
	if root == nil {
		return common.NewError("JSON 订阅扩展顶层必须是对象")
	}
	return validateSubJSONExtension(root)
}

func ValidateSubClashExtension(root map[string]interface{}) error {
	if root == nil {
		return common.NewError("Clash 订阅扩展顶层必须是映射对象")
	}
	return validateSubClashExtension(root)
}

func normalizeSubJSONExtension(raw string) (string, error) {
	root, err := ParseSubJSONExtension(raw)
	if err != nil {
		return "", err
	}
	if err := validateSubJSONExtension(root); err != nil {
		return "", err
	}
	encoded, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", common.NewErrorf("JSON 订阅扩展序列化失败: %v", err)
	}
	return string(encoded), nil
}

func normalizeSubClashExtension(raw string) (string, error) {
	root, err := ParseSubClashExtension(raw)
	if err != nil {
		return "", err
	}
	if err := validateSubClashExtension(root); err != nil {
		return "", err
	}
	encoded, err := yaml.Marshal(root)
	if err != nil {
		return "", common.NewErrorf("Clash 订阅扩展序列化失败: %v", err)
	}
	return string(encoded), nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return common.NewError("JSON 订阅扩展只能包含一个 JSON 值")
		}
		return common.NewErrorf("JSON 订阅扩展解析失败: %v", err)
	}
	return nil
}

func yamlDocumentRoot(document *yaml.Node) *yaml.Node {
	if document == nil {
		return nil
	}
	if document.Kind == yaml.DocumentNode {
		if len(document.Content) == 0 {
			return nil
		}
		return document.Content[0]
	}
	return document
}

func rejectDuplicateYAMLKeys(node *yaml.Node, path string) error {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.DocumentNode {
		for _, child := range node.Content {
			if err := rejectDuplicateYAMLKeys(child, path); err != nil {
				return err
			}
		}
		return nil
	}
	if node.Kind == yaml.MappingNode {
		seen := make(map[string]struct{}, len(node.Content)/2)
		for i := 0; i+1 < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valueNode := node.Content[i+1]
			key := keyNode.Value
			if _, exists := seen[key]; exists {
				return common.NewErrorf("Clash 订阅扩展存在重复键 %s.%s", path, key)
			}
			seen[key] = struct{}{}
			if err := rejectDuplicateYAMLKeys(valueNode, path+"."+key); err != nil {
				return err
			}
		}
		return nil
	}
	for _, child := range node.Content {
		if err := rejectDuplicateYAMLKeys(child, path); err != nil {
			return err
		}
	}
	return nil
}

var subscriptionIntervalPattern = regexp.MustCompile(`^[1-9][0-9]*[smhd]$`)

func validateSubJSONExtension(root map[string]interface{}) error {
	if value, exists := root["latency_test_interval"]; exists {
		interval, ok := value.(string)
		if !ok || !subscriptionIntervalPattern.MatchString(strings.ToLower(strings.TrimSpace(interval))) {
			return common.NewError("JSON 延迟测试间隔必须是正整数并使用 s/m/h/d 后缀")
		}
	}
	if value, exists := root["latency_tolerance"]; exists {
		if _, err := positiveInteger(value, "JSON 延迟容差", 1_000_000); err != nil {
			return err
		}
	}
	if value, exists := root["latency_test_url"]; exists {
		if err := validateHTTPURLValue(value, "JSON 延迟测试地址"); err != nil {
			return err
		}
	}

	if inbounds, exists := root["inbounds"]; exists {
		items, ok := inbounds.([]interface{})
		if !ok {
			return common.NewError("JSON inbounds 必须是数组")
		}
		for index, item := range items {
			inbound, ok := item.(map[string]interface{})
			if !ok {
				return common.NewErrorf("JSON inbounds[%d] 必须是对象", index)
			}
			if value, exists := inbound["listen_port"]; exists {
				if err := validatePortValue(value, fmt.Sprintf("JSON inbounds[%d].listen_port", index)); err != nil {
					return err
				}
			}
			if value, exists := inbound["mtu"]; exists {
				mtu, err := positiveInteger(value, fmt.Sprintf("JSON inbounds[%d].mtu", index), 65535)
				if err != nil {
					return err
				}
				if mtu < 576 {
					return common.NewErrorf("JSON inbounds[%d].mtu 不能小于 576", index)
				}
			}
		}
	}
	if dns, exists := root["dns"]; exists {
		dnsMap, ok := dns.(map[string]interface{})
		if !ok {
			return common.NewError("JSON dns 必须是对象")
		}
		if err := validateSubJSONDNSServers(dnsMap); err != nil {
			return err
		}
	}
	if ruleSets, exists := root["rule_set"]; exists {
		if err := validateSingboxRuleSets(ruleSets); err != nil {
			return err
		}
	}
	if err := validateUIRuleSetSources(root["_uiConfig"], "ruleRows", "json", "ruleSetSource"); err != nil {
		return err
	}
	return validateUIRuleNameConflicts(root["_uiConfig"], "ruleRows")
}

func validateSubJSONDNSServers(dns map[string]interface{}) error {
	rawServers, exists := dns["servers"]
	if !exists {
		return nil
	}
	servers, ok := rawServers.([]interface{})
	if !ok {
		return common.NewError("JSON dns.servers 必须是数组")
	}
	seen := map[string]struct{}{}
	for index, raw := range servers {
		server, ok := raw.(map[string]interface{})
		if !ok {
			return common.NewErrorf("JSON dns.servers[%d] 必须是对象", index)
		}
		tag := strings.TrimSpace(stringValue(server["tag"]))
		serverType := strings.ToLower(strings.TrimSpace(stringValue(server["type"])))
		if tag == "" || serverType == "" {
			return common.NewErrorf("JSON dns.servers[%d] 必须填写 tag 和 type", index)
		}
		if _, duplicate := seen[tag]; duplicate {
			return common.NewErrorf("JSON DNS 标签重复: %s", tag)
		}
		seen[tag] = struct{}{}

		switch serverType {
		case "local", "dhcp", "fakeip":
		case "udp", "tcp", "tls", "quic", "https", "h3":
			if strings.TrimSpace(stringValue(server["server"])) == "" {
				return common.NewErrorf("JSON dns.servers[%d].server 不能为空", index)
			}
			if value, exists := server["server_port"]; exists {
				if err := validatePortValue(value, fmt.Sprintf("JSON dns.servers[%d].server_port", index)); err != nil {
					return err
				}
			}
		default:
			return common.NewErrorf("JSON dns.servers[%d].type 不受支持: %s", index, serverType)
		}
	}
	return nil
}

func validateSingboxRuleSets(raw interface{}) error {
	items, ok := raw.([]interface{})
	if !ok {
		return common.NewError("JSON rule_set 必须是数组")
	}
	seen := map[string]struct{}{}
	for index, rawItem := range items {
		item, ok := rawItem.(map[string]interface{})
		if !ok {
			return common.NewErrorf("JSON rule_set[%d] 必须是对象", index)
		}
		tag := strings.TrimSpace(stringValue(item["tag"]))
		if tag == "" {
			return common.NewErrorf("JSON rule_set[%d].tag 不能为空", index)
		}
		if _, exists := seen[tag]; exists {
			return common.NewErrorf("JSON rule_set 标签重复: %s", tag)
		}
		seen[tag] = struct{}{}
		format := strings.ToLower(strings.TrimSpace(stringValue(item["format"])))
		if format != "" && format != "source" && format != "binary" {
			return common.NewErrorf("JSON rule_set[%d].format 只能是 source 或 binary", index)
		}
		if remoteURL := strings.TrimSpace(stringValue(item["url"])); remoteURL != "" {
			parsed, err := url.Parse(remoteURL)
			if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
				return common.NewErrorf("JSON rule_set[%d].url 必须是有效的 HTTP/HTTPS 地址", index)
			}
			expectedFormat, err := ruleSetProbeFormatForURL("json", remoteURL)
			if err != nil {
				return common.NewErrorf("JSON rule_set[%d].url: %v", index, err)
			}
			if expectedFormat == "srs" && format != "binary" {
				return common.NewErrorf("JSON rule_set[%d] 的 .srs URL 必须使用 binary format", index)
			}
			if expectedFormat == "json" && format == "binary" {
				return common.NewErrorf("JSON rule_set[%d] 的 .json URL 不能使用 binary format", index)
			}
		}
	}
	return nil
}

func validateSubClashExtension(root map[string]interface{}) error {
	for _, key := range []string{"mixed-port", "port", "socks-port", "redir-port", "tproxy-port"} {
		if value, exists := root[key]; exists {
			if err := validatePortValue(value, "Clash "+key); err != nil {
				return err
			}
		}
	}
	if tun, ok := root["tun"].(map[string]interface{}); ok {
		if value, exists := tun["mtu"]; exists {
			mtu, err := positiveInteger(value, "Clash tun.mtu", 65535)
			if err != nil {
				return err
			}
			if mtu < 576 {
				return common.NewError("Clash tun.mtu 不能小于 576")
			}
		}
	}
	if dns, exists := root["dns"]; exists {
		dnsMap, ok := dns.(map[string]interface{})
		if !ok {
			return common.NewError("Clash dns 必须是对象")
		}
		if enabled, present := dnsMap["enable"].(bool); present && enabled {
			if !nonEmptyStringSequence(dnsMap["nameserver"]) {
				return common.NewError("Clash DNS 启用时 nameserver 不能为空")
			}
			if !nonEmptyStringSequence(dnsMap["default-nameserver"]) {
				return common.NewError("Clash DNS 启用时 default-nameserver 不能为空")
			}
		}
	}
	if err := validateNamedClashSequence(root["proxies"], "proxies"); err != nil {
		return err
	}
	if err := validateNamedClashSequence(root["proxy-groups"], "proxy-groups"); err != nil {
		return err
	}
	if providers, exists := root["rule-providers"]; exists {
		if err := validateClashRuleProviders(providers); err != nil {
			return err
		}
	}
	if uiConfig, ok := root["_uiConfig"].(map[string]interface{}); ok {
		if value, exists := uiConfig["latencyTestInterval"]; exists {
			interval := strings.ToLower(strings.TrimSpace(stringValue(value)))
			if !regexp.MustCompile(`^[1-9][0-9]*s$`).MatchString(interval) {
				return common.NewError("Clash 延迟测试间隔必须是正整数秒，例如 180s")
			}
		}
		if value, exists := uiConfig["latencyTolerance"]; exists {
			if _, err := positiveInteger(value, "Clash 延迟容差", 1_000_000); err != nil {
				return err
			}
		}
		if value, exists := uiConfig["latencyTestUrl"]; exists {
			if err := validateHTTPURLValue(value, "Clash 延迟测试地址"); err != nil {
				return err
			}
		}
		if value, exists := uiConfig["rejectUdpPortsInput"]; exists {
			if err := validateUDPPortRanges(stringValue(value)); err != nil {
				return err
			}
		}
	}
	if err := validateUIRuleSetSources(root["_uiConfig"], "clashRuleRows", "clash", "ruleSetSource"); err != nil {
		return err
	}
	return validateUIRuleNameConflicts(root["_uiConfig"], "clashRuleRows")
}

func nonEmptyStringSequence(raw interface{}) bool {
	items, ok := raw.([]interface{})
	if !ok || len(items) == 0 {
		return false
	}
	for _, item := range items {
		if strings.TrimSpace(stringValue(item)) != "" {
			return true
		}
	}
	return false
}

func validateNamedClashSequence(raw interface{}, label string) error {
	if raw == nil {
		return nil
	}
	items, ok := raw.([]interface{})
	if !ok {
		return common.NewErrorf("Clash %s 必须是数组", label)
	}
	seen := map[string]struct{}{}
	for index, rawItem := range items {
		item, ok := rawItem.(map[string]interface{})
		if !ok {
			return common.NewErrorf("Clash %s[%d] 必须是对象", label, index)
		}
		name := strings.TrimSpace(stringValue(item["name"]))
		if name == "" {
			return common.NewErrorf("Clash %s[%d].name 不能为空", label, index)
		}
		key := strings.ToLower(name)
		if _, exists := seen[key]; exists {
			return common.NewErrorf("Clash %s 名称重复: %s", label, name)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func validateClashRuleProviders(raw interface{}) error {
	providers, ok := raw.(map[string]interface{})
	if !ok {
		return common.NewError("Clash rule-providers 必须是对象")
	}
	for name, rawProvider := range providers {
		provider, ok := rawProvider.(map[string]interface{})
		if !ok {
			return common.NewErrorf("Clash rule-providers.%s 必须是对象", name)
		}
		behavior := strings.ToLower(strings.TrimSpace(stringValue(provider["behavior"])))
		if behavior != "" && behavior != "domain" && behavior != "ipcidr" && behavior != "classical" {
			return common.NewErrorf("Clash rule-providers.%s.behavior 不受支持", name)
		}
		format := strings.ToLower(strings.TrimSpace(stringValue(provider["format"])))
		if format != "" && format != "yaml" && format != "text" && format != "mrs" {
			return common.NewErrorf("Clash rule-providers.%s.format 不受支持", name)
		}
		if remoteURL := strings.TrimSpace(stringValue(provider["url"])); remoteURL != "" {
			parsed, err := url.Parse(remoteURL)
			if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
				return common.NewErrorf("Clash rule-providers.%s.url 必须是有效的 HTTP/HTTPS 地址", name)
			}
			expectedFormat, err := ruleSetProbeFormatForURL("clash", remoteURL)
			if err != nil {
				return common.NewErrorf("Clash rule-providers.%s.url: %v", name, err)
			}
			if format != expectedFormat {
				return common.NewErrorf("Clash rule-providers.%s.format 必须是 %s", name, expectedFormat)
			}
			if expectedFormat == "mrs" && behavior != "domain" && behavior != "ipcidr" {
				return common.NewErrorf("Clash rule-providers.%s 的 MRS behavior 必须是 domain 或 ipcidr", name)
			}
		}
	}
	return nil
}

func validateUIRuleNameConflicts(rawUI interface{}, rowsKey string) error {
	ui, ok := rawUI.(map[string]interface{})
	if !ok {
		return nil
	}
	rawRows, ok := ui[rowsKey].([]interface{})
	if !ok {
		return nil
	}
	ruleSetNames := map[string]struct{}{}
	customNameTypes := map[string]string{}
	for _, rawRow := range rawRows {
		row, ok := rawRow.(map[string]interface{})
		if !ok {
			continue
		}
		kind := strings.ToLower(strings.TrimSpace(stringValue(row["kind"])))
		name := strings.ToLower(strings.TrimSpace(stringValue(row["name"])))
		if kind == "ruleset" || kind == "rule-set" {
			if name != "" {
				ruleSetNames[name] = struct{}{}
			}
			if values, ok := row["values"].([]interface{}); ok {
				for _, value := range values {
					candidate := strings.ToLower(strings.TrimSpace(stringValue(value)))
					if candidate != "" {
						ruleSetNames[candidate] = struct{}{}
					}
				}
			}
			continue
		}
		if name != "" {
			customType := strings.ToLower(strings.TrimSpace(stringValue(row["customType"])))
			if customType == "" {
				customType = strings.ToLower(strings.TrimSpace(stringValue(row["type"])))
			}
			if existingType, exists := customNameTypes[name]; exists && existingType != customType {
				return common.NewErrorf("同名自定义规则的匹配类型冲突: %s", name)
			}
			customNameTypes[name] = customType
		}
	}
	for name := range customNameTypes {
		if _, exists := ruleSetNames[name]; exists {
			return common.NewErrorf("自定义规则名称与规则集冲突: %s", name)
		}
	}
	return nil
}

func validateUIRuleSetSources(rawUI interface{}, rowsKey string, kind string, globalKey string) error {
	ui, ok := rawUI.(map[string]interface{})
	if !ok {
		return nil
	}
	globalSource := strings.TrimSpace(stringValue(ui[globalKey]))
	rows, ok := ui[rowsKey].([]interface{})
	if !ok {
		return nil
	}
	for index, rawRow := range rows {
		row, ok := rawRow.(map[string]interface{})
		if !ok {
			continue
		}
		rowKind := strings.ToLower(strings.TrimSpace(stringValue(row["kind"])))
		if rowKind != "ruleset" && rowKind != "rule-set" {
			continue
		}
		scope := strings.ToLower(strings.TrimSpace(stringValue(row["ruleSetScope"])))
		if scope == "" {
			scope = strings.ToLower(strings.TrimSpace(stringValue(row["scope"])))
		}
		if scope != "domain" && scope != "ip" {
			return common.NewErrorf("%s 规则集行 %d 的 scope 无效", kind, index+1)
		}
		source := strings.TrimSpace(stringValue(row["ruleSetSourceOverride"]))
		if source == "" {
			source = globalSource
		}
		if allRuleSetValuesAreHTTPURLs(row["values"]) {
			if err := validateUIRuleSetURLs(kind, row["values"], index+1); err != nil {
				return err
			}
			continue
		}
		entry, exists := SubscriptionRuleSetSource(kind, source)
		if !exists {
			return common.NewErrorf("%s 规则集行 %d 的来源不存在: %s", kind, index+1, source)
		}
		if !entry.SupportsScope(scope) {
			return common.NewErrorf("%s 规则集来源 %s 不支持 %s scope", kind, source, scope)
		}
	}
	return nil
}

func validateUIRuleSetURLs(kind string, raw interface{}, row int) error {
	values, _ := raw.([]interface{})
	for _, value := range values {
		rawURL := strings.TrimSpace(stringValue(value))
		if _, err := ruleSetProbeFormatForURL(kind, rawURL); err != nil {
			return common.NewErrorf("%s 规则集行 %d 的 URL 格式无效: %v", kind, row, err)
		}
	}
	return nil
}

func allRuleSetValuesAreHTTPURLs(raw interface{}) bool {
	values, ok := raw.([]interface{})
	if !ok || len(values) == 0 {
		return false
	}
	for _, value := range values {
		parsed, err := url.Parse(strings.TrimSpace(stringValue(value)))
		if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return false
		}
	}
	return true
}

func validateUDPPortRanges(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	for _, token := range strings.FieldsFunc(trimmed, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r' || r == ' '
	}) {
		parts := strings.Split(strings.TrimSpace(token), "-")
		if len(parts) < 1 || len(parts) > 2 {
			return common.NewErrorf("UDP 端口范围无效: %s", token)
		}
		start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil || start < 1 || start > 65535 {
			return common.NewErrorf("UDP 端口范围无效: %s", token)
		}
		end := start
		if len(parts) == 2 {
			end, err = strconv.Atoi(strings.TrimSpace(parts[1]))
			if err != nil || end < start || end > 65535 {
				return common.NewErrorf("UDP 端口范围无效: %s", token)
			}
		}
	}
	return nil
}

func validateHTTPURLValue(raw interface{}, label string) error {
	value := strings.TrimSpace(stringValue(raw))
	parsed, err := url.Parse(value)
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return common.NewErrorf("%s 必须是有效的 HTTP/HTTPS 地址", label)
	}
	return nil
}

func validatePortValue(raw interface{}, label string) error {
	_, err := positiveInteger(raw, label, 65535)
	return err
}

func positiveInteger(raw interface{}, label string, max int64) (int64, error) {
	var value int64
	switch typed := raw.(type) {
	case json.Number:
		parsed, err := strconv.ParseInt(string(typed), 10, 64)
		if err != nil {
			return 0, common.NewErrorf("%s 必须是整数", label)
		}
		value = parsed
	case int:
		value = int64(typed)
	case int64:
		value = typed
	case uint64:
		if typed > uint64(max) {
			return 0, common.NewErrorf("%s 超出有效范围", label)
		}
		value = int64(typed)
	case float64:
		value = int64(typed)
		if float64(value) != typed {
			return 0, common.NewErrorf("%s 必须是整数", label)
		}
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err != nil {
			return 0, common.NewErrorf("%s 必须是整数", label)
		}
		value = parsed
	default:
		return 0, common.NewErrorf("%s 必须是整数", label)
	}
	if value <= 0 || value > max {
		return 0, common.NewErrorf("%s 必须在 1-%d 之间", label, max)
	}
	return value, nil
}

func stringValue(raw interface{}) string {
	if raw == nil {
		return ""
	}
	if value, ok := raw.(string); ok {
		return value
	}
	return fmt.Sprint(raw)
}

func compactYAMLForComparison(raw string) ([]byte, error) {
	root, err := ParseSubClashExtension(raw)
	if err != nil {
		return nil, err
	}
	return yaml.Marshal(root)
}

func equalNormalizedYAML(left string, right string) bool {
	leftRaw, leftErr := compactYAMLForComparison(left)
	rightRaw, rightErr := compactYAMLForComparison(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftRaw, rightRaw)
}
