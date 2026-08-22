package service

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	SingboxBasicsMaxBytes       = 128 * 1024
	singboxBasicsMaxStringBytes = 4096
	singboxBasicsMaxListItems   = 256
	singboxBasicsMaxDuration    = 7 * 24 * time.Hour
)

type SingboxBasicsRevisionConflictError struct {
	CurrentRevision uint64
}

func (e *SingboxBasicsRevisionConflictError) Error() string {
	return "sing-box basics revision conflict"
}

type SingboxBasicsEditorContext struct {
	Revision      uint64          `json:"revision"`
	Basics        json.RawMessage `json:"basics"`
	DialTags      []string        `json:"dialTags"`
	DNSServerTags []string        `json:"dnsServerTags"`
	InboundTags   []string        `json:"inboundTags"`
	ClientNames   []string        `json:"clientNames"`
}

type SingboxBasicsSaveRequest struct {
	ExpectedRevision uint64          `json:"expectedRevision"`
	Basics           json.RawMessage `json:"basics"`
	RetryRuntime     bool            `json:"retryRuntime,omitempty"`
}

type SingboxBasicsSaveResult struct {
	Revision uint64          `json:"revision"`
	Basics   json.RawMessage `json:"basics"`
	Changed  bool            `json:"changed"`
}

var regenerateSingboxBasicsRuntimeConfig = func(configService *ConfigService) error {
	return GetProManagerService(configService).RegenerateCoreConfig()
}

func (s *ConfigService) GetSingboxBasicsEditorContext() (*SingboxBasicsEditorContext, error) {
	db := database.GetDB()
	if db == nil {
		return nil, common.NewError("database is not ready")
	}
	context := &SingboxBasicsEditorContext{}
	err := db.Transaction(func(tx *gorm.DB) error {
		revision, err := ensureSingboxConfigRevisionState(tx)
		if err != nil {
			return err
		}
		config, err := loadSingboxConfigForDNS(tx)
		if err != nil {
			return err
		}
		basics, err := extractSingboxBasics(config)
		if err != nil {
			return err
		}
		dialTags, err := loadSingboxRouteOutboundTags(tx)
		if err != nil {
			return err
		}
		inboundTags, err := loadSingboxDNSInboundTags(tx)
		if err != nil {
			return err
		}
		clientNames, err := loadSingboxDNSClientNames(tx)
		if err != nil {
			return err
		}
		dnsServerTags, err := loadSingboxEffectiveDNSServerTags(tx)
		if err != nil {
			return err
		}

		context.Revision = revision
		context.Basics = basics
		context.DialTags = compactUniqueStrings(dialTags)
		context.DNSServerTags = compactUniqueStrings(dnsServerTags)
		context.InboundTags = compactUniqueStrings(inboundTags)
		context.ClientNames = compactUniqueStrings(clientNames)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return context, nil
}

func (s *ConfigService) SaveSingboxBasics(request SingboxBasicsSaveRequest, actor string) (*SingboxBasicsSaveResult, error) {
	retryOnly := request.RetryRuntime && len(request.Basics) == 0
	if request.ExpectedRevision == 0 && !retryOnly {
		return nil, common.NewError("expectedRevision is required")
	}
	if len(request.Basics) == 0 && !request.RetryRuntime {
		return nil, common.NewError("basics is required")
	}
	db := database.GetDB()
	if db == nil {
		return nil, common.NewError("database is not ready")
	}
	result := &SingboxBasicsSaveResult{}
	err := db.Transaction(func(tx *gorm.DB) error {
		currentRevision, err := ensureSingboxConfigRevisionState(tx)
		if err != nil {
			return err
		}
		if !retryOnly && currentRevision != request.ExpectedRevision {
			return &SingboxBasicsRevisionConflictError{CurrentRevision: currentRevision}
		}

		currentConfig, err := loadSingboxConfigForDNS(tx)
		if err != nil {
			return err
		}
		if request.RetryRuntime && len(request.Basics) == 0 {
			basics, err := extractSingboxBasics(currentConfig)
			if err != nil {
				return err
			}
			result.Revision = currentRevision
			result.Basics = basics
			return nil
		}

		basics, err := normalizeAndValidateSingboxBasics(request.Basics, tx)
		if err != nil {
			return err
		}
		oldBasics, err := extractSingboxBasics(currentConfig)
		if err != nil {
			return err
		}
		oldBasics, err = normalizeAndValidateSingboxBasics(oldBasics, tx)
		if err != nil {
			return err
		}
		if bytes.Equal(oldBasics, basics) {
			result.Revision = currentRevision
			result.Basics = basics
			return nil
		}
		updated, err := replaceSingboxBasics(currentConfig, basics)
		if err != nil {
			return err
		}
		if err := (&SettingService{}).SaveConfig(tx, updated); err != nil {
			return err
		}
		newRevision, err := bumpSingboxConfigRevision(tx, currentRevision)
		if err != nil {
			return err
		}
		result.Revision = newRevision
		result.Basics = basics
		result.Changed = true
		return recordChange(tx, model.Changes{
			DateTime: time.Now().Unix(),
			Actor:    actor,
			Key:      "singbox_basics",
			Action:   "set",
			Obj:      basics,
		})
	})
	if err != nil {
		return nil, err
	}

	if result.Changed {
		markLastUpdate(time.Now().Unix())
	}
	if result.Changed || request.RetryRuntime {
		if err := regenerateSingboxBasicsRuntimeConfig(s); err != nil {
			runtimeErr := fmt.Errorf("regenerate sing-box config after basics save failed: %w", err)
			logger.Warning(runtimeErr)
			return result, &CommittedSaveError{Err: runtimeErr}
		}
	}
	return result, nil
}

func extractSingboxBasics(config json.RawMessage) (json.RawMessage, error) {
	root := map[string]json.RawMessage{}
	if err := json.Unmarshal(config, &root); err != nil {
		return nil, err
	}
	basics := map[string]json.RawMessage{}
	if raw, ok := root["ntp"]; ok && !isJSONNull(raw) {
		basics["ntp"] = append(json.RawMessage(nil), raw...)
	}
	if raw, ok := root["experimental"]; ok && !isJSONNull(raw) {
		basics["experimental"] = append(json.RawMessage(nil), raw...)
	} else {
		basics["experimental"] = json.RawMessage(`{}`)
	}
	return json.Marshal(basics)
}

func replaceSingboxBasics(config, basics json.RawMessage) (json.RawMessage, error) {
	root := map[string]json.RawMessage{}
	if err := json.Unmarshal(config, &root); err != nil {
		return nil, err
	}
	values := map[string]json.RawMessage{}
	if err := json.Unmarshal(basics, &values); err != nil || values == nil {
		if err == nil {
			err = fmt.Errorf("basics must be an object")
		}
		return nil, err
	}
	for _, key := range []string{"ntp", "experimental"} {
		raw, exists := values[key]
		if !exists || isJSONNull(raw) {
			delete(root, key)
			continue
		}
		root[key] = append(json.RawMessage(nil), raw...)
	}
	return json.Marshal(root)
}

func normalizeAndValidateSingboxBasics(raw json.RawMessage, tx *gorm.DB) (json.RawMessage, error) {
	if len(raw) > SingboxBasicsMaxBytes {
		return nil, common.NewErrorf("sing-box basics exceeds %d bytes", SingboxBasicsMaxBytes)
	}
	if !utf8.Valid(raw) {
		return nil, common.NewError("sing-box basics is not valid UTF-8")
	}
	values := map[string]any{}
	if err := json.Unmarshal(raw, &values); err != nil || values == nil {
		if err == nil {
			err = fmt.Errorf("basics must be an object")
		}
		return nil, common.NewErrorf("invalid sing-box basics: %v", err)
	}
	for key := range values {
		switch key {
		case "ntp", "experimental":
		default:
			return nil, common.NewErrorf("sing-box basics contains unsupported field %q", key)
		}
	}
	if err := validateSingboxBasicsNTP(values["ntp"], tx); err != nil {
		return nil, err
	}
	if err := validateSingboxBasicsExperimental(values["experimental"], tx); err != nil {
		return nil, err
	}
	return json.Marshal(values)
}

func validateSingboxBasicsNTP(raw any, tx *gorm.DB) error {
	if raw == nil {
		return nil
	}
	ntp, ok := raw.(map[string]any)
	if !ok {
		return common.NewError("sing-box ntp must be an object")
	}
	if value, exists := ntp["enabled"]; exists && value != nil {
		if _, ok := value.(bool); !ok {
			return common.NewError("sing-box ntp.enabled must be boolean")
		}
	}
	if value, exists := ntp["server"]; exists && value != nil {
		if err := validateSingboxBasicsString(value, "sing-box ntp.server"); err != nil {
			return err
		}
	}
	if value, exists := ntp["server_port"]; exists && value != nil {
		parsed, ok := strictSingboxRouteInteger(value)
		if !ok || parsed < 1 || parsed > 65535 {
			return common.NewError("sing-box ntp.server_port must be an integer from 1 to 65535")
		}
		ntp["server_port"] = parsed
	}
	if value, exists := ntp["interval"]; exists && value != nil {
		interval, err := normalizeSingboxBasicsDuration(value, "sing-box ntp.interval")
		if err != nil {
			return err
		}
		ntp["interval"] = interval
	}
	if err := validateSingboxBasicsDial(ntp, tx, "sing-box ntp"); err != nil {
		return err
	}
	return nil
}

func validateSingboxBasicsExperimental(raw any, tx *gorm.DB) error {
	if raw == nil {
		return nil
	}
	experimental, ok := raw.(map[string]any)
	if !ok {
		return common.NewError("sing-box experimental must be an object")
	}
	if cache, exists := experimental["cache_file"]; exists && cache != nil {
		cacheMap, ok := cache.(map[string]any)
		if !ok {
			return common.NewError("sing-box experimental.cache_file must be an object")
		}
		if value, exists := cacheMap["enabled"]; exists && value != nil {
			if _, ok := value.(bool); !ok {
				return common.NewError("sing-box experimental.cache_file.enabled must be boolean")
			}
		}
		for _, field := range []string{"path", "cache_id"} {
			if value, exists := cacheMap[field]; exists && value != nil {
				if err := validateSingboxBasicsString(value, "sing-box experimental.cache_file."+field); err != nil {
					return err
				}
			}
		}
		if value, exists := cacheMap["store_fakeip"]; exists && value != nil {
			if _, ok := value.(bool); !ok {
				return common.NewError("sing-box experimental.cache_file.store_fakeip must be boolean")
			}
		}
	}

	outboundTags, err := loadSingboxRouteOutboundTags(tx)
	if err != nil {
		return err
	}
	outboundSet := stringSet(outboundTags)
	inboundTags, err := loadSingboxDNSInboundTags(tx)
	if err != nil {
		return err
	}
	inboundSet := stringSet(inboundTags)
	clientNames, err := loadSingboxDNSClientNames(tx)
	if err != nil {
		return err
	}
	clientSet := stringSet(clientNames)

	if clash, exists := experimental["clash_api"]; exists && clash != nil {
		clashMap, ok := clash.(map[string]any)
		if !ok {
			return common.NewError("sing-box experimental.clash_api must be an object")
		}
		for _, field := range []string{"external_controller", "external_ui", "external_ui_download_url", "secret", "default_mode"} {
			if value, exists := clashMap[field]; exists && value != nil {
				if err := validateSingboxBasicsString(value, "sing-box experimental.clash_api."+field); err != nil {
					return err
				}
			}
		}
		if value, exists := clashMap["external_ui_download_detour"]; exists && value != nil {
			if err := validateSingboxBasicsReference(value, outboundSet, "sing-box experimental.clash_api.external_ui_download_detour"); err != nil {
				return err
			}
		}
		if value, exists := clashMap["access_control_allow_origin"]; exists && value != nil {
			if err := validateSingboxBasicsStringList(value, "sing-box experimental.clash_api.access_control_allow_origin", nil); err != nil {
				return err
			}
		}
		if value, exists := clashMap["access_control_allow_private_network"]; exists && value != nil {
			if _, ok := value.(bool); !ok {
				return common.NewError("sing-box experimental.clash_api.access_control_allow_private_network must be boolean")
			}
		}
	}

	if api, exists := experimental["v2ray_api"]; exists && api != nil {
		apiMap, ok := api.(map[string]any)
		if !ok {
			return common.NewError("sing-box experimental.v2ray_api must be an object")
		}
		if value, exists := apiMap["listen"]; exists && value != nil {
			if err := validateSingboxBasicsString(value, "sing-box experimental.v2ray_api.listen"); err != nil {
				return err
			}
		}
		if stats, exists := apiMap["stats"]; exists && stats != nil {
			statsMap, ok := stats.(map[string]any)
			if !ok {
				return common.NewError("sing-box experimental.v2ray_api.stats must be an object")
			}
			if value, exists := statsMap["enabled"]; exists && value != nil {
				if _, ok := value.(bool); !ok {
					return common.NewError("sing-box experimental.v2ray_api.stats.enabled must be boolean")
				}
			}
			if value, exists := statsMap["inbounds"]; exists && value != nil {
				if err := validateSingboxBasicsStringList(value, "sing-box experimental.v2ray_api.stats.inbounds", inboundSet); err != nil {
					return err
				}
			}
			if value, exists := statsMap["outbounds"]; exists && value != nil {
				if err := validateSingboxBasicsStringList(value, "sing-box experimental.v2ray_api.stats.outbounds", outboundSet); err != nil {
					return err
				}
			}
			if value, exists := statsMap["users"]; exists && value != nil {
				if err := validateSingboxBasicsStringList(value, "sing-box experimental.v2ray_api.stats.users", clientSet); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateSingboxBasicsDial(dial map[string]any, tx *gorm.DB, owner string) error {
	if value, exists := dial["detour"]; exists && value != nil {
		outboundTags, err := loadSingboxRouteOutboundTags(tx)
		if err != nil {
			return err
		}
		if err := validateSingboxBasicsReference(value, stringSet(outboundTags), owner+".detour"); err != nil {
			return err
		}
	}
	if value, exists := dial["domain_resolver"]; exists && value != nil {
		dnsServerTags, err := loadSingboxEffectiveDNSServerTags(tx)
		if err != nil {
			return err
		}
		if err := validateSingboxBasicsReference(value, stringSet(dnsServerTags), owner+".domain_resolver"); err != nil {
			return err
		}
	}
	if value, exists := dial["routing_mark"]; exists && value != nil {
		parsed, ok := strictSingboxRouteInteger(value)
		if !ok || parsed < 0 {
			return common.NewError(owner + ".routing_mark must be a non-negative integer")
		}
		dial["routing_mark"] = parsed
	}
	for _, field := range []string{"bind_interface", "inet4_bind_address", "inet6_bind_address"} {
		if value, exists := dial[field]; exists && value != nil {
			if err := validateSingboxBasicsString(value, owner+"."+field); err != nil {
				return err
			}
		}
	}
	for _, field := range []string{"reuse_addr", "tcp_fast_open", "tcp_multi_path", "udp_fragment"} {
		if value, exists := dial[field]; exists && value != nil {
			if _, ok := value.(bool); !ok {
				return common.NewError(owner + "." + field + " must be boolean")
			}
		}
	}
	if value, exists := dial["connect_timeout"]; exists && value != nil {
		duration, err := normalizeSingboxBasicsDuration(value, owner+".connect_timeout")
		if err != nil {
			return err
		}
		dial["connect_timeout"] = duration
	}
	return nil
}

func normalizeSingboxBasicsDuration(raw any, field string) (string, error) {
	value, ok := raw.(string)
	if !ok {
		return "", common.NewError(field + " must be a duration string")
	}
	value = strings.TrimSpace(value)
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 || parsed > singboxBasicsMaxDuration {
		return "", common.NewError(field + " must be a positive duration no longer than 7 days")
	}
	return value, nil
}

func loadSingboxEffectiveDNSServerTags(tx *gorm.DB) ([]string, error) {
	config, err := loadSingboxConfigForDNS(tx)
	if err != nil {
		return nil, err
	}
	root := map[string]any{}
	if err := json.Unmarshal(config, &root); err != nil {
		return nil, err
	}
	dns, _ := root["dns"].(map[string]any)
	finalTag := ""
	if dns != nil {
		finalTag, _ = dns["final"].(string)
	}
	raw, found, err := (&DnsServerService{}).GetSelectedConfig(tx, finalTag)
	if err != nil || !found {
		return nil, err
	}
	server := map[string]any{}
	if err := json.Unmarshal(raw, &server); err != nil {
		return nil, err
	}
	tag, _ := server["tag"].(string)
	return compactUniqueStrings([]string{tag}), nil
}

func validateSingboxBasicsReference(raw any, known map[string]struct{}, field string) error {
	value, ok := raw.(string)
	if !ok {
		return common.NewError(field + " must be a string")
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if _, exists := known[value]; !exists {
		return common.NewErrorf("%s references unknown target %q", field, value)
	}
	return nil
}

func validateSingboxBasicsString(raw any, field string) error {
	value, ok := raw.(string)
	if !ok {
		return common.NewError(field + " must be a string")
	}
	if len(value) > singboxBasicsMaxStringBytes || !utf8.ValidString(value) {
		return common.NewErrorf("%s is too long or invalid UTF-8", field)
	}
	return nil
}

func validateSingboxBasicsStringList(raw any, field string, known map[string]struct{}) error {
	values, ok := raw.([]any)
	if !ok {
		return common.NewError(field + " must be an array of strings")
	}
	if len(values) > singboxBasicsMaxListItems {
		return common.NewErrorf("%s has too many items", field)
	}
	seen := make(map[string]struct{}, len(values))
	for index, rawValue := range values {
		value, ok := rawValue.(string)
		if !ok {
			return common.NewErrorf("%s[%d] must be a string", field, index)
		}
		value = strings.TrimSpace(value)
		if value == "" {
			return common.NewErrorf("%s[%d] must not be empty", field, index)
		}
		if _, duplicate := seen[value]; duplicate {
			return common.NewErrorf("%s contains duplicate target %q", field, value)
		}
		seen[value] = struct{}{}
		if known != nil {
			if _, exists := known[value]; !exists {
				return common.NewErrorf("%s references unknown target %q", field, value)
			}
		}
		values[index] = value
	}
	return nil
}

func stringSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result[value] = struct{}{}
		}
	}
	return result
}

func isJSONNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func validateSingboxConfigBasicsReferences(config json.RawMessage, tx *gorm.DB) error {
	basics, err := extractSingboxBasics(config)
	if err != nil {
		return err
	}
	_, err = normalizeAndValidateSingboxBasics(basics, tx)
	return err
}
