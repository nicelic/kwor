package service

import "strings"

type RuleSetSourceEntry struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	DomainTemplate string `json:"domainTemplate,omitempty"`
	IPTemplate     string `json:"ipTemplate,omitempty"`
	Format         string `json:"format"`
}

func (entry RuleSetSourceEntry) SupportsScope(scope string) bool {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "domain", "geosite":
		return strings.TrimSpace(entry.DomainTemplate) != ""
	case "ip", "geoip", "ipcidr":
		return strings.TrimSpace(entry.IPTemplate) != ""
	default:
		return false
	}
}

func (entry RuleSetSourceEntry) TemplateForScope(scope string) string {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "domain", "geosite":
		return entry.DomainTemplate
	case "ip", "geoip", "ipcidr":
		return entry.IPTemplate
	default:
		return ""
	}
}

var subscriptionRuleSetSourceRegistry = map[string][]RuleSetSourceEntry{
	"json": {
		{ID: "karingx_github", Title: "KaringX GitHub", DomainTemplate: "https://github.com/KaringX/karing-ruleset/raw/refs/heads/sing/geo/geosite/{name}.srs", IPTemplate: "https://github.com/KaringX/karing-ruleset/raw/refs/heads/sing/geo/geoip/{name}.srs", Format: "srs"},
		{ID: "karingx_cdn", Title: "KaringX CDN", DomainTemplate: "https://fastly.jsdelivr.net/gh/KaringX/karing-ruleset@sing/geo/geosite/{name}.srs", IPTemplate: "https://fastly.jsdelivr.net/gh/KaringX/karing-ruleset@sing/geo/geoip/{name}.srs", Format: "srs"},
		{ID: "loyalsoldier_ip_github", Title: "Loyalsoldier IP GitHub", IPTemplate: "https://raw.githubusercontent.com/Loyalsoldier/geoip/release/srs/{name}.srs", Format: "srs"},
		{ID: "loyalsoldier_ip_cdn", Title: "Loyalsoldier IP CDN", IPTemplate: "https://fastly.jsdelivr.net/gh/Loyalsoldier/geoip@release/srs/{name}.srs", Format: "srs"},
		{ID: "quixoticheart_github", Title: "QuixoticHeart GitHub", DomainTemplate: "https://github.com/QuixoticHeart/rule-set/raw/refs/heads/ruleset/singbox/version4/{name}.srs", IPTemplate: "https://github.com/QuixoticHeart/rule-set/raw/refs/heads/ruleset/singbox/version4/{name}.srs", Format: "srs"},
		{ID: "sagernet_github", Title: "SagerNet GitHub", DomainTemplate: "https://github.com/SagerNet/sing-geosite/raw/rule-set/geosite-{name}.srs", IPTemplate: "https://github.com/SagerNet/sing-geoip/raw/rule-set/geoip-{name}.srs", Format: "srs"},
		{ID: "sagernet_cdn", Title: "SagerNet CDN", DomainTemplate: "https://fastly.jsdelivr.net/gh/SagerNet/sing-geosite@rule-set/geosite-{name}.srs", IPTemplate: "https://fastly.jsdelivr.net/gh/SagerNet/sing-geoip@rule-set/geoip-{name}.srs", Format: "srs"},
		{ID: "metacubex_github", Title: "MetaCubeX GitHub", DomainTemplate: "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/{name}.srs", IPTemplate: "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geoip/{name}.srs", Format: "srs"},
		{ID: "metacubex_cdn", Title: "MetaCubeX CDN", DomainTemplate: "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@sing/geo/geosite/{name}.srs", IPTemplate: "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@sing/geo/geoip/{name}.srs", Format: "srs"},
		{ID: "chocolate4u_github", Title: "Chocolate4U GitHub", DomainTemplate: "https://raw.githubusercontent.com/Chocolate4U/Iran-sing-box-rules/rule-set/geosite-{name}.srs", IPTemplate: "https://raw.githubusercontent.com/Chocolate4U/Iran-sing-box-rules/rule-set/geoip-{name}.srs", Format: "srs"},
		{ID: "chocolate4u_cdn", Title: "Chocolate4U CDN", DomainTemplate: "https://cdn.jsdelivr.net/gh/Chocolate4U/Iran-sing-box-rules@rule-set/geosite-{name}.srs", IPTemplate: "https://cdn.jsdelivr.net/gh/Chocolate4U/Iran-sing-box-rules@rule-set/geoip-{name}.srs", Format: "srs"},
		{ID: "lyc8503_github", Title: "lyc8503 GitHub", DomainTemplate: "https://github.com/lyc8503/sing-box-rules/raw/refs/heads/rule-set-geosite/geosite-{name}.srs", IPTemplate: "https://github.com/lyc8503/sing-box-rules/raw/refs/heads/rule-set-geoip/geoip-{name}.srs", Format: "srs"},
		{ID: "lyc8503_cdn", Title: "lyc8503 CDN", DomainTemplate: "https://cdn.jsdelivr.net/gh/lyc8503/sing-box-rules@rule-set-geosite/geosite-{name}.srs", IPTemplate: "https://cdn.jsdelivr.net/gh/lyc8503/sing-box-rules@rule-set-geoip/geoip-{name}.srs", Format: "srs"},
		{ID: "lyc8503_cdn1", Title: "lyc8503 CDN 1", DomainTemplate: "https://fastly.jsdelivr.net/gh/lyc8503/sing-box-rules@rule-set-geosite/geosite-{name}.srs", IPTemplate: "https://fastly.jsdelivr.net/gh/lyc8503/sing-box-rules@rule-set-geoip/geoip-{name}.srs", Format: "srs"},
	},
	"clash": {
		{ID: "metacubex_github", Title: "MetaCubeX GitHub", DomainTemplate: "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/meta/geo/geosite/{name}.mrs", IPTemplate: "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/meta/geo/geoip/{name}.mrs", Format: "mrs"},
		{ID: "metacubex_cdn", Title: "MetaCubeX CDN", DomainTemplate: "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@meta/geo/geosite/{name}.mrs", IPTemplate: "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@meta/geo/geoip/{name}.mrs", Format: "mrs"},
		{ID: "quixoticheart_github", Title: "QuixoticHeart GitHub", DomainTemplate: "https://github.com/QuixoticHeart/rule-set/raw/refs/heads/ruleset/meta/domain/{name}.mrs", IPTemplate: "https://github.com/QuixoticHeart/rule-set/raw/refs/heads/ruleset/meta/ipcidr/{name}.mrs", Format: "mrs"},
		{ID: "loyalsoldier_github", Title: "Loyalsoldier GitHub", DomainTemplate: "https://raw.githubusercontent.com/Loyalsoldier/clash-rules/release/{name}.txt", IPTemplate: "https://raw.githubusercontent.com/Loyalsoldier/clash-rules/release/{name}.txt", Format: "text"},
		{ID: "loyalsoldier_ip_github", Title: "Loyalsoldier IP GitHub", IPTemplate: "https://raw.githubusercontent.com/Loyalsoldier/geoip/release/clash/ipcidr/{name}.txt", Format: "text"},
		{ID: "loyalsoldier_ip_cdn", Title: "Loyalsoldier IP CDN", IPTemplate: "https://fastly.jsdelivr.net/gh/Loyalsoldier/geoip@release/clash/ipcidr/{name}.txt", Format: "text"},
	},
}

func SubscriptionRuleSetSources() map[string][]RuleSetSourceEntry {
	result := make(map[string][]RuleSetSourceEntry, len(subscriptionRuleSetSourceRegistry))
	for kind, entries := range subscriptionRuleSetSourceRegistry {
		cloned := make([]RuleSetSourceEntry, len(entries))
		copy(cloned, entries)
		result[kind] = cloned
	}
	return result
}

func SubscriptionRuleSetSource(kind string, sourceID string) (RuleSetSourceEntry, bool) {
	for _, entry := range subscriptionRuleSetSourceRegistry[strings.ToLower(strings.TrimSpace(kind))] {
		if entry.ID == strings.TrimSpace(sourceID) {
			return entry, true
		}
	}
	return RuleSetSourceEntry{}, false
}
