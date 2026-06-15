package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
)

type MihomoManagerService struct {
	MihomoConfigService
	MihomoInboundService
	MihomoOutboundService
}

func NewMihomoManagerService() *MihomoManagerService {
	return &MihomoManagerService{}
}

func (s *MihomoManagerService) GenerateServerDocument() (map[string]interface{}, error) {
	baseData, err := s.MihomoConfigService.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("get mihomo base config failed: %w", err)
	}

	base := map[string]interface{}{}
	if strings.TrimSpace(baseData) != "" {
		if err := json.Unmarshal([]byte(baseData), &base); err != nil {
			return nil, fmt.Errorf("parse mihomo base config failed: %w", err)
		}
	}

	document := copyMihomoGeneralConfig(base)
	route := requireRouteMap(base)
	applyMihomoRouteGeneralConfig(document, route)
	if dns := buildMihomoDNSDocument(base["dns"]); len(dns) > 0 {
		document["dns"] = dns
	}

	db := database.GetDB()

	var outbounds []model.MihomoOutbound
	if err := db.Model(model.MihomoOutbound{}).Order("id ASC").Find(&outbounds).Error; err != nil {
		return nil, fmt.Errorf("load mihomo outbounds failed: %w", err)
	}

	rawOutbounds := make([]map[string]interface{}, 0, len(outbounds))
	for _, outbound := range outbounds {
		rawJSON, err := resolveMihomoOutboundJSON(&outbound)
		if err != nil {
			return nil, fmt.Errorf("marshal mihomo outbound %s failed: %w", outbound.Tag, err)
		}
		rawMap, err := marshalJSONMap(rawJSON)
		if err != nil {
			return nil, fmt.Errorf("decode mihomo outbound %s failed: %w", outbound.Tag, err)
		}
		rawOutbounds = append(rawOutbounds, rawMap)
	}

	proxyResult := convertMihomoOutboundsToClash(rawOutbounds)
	if len(proxyResult.ValidationErrs) > 0 {
		return nil, fmt.Errorf("invalid mihomo outbound config: %s", strings.Join(proxyResult.ValidationErrs, "; "))
	}
	if len(proxyResult.Proxies) > 0 {
		proxies := make([]interface{}, 0, len(proxyResult.Proxies))
		for _, proxy := range proxyResult.Proxies {
			proxies = append(proxies, proxy)
		}
		document["proxies"] = proxies
	}
	if len(proxyResult.ProxyGroups) > 0 {
		proxyGroups := make([]interface{}, 0, len(proxyResult.ProxyGroups))
		for _, group := range proxyResult.ProxyGroups {
			proxyGroups = append(proxyGroups, group)
		}
		document["proxy-groups"] = proxyGroups
	}

	providers, providerTags := buildMihomoRuleProviders(route["rule_set"], proxyResult)
	ipRuleProviderTags := collectMihomoIPRuleProviderTags(providers)
	if len(providers) > 0 {
		document["rule-providers"] = providers
	}

	fallbackFinalCandidates := collectMihomoRouteFinalFallbackCandidates(rawOutbounds, proxyResult)
	globalFinal, ok := resolveMihomoGlobalFinalTarget(route, proxyResult, fallbackFinalCandidates)
	if !ok {
		rawFinal := strings.TrimSpace(firstString(route["final"]))
		if fallbackFinal, fallbackOK := resolveMihomoGlobalFinalTarget(map[string]interface{}{}, proxyResult, fallbackFinalCandidates); fallbackOK {
			logger.Warningf("[Mihomo] route.final %q is invalid; fallback to %q", rawFinal, fallbackFinal)
			globalFinal = fallbackFinal
		} else {
			return nil, fmt.Errorf("invalid mihomo route.final target: %s", rawFinal)
		}
	}

	var inbounds []model.MihomoInbound
	if err := db.Model(model.MihomoInbound{}).Preload("Tls").Find(&inbounds).Error; err != nil {
		return nil, fmt.Errorf("load mihomo inbounds failed: %w", err)
	}
	inbounds = filterSupportedMihomoListeners(inbounds)

	inboundAlias := buildMihomoInboundAliasMap(inbounds)
	inboundRefs := make(map[string]mihomoInboundRouteRef, len(inbounds))
	listeners := make([]interface{}, 0, len(inbounds))

	for _, inbound := range inbounds {
		rawJSON, err := inbound.MarshalJSON()
		if err != nil {
			return nil, fmt.Errorf("marshal mihomo inbound %s failed: %w", inbound.Tag, err)
		}
		rawJSON, err = s.MihomoInboundService.addUsers(db, rawJSON, inbound.Id, inbound.Type)
		if err != nil {
			return nil, fmt.Errorf("append mihomo inbound users for %s failed: %w", inbound.Tag, err)
		}
		if strings.EqualFold(inbound.Type, "snell") {
			rawJSON, err = s.MihomoInboundService.processSnellInbound(db, rawJSON, &inbound)
			if err != nil {
				return nil, fmt.Errorf("process mihomo snell inbound %s failed: %w", inbound.Tag, err)
			}
		}

		payload, err := marshalJSONMap(rawJSON)
		if err != nil {
			return nil, fmt.Errorf("decode mihomo inbound %s failed: %w", inbound.Tag, err)
		}

		ref, err := buildMihomoInboundRouteRef(inbound, proxyResult, globalFinal)
		if err != nil {
			return nil, err
		}
		if ref.RuleName != "" {
			inboundRefs[ref.RuleName] = ref
		}

		listener := buildMihomoListener(inbound, payload, ref)
		if len(listener) == 0 {
			continue
		}
		listeners = append(listeners, listener)
	}

	if len(listeners) > 0 {
		document["listeners"] = listeners
	}

	routeResult := renderMihomoRoutes(route, providerTags, ipRuleProviderTags, proxyResult, globalFinal, inboundRefs, inboundAlias)
	if len(routeResult.ValidationErrs) > 0 {
		return nil, fmt.Errorf("invalid mihomo route config: %s", strings.Join(routeResult.ValidationErrs, "; "))
	}
	routeResult.SubRules = pruneRedundantMihomoListenerRules(listeners, routeResult)
	if len(routeResult.Rules) > 0 {
		rules := make([]interface{}, 0, len(routeResult.Rules))
		for _, rule := range routeResult.Rules {
			rules = append(rules, rule)
		}
		document["rules"] = rules
	}
	if len(routeResult.SubRules) > 0 {
		document["sub-rules"] = routeResult.SubRules
	}

	if sniffer := normalizeMihomoSniffer(base["sniffer"]); sniffer != nil {
		document["sniffer"] = sniffer
	}

	return normalizeMihomoDocument(document), nil
}

func (s *MihomoManagerService) RegenerateServerConfig() error {
	if err := EnsureManagedCoreLayout(); err != nil {
		return err
	}

	document, err := s.GenerateServerDocument()
	if err != nil {
		return err
	}

	rawByTag, err := loadMihomoRawClashYAMLByTag(database.GetDB())
	if err != nil {
		return fmt.Errorf("load mihomo raw clash yaml failed: %w", err)
	}

	yamlData, err := renderMihomoDocumentYAML(document, rawByTag)
	if err != nil {
		return fmt.Errorf("marshal mihomo yaml failed: %w", err)
	}

	coreDir := GetManagedCoreRootDir()
	filePath := GetMihomoConfigPath()
	if err := ManagedRuntimeWriteFile(filePath, yamlData); err != nil {
		return fmt.Errorf("write mihomo config failed: %w", err)
	}
	if err := writeMihomoInboundMetaFile(coreDir); err != nil {
		return err
	}

	logger.Infof("[Mihomo] wrote server config: %s", filePath)
	return nil
}

func collectMihomoRouteFinalFallbackCandidates(rawOutbounds []map[string]interface{}, targets *mihomoProxyConversionResult) []string {
	if len(rawOutbounds) == 0 || targets == nil {
		return nil
	}

	candidates := make([]string, 0, len(rawOutbounds))
	seen := map[string]struct{}{}
	for _, outbound := range rawOutbounds {
		tag := strings.TrimSpace(firstString(outbound["tag"]))
		if tag == "" {
			continue
		}
		normalized, ok := normalizeMihomoRouteTarget(tag, targets)
		if !ok {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		candidates = append(candidates, normalized)
	}

	return candidates
}

func pruneRedundantMihomoListenerRules(listeners []interface{}, routeResult *mihomoRouteRenderResult) map[string][]string {
	if routeResult == nil || len(routeResult.SubRules) == 0 {
		return nil
	}

	remaining := make(map[string][]string, len(routeResult.SubRules))
	for ruleName, rules := range routeResult.SubRules {
		remaining[ruleName] = rules
	}

	for _, rawListener := range listeners {
		listener, ok := rawListener.(map[string]interface{})
		if !ok || listener == nil {
			continue
		}

		ruleName := strings.TrimSpace(firstString(listener["rule"]))
		if ruleName == "" {
			continue
		}

		subRules, exists := remaining[ruleName]
		if !exists {
			delete(listener, "rule")
			continue
		}

		if mihomoRuleListsEqual(subRules, routeResult.Rules) {
			delete(listener, "rule")
			delete(remaining, ruleName)
		}
	}

	if len(remaining) == 0 {
		return nil
	}
	return remaining
}

func mihomoRuleListsEqual(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if a[index] != b[index] {
			return false
		}
	}
	return true
}
