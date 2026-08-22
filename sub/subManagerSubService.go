package sub

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/service"
	"github.com/alireza0/s-ui/util"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
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
	if len(outbounds) > service.SubscriptionGroupMaxOutbounds {
		return nil, fmt.Errorf("subscription group renders more than %d nodes", service.SubscriptionGroupMaxOutbounds)
	}
	for i := range outbounds {
		util.SanitizeSingboxSubscriptionOutbound(outbounds[i])
	}

	tlsStore := extractTlsStoreFromOutbounds(outbounds)
	tlsStore = s.SettingService.ResolveSubscriptionTLSStore(tlsStore)
	resultStr, err := s.JsonService.renderJSONSubscription(&outbounds, &outTags, tlsStore)
	if err != nil {
		return nil, err
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

	extension, err := s.ClashService.loadClashExtension()
	if err != nil {
		return nil, err
	}

	proxies := make([]map[string]interface{}, 0, 1)
	renderEntries := make([]clashProxyRenderEntry, 0, 1)
	if s.shouldUseStoredClashProxy(subOutbound) {
		if proxy, ok := parseSubOutboundClashProxy(subOutbound); ok {
			if strings.EqualFold(strings.TrimSpace(subOutbound.Type), "shadowquic") {
				// Keep ClashOptions as the preferred source, but normalize it through
				// the strict ShadowQUIC schema so old TLS/generic fields cannot leak.
				name := strings.TrimSpace(firstString(proxy["name"]))
				if normalized, valid := util.BuildMihomoShadowQUICClashProxy(proxy, name); valid {
					proxy = normalized
				} else {
					proxy = nil
				}
			}
			if proxy != nil {
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
	}

	if len(proxies) == 0 {
		fallbackEntries, convErr := s.buildRuntimeClashRenderEntries(
			[]map[string]interface{}{runtimeOutbound},
			extension.LatencyURL,
			extension.LatencyInterval,
			extension.LatencyTolerance,
			extension.SelectorGroups,
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

	generated := buildClashSubscriptionMapFromEntries(
		renderEntries,
		extension.LatencyURL,
		extension.LatencyInterval,
		extension.LatencyTolerance,
		extension.SelectorGroups,
	)
	resultStr, err := renderMergedClashSubscription(extension.Config, generated)
	if err != nil {
		return nil, err
	}
	return &resultStr, nil
}

func (s *SubManagerSubService) shouldUseStoredClashProxy(subOutbound *model.SubOutbound) bool {
	if subOutbound == nil {
		return false
	}
	if isServerOnlySubscriptionSubOutbound(subOutbound) {
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
	if strings.EqualFold(strings.TrimSpace(firstString(outboundMap["type"])), "shadowquic") {
		util.SanitizeMihomoShadowQUICOutbound(outboundMap)
		return outboundMap, nil
	}
	s.refreshSubOutboundTLS(outboundMap, subOutbound)
	return outboundMap, nil
}

func (s *SubManagerSubService) refreshSubOutboundTLS(outboundMap map[string]interface{}, subOutbound *model.SubOutbound) {
	if outboundMap == nil {
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
	if strings.EqualFold(strings.TrimSpace(firstString(proxy["type"])), "shadowquic") ||
		strings.EqualFold(strings.TrimSpace(subOutbound.Type), "shadowquic") {
		return
	}

	tlsConfig, ok := s.loadManagedSourceTLS(subOutbound)
	if !ok || tlsConfig == nil {
		return
	}
	proxyType := strings.ToLower(strings.TrimSpace(firstString(proxy["type"])))
	mode := model.NormalizeMihomoTlsMode(tlsConfig.Mode, tlsConfig.Server, tlsConfig.Client)
	if proxyType == "shadowsocks" || isMihomoWrapperTLSMode(mode) {
		refreshMihomoClashProxyTLS(proxy, tlsConfig)
		return
	}
	clearMihomoClashWrapperProjection(proxy)

	serverTLS := decodeSubscriptionTLSRaw(tlsConfig.Server)
	clientTLS := decodeSubscriptionTLSRaw(tlsConfig.Client)
	tlsEnabled := true
	if value, ok := toBool(serverTLS["enabled"]); ok {
		tlsEnabled = value
	} else if value, ok := toBool(clientTLS["enabled"]); ok {
		tlsEnabled = value
	}
	if !tlsEnabled {
		for _, key := range []string{"tls", "sni", "servername", "alpn", "reality-opts", "ech-opts", "fingerprint", "skip-cert-verify", "disable-sni", "client-fingerprint"} {
			delete(proxy, key)
		}
		return
	}
	proxy["tls"] = true

	if shouldIncludeSubscriptionClashFingerprint(clientTLS) {
		if _, serverCertPEM, ok := loadSubscriptionPEM(serverTLS["certificate"], serverTLS["certificate_path"], "CERTIFICATE"); ok {
			if fingerprint, ok := calculateSubscriptionTLSFingerprint(serverCertPEM); ok {
				proxy["fingerprint"] = fingerprint
			} else {
				delete(proxy, "fingerprint")
			}
		} else {
			delete(proxy, "fingerprint")
		}
	} else {
		delete(proxy, "fingerprint")
	}

	if insecure, ok := clientTLS["insecure"].(bool); ok {
		proxy["skip-cert-verify"] = insecure
	} else {
		delete(proxy, "skip-cert-verify")
	}
	if disableSNI, ok := clientTLS["disable_sni"].(bool); ok {
		proxy["disable-sni"] = disableSNI
	} else {
		delete(proxy, "disable-sni")
	}
	if alpn := toStringSlice(clientTLS["alpn"]); len(alpn) > 0 {
		proxy["alpn"] = alpn
	} else {
		delete(proxy, "alpn")
	}

	serverName := strings.TrimSpace(firstStringValue(clientTLS["server_name"]))
	if serverName == "" {
		serverName = strings.TrimSpace(firstStringValue(serverTLS["server_name"]))
	}
	if serverName != "" {
		sni := strings.TrimSpace(serverName)
		switch proxyType {
		case "vmess", "vless":
			proxy["servername"] = sni
			delete(proxy, "sni")
		default:
			proxy["sni"] = sni
			delete(proxy, "servername")
		}
	} else {
		delete(proxy, "sni")
		delete(proxy, "servername")
	}
	delete(proxy, "reality-opts")
	if tlsReality, ok := clientTLS["reality"].(map[string]interface{}); ok && tlsReality != nil {
		if enabled, ok := toBool(tlsReality["enabled"]); !ok || enabled {
			realityOpts := map[string]interface{}{}
			if publicKey := strings.TrimSpace(firstString(tlsReality["public_key"])); publicKey != "" {
				realityOpts["public-key"] = publicKey
			}
			if shortID := strings.TrimSpace(firstString(tlsReality["short_id"])); shortID != "" {
				realityOpts["short-id"] = shortID
			}
			if len(realityOpts) > 0 {
				proxy["reality-opts"] = realityOpts
			}
		}
	}
	delete(proxy, "ech-opts")
	if ech, ok := clientTLS["ech"].(map[string]interface{}); ok && ech != nil {
		echoEnabled, _ := toBool(ech["enabled"])
		echConfig := flattenECHConfig(ech["config"])
		queryServerName := strings.TrimSpace(firstString(ech["query_server_name"]))
		if echoEnabled || echConfig != "" || queryServerName != "" {
			echOpts := map[string]interface{}{"enable": true}
			if echConfig != "" {
				echOpts["config"] = echConfig
			}
			if queryServerName != "" {
				echOpts["query-server-name"] = queryServerName
			}
			proxy["ech-opts"] = echOpts
		}
	}
	if utls, ok := clientTLS["utls"].(map[string]interface{}); ok && utls != nil {
		if fp, ok := utls["fingerprint"].(string); ok && strings.TrimSpace(fp) != "" {
			proxy["client-fingerprint"] = strings.TrimSpace(fp)
		} else {
			delete(proxy, "client-fingerprint")
		}
	} else {
		delete(proxy, "client-fingerprint")
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
	if isServerOnlySubscriptionSubOutbound(subOutbound) {
		return nil, fmt.Errorf("suboutbound %s uses server-only mixed listener type", tag)
	}
	return subOutbound, nil
}

func isServerOnlySubscriptionSubOutbound(subOutbound *model.SubOutbound) bool {
	if subOutbound == nil {
		return false
	}
	sourceType := strings.TrimSpace(subOutbound.SourceType)
	if sourceType != subManagerSourceClient && sourceType != subManagerSourceMihomoClient {
		return false
	}
	if util.IsSubscriptionServerOnlyInboundType(subOutbound.Type) {
		return true
	}
	if len(subOutbound.RawOutbound) == 0 {
		return false
	}

	payload := map[string]interface{}{}
	if err := json.Unmarshal(subOutbound.RawOutbound, &payload); err != nil {
		return false
	}
	payloadType, _ := payload["type"].(string)
	return util.IsSubscriptionServerOnlyInboundType(payloadType)
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
	if proxy == nil {
		return nil, false
	}

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

// SaveSubJsonToFile is retained for compatibility. Subscription JSON is now
// served dynamically, so this wrapper only preserves filename validation.
func SaveSubJsonToFile(tag string, jsonContent string) error {
	_ = jsonContent
	if err := service.ValidateManagedSubOutboundTagForSubJSON(tag); err != nil {
		return err
	}
	return nil
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

	outboundTags, err := parseSubscriptionGroupOutboundTags(subGroup.Outbounds)
	if err != nil {
		return nil, err
	}
	subOutbounds, err := s.loadSubscriptionGroupOutbounds(db, outboundTags)
	if err != nil {
		return nil, err
	}
	rawOutbounds := make([]map[string]interface{}, 0, len(subOutbounds))
	for _, subOutbound := range subOutbounds {
		outboundMap, mapErr := s.buildRuntimeOutboundMap(subOutbound)
		if mapErr != nil {
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
	if len(outbounds) > service.SubscriptionGroupMaxOutbounds {
		return nil, fmt.Errorf("subscription group renders more than %d nodes", service.SubscriptionGroupMaxOutbounds)
	}
	for i := range outbounds {
		util.SanitizeSingboxSubscriptionOutbound(outbounds[i])
	}

	tlsStore := extractTlsStoreFromOutbounds(outbounds)
	tlsStore = s.SettingService.ResolveSubscriptionTLSStore(tlsStore)
	resultStr, err := s.JsonService.renderJSONSubscription(&outbounds, &outTags, tlsStore)
	if err != nil {
		return nil, err
	}
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

	outboundTags, err := parseSubscriptionGroupOutboundTags(subGroup.Outbounds)
	if err != nil {
		return nil, err
	}
	subOutbounds, err := s.loadSubscriptionGroupOutbounds(db, outboundTags)
	if err != nil {
		return nil, err
	}

	extension, err := s.ClashService.loadClashExtension()
	if err != nil {
		return nil, err
	}

	renderEntries := make([]clashProxyRenderEntry, 0, len(subOutbounds))
	for _, subOutbound := range subOutbounds {
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
				extension.LatencyURL,
				extension.LatencyInterval,
				extension.LatencyTolerance,
				extension.SelectorGroups,
			)
			if convErr != nil {
				return nil, convErr
			}
			renderEntries = append(renderEntries, fallbackEntries...)
		}
	}
	if len(renderEntries) > service.SubscriptionGroupMaxOutbounds {
		return nil, fmt.Errorf("subscription group renders more than %d nodes", service.SubscriptionGroupMaxOutbounds)
	}

	generated := buildClashSubscriptionMapFromEntries(
		renderEntries,
		extension.LatencyURL,
		extension.LatencyInterval,
		extension.LatencyTolerance,
		extension.SelectorGroups,
	)
	resultStr, err := renderMergedClashSubscription(extension.Config, generated)
	if err != nil {
		return nil, err
	}
	return &resultStr, nil
}

func parseSubscriptionGroupOutboundTags(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return []string{}, nil
	}
	var tags []string
	if err := json.Unmarshal([]byte(raw), &tags); err != nil {
		return nil, err
	}
	result := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, exists := seen[tag]; exists {
			continue
		}
		seen[tag] = struct{}{}
		result = append(result, tag)
		if len(result) > service.SubscriptionGroupMaxOutbounds {
			return nil, fmt.Errorf("subscription group contains more than %d nodes", service.SubscriptionGroupMaxOutbounds)
		}
	}
	return result, nil
}

func (s *SubManagerSubService) loadSubscriptionGroupOutbounds(db *gorm.DB, tags []string) ([]*model.SubOutbound, error) {
	if len(tags) == 0 {
		return []*model.SubOutbound{}, nil
	}
	rows := make([]*model.SubOutbound, 0, len(tags))
	if err := db.Model(model.SubOutbound{}).Where("tag IN ?", tags).Find(&rows).Error; err != nil {
		return nil, err
	}
	byTag := make(map[string]*model.SubOutbound, len(rows))
	for _, row := range rows {
		if row == nil || isServerOnlySubscriptionSubOutbound(row) {
			continue
		}
		byTag[strings.TrimSpace(row.Tag)] = row
	}
	ordered := make([]*model.SubOutbound, 0, len(tags))
	for _, tag := range tags {
		if row := byTag[tag]; row != nil {
			ordered = append(ordered, row)
		}
	}
	return ordered, nil
}

// buildSubManagerRuntimeOutbounds expands manually configured Mixed endpoints,
// standalone ShadowTLS, and Mihomo Shadowsocks shadow-tls plugins into
// client-compatible subscription outbounds.
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

		if strings.EqualFold(strings.TrimSpace(outType), "mixed") {
			socksOutbound, httpOutbound := util.BuildMixedSubscriptionOutboundPair(outbound)
			for _, expanded := range []map[string]interface{}{socksOutbound, httpOutbound} {
				if expanded == nil {
					continue
				}
				expandedTag, _ := expanded["tag"].(string)
				outbounds = append(outbounds, expanded)
				outTags = append(outTags, expandedTag)
			}
			continue
		}

		if ssOutbound, shadowTLSOutbound, ok := util.BuildMihomoShadowsocksShadowTLSClientPair(outbound); ok {
			outbounds = append(outbounds, ssOutbound, shadowTLSOutbound)
			outTags = append(outTags, tag)
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
	return util.CloneJSONMap(src)
}
