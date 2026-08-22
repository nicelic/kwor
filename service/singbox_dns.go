package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/util/common"
	"gorm.io/gorm"
)

const (
	SingboxDNSMaxConfigBytes          = 64 * 1024
	SingboxDNSMaxServers              = 32
	SingboxDNSMaxServerOptionsBytes   = 24 * 1024
	SingboxDNSMaxServerOptionsTotal   = 96 * 1024
	SingboxDNSMaxRules                = 128
	SingboxDNSMaxRuleBytes            = 8 * 1024
	SingboxDNSMaxRulesBytes           = 48 * 1024
	SingboxDNSMaxRuleDepth            = 4
	SingboxDNSMaxLogicalRuleChildren  = 16
	SingboxDNSMaxHostsPredefined      = 128
	SingboxDNSMaxHeaders              = 16
	SingboxDNSMaxHeaderNameBytes      = 128
	SingboxDNSMaxHeaderValueBytes     = 1024
	SingboxDNSMaxCacheCapacity        = 262144
	singboxDNSMaxTagBytes             = 128
	singboxDNSMaxTypeBytes            = 32
	singboxDNSMaxGenericStringBytes   = 4096
	singboxDNSMaxGenericObjectEntries = 128
	singboxDNSMaxGenericArrayItems    = 128
	singboxDNSMaxGenericValueDepth    = 16
)

var supportedSingboxDNSServerTypes = map[string]struct{}{
	"local": {}, "hosts": {}, "tcp": {}, "udp": {}, "tls": {}, "quic": {},
	"https": {}, "h3": {}, "dhcp": {}, "fakeip": {}, "tailscale": {}, "resolved": {},
}

type SingboxDNSRevisionConflictError struct {
	CurrentRevision uint64
}

func (e *SingboxDNSRevisionConflictError) Error() string {
	return "sing-box DNS revision conflict"
}

func IsSingboxDNSRevisionConflict(err error) bool {
	var target *SingboxDNSRevisionConflictError
	return errors.As(err, &target)
}

func SingboxDNSRevisionFromConflict(err error) uint64 {
	var target *SingboxDNSRevisionConflictError
	if errors.As(err, &target) && target != nil {
		return target.CurrentRevision
	}
	return 0
}

type SingboxDNSSnapshot struct {
	Revision      uint64           `json:"revision"`
	DNS           json.RawMessage  `json:"dns"`
	Servers       []map[string]any `json:"servers"`
	DialTags      []string         `json:"dialTags"`
	TailscaleTags []string         `json:"tailscaleTags"`
	ResolvedTags  []string         `json:"resolvedTags"`
	ClientNames   []string         `json:"clientNames"`
	InboundTags   []string         `json:"inboundTags"`
	RuleSetTags   []string         `json:"ruleSetTags"`
}

type SingboxDNSMutationRequest struct {
	ExpectedRevision uint64          `json:"expectedRevision"`
	DNS              json.RawMessage `json:"dns,omitempty"`
	ServerAction     string          `json:"serverAction,omitempty"`
	Server           json.RawMessage `json:"server,omitempty"`
	ServerID         uint            `json:"serverId,omitempty"`
	RetryRuntime     bool            `json:"retryRuntime,omitempty"`
}

type SingboxDNSMutationResult struct {
	Snapshot *SingboxDNSSnapshot `json:"snapshot"`
	Changed  bool                `json:"changed"`
}

var regenerateSingboxDNSRuntimeConfig = func(configService *ConfigService) error {
	return GetProManagerService(configService).RegenerateCoreConfig()
}

func ensureSingboxConfigRevisionState(tx *gorm.DB) (uint64, error) {
	if tx == nil {
		return 0, common.NewError("database transaction is not ready")
	}
	state := model.SingboxConfigState{Id: 1, Revision: 1}
	err := tx.Where("id = ?", state.Id).First(&state).Error
	if database.IsNotFound(err) {
		if err := tx.Create(&state).Error; err != nil {
			return 0, err
		}
		return state.Revision, nil
	}
	if err != nil {
		return 0, err
	}
	return state.Revision, nil
}

func bumpSingboxConfigRevision(tx *gorm.DB, currentRevision uint64) (uint64, error) {
	if currentRevision == 0 {
		var err error
		currentRevision, err = ensureSingboxConfigRevisionState(tx)
		if err != nil {
			return 0, err
		}
	}
	update := tx.Model(&model.SingboxConfigState{}).
		Where("id = ? AND revision = ?", 1, currentRevision).
		Update("revision", gorm.Expr("revision + ?", 1))
	if update.Error != nil {
		return 0, update.Error
	}
	if update.RowsAffected != 1 {
		latest, err := ensureSingboxConfigRevisionState(tx)
		if err != nil {
			return 0, err
		}
		return 0, &SingboxDNSRevisionConflictError{CurrentRevision: latest}
	}
	return currentRevision + 1, nil
}

func isSingboxConfigMutationObject(obj string) bool {
	switch obj {
	case "config", "dnsservers", "inbounds", "outbounds", "outboundgroups", "clients", "tls", "services", "endpoints":
		return true
	default:
		return false
	}
}

func (s *ConfigService) GetSingboxDNSSnapshot() (*SingboxDNSSnapshot, error) {
	db := database.GetDB()
	if db == nil {
		return nil, common.NewError("database is not ready")
	}

	snapshot := &SingboxDNSSnapshot{}
	err := db.Transaction(func(tx *gorm.DB) error {
		revision, err := ensureSingboxConfigRevisionState(tx)
		if err != nil {
			return err
		}
		dns, err := loadSingboxDNSSection(tx)
		if err != nil {
			return err
		}
		servers, err := loadSingboxDNSServers(tx)
		if err != nil {
			return err
		}
		dialTags, err := loadSingboxRouteOutboundTags(tx)
		if err != nil {
			return err
		}
		tailscaleTags, err := loadSingboxDNSEndpointTags(tx, "tailscale", false)
		if err != nil {
			return err
		}
		resolvedTags, err := loadSingboxDNSServiceTags(tx, "resolved")
		if err != nil {
			return err
		}
		clientNames, err := loadSingboxDNSClientNames(tx)
		if err != nil {
			return err
		}
		inboundTags, err := loadSingboxDNSInboundTags(tx)
		if err != nil {
			return err
		}
		ruleSetTags, err := loadSingboxDNSRuleSetTags(tx)
		if err != nil {
			return err
		}

		snapshot.Revision = revision
		snapshot.DNS = dns
		snapshot.Servers = servers
		snapshot.DialTags = dialTags
		snapshot.TailscaleTags = tailscaleTags
		snapshot.ResolvedTags = resolvedTags
		snapshot.ClientNames = clientNames
		snapshot.InboundTags = inboundTags
		snapshot.RuleSetTags = ruleSetTags
		return nil
	})
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

func (s *ConfigService) SaveSingboxDNS(request SingboxDNSMutationRequest, actor string) (*SingboxDNSMutationResult, error) {
	retryOnly := request.RetryRuntime && len(request.DNS) == 0 && strings.TrimSpace(request.ServerAction) == ""
	if request.ExpectedRevision == 0 && !retryOnly {
		return nil, common.NewError("expectedRevision is required")
	}
	if len(request.DNS) > 0 && strings.TrimSpace(request.ServerAction) != "" {
		return nil, common.NewError("DNS configuration and DNS server mutations must be submitted separately")
	}
	if len(request.DNS) == 0 && strings.TrimSpace(request.ServerAction) == "" && !request.RetryRuntime {
		return nil, common.NewError("DNS configuration or DNS server mutation is required")
	}

	db := database.GetDB()
	if db == nil {
		return nil, common.NewError("database is not ready")
	}

	result := &SingboxDNSMutationResult{}
	err := db.Transaction(func(tx *gorm.DB) error {
		currentRevision, err := ensureSingboxConfigRevisionState(tx)
		if err != nil {
			return err
		}
		if !retryOnly && currentRevision != request.ExpectedRevision {
			return &SingboxDNSRevisionConflictError{CurrentRevision: currentRevision}
		}

		if request.RetryRuntime && len(request.DNS) == 0 && strings.TrimSpace(request.ServerAction) == "" {
			return nil
		}
		if len(request.DNS) > 0 {
			changed, err := saveSingboxDNSConfigSection(tx, request.DNS)
			if err != nil {
				return err
			}
			result.Changed = changed
			if changed {
				config, err := loadSingboxConfigForDNS(tx)
				if err != nil {
					return err
				}
				if err := validateSingboxConfiguredInboundReferences(tx, config); err != nil {
					return err
				}
			}
		} else {
			changed, err := (&DnsServerService{}).SaveWithChange(tx, request.ServerAction, request.Server, request.ServerID)
			if err != nil {
				return err
			}
			result.Changed = changed
		}

		if !result.Changed {
			return nil
		}
		_, err = bumpSingboxConfigRevision(tx, currentRevision)
		if err != nil {
			return err
		}
		return recordChange(tx, model.Changes{
			DateTime: time.Now().Unix(),
			Actor:    actor,
			Key:      "singbox_dns",
			Action:   singboxDNSAuditAction(request),
			Obj:      buildSingboxDNSAudit(request),
		})
	})
	if err != nil {
		return nil, err
	}

	snapshot, snapshotErr := s.GetSingboxDNSSnapshot()
	if snapshotErr != nil {
		return result, snapshotErr
	}
	result.Snapshot = snapshot

	if result.Changed {
		markLastUpdate(time.Now().Unix())
	}
	if result.Changed || request.RetryRuntime {
		if err := regenerateSingboxDNSRuntimeConfig(s); err != nil {
			runtimeErr := fmt.Errorf("regenerate sing-box config after DNS save failed: %w", err)
			logger.Warning(runtimeErr)
			return result, &CommittedSaveError{Err: runtimeErr}
		}
	}
	return result, nil
}

func loadSingboxDNSSection(tx *gorm.DB) (json.RawMessage, error) {
	config, err := loadSingboxConfigForDNS(tx)
	if err != nil {
		return nil, err
	}
	root := map[string]any{}
	if err := json.Unmarshal(config, &root); err != nil {
		return nil, err
	}
	dns, ok := root["dns"].(map[string]any)
	if !ok || dns == nil {
		dns = map[string]any{}
	}
	delete(dns, "servers")
	delete(dns, "independent_cache")
	if _, ok := dns["rules"].([]any); !ok {
		dns["rules"] = []any{}
	}
	return json.Marshal(dns)
}

func loadSingboxConfigForDNS(tx *gorm.DB) (json.RawMessage, error) {
	if tx == nil {
		return nil, common.NewError("database transaction is not ready")
	}
	setting := model.Setting{}
	err := tx.Where("key = ?", "config").First(&setting).Error
	if err == nil {
		return json.RawMessage(setting.Value), nil
	}
	if !database.IsNotFound(err) {
		return nil, err
	}
	value, err := (&SettingService{}).defaultSettingValue("config")
	if err != nil {
		return nil, err
	}
	return json.RawMessage(value), nil
}

func loadSingboxDNSServers(tx *gorm.DB) ([]map[string]any, error) {
	servers := make([]model.DnsServer, 0)
	if err := tx.Order("id ASC").Find(&servers).Error; err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(servers))
	for _, server := range servers {
		full, err := server.MarshalFull()
		if err != nil {
			return nil, err
		}
		result = append(result, full)
	}
	return result, nil
}

func loadSingboxDNSClientNames(tx *gorm.DB) ([]string, error) {
	values := make([]string, 0)
	if err := tx.Model(&model.Client{}).Order("id ASC").Pluck("name", &values).Error; err != nil {
		return nil, err
	}
	return compactUniqueStrings(values), nil
}

func loadSingboxDNSServiceTags(tx *gorm.DB, serviceType string) ([]string, error) {
	values := make([]string, 0)
	if err := tx.Model(&model.Service{}).Where("type = ?", serviceType).Order("id ASC").Pluck("tag", &values).Error; err != nil {
		return nil, err
	}
	return compactUniqueStrings(values), nil
}

func loadSingboxDNSEndpointTags(tx *gorm.DB, endpointType string, onlyListening bool) ([]string, error) {
	endpoints := make([]model.Endpoint, 0)
	if err := tx.Select("tag", "options").Where("type = ?", endpointType).Order("id ASC").Find(&endpoints).Error; err != nil {
		return nil, err
	}
	values := make([]string, 0, len(endpoints))
	for _, endpoint := range endpoints {
		if onlyListening {
			options := map[string]any{}
			if len(endpoint.Options) == 0 || json.Unmarshal(endpoint.Options, &options) != nil {
				continue
			}
			port, ok := asPositiveInteger(options["listen_port"])
			if !ok || port == 0 {
				continue
			}
		}
		values = append(values, endpoint.Tag)
	}
	return compactUniqueStrings(values), nil
}

func loadSingboxDNSInboundTags(tx *gorm.DB) ([]string, error) {
	inbounds := make([]model.Inbound, 0)
	if err := tx.Select("type", "tag", "options").Order("id ASC").Find(&inbounds).Error; err != nil {
		return nil, err
	}
	values := make([]string, 0, len(inbounds))
	for _, inbound := range inbounds {
		values = append(values, deriveEffectiveInboundRouteTagFromRaw(inbound.Tag, inbound.Type, inbound.Options))
	}
	endpoints := make([]model.Endpoint, 0)
	if err := tx.Select("tag", "options").Order("id ASC").Find(&endpoints).Error; err != nil {
		return nil, err
	}
	for _, endpoint := range endpoints {
		options := map[string]any{}
		if len(endpoint.Options) == 0 || json.Unmarshal(endpoint.Options, &options) != nil {
			continue
		}
		port, ok := asPositiveInteger(options["listen_port"])
		if !ok || port == 0 {
			continue
		}
		values = append(values, deriveEffectiveEndpointRouteTagFromRaw(endpoint.Tag, endpoint.Options))
	}
	return compactUniqueStrings(values), nil
}

func compactUniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func loadSingboxDNSRuleSetTags(tx *gorm.DB) ([]string, error) {
	config, err := loadSingboxConfigForDNS(tx)
	if err != nil {
		return nil, err
	}
	root := map[string]any{}
	if err := json.Unmarshal(config, &root); err != nil {
		return nil, err
	}
	route, _ := root["route"].(map[string]any)
	if route == nil {
		return []string{}, nil
	}
	rawRuleSets, _ := route["rule_set"].([]any)
	values := make([]string, 0, len(rawRuleSets))
	for _, rawRuleSet := range rawRuleSets {
		ruleSet, _ := rawRuleSet.(map[string]any)
		tag, _ := ruleSet["tag"].(string)
		values = append(values, tag)
	}
	return compactUniqueStrings(values), nil
}

func saveSingboxDNSConfigSection(tx *gorm.DB, rawDNS json.RawMessage) (bool, error) {
	if len(rawDNS) > SingboxDNSMaxConfigBytes {
		return false, common.NewErrorf("DNS configuration exceeds %d bytes", SingboxDNSMaxConfigBytes)
	}
	if !utf8.Valid(rawDNS) {
		return false, common.NewError("DNS configuration is not valid UTF-8")
	}

	dns := map[string]any{}
	if err := json.Unmarshal(rawDNS, &dns); err != nil {
		return false, common.NewErrorf("DNS configuration is invalid: %v", err)
	}
	if dns == nil {
		return false, common.NewError("DNS configuration must be an object")
	}
	if _, exists := dns["servers"]; exists {
		return false, common.NewError("DNS servers must be saved through DNS server cards")
	}
	if err := validateSingboxDNSConfigMap(dns); err != nil {
		return false, err
	}

	current, err := loadSingboxConfigForDNS(tx)
	if err != nil {
		return false, err
	}
	root := map[string]any{}
	if err := json.Unmarshal(current, &root); err != nil {
		return false, err
	}
	root["dns"] = dns
	updated, err := json.Marshal(root)
	if err != nil {
		return false, err
	}
	updated, err = sanitizeAndValidateSingboxConfigJSON(updated)
	if err != nil {
		return false, err
	}
	updated, err = (&DnsServerService{}).NormalizeConfigForStorage(tx, updated)
	if err != nil {
		return false, err
	}

	currentCanonical, err := canonicalSingboxConfigForDNSCompare(tx, current)
	if err != nil {
		return false, err
	}
	updatedCanonical, err := canonicalSingboxConfigForDNSCompare(tx, updated)
	if err != nil {
		return false, err
	}
	if bytes.Equal(currentCanonical, updatedCanonical) {
		return false, nil
	}
	if err := (&SettingService{}).SaveConfig(tx, updated); err != nil {
		return false, err
	}
	return true, nil
}

func canonicalSingboxConfigForDNSCompare(tx *gorm.DB, raw json.RawMessage) ([]byte, error) {
	normalized, err := sanitizeAndValidateSingboxConfigJSON(raw)
	if err != nil {
		return nil, err
	}
	normalized, err = (&DnsServerService{}).NormalizeConfigForStorage(tx, normalized)
	if err != nil {
		return nil, err
	}
	root := map[string]any{}
	if err := json.Unmarshal(normalized, &root); err != nil {
		return nil, err
	}
	return json.Marshal(root)
}

func validateSingboxDNSConfigMap(dns map[string]any) error {
	delete(dns, "independent_cache")
	if _, exists := dns["servers"]; exists {
		return common.NewError("DNS servers must be saved through DNS server cards")
	}
	if err := validateSingboxDNSValue(dns, 0); err != nil {
		return err
	}
	if capacity, exists := dns["cache_capacity"]; exists {
		value, ok := asPositiveInteger(capacity)
		if !ok || value < 1024 || value > SingboxDNSMaxCacheCapacity {
			return common.NewErrorf("cache_capacity must be between 1024 and %d", SingboxDNSMaxCacheCapacity)
		}
		dns["cache_capacity"] = value
	}
	rules, exists := dns["rules"]
	if !exists {
		dns["rules"] = []any{}
		rules = dns["rules"]
	}
	ruleList, ok := rules.([]any)
	if !ok {
		return common.NewError("dns.rules must be an array")
	}
	if len(ruleList) > SingboxDNSMaxRules {
		return common.NewErrorf("DNS rule count exceeds %d", SingboxDNSMaxRules)
	}
	totalBytes := 0
	for _, rule := range ruleList {
		bytes, err := json.Marshal(rule)
		if err != nil {
			return err
		}
		if len(bytes) > SingboxDNSMaxRuleBytes {
			return common.NewErrorf("a DNS rule exceeds %d bytes", SingboxDNSMaxRuleBytes)
		}
		totalBytes += len(bytes)
		if totalBytes > SingboxDNSMaxRulesBytes {
			return common.NewErrorf("DNS rules exceed %d bytes", SingboxDNSMaxRulesBytes)
		}
		if err := validateSingboxDNSRule(rule, 1); err != nil {
			return err
		}
	}
	if err := validateSingboxDNSRuleServerReferences(ruleList, dns); err != nil {
		return err
	}

	raw, err := json.Marshal(dns)
	if err != nil {
		return err
	}
	if len(raw) > SingboxDNSMaxConfigBytes {
		return common.NewErrorf("DNS configuration exceeds %d bytes", SingboxDNSMaxConfigBytes)
	}
	return nil
}

func validateSingboxDNSRuleServerReferences(rules []any, dns map[string]any) error {
	servers := map[string]struct{}{}
	if raw, ok := dns["servers"].([]any); ok {
		for _, item := range raw {
			if server, ok := item.(map[string]any); ok {
				if tag, ok := server["tag"].(string); ok && strings.TrimSpace(tag) != "" {
					servers[strings.TrimSpace(tag)] = struct{}{}
				}
			}
		}
	}
	// Focused DNS saves intentionally omit embedded servers; in that case the
	// reusable card table is checked by NormalizeConfigForStorage. This helper
	// only rejects an explicit route action that names an empty server tag.
	var walk func([]any) error
	walk = func(items []any) error {
		for _, raw := range items {
			rule, ok := raw.(map[string]any)
			if !ok || rule == nil {
				continue
			}
			if action, _ := rule["action"].(string); action == "route" {
				if server, exists := rule["server"]; exists {
					text, ok := server.(string)
					if !ok || strings.TrimSpace(text) == "" {
						return common.NewError("DNS route action requires a server tag")
					}
					if len(servers) > 0 {
						if _, exists := servers[strings.TrimSpace(text)]; !exists {
							return common.NewErrorf("DNS route action references unknown server %q", strings.TrimSpace(text))
						}
					}
				}
			}
			if children, ok := rule["rules"].([]any); ok {
				if err := walk(children); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return walk(rules)
}

// validateSingboxDNSConfigJSON is used by every sing-box config save, not
// only by the focused DNS page. Legacy embedded servers remain accepted here
// so startup and old full-config imports can migrate them into dns_servers;
// their persisted rows are still checked by importLegacyServers.
func validateSingboxDNSConfigJSON(config json.RawMessage) error {
	if len(config) == 0 {
		return nil
	}
	root := map[string]any{}
	if err := json.Unmarshal(config, &root); err != nil {
		return err
	}
	dns, ok := root["dns"].(map[string]any)
	if !ok || dns == nil {
		return nil
	}
	legacyServers, hasLegacyServers := dns["servers"]
	delete(dns, "servers")
	if err := validateSingboxDNSConfigMap(dns); err != nil {
		return err
	}
	if !hasLegacyServers {
		return nil
	}
	servers, ok := legacyServers.([]any)
	if !ok {
		return common.NewError("legacy DNS servers must be an array")
	}
	if len(servers) > SingboxDNSMaxServers {
		return common.NewErrorf("DNS server count exceeds %d", SingboxDNSMaxServers)
	}
	totalBytes := 0
	for _, rawServer := range servers {
		payload, err := json.Marshal(rawServer)
		if err != nil {
			return err
		}
		server := model.DnsServer{}
		if err := server.UnmarshalJSON(payload); err != nil {
			return err
		}
		if err := validateSingboxDNSServerPayload(&server); err != nil {
			return err
		}
		totalBytes += len(server.Options)
		if totalBytes > SingboxDNSMaxServerOptionsTotal {
			return common.NewErrorf("total DNS server options exceed %d bytes", SingboxDNSMaxServerOptionsTotal)
		}
	}
	return nil
}

func validateSingboxDNSRule(raw any, depth int) error {
	rule, ok := raw.(map[string]any)
	if !ok || rule == nil {
		return common.NewError("each DNS rule must be an object")
	}
	if depth > SingboxDNSMaxRuleDepth {
		return common.NewErrorf("DNS logical rule nesting exceeds %d levels", SingboxDNSMaxRuleDepth)
	}
	if err := validateSingboxDNSRuleFields(rule); err != nil {
		return err
	}
	if err := validateSingboxDNSValue(rule, 0); err != nil {
		return err
	}
	children, exists := rule["rules"]
	if !exists {
		return nil
	}
	list, ok := children.([]any)
	if !ok {
		return common.NewError("DNS logical rule children must be an array")
	}
	if len(list) == 0 || len(list) > SingboxDNSMaxLogicalRuleChildren {
		return common.NewErrorf("DNS logical rule must contain 1 to %d child rules", SingboxDNSMaxLogicalRuleChildren)
	}
	for _, child := range list {
		if err := validateSingboxDNSRule(child, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func validateSingboxDNSRuleFields(rule map[string]any) error {
	if rawType, exists := rule["type"]; exists && rawType != nil {
		typeValue, ok := rawType.(string)
		if !ok || strings.TrimSpace(typeValue) != "logical" {
			return common.NewError("DNS rule type must be logical when present")
		}
		mode, ok := rule["mode"].(string)
		if !ok || (mode != "and" && mode != "or") {
			return common.NewError("DNS logical rule mode must be and or or")
		}
	}
	if rawAction, exists := rule["action"]; exists && rawAction != nil {
		action, ok := rawAction.(string)
		_, supported := map[string]struct{}{"route": {}, "route-options": {}, "reject": {}, "predefined": {}}[action]
		if !ok || !supported {
			return common.NewError("DNS rule action is unsupported")
		}
	}
	if rawServer, exists := rule["server"]; exists && rawServer != nil {
		if _, ok := rawServer.(string); !ok {
			return common.NewError("DNS rule server must be a string")
		}
	}
	if rawStrategy, exists := rule["strategy"]; exists && rawStrategy != nil {
		if _, ok := rawStrategy.(string); !ok {
			return common.NewError("DNS rule strategy must be a string")
		}
	}
	if rawVersion, exists := rule["ip_version"]; exists && rawVersion != nil {
		version, ok := strictSingboxDNSInteger(rawVersion)
		if !ok || (version != 4 && version != 6) {
			return common.NewError("DNS rule ip_version must be 4 or 6")
		}
	}
	for _, field := range []string{"port", "source_port"} {
		if err := validateSingboxDNSIntegerList(rule[field], field, 1, 65535); err != nil {
			return err
		}
	}
	if err := validateSingboxDNSIntegerList(rule["user_id"], "user_id", 0, math.MaxInt64); err != nil {
		return err
	}
	if rawTTL, exists := rule["rewrite_ttl"]; exists && rawTTL != nil {
		value, ok := strictSingboxDNSInteger(rawTTL)
		if !ok || value < 0 {
			return common.NewError("DNS rule rewrite_ttl must be a non-negative integer")
		}
	}
	return nil
}

func strictSingboxDNSInteger(value any) (int64, bool) {
	switch typed := value.(type) {
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) || typed != math.Trunc(typed) || typed < 0 || typed > math.MaxInt64 {
			return 0, false
		}
		return int64(typed), true
	case float32:
		value := float64(typed)
		if math.IsNaN(value) || math.IsInf(value, 0) || value != math.Trunc(value) || value < 0 || value > math.MaxInt64 {
			return 0, false
		}
		return int64(value), true
	case int:
		if typed < 0 {
			return 0, false
		}
		return int64(typed), true
	case int64:
		return typed, typed >= 0
	case uint:
		if uint64(typed) > math.MaxInt64 {
			return 0, false
		}
		return int64(typed), true
	case uint64:
		if typed > math.MaxInt64 {
			return 0, false
		}
		return int64(typed), true
	default:
		return 0, false
	}
}

func validateSingboxDNSIntegerList(value any, field string, min, max int64) error {
	if value == nil {
		return nil
	}
	values := []any{value}
	if list, ok := value.([]any); ok {
		values = list
	}
	for index, item := range values {
		parsed, ok := strictSingboxDNSInteger(item)
		if !ok || parsed < min || parsed > max {
			return common.NewErrorf("DNS rule %s[%d] must be an integer from %d to %d", field, index+1, min, max)
		}
	}
	return nil
}

func validateSingboxDNSValue(value any, depth int) error {
	if depth > singboxDNSMaxGenericValueDepth {
		return common.NewError("DNS data nesting is too deep")
	}
	switch typed := value.(type) {
	case nil, bool, float64, float32,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64:
		return nil
	case string:
		if !utf8.ValidString(typed) || len(typed) > singboxDNSMaxGenericStringBytes {
			return common.NewErrorf("DNS text value exceeds %d bytes or is not valid UTF-8", singboxDNSMaxGenericStringBytes)
		}
		return nil
	case []any:
		if len(typed) > singboxDNSMaxGenericArrayItems {
			return common.NewErrorf("DNS array exceeds %d items", singboxDNSMaxGenericArrayItems)
		}
		for _, item := range typed {
			if err := validateSingboxDNSValue(item, depth+1); err != nil {
				return err
			}
		}
		return nil
	case map[string]any:
		if len(typed) > singboxDNSMaxGenericObjectEntries {
			return common.NewErrorf("DNS object exceeds %d fields", singboxDNSMaxGenericObjectEntries)
		}
		for key, item := range typed {
			if !utf8.ValidString(key) || len(key) > singboxDNSMaxGenericStringBytes {
				return common.NewError("DNS field name is invalid")
			}
			if err := validateSingboxDNSValue(item, depth+1); err != nil {
				return err
			}
		}
		return nil
	default:
		return common.NewError("DNS data contains an unsupported value")
	}
}

func validateSingboxDNSServerPayload(server *model.DnsServer) error {
	if server == nil {
		return common.NewError("DNS server is required")
	}
	server.Tag = strings.TrimSpace(server.Tag)
	server.Type = strings.ToLower(strings.TrimSpace(server.Type))
	if server.Tag == "" || server.Type == "" {
		return common.NewError("DNS server tag and type are required")
	}
	if !utf8.ValidString(server.Tag) || len(server.Tag) > singboxDNSMaxTagBytes {
		return common.NewErrorf("DNS server tag exceeds %d bytes or is not valid UTF-8", singboxDNSMaxTagBytes)
	}
	if server.Tag == singboxRuntimeBootstrapDNSTag {
		return common.NewErrorf("DNS server tag %q is reserved for the runtime bootstrap DNS", singboxRuntimeBootstrapDNSTag)
	}
	if !utf8.ValidString(server.Type) || len(server.Type) > singboxDNSMaxTypeBytes {
		return common.NewErrorf("DNS server type exceeds %d bytes or is not valid UTF-8", singboxDNSMaxTypeBytes)
	}
	if _, supported := supportedSingboxDNSServerTypes[server.Type]; !supported {
		return common.NewErrorf("DNS server type %q is not supported", server.Type)
	}
	if len(server.Options) > SingboxDNSMaxServerOptionsBytes {
		return common.NewErrorf("DNS server options exceed %d bytes", SingboxDNSMaxServerOptionsBytes)
	}
	if !utf8.Valid(server.Options) {
		return common.NewError("DNS server options are not valid UTF-8")
	}
	options := map[string]any{}
	if len(server.Options) > 0 && string(server.Options) != "null" {
		if err := json.Unmarshal(server.Options, &options); err != nil {
			return common.NewErrorf("DNS server options are invalid: %v", err)
		}
	}
	if options == nil {
		options = map[string]any{}
	}
	// Validate the shape and resource bounds of nested options before checking
	// protocol-specific required fields so callers receive the precise field
	// error for malformed payloads (and legacy API error semantics remain stable).
	if err := validateSingboxDNSValue(options, 0); err != nil {
		return err
	}
	if err := validateSingboxDNSHostsPredefined(options["predefined"]); err != nil {
		return err
	}
	if err := validateSingboxDNSHeaders(options["headers"]); err != nil {
		return err
	}
	if server.Type == "tcp" || server.Type == "udp" || server.Type == "tls" || server.Type == "quic" || server.Type == "https" || server.Type == "h3" {
		address, ok := options["server"].(string)
		if !ok || strings.TrimSpace(address) == "" {
			return common.NewErrorf("DNS server type %q requires a server address", server.Type)
		}
		options["server"] = strings.TrimSpace(address)
		if rawPort, exists := options["server_port"]; exists && rawPort != nil {
			port, ok := strictSingboxDNSInteger(rawPort)
			if !ok || port < 1 || port > 65535 {
				return common.NewError("DNS server server_port must be an integer from 1 to 65535")
			}
			options["server_port"] = port
		}
	}
	pathNormalized, err := normalizeSingboxDNSHTTPPath(server.Type, options)
	if err != nil {
		return err
	}
	if pathNormalized {
		normalizedOptions, err := json.MarshalIndent(options, "", "  ")
		if err != nil {
			return err
		}
		if len(normalizedOptions) > SingboxDNSMaxServerOptionsBytes {
			return common.NewErrorf("DNS server options exceed %d bytes", SingboxDNSMaxServerOptionsBytes)
		}
		server.Options = normalizedOptions
	}
	return nil
}

// normalizeSingboxDNSHTTPPath keeps DoH and DoH3 cards in the canonical
// sing-box form. sing-box defaults this option to /dns-query, so an omitted or
// blank value is made explicit before the card is stored and returned to the UI.
func normalizeSingboxDNSHTTPPath(serverType string, options map[string]any) (bool, error) {
	if serverType != "https" && serverType != "h3" {
		return false, nil
	}

	rawPath, exists := options["path"]
	if !exists || rawPath == nil {
		options["path"] = "/dns-query"
		return true, nil
	}
	path, ok := rawPath.(string)
	if !ok {
		return false, common.NewError("DNS HTTPS/H3 path must be a string")
	}
	normalizedPath := strings.TrimSpace(path)
	if normalizedPath == "" {
		normalizedPath = "/dns-query"
	} else if !strings.HasPrefix(normalizedPath, "/") {
		normalizedPath = "/" + normalizedPath
	}
	if normalizedPath == path {
		return false, nil
	}
	options["path"] = normalizedPath
	return true, nil
}

func validateSingboxDNSHostsPredefined(value any) error {
	if value == nil {
		return nil
	}
	predefined, ok := value.(map[string]any)
	if !ok {
		return common.NewError("DNS hosts predefined must be an object")
	}
	if len(predefined) > SingboxDNSMaxHostsPredefined {
		return common.NewErrorf("DNS hosts predefined entries exceed %d", SingboxDNSMaxHostsPredefined)
	}
	for name, addresses := range predefined {
		if !utf8.ValidString(name) || len(name) == 0 || len(name) > 253 {
			return common.NewError("DNS hosts predefined name is invalid")
		}
		if err := validateSingboxDNSValue(addresses, 0); err != nil {
			return err
		}
	}
	return nil
}

func validateSingboxDNSHeaders(value any) error {
	if value == nil {
		return nil
	}
	headers, ok := value.(map[string]any)
	if !ok {
		return common.NewError("DNS headers must be an object")
	}
	if len(headers) > SingboxDNSMaxHeaders {
		return common.NewErrorf("DNS headers exceed %d fields", SingboxDNSMaxHeaders)
	}
	for name, rawValue := range headers {
		if !utf8.ValidString(name) || len(name) == 0 || len(name) > SingboxDNSMaxHeaderNameBytes {
			return common.NewError("DNS header name is invalid")
		}
		switch value := rawValue.(type) {
		case string:
			if !utf8.ValidString(value) || len(value) > SingboxDNSMaxHeaderValueBytes {
				return common.NewErrorf("DNS header value exceeds %d bytes", SingboxDNSMaxHeaderValueBytes)
			}
		case []any:
			if len(value) > SingboxDNSMaxHeaders {
				return common.NewError("DNS header has too many values")
			}
			for _, item := range value {
				text, ok := item.(string)
				if !ok || !utf8.ValidString(text) || len(text) > SingboxDNSMaxHeaderValueBytes {
					return common.NewErrorf("DNS header value exceeds %d bytes or is invalid", SingboxDNSMaxHeaderValueBytes)
				}
			}
		default:
			return common.NewError("DNS header value must be text or a text array")
		}
	}
	return nil
}

func asPositiveInteger(value any) (int, bool) {
	switch typed := value.(type) {
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) || typed != math.Trunc(typed) || typed < 0 || typed > math.MaxInt {
			return 0, false
		}
		return int(typed), true
	case int:
		return typed, typed >= 0
	case int64:
		if typed < 0 || typed > math.MaxInt {
			return 0, false
		}
		return int(typed), true
	default:
		return 0, false
	}
}

func singboxDNSAuditAction(request SingboxDNSMutationRequest) string {
	if len(request.DNS) > 0 {
		return "config"
	}
	return "server-" + strings.TrimSpace(request.ServerAction)
}

func buildSingboxDNSAudit(request SingboxDNSMutationRequest) json.RawMessage {
	audit := map[string]any{}
	if len(request.DNS) > 0 {
		dns := map[string]any{}
		_ = json.Unmarshal(request.DNS, &dns)
		rules, _ := dns["rules"].([]any)
		audit["kind"] = "config"
		audit["ruleCount"] = len(rules)
		if final, ok := dns["final"].(string); ok {
			audit["final"] = final
		}
	} else {
		server := model.DnsServer{}
		_ = server.UnmarshalJSON(request.Server)
		audit["kind"] = "server"
		audit["action"] = request.ServerAction
		audit["id"] = request.ServerID
		if server.Id > 0 {
			audit["id"] = server.Id
		}
		audit["tag"] = strings.TrimSpace(server.Tag)
		audit["type"] = strings.TrimSpace(server.Type)
	}
	data, _ := json.Marshal(audit)
	return data
}
