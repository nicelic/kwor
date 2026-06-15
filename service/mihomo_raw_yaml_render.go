package service

import (
	"bytes"

	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
)

type mihomoProxyRenderEntry struct {
	Name    string
	Proxy   map[string]interface{}
	RawYAML []byte
}

func loadMihomoRawClashYAMLByTag(db *gorm.DB) (map[string][]byte, error) {
	if db == nil {
		return map[string][]byte{}, nil
	}

	var outbounds []model.MihomoOutbound
	if err := db.Model(model.MihomoOutbound{}).Select("tag", "raw_clash_yaml").Find(&outbounds).Error; err != nil {
		return nil, err
	}

	rawByTag := make(map[string][]byte, len(outbounds))
	for _, outbound := range outbounds {
		if len(outbound.RawClashYAML) == 0 {
			continue
		}
		rawByTag[outbound.Tag] = cloneRawYAMLBytes(outbound.RawClashYAML)
	}
	return rawByTag, nil
}

func renderMihomoDocumentYAML(document map[string]interface{}, rawByTag map[string][]byte) ([]byte, error) {
	if len(rawByTag) == 0 {
		util.ApplySudokuCustomTablesFlowYAML(document)
		raw, err := yaml.Marshal(document)
		if err != nil {
			return nil, err
		}
		return util.CompactSudokuCustomTablesFlowYAML(raw), nil
	}

	rawProxies, ok := document["proxies"].([]interface{})
	if !ok || len(rawProxies) == 0 {
		util.ApplySudokuCustomTablesFlowYAML(document)
		raw, err := yaml.Marshal(document)
		if err != nil {
			return nil, err
		}
		return util.CompactSudokuCustomTablesFlowYAML(raw), nil
	}

	entries := make([]mihomoProxyRenderEntry, 0, len(rawProxies))
	for _, item := range rawProxies {
		proxy, ok := item.(map[string]interface{})
		if !ok || proxy == nil {
			continue
		}
		name, _ := proxy["name"].(string)
		entries = append(entries, mihomoProxyRenderEntry{
			Name:    name,
			Proxy:   proxy,
			RawYAML: cloneRawYAMLBytes(rawByTag[name]),
		})
	}
	if len(entries) == 0 {
		util.ApplySudokuCustomTablesFlowYAML(document)
		raw, err := yaml.Marshal(document)
		if err != nil {
			return nil, err
		}
		return util.CompactSudokuCustomTablesFlowYAML(raw), nil
	}

	proxySection, err := renderMihomoProxySection(entries)
	if err != nil {
		return nil, err
	}

	remaining := make(map[string]interface{}, len(document))
	for key, value := range document {
		if key == "proxies" {
			continue
		}
		remaining[key] = value
	}
	util.ApplySudokuCustomTablesFlowYAML(remaining)

	remainderYAML, err := yaml.Marshal(remaining)
	if err != nil {
		return nil, err
	}
	remainderYAML = util.CompactSudokuCustomTablesFlowYAML(remainderYAML)
	if len(remainderYAML) == 0 {
		return proxySection, nil
	}

	combined := append([]byte(nil), proxySection...)
	combined = append(combined, remainderYAML...)
	return combined, nil
}

func renderMihomoProxySection(entries []mihomoProxyRenderEntry) ([]byte, error) {
	buffer := &bytes.Buffer{}
	buffer.WriteString("proxies:\n")
	for _, entry := range entries {
		itemYAML := cloneRawYAMLBytes(entry.RawYAML)
		if len(itemYAML) == 0 {
			var err error
			itemYAML, err = marshalSingleMihomoProxyItemYAML(entry.Proxy)
			if err != nil {
				return nil, err
			}
		}
		buffer.Write(ensureTrailingLineFeed(itemYAML))
	}
	return buffer.Bytes(), nil
}

func marshalSingleMihomoProxyItemYAML(proxy map[string]interface{}) ([]byte, error) {
	section := map[string]interface{}{
		"proxies": []interface{}{proxy},
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
