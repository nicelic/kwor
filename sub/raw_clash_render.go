package sub

import (
	"bytes"
	"strings"

	"github.com/alireza0/s-ui/util"
	"gopkg.in/yaml.v3"
)

type clashProxyRenderEntry struct {
	Name    string
	Proxy   map[string]interface{}
	RawYAML []byte
}

func renderClashSubscriptionFromEntries(
	entries []clashProxyRenderEntry,
	latencyUrl string,
	latencyInterval int,
	latencyTolerance int,
	selectorGroups []clashSelectorGroupConfig,
) ([]byte, error) {
	uniqueEntries := dedupeClashRenderEntries(entries)
	buffer := &bytes.Buffer{}
	buffer.WriteString("proxies:\n")

	nodeTags := make([]string, 0, len(uniqueEntries))
	for _, entry := range uniqueEntries {
		if entry.Proxy == nil {
			continue
		}
		sanitizedProxy, changed := sanitizeMihomoClashProxy(entry.Proxy)
		if sanitizedProxy == nil {
			continue
		}
		entry.Proxy = sanitizedProxy
		proxyType, _ := entry.Proxy["type"].(string)
		if !util.SupportsMihomoSubscriptionClashProxyType(proxyType) {
			continue
		}

		itemYAML := cloneRawBytes(entry.RawYAML)
		if len(itemYAML) == 0 || changed || shouldRegenerateMihomoClashRawYAML(proxyType) {
			var err error
			itemYAML, err = marshalSingleClashProxyItemYAML(entry.Proxy)
			if err != nil {
				return nil, err
			}
		}
		itemYAML = ensureTrailingLF(itemYAML)
		buffer.Write(itemYAML)
		nodeTags = append(nodeTags, entry.Name)
	}

	proxyGroups := buildFixedMihomoProxyGroups(nodeTags, latencyUrl, latencyInterval, latencyTolerance)
	proxyGroups = append(proxyGroups, buildNamedClashProxyGroups(selectorGroups, nodeTags)...)
	groupYAML, err := renderClashProxyGroupsYAML(proxyGroups)
	if err != nil {
		return nil, err
	}
	buffer.Write(groupYAML)

	return buffer.Bytes(), nil
}

func dedupeClashRenderEntries(entries []clashProxyRenderEntry) []clashProxyRenderEntry {
	result := make([]clashProxyRenderEntry, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if entry.Proxy == nil {
			continue
		}
		if entry.Name == "" {
			name, _ := entry.Proxy["name"].(string)
			entry.Name = name
		}
		if entry.Name == "" {
			continue
		}
		if _, exists := seen[entry.Name]; exists {
			continue
		}
		seen[entry.Name] = struct{}{}
		result = append(result, entry)
	}
	return result
}

func shouldRegenerateMihomoClashRawYAML(proxyType string) bool {
	switch strings.ToLower(strings.TrimSpace(proxyType)) {
	case "hysteria2", "tuic":
		return true
	default:
		return false
	}
}

func marshalSingleClashProxyItemYAML(proxy map[string]interface{}) ([]byte, error) {
	copied := make(map[string]interface{}, len(proxy))
	for key, value := range proxy {
		copied[key] = value
	}
	section := map[string]interface{}{
		"proxies": []interface{}{normalizeProxyForYAML(copied)},
	}
	if normalized, ok := normalizeNumericTypesForYAML(section).(map[string]interface{}); ok && normalized != nil {
		section = normalized
	}
	util.ApplySudokuCustomTablesFlowYAML(section)

	raw, err := yaml.Marshal(section)
	if err != nil {
		return nil, err
	}
	raw = util.CompactSudokuCustomTablesFlowYAML(raw)
	prefix := []byte("proxies:\n")
	if bytes.HasPrefix(raw, prefix) {
		return append([]byte(nil), raw[len(prefix):]...), nil
	}
	return raw, nil
}

func renderClashProxyGroupsYAML(proxyGroups []map[string]interface{}) ([]byte, error) {
	section := map[string]interface{}{
		"proxy-groups": proxyGroups,
	}
	if normalized, ok := normalizeNumericTypesForYAML(section).(map[string]interface{}); ok && normalized != nil {
		section = normalized
	}
	return yaml.Marshal(section)
}

func cloneRawBytes(raw []byte) []byte {
	if len(raw) == 0 {
		return nil
	}
	return append([]byte(nil), raw...)
}

func ensureTrailingLF(raw []byte) []byte {
	if len(raw) == 0 {
		return nil
	}
	if raw[len(raw)-1] == '\n' {
		return cloneRawBytes(raw)
	}
	withLF := cloneRawBytes(raw)
	return append(withLF, '\n')
}
