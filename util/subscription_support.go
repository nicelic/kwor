package util

import "strings"

var singboxSubscriptionOutboundTypes = map[string]struct{}{
	"anytls":      {},
	"block":       {},
	"dns":         {},
	"direct":      {},
	"http":        {},
	"hysteria":    {},
	"hysteria2":   {},
	"naive":       {},
	"selector":    {},
	"shadowtls":   {},
	"shadowsocks": {},
	"socks":       {},
	"ssh":         {},
	"tor":         {},
	"trojan":      {},
	"tuic":        {},
	"urltest":     {},
	"vless":       {},
	"vmess":       {},
	"wireguard":   {},
}

var mihomoSubscriptionOutboundTypes = map[string]struct{}{
	"anytls":      {},
	"http":        {},
	"hysteria":    {},
	"hysteria2":   {},
	"mieru":       {},
	"snell":       {},
	"sudoku":      {},
	"shadowsocks": {},
	"ssh":         {},
	"socks":       {},
	"trusttunnel": {},
	"trojan":      {},
	"tuic":        {},
	"vless":       {},
	"vmess":       {},
}

var mihomoSubscriptionClashProxyTypes = map[string]struct{}{
	"anytls":      {},
	"http":        {},
	"hysteria":    {},
	"hysteria2":   {},
	"mieru":       {},
	"snell":       {},
	"sudoku":      {},
	"ssh":         {},
	"ss":          {},
	"socks5":      {},
	"trusttunnel": {},
	"trojan":      {},
	"tuic":        {},
	"vless":       {},
	"vmess":       {},
}

func SupportsSingboxSubscriptionOutboundType(outboundType string) bool {
	_, ok := singboxSubscriptionOutboundTypes[normalizeSubscriptionType(outboundType)]
	return ok
}

func SupportsMihomoSubscriptionOutboundType(outboundType string) bool {
	_, ok := mihomoSubscriptionOutboundTypes[normalizeSubscriptionType(outboundType)]
	return ok
}

func SupportsMihomoSubscriptionClashProxyType(proxyType string) bool {
	_, ok := mihomoSubscriptionClashProxyTypes[normalizeSubscriptionType(proxyType)]
	return ok
}

func FilterTaggedSubscriptionOutbounds(
	outbounds []map[string]interface{},
	outTags []string,
	support func(string) bool,
) ([]map[string]interface{}, []string) {
	if support == nil {
		return outbounds, outTags
	}

	filteredOutbounds := make([]map[string]interface{}, 0, len(outbounds))
	availableTags := make(map[string]struct{}, len(outbounds))
	for _, outbound := range outbounds {
		if outbound == nil {
			continue
		}
		outType, _ := outbound["type"].(string)
		if !support(outType) {
			continue
		}
		filteredOutbounds = append(filteredOutbounds, outbound)
		tag, _ := outbound["tag"].(string)
		tag = strings.TrimSpace(tag)
		if tag != "" {
			availableTags[tag] = struct{}{}
		}
	}

	filteredTags := make([]string, 0, len(outTags))
	seen := make(map[string]struct{}, len(outTags))
	for _, tag := range outTags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, ok := availableTags[tag]; !ok {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		filteredTags = append(filteredTags, tag)
	}

	return filteredOutbounds, filteredTags
}

func FilterMihomoSubscriptionClashProxies(proxies []map[string]interface{}) []map[string]interface{} {
	filtered := make([]map[string]interface{}, 0, len(proxies))
	for _, proxy := range proxies {
		if proxy == nil {
			continue
		}
		proxyType, _ := proxy["type"].(string)
		if !SupportsMihomoSubscriptionClashProxyType(proxyType) {
			continue
		}
		filtered = append(filtered, proxy)
	}
	return filtered
}

func normalizeSubscriptionType(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
