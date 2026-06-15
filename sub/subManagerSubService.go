package sub

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/alireza0/s-ui/config"
	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/service"
	"github.com/alireza0/s-ui/util"
	"gopkg.in/yaml.v3"
)

// SubManagerSubService renders subscriptions for sub-manager nodes/groups.
// It reuses JsonService and ClashService conversion logic.
type SubManagerSubService struct {
	service.SettingService
	JsonService
	ClashService
}

const (
	subManagerSourceClient       = "client"
	subManagerSourceMihomoClient = "mihomo_client"
	subManagerSourceSubGroup     = "subgroup"
)

// GetSubManagerJson renders sing-box JSON subscription by one suboutbound tag.
func (s *SubManagerSubService) GetSubManagerJson(tag string) (*string, error) {
	subOutbound, err := s.getSubOutboundRecord(tag)
	if err != nil {
		return nil, err
	}
	outboundMap, err := s.buildRuntimeOutboundMap(subOutbound)
	if err != nil {
		return nil, err
	}

	outTag, _ := outboundMap["tag"].(string)
	if outTag == "" {
		outTag = tag
	}

	rawOutbounds := []map[string]interface{}{outboundMap}
	outbounds, outTags := buildSubManagerRuntimeOutbounds(rawOutbounds)
	if len(outbounds) == 0 {
		return nil, fmt.Errorf("no valid outbounds for tag %s", outTag)
	}
	outbounds, outTags = util.FilterTaggedSubscriptionOutbounds(
		outbounds,
		outTags,
		util.SupportsSingboxSubscriptionOutboundType,
	)
	for i := range outbounds {
		util.SanitizeSingboxSubscriptionOutbound(outbounds[i])
	}

	// Read latency test settings.
	latencyUrl := "http://www.gstatic.com/generate_204"
	latencyInterval := "10m"
	latencyTolerance := 50
	var extJson map[string]interface{}
	othersStr, _ := s.SettingService.GetSubJsonExt()
	if len(othersStr) > 0 {
		if err := json.Unmarshal([]byte(othersStr), &extJson); err == nil {
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

	// Remove mihomo-only fields from sing-box JSON output.
	stripMihomoFields(&outbounds)

	// Reuse JsonService defaults for selector/urltest/direct/block/final groups.
	s.JsonService.addDefaultOutbounds(&outbounds, &outTags, latencyUrl, latencyInterval, latencyTolerance, selectorGroups)

	var jsonConfig map[string]interface{}
	if err := json.Unmarshal([]byte(defaultJson), &jsonConfig); err != nil {
		return nil, err
	}
	jsonConfig["outbounds"] = outbounds

	// Move tls_store from outbound.tls blocks into root certificate.store.
	tlsStore := extractTlsStoreFromOutbounds(outbounds)
	tlsStore = s.SettingService.ResolveSubscriptionTLSStore(tlsStore)

	// Reuse JsonService extra fields merge.
	s.JsonService.addOthers(&jsonConfig)
	applyCertificateStore(&jsonConfig, tlsStore)

	result, err := json.MarshalIndent(jsonConfig, "", "  ")
	if err != nil {
		return nil, err
	}
	resultStr := string(result)

	// Save a copy into sub_json directory.
	if err := SaveSubJsonToFile(tag, resultStr); err != nil {
		logger.Errorf("[SubManagerSub] failed to save subscription JSON file: %v", err)
		// Do not return error here because HTTP response payload is already generated.
	}

	return &resultStr, nil
}

// GetSubManagerClash renders Clash subscription for one suboutbound tag.
// It reuses ClashService.ConvertToClashMeta as fallback.
func (s *SubManagerSubService) GetSubManagerClash(tag string) (*string, error) {
	subOutbound, err := s.getSubOutboundRecord(tag)
	if err != nil {
		return nil, err
	}
	runtimeOutbound, err := s.buildRuntimeOutboundMap(subOutbound)
	if err != nil {
		return nil, err
	}

	clashConfig, latencyUrl, latencyInterval, latencyTolerance, selectorGroups, err := s.ClashService.getClashConfigClean()
	if err != nil || len(clashConfig) == 0 {
		clashConfig = basicClashConfig
		latencyUrl = "http://www.gstatic.com/generate_204"
		latencyInterval = 300
		latencyTolerance = 50
		selectorGroups = nil
	}

	proxies := make([]map[string]interface{}, 0, 1)
	renderEntries := make([]clashProxyRenderEntry, 0, 1)
	if s.shouldUseStoredClashProxy(subOutbound) {
		if proxy, ok := parseSubOutboundClashProxy(subOutbound); ok {
			s.refreshSubOutboundClashProxyTLS(proxy, subOutbound)
			proxies = append(proxies, proxy)
			name, _ := proxy["name"].(string)
			renderEntries = append(renderEntries, clashProxyRenderEntry{
				Name:    strings.TrimSpace(name),
				Proxy:   proxy,
				RawYAML: s.storedSubOutboundRawClashYAML(subOutbound),
			})
		}
	}
	if len(proxies) == 0 {
		fallbackEntries, convErr := s.buildRuntimeClashRenderEntries(
			[]map[string]interface{}{runtimeOutbound},
			latencyUrl,
			latencyInterval,
			latencyTolerance,
			selectorGroups,
		)
		if convErr != nil {
			return nil, convErr
		}
		renderEntries = append(renderEntries, fallbackEntries...)
		for _, entry := range fallbackEntries {
			if entry.Proxy == nil {
				continue
			}
			proxies = append(proxies, entry.Proxy)
		}
	}

	rendered, err := renderClashSubscriptionFromEntries(renderEntries, latencyUrl, latencyInterval, latencyTolerance, selectorGroups)
	if err != nil {
		return nil, err
	}

	resultStr := clashConfig + "\n" + string(rendered)
	return &resultStr, nil
}

func (s *SubManagerSubService) shouldUseStoredClashProxy(subOutbound *model.SubOutbound) bool {
	if subOutbound == nil {
		return false
	}
	if len(subOutbound.ClashOptions) == 0 {
		return false
	}
	// Subscription-manager rendering should prefer the node payload that was
	// already bound and stored for this suboutbound, instead of re-deriving a
	// fresh Clash proxy from RawOutbound. This keeps user-sync nodes and
	// subgroup-imported nodes aligned with the "lookup id first, then read the
	// bound node data" design.
	return true
}

func (s *SubManagerSubService) storedSubOutboundRawClashYAML(subOutbound *model.SubOutbound) []byte {
	if subOutbound == nil {
		return nil
	}
	if len(subOutbound.RawClashYAML) == 0 {
		return nil
	}
	if strings.TrimSpace(subOutbound.SourceType) != subManagerSourceSubGroup {
		return nil
	}
	if subOutbound.SourceInboundId != 0 {
		return nil
	}
	return cloneRawBytes(subOutbound.RawClashYAML)
}

func (s *SubManagerSubService) buildRuntimeClashRenderEntries(
	rawOutbounds []map[string]interface{},
	latencyURL string,
	latencyInterval int,
	latencyTolerance int,
	selectorGroups []clashSelectorGroupConfig,
) ([]clashProxyRenderEntry, error) {
	if len(rawOutbounds) == 0 {
		return []clashProxyRenderEntry{}, nil
	}

	outbounds, _ := buildSubManagerRuntimeOutbounds(rawOutbounds)
	if len(outbounds) == 0 {
		return []clashProxyRenderEntry{}, nil
	}

	renderedFallback, err := s.ClashService.ConvertToClashMeta(&outbounds, latencyURL, latencyInterval, latencyTolerance, selectorGroups)
	if err != nil {
		return nil, err
	}
	fallbackProxies, err := extractClashProxiesFromRenderedConfig(renderedFallback)
	if err != nil {
		return nil, err
	}

	entries := make([]clashProxyRenderEntry, 0, len(fallbackProxies))
	for _, proxy := range fallbackProxies {
		name, _ := proxy["name"].(string)
		entries = append(entries, clashProxyRenderEntry{
			Name:  strings.TrimSpace(name),
			Proxy: proxy,
		})
	}
	return entries, nil
}

func (s *SubManagerSubService) buildRuntimeOutboundMap(subOutbound *model.SubOutbound) (map[string]interface{}, error) {
	outboundMap, err := s.decodeSubOutboundMap(subOutbound)
	if err != nil {
		return nil, err
	}
	s.refreshSubOutboundTLS(outboundMap, subOutbound)
	return outboundMap, nil
}

func (s *SubManagerSubService) refreshSubOutboundTLS(outboundMap map[string]interface{}, subOutbound *model.SubOutbound) {
	if outboundMap == nil {
		return
	}
	if _, ok := outboundMap["tls"].(map[string]interface{}); !ok {
		return
	}

	if tlsConfig, ok := s.loadManagedSourceTLS(subOutbound); ok && tlsConfig != nil {
		refreshSubscriptionOutboundTLS(outboundMap, tlsConfig)
		return
	}

	if !shouldFallbackRefreshSubOutboundTLS(subOutbound) {
		return
	}

	if tlsConfig, ok := buildFallbackTLSConfigFromOutbound(outboundMap); ok && tlsConfig != nil {
		refreshSubscriptionOutboundTLS(outboundMap, tlsConfig)
	}
}

func shouldFallbackRefreshSubOutboundTLS(subOutbound *model.SubOutbound) bool {
	if subOutbound == nil {
		return true
	}
	if strings.TrimSpace(subOutbound.SourceType) == "" {
		return true
	}
	return false
}

func (s *SubManagerSubService) refreshSubOutboundClashProxyTLS(proxy map[string]interface{}, subOutbound *model.SubOutbound) {
	if proxy == nil || subOutbound == nil {
		return
	}

	tlsConfig, ok := s.loadManagedSourceTLS(subOutbound)
	if !ok || tlsConfig == nil {
		return
	}

	serverTLS := decodeSubscriptionTLSRaw(tlsConfig.Server)
	clientTLS := decodeSubscriptionTLSRaw(tlsConfig.Client)

	if shouldIncludeSubscriptionClashFingerprint(clientTLS) {
		if _, serverCertPEM, ok := loadSubscriptionPEM(serverTLS["certificate"], serverTLS["certificate_path"], "CERTIFICATE"); ok {
			if fingerprint, ok := calculateSubscriptionTLSFingerprint(serverCertPEM); ok {
				proxy["fingerprint"] = fingerprint
			}
		}
	} else {
		delete(proxy, "fingerprint")
	}

	if insecure, ok := clientTLS["insecure"].(bool); ok {
		proxy["skip-cert-verify"] = insecure
	}
	if disableSNI, ok := clientTLS["disable_sni"].(bool); ok {
		proxy["disable-sni"] = disableSNI
	}

	if serverName, ok := serverTLS["server_name"].(string); ok && strings.TrimSpace(serverName) != "" {
		sni := strings.TrimSpace(serverName)
		proxy["sni"] = sni
		proxy["servername"] = sni
	}
	if utls, ok := clientTLS["utls"].(map[string]interface{}); ok && utls != nil {
		if fp, ok := utls["fingerprint"].(string); ok && strings.TrimSpace(fp) != "" {
			proxy["client-fingerprint"] = strings.TrimSpace(fp)
		}
	}
}

func (s *SubManagerSubService) loadManagedSourceTLS(subOutbound *model.SubOutbound) (*model.Tls, bool) {
	if subOutbound == nil || subOutbound.SourceInboundId == 0 {
		return nil, false
	}

	db := database.GetDB()
	sourceType := strings.TrimSpace(subOutbound.SourceType)
	switch sourceType {
	case subManagerSourceClient:
		inbound := &model.Inbound{}
		if err := db.Model(model.Inbound{}).
			Preload("Tls").
			Where("id = ?", subOutbound.SourceInboundId).
			First(inbound).Error; err != nil {
			return nil, false
		}
		if inbound.Tls == nil {
			return nil, false
		}
		return inbound.Tls, true
	case subManagerSourceMihomoClient:
		inbound := &model.MihomoInbound{}
		if err := db.Model(model.MihomoInbound{}).
			Preload("Tls").
			Where("id = ?", subOutbound.SourceInboundId).
			First(inbound).Error; err != nil {
			return nil, false
		}
		if inbound.Tls == nil {
			return nil, false
		}
		return inbound.Tls.ToBase(), true
	default:
		return nil, false
	}
}

func buildFallbackTLSConfigFromOutbound(outboundMap map[string]interface{}) (*model.Tls, bool) {
	if outboundMap == nil {
		return nil, false
	}

	tlsMap, ok := outboundMap["tls"].(map[string]interface{})
	if !ok || tlsMap == nil {
		return nil, false
	}

	raw, err := json.Marshal(tlsMap)
	if err != nil {
		return nil, false
	}

	server := append(json.RawMessage(nil), raw...)
	client := append(json.RawMessage(nil), raw...)
	return &model.Tls{Server: server, Client: client}, true
}

// getSubOutboundMap reads one suboutbound and converts it to map.
func (s *SubManagerSubService) getSubOutboundMap(tag string) (map[string]interface{}, error) {
	subOutbound, err := s.getSubOutboundRecord(tag)
	if err != nil {
		return nil, err
	}
	return s.decodeSubOutboundMap(subOutbound)
}

func (s *SubManagerSubService) getSubOutboundRecord(tag string) (*model.SubOutbound, error) {
	db := database.GetDB()
	subOutbound := &model.SubOutbound{}
	err := db.Model(model.SubOutbound{}).Where("tag = ?", tag).First(subOutbound).Error
	if err != nil {
		return nil, err
	}
	return subOutbound, nil
}

func (s *SubManagerSubService) decodeSubOutboundMap(subOutbound *model.SubOutbound) (map[string]interface{}, error) {
	if subOutbound == nil {
		return nil, fmt.Errorf("suboutbound is nil")
	}

	outboundJson := append(json.RawMessage(nil), subOutbound.RawOutbound...)
	if len(outboundJson) == 0 {
		var err error
		outboundJson, err = subOutbound.MarshalJSON()
		if err != nil {
			return nil, err
		}
	}

	var outboundMap map[string]interface{}
	if err := json.Unmarshal(outboundJson, &outboundMap); err != nil {
		return nil, err
	}
	delete(outboundMap, "id")
	if tag, _ := outboundMap["tag"].(string); strings.TrimSpace(tag) == "" {
		outboundMap["tag"] = subOutbound.Tag
	}
	if outType, _ := outboundMap["type"].(string); strings.TrimSpace(outType) == "" {
		outboundMap["type"] = subOutbound.Type
	}
	return outboundMap, nil
}

func parseSubOutboundClashProxy(subOutbound *model.SubOutbound) (map[string]interface{}, bool) {
	if subOutbound == nil || len(subOutbound.ClashOptions) == 0 {
		return nil, false
	}

	proxy, err := decodeJSONMapUseNumber(subOutbound.ClashOptions)
	if err != nil {
		return nil, false
	}
	proxy = normalizeProxyForYAML(proxy)
	proxy, _ = sanitizeMihomoClashProxy(proxy)

	name, _ := proxy["name"].(string)
	name = strings.TrimSpace(name)
	if name == "" {
		proxy["name"] = subOutbound.Tag
	}
	proxyType, _ := proxy["type"].(string)
	if !util.SupportsMihomoSubscriptionClashProxyType(proxyType) {
		return nil, false
	}
	return proxy, true
}

func extractClashProxiesFromRenderedConfig(raw []byte) ([]map[string]interface{}, error) {
	var doc map[string]interface{}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}

	proxiesRaw, ok := doc["proxies"].([]interface{})
	if !ok {
		return []map[string]interface{}{}, nil
	}

	proxies := make([]map[string]interface{}, 0, len(proxiesRaw))
	for _, item := range proxiesRaw {
		proxy, ok := item.(map[string]interface{})
		if !ok || proxy == nil {
			continue
		}
		name, _ := proxy["name"].(string)
		if strings.TrimSpace(name) == "" {
			continue
		}
		copied := make(map[string]interface{}, len(proxy))
		for k, v := range proxy {
			copied[k] = v
		}
		proxies = append(proxies, normalizeProxyForYAML(copied))
	}

	return proxies, nil
}

func renderClashSubscriptionFromProxies(
	proxies []map[string]interface{},
	latencyUrl string,
	latencyInterval int,
	latencyTolerance int,
	selectorGroups []clashSelectorGroupConfig,
) ([]byte, error) {
	proxies = util.FilterMihomoSubscriptionClashProxies(proxies)
	unique := dedupeClashProxiesByName(proxies)
	proxyEntries := make([]interface{}, 0, len(unique))
	nodeTags := make([]string, 0, len(unique))

	for _, proxy := range unique {
		sanitizedProxy, _ := sanitizeMihomoClashProxy(proxy)
		if sanitizedProxy == nil {
			continue
		}
		proxy = sanitizedProxy

		name, _ := proxy["name"].(string)
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		copied := make(map[string]interface{}, len(proxy))
		for k, v := range proxy {
			copied[k] = v
		}
		proxyEntries = append(proxyEntries, normalizeProxyForYAML(copied))
		nodeTags = append(nodeTags, name)
	}

	proxyGroups := buildFixedMihomoProxyGroups(nodeTags, latencyUrl, latencyInterval, latencyTolerance)
	proxyGroups = append(proxyGroups, buildNamedClashProxyGroups(selectorGroups, nodeTags)...)

	output := map[string]interface{}{
		"proxies":      proxyEntries,
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

func dedupeClashProxiesByName(proxies []map[string]interface{}) []map[string]interface{} {
	if len(proxies) == 0 {
		return []map[string]interface{}{}
	}
	result := make([]map[string]interface{}, 0, len(proxies))
	seen := make(map[string]struct{}, len(proxies))
	for _, proxy := range proxies {
		if proxy == nil {
			continue
		}
		name, _ := proxy["name"].(string)
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, proxy)
	}
	return result
}

// extractTlsStoreFromOutbounds extracts tls_store/store from outbound.tls blocks
// and returns the first store value for root certificate.store.
func extractTlsStoreFromOutbounds(outbounds []map[string]interface{}) string {
	var tlsStore string
	for _, outbound := range outbounds {
		tlsRaw, ok := outbound["tls"]
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

// SaveSubJsonToFile writes generated subscription JSON into sub_json directory.
func SaveSubJsonToFile(tag string, jsonContent string) error {
	logger.Infof("[SubManagerSub] start saving subscription JSON, tag: %s", tag)
	if err := service.ValidateManagedSubOutboundTagForSubJSON(tag); err != nil {
		return err
	}

	subJsonDir := filepath.Join(config.GetDataDir(), "sub_json")
	logger.Infof("[SubManagerSub] target directory: %s", subJsonDir)

	// Sanitize filename.
	baseFilename := sanitizeFilename(tag)
	filePath := filepath.Join(subJsonDir, fmt.Sprintf("%s.json", baseFilename))
	logger.Infof("[SubManagerSub] file path: %s", filePath)

	// Write file content.
	if err := service.ManagedRuntimeWriteFile(filePath, []byte(jsonContent)); err != nil {
		logger.Errorf("[SubManagerSub] failed to write subscription file: %v", err)
		return fmt.Errorf("failed to write subscription file: %w", err)
	}

	logger.Infof("[SubManagerSub] subscription JSON saved: %s (size=%d bytes)", filePath, len(jsonContent))
	return nil
}

// sanitizeFilename removes unsafe path characters.
func sanitizeFilename(name string) string {
	unsafe := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	result := name
	for _, char := range unsafe {
		for i := 0; i < len(result); i++ {
			if i+len(char) <= len(result) && result[i:i+len(char)] == char {
				result = result[:i] + "_" + result[i+len(char):]
			}
		}
	}
	return result
}

// GetSubGroupJson renders sing-box JSON subscription for a group.
func (s *SubManagerSubService) GetSubGroupJson(groupName string) (*string, error) {
	db := database.GetDB()

	// Load group record.
	subGroup := &model.SubGroup{}
	err := db.Model(model.SubGroup{}).Where("name = ?", groupName).First(subGroup).Error
	if err != nil {
		return nil, err
	}

	// Parse outbound tags from group.
	var outboundTags []string
	if strings.TrimSpace(subGroup.Outbounds) != "" {
		if err := json.Unmarshal([]byte(subGroup.Outbounds), &outboundTags); err != nil {
			return nil, err
		}
	}
	if outboundTags == nil {
		outboundTags = []string{}
	}

	// Load all suboutbounds referenced by group tags.
	var outbounds []map[string]interface{}
	var rawOutbounds []map[string]interface{}

	for _, tag := range outboundTags {
		subOutbound, getErr := s.getSubOutboundRecord(tag)
		if getErr != nil {
			// Skip missing/invalid records.
			continue
		}

		outboundMap, mapErr := s.buildRuntimeOutboundMap(subOutbound)
		if mapErr != nil {
			// Skip missing/invalid records.
			continue
		}

		rawOutbounds = append(rawOutbounds, outboundMap)
	}

	outbounds, outTags := buildSubManagerRuntimeOutbounds(rawOutbounds)
	if outbounds == nil {
		outbounds = []map[string]interface{}{}
	}
	if outTags == nil {
		outTags = []string{}
	}
	outbounds, outTags = util.FilterTaggedSubscriptionOutbounds(
		outbounds,
		outTags,
		util.SupportsSingboxSubscriptionOutboundType,
	)
	for i := range outbounds {
		util.SanitizeSingboxSubscriptionOutbound(outbounds[i])
	}

	// Read latency test settings.
	latencyUrl := "http://www.gstatic.com/generate_204"
	latencyInterval := "10m"
	latencyTolerance := 50
	var extJson map[string]interface{}
	othersStr, _ := s.SettingService.GetSubJsonExt()
	if len(othersStr) > 0 {
		if err := json.Unmarshal([]byte(othersStr), &extJson); err == nil {
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

	// Remove mihomo-only fields from sing-box JSON output.
	stripMihomoFields(&outbounds)

	// Reuse JsonService defaults for selector/urltest/direct/block/final groups.
	s.JsonService.addDefaultOutbounds(&outbounds, &outTags, latencyUrl, latencyInterval, latencyTolerance, selectorGroups)

	var jsonConfig map[string]interface{}
	if err := json.Unmarshal([]byte(defaultJson), &jsonConfig); err != nil {
		return nil, err
	}
	jsonConfig["outbounds"] = outbounds

	// Move tls_store from outbound.tls blocks into root certificate.store.
	tlsStore := extractTlsStoreFromOutbounds(outbounds)
	tlsStore = s.SettingService.ResolveSubscriptionTLSStore(tlsStore)

	// Reuse JsonService extra fields merge.
	s.JsonService.addOthers(&jsonConfig)
	applyCertificateStore(&jsonConfig, tlsStore)

	result, err := json.MarshalIndent(jsonConfig, "", "  ")
	if err != nil {
		return nil, err
	}
	resultStr := string(result)
	return &resultStr, nil
}

// GetSubGroupClash renders Clash subscription for a group.
func (s *SubManagerSubService) GetSubGroupClash(groupName string) (*string, error) {
	db := database.GetDB()

	subGroup := &model.SubGroup{}
	err := db.Model(model.SubGroup{}).Where("name = ?", groupName).First(subGroup).Error
	if err != nil {
		return nil, err
	}

	var outboundTags []string
	if strings.TrimSpace(subGroup.Outbounds) != "" {
		if err := json.Unmarshal([]byte(subGroup.Outbounds), &outboundTags); err != nil {
			return nil, err
		}
	}
	if outboundTags == nil {
		outboundTags = []string{}
	}

	clashConfig, latencyUrl, latencyInterval, latencyTolerance, selectorGroups, err := s.ClashService.getClashConfigClean()
	if err != nil || len(clashConfig) == 0 {
		clashConfig = basicClashConfig
		latencyUrl = "http://www.gstatic.com/generate_204"
		latencyInterval = 300
		latencyTolerance = 50
		selectorGroups = nil
	}

	renderEntries := make([]clashProxyRenderEntry, 0, len(outboundTags))
	for _, tag := range outboundTags {
		subOutbound, getErr := s.getSubOutboundRecord(tag)
		if getErr != nil {
			continue
		}
		if s.shouldUseStoredClashProxy(subOutbound) {
			if proxy, ok := parseSubOutboundClashProxy(subOutbound); ok {
				s.refreshSubOutboundClashProxyTLS(proxy, subOutbound)
				name, _ := proxy["name"].(string)
				renderEntries = append(renderEntries, clashProxyRenderEntry{
					Name:    strings.TrimSpace(name),
					Proxy:   proxy,
					RawYAML: s.storedSubOutboundRawClashYAML(subOutbound),
				})
				continue
			}
		}

		outboundMap, mapErr := s.buildRuntimeOutboundMap(subOutbound)
		if mapErr == nil {
			fallbackEntries, convErr := s.buildRuntimeClashRenderEntries(
				[]map[string]interface{}{outboundMap},
				latencyUrl,
				latencyInterval,
				latencyTolerance,
				selectorGroups,
			)
			if convErr != nil {
				return nil, convErr
			}
			renderEntries = append(renderEntries, fallbackEntries...)
		}
	}

	rendered, err := renderClashSubscriptionFromEntries(renderEntries, latencyUrl, latencyInterval, latencyTolerance, selectorGroups)
	if err != nil {
		return nil, err
	}

	resultStr := clashConfig + "\n" + string(rendered)
	return &resultStr, nil
}

// buildSubManagerRuntimeOutbounds expands runtime outbounds used by SubManager subscriptions.
// ShadowTLS with ss_config is split into a shadowsocks detour and a shadowtls outbound.
func buildSubManagerRuntimeOutbounds(raw []map[string]interface{}) ([]map[string]interface{}, []string) {
	outbounds := make([]map[string]interface{}, 0, len(raw))
	outTags := make([]string, 0, len(raw))

	for _, outbound := range raw {
		if outbound == nil {
			continue
		}

		outType, _ := outbound["type"].(string)
		tag, _ := outbound["tag"].(string)
		if tag == "" {
			continue
		}

		if outType != "shadowtls" {
			outbounds = append(outbounds, cloneRuntimeMap(outbound))
			outTags = append(outTags, tag)
			continue
		}

		ssOutbound, stlsOutbound := util.BuildShadowTLSRuntimeOutboundPairMap(outbound, false)
		if ssOutbound == nil {
			if stlsOutbound != nil {
				outbounds = append(outbounds, stlsOutbound)
			}
			outTags = append(outTags, tag)
			continue
		}

		outbounds = append(outbounds, ssOutbound, stlsOutbound)
		outTags = append(outTags, tag)
	}

	return outbounds, outTags
}

func cloneRuntimeMap(src map[string]interface{}) map[string]interface{} {
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
