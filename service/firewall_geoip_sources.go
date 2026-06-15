package service

import (
	"fmt"
	"strings"
)

type firewallGeoSourceDefinition struct {
	Key         string
	Label       string
	Format      string
	URLTemplate string
	NameMap     map[string]string
}

var firewallGeoMetaCubeXNameMap = map[string]string{
	"ads": "category-ads-all",
	"ir":  "category-ir",
}

var firewallGeoSourceDefinitions = []firewallGeoSourceDefinition{
	{
		Key:         "json.sagernet_github",
		Label:       "JSON / SagerNet Github",
		Format:      firewallGeoFormatSRS,
		URLTemplate: "https://github.com/SagerNet/sing-geoip/raw/rule-set/geoip-{name}.srs",
	},
	{
		Key:         "json.sagernet_cdn",
		Label:       "JSON / SagerNet CDN",
		Format:      firewallGeoFormatSRS,
		URLTemplate: "https://fastly.jsdelivr.net/gh/SagerNet/sing-geoip@rule-set/geoip-{name}.srs",
	},
	{
		Key:         "json.karingx_github",
		Label:       "JSON / KaringX Github",
		Format:      firewallGeoFormatSRS,
		URLTemplate: "https://github.com/KaringX/karing-ruleset/raw/refs/heads/sing/geo/geoip/{name}.srs",
		NameMap:     firewallGeoMetaCubeXNameMap,
	},
	{
		Key:         "json.karingx_cdn",
		Label:       "JSON / KaringX CDN",
		Format:      firewallGeoFormatSRS,
		URLTemplate: "https://fastly.jsdelivr.net/gh/KaringX/karing-ruleset@sing/geo/geoip/{name}.srs",
		NameMap:     firewallGeoMetaCubeXNameMap,
	},
	{
		Key:         "json.loyalsoldier_ip_github",
		Label:       "JSON / Loyalsoldier_IP Github",
		Format:      firewallGeoFormatSRS,
		URLTemplate: "https://raw.githubusercontent.com/Loyalsoldier/geoip/release/srs/{name}.srs",
	},
	{
		Key:         "json.loyalsoldier_ip_cdn",
		Label:       "JSON / Loyalsoldier_IP CDN",
		Format:      firewallGeoFormatSRS,
		URLTemplate: "https://fastly.jsdelivr.net/gh/Loyalsoldier/geoip@release/srs/{name}.srs",
	},
	{
		Key:         "json.quixoticheart_github",
		Label:       "JSON / QuixoticHeart Github",
		Format:      firewallGeoFormatSRS,
		URLTemplate: "https://github.com/QuixoticHeart/rule-set/raw/refs/heads/ruleset/singbox/version4/{name}.srs",
	},
	{
		Key:         "json.metacubex_github",
		Label:       "JSON / MetaCubeX Github",
		Format:      firewallGeoFormatSRS,
		URLTemplate: "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geoip/{name}.srs",
		NameMap:     firewallGeoMetaCubeXNameMap,
	},
	{
		Key:         "json.metacubex_cdn",
		Label:       "JSON / MetaCubeX CDN",
		Format:      firewallGeoFormatSRS,
		URLTemplate: "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@sing/geo/geoip/{name}.srs",
		NameMap:     firewallGeoMetaCubeXNameMap,
	},
	{
		Key:         "json.chocolate4u_github",
		Label:       "JSON / Chocolate4U Github",
		Format:      firewallGeoFormatSRS,
		URLTemplate: "https://raw.githubusercontent.com/Chocolate4U/Iran-sing-box-rules/rule-set/geoip-{name}.srs",
	},
	{
		Key:         "json.chocolate4u_cdn",
		Label:       "JSON / Chocolate4U CDN",
		Format:      firewallGeoFormatSRS,
		URLTemplate: "https://cdn.jsdelivr.net/gh/Chocolate4U/Iran-sing-box-rules@rule-set/geoip-{name}.srs",
	},
	{
		Key:         "json.lyc8503_github",
		Label:       "JSON / lyc8503 Github",
		Format:      firewallGeoFormatSRS,
		URLTemplate: "https://github.com/lyc8503/sing-box-rules/raw/refs/heads/rule-set-geoip/geoip-{name}.srs",
	},
	{
		Key:         "json.lyc8503_cdn",
		Label:       "JSON / lyc8503 CDN",
		Format:      firewallGeoFormatSRS,
		URLTemplate: "https://cdn.jsdelivr.net/gh/lyc8503/sing-box-rules@rule-set-geoip/geoip-{name}.srs",
	},
	{
		Key:         "json.lyc8503_cdn1",
		Label:       "JSON / lyc8503 CDN 1",
		Format:      firewallGeoFormatSRS,
		URLTemplate: "https://fastly.jsdelivr.net/gh/lyc8503/sing-box-rules@rule-set-geoip/geoip-{name}.srs",
	},
	{
		Key:         "clash.metacubex_github",
		Label:       "Clash / MetaCubeX Github",
		Format:      firewallGeoFormatMRS,
		URLTemplate: "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/meta/geo/geoip/{name}.mrs",
		NameMap:     firewallGeoMetaCubeXNameMap,
	},
	{
		Key:         "clash.metacubex_cdn",
		Label:       "Clash / MetaCubeX CDN",
		Format:      firewallGeoFormatMRS,
		URLTemplate: "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@meta/geo/geoip/{name}.mrs",
		NameMap:     firewallGeoMetaCubeXNameMap,
	},
	{
		Key:         "clash.quixoticheart_github",
		Label:       "Clash / QuixoticHeart Github",
		Format:      firewallGeoFormatMRS,
		URLTemplate: "https://github.com/QuixoticHeart/rule-set/raw/refs/heads/ruleset/meta/ipcidr/{name}.mrs",
	},
	{
		Key:         "clash.loyalsoldier_ip_github",
		Label:       "Clash / Loyalsoldier_IP Github",
		Format:      firewallGeoFormatTXT,
		URLTemplate: "https://raw.githubusercontent.com/Loyalsoldier/geoip/release/clash/ipcidr/{name}.txt",
	},
	{
		Key:         "clash.loyalsoldier_ip_cdn",
		Label:       "Clash / Loyalsoldier_IP CDN",
		Format:      firewallGeoFormatTXT,
		URLTemplate: "https://fastly.jsdelivr.net/gh/Loyalsoldier/geoip@release/clash/ipcidr/{name}.txt",
	},
}

var firewallGeoSourceDefinitionMap = func() map[string]firewallGeoSourceDefinition {
	result := make(map[string]firewallGeoSourceDefinition, len(firewallGeoSourceDefinitions))
	for _, source := range firewallGeoSourceDefinitions {
		result[source.Key] = source
	}
	return result
}()

func firewallGeoDefaultSourceKeys() []string {
	result := make([]string, 0, len(firewallGeoSourceDefinitions))
	for _, source := range firewallGeoSourceDefinitions {
		result = append(result, source.Key)
	}
	return result
}

func normalizeFirewallGeoProviderKeys(raw []string) []string {
	if len(raw) == 0 {
		return nil
	}
	result := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, item := range raw {
		key := strings.TrimSpace(strings.ToLower(item))
		if key == "" {
			continue
		}
		if _, exists := firewallGeoSourceDefinitionMap[key]; !exists {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, key)
	}
	return result
}

func buildFirewallGeoProviderURL(providerKey string, countryCode string) (string, string, error) {
	definition, exists := firewallGeoSourceDefinitionMap[strings.TrimSpace(strings.ToLower(providerKey))]
	if !exists {
		return "", "", fmt.Errorf("unknown geo source provider: %s", providerKey)
	}
	name := normalizeFirewallGeoCountryCode(countryCode)
	if name == "" {
		return "", "", fmt.Errorf("country code is required")
	}
	if len(definition.NameMap) > 0 {
		if mapped, exists := definition.NameMap[name]; exists {
			name = mapped
		}
	}
	return strings.ReplaceAll(definition.URLTemplate, "{name}", name), definition.Format, nil
}

func normalizeFirewallGeoCountryCode(raw string) string {
	value := strings.TrimSpace(strings.ToLower(raw))
	value = strings.ReplaceAll(value, " ", "")
	for _, prefix := range []string{"geoip-", "geoip_", "geoip:"} {
		value = strings.TrimPrefix(value, prefix)
	}
	value = strings.Trim(value, "/")
	return value
}
