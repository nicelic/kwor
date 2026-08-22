package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"gorm.io/gorm"
)

type MihomoManagerService struct {
	MihomoConfigService
	MihomoInboundService
	MihomoOutboundService
}

var mihomoServerConfigRegenerationMu sync.Mutex

func NewMihomoManagerService() *MihomoManagerService {
	return &MihomoManagerService{}
}

func (s *MihomoManagerService) GenerateServerDocument() (map[string]interface{}, error) {
	return s.generateServerDocument(database.GetDB())
}

func (s *MihomoManagerService) generateServerDocument(db *gorm.DB) (map[string]interface{}, error) {
	if db == nil {
		return nil, fmt.Errorf("mihomo config generation requires a database connection")
	}

	baseData, err := s.MihomoConfigService.GetConfigWithDB(db)
	if err != nil {
		return nil, fmt.Errorf("get mihomo base config failed: %w", err)
	}

	base := map[string]interface{}{}
	if strings.TrimSpace(baseData) != "" {
		if err := json.Unmarshal([]byte(baseData), &base); err != nil {
			return nil, fmt.Errorf("parse mihomo base config failed: %w", err)
		}
		// Legacy records can predate route-save validation. Refuse to replace the
		// active YAML with one that silently drops an inbound-scoped rule.
		if err := validateMihomoConfigInboundReferences(db, json.RawMessage(baseData)); err != nil {
			return nil, fmt.Errorf("invalid mihomo route inbound reference: %w", err)
		}
	}

	document := copyMihomoGeneralConfig(base)
	if err := applyCurrentMihomoCoreLogLevel(db, document); err != nil {
		return nil, fmt.Errorf("get mihomo core log level failed: %w", err)
	}
	route := requireRouteMap(base)
	applyMihomoRouteGeneralConfig(document, route)
	if dns := buildMihomoDNSDocument(base["dns"]); len(dns) > 0 {
		document["dns"] = dns
	}

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
		if inbound.TlsId > 0 {
			if inbound.Tls == nil {
				return nil, fmt.Errorf("mihomo inbound %s references missing TLS configuration %d", inbound.Tag, inbound.TlsId)
			}
			if err := validateMihomoTLSMode(inbound.Tls); err != nil {
				return nil, fmt.Errorf("mihomo inbound %s references invalid TLS configuration: %w", inbound.Tag, err)
			}
			if err := validateMihomoInboundTLSMode(&inbound); err != nil {
				return nil, fmt.Errorf("mihomo inbound %s has invalid TLS binding: %w", inbound.Tag, err)
			}
			if err := normalizeMihomoTLSOutboundReferences(inbound.Tls, proxyResult); err != nil {
				return nil, fmt.Errorf("mihomo inbound %s has invalid TLS outbound reference: %w", inbound.Tag, err)
			}
		}
		if strings.EqualFold(strings.TrimSpace(inbound.Type), "shadowquic") {
			if err := repairMihomoShadowQUICInboundClientCredentials(db, inbound.Id); err != nil {
				return nil, fmt.Errorf("repair mihomo shadowquic client credentials for %s failed: %w", inbound.Tag, err)
			}
		}
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
	if err := validateMihomoRenderedRouteSize(routeResult); err != nil {
		return nil, err
	}
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

func (s *MihomoManagerService) renderServerConfig(db *gorm.DB) ([]byte, error) {
	document, err := s.generateServerDocument(db)
	if err != nil {
		return nil, err
	}

	rawByTag, err := loadMihomoRawClashYAMLByTag(db)
	if err != nil {
		return nil, fmt.Errorf("load mihomo raw clash yaml failed: %w", err)
	}

	yamlData, err := renderMihomoDocumentYAML(document, rawByTag)
	if err != nil {
		return nil, fmt.Errorf("marshal mihomo yaml failed: %w", err)
	}
	if len(yamlData) > maxMihomoGeneratedYAMLBytes {
		return nil, fmt.Errorf("generated mihomo server.yaml exceeds the %d byte safety limit", maxMihomoGeneratedYAMLBytes)
	}
	return yamlData, nil
}

// ValidateServerConfig performs the same parse, conversion and size checks as
// the runtime writer without touching the active server.yaml file.
func (s *MihomoManagerService) ValidateServerConfig(db *gorm.DB) error {
	_, err := s.renderServerConfig(db)
	return err
}

func (s *MihomoManagerService) RegenerateServerConfig() error {
	mihomoServerConfigRegenerationMu.Lock()
	defer mihomoServerConfigRegenerationMu.Unlock()

	yamlData, err := s.renderServerConfig(database.GetDB())
	if err != nil {
		return err
	}
	return s.writeRenderedServerConfig(yamlData)
}

func (s *MihomoManagerService) writeRenderedServerConfig(yamlData []byte) error {
	if len(yamlData) > maxMihomoGeneratedYAMLBytes {
		return fmt.Errorf("generated mihomo server.yaml exceeds the %d byte safety limit", maxMihomoGeneratedYAMLBytes)
	}
	if err := EnsureManagedCoreLayout(); err != nil {
		return err
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

func validateMihomoRenderedRouteSize(result *mihomoRouteRenderResult) error {
	if result == nil {
		return nil
	}
	count := len(result.Rules)
	for _, rules := range result.SubRules {
		count += len(rules)
	}
	if count > maxMihomoRouteRenderedRules {
		return fmt.Errorf("mihomo route generation exceeds the %d generated-rule safety limit", maxMihomoRouteRenderedRules)
	}
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
