package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/util/common"
	"gorm.io/gorm"
)

// MihomoDNSPatchRequest is deliberately limited to the fields owned by the
// Mihomo DNS page. It prevents a DNS save from replacing unrelated route,
// sniffer, or general Mihomo settings from another open page.
type MihomoDNSPatchRequest struct {
	ExpectedRevision *int64          `json:"expectedRevision"`
	IPv6             *bool           `json:"ipv6"`
	TCPConcurrent    *bool           `json:"tcpConcurrent"`
	DNS              json.RawMessage `json:"dns"`
	// RetryRuntime asks the service to rewrite server.yaml even when the
	// normalized database snapshot is unchanged. This is needed after a
	// previous post-commit runtime-file failure.
	RetryRuntime bool `json:"retryRuntime,omitempty"`
}

type MihomoDNSPatchResult struct {
	Config   json.RawMessage `json:"config"`
	Changed  bool            `json:"changed"`
	Revision int64           `json:"revision"`
}

func (s *ConfigService) SaveMihomoDNSPatch(request MihomoDNSPatchRequest, actor string) (*MihomoDNSPatchResult, error) {
	if len(request.DNS) == 0 {
		return nil, common.NewError("dns 是必填项；使用 null 可清空 DNS 配置")
	}
	// Keep this focused save in the same write domain as the generic Mihomo
	// config editor and outbound subscription imports. Without this lock an
	// import can finish after the DNS transaction and overwrite its newer base
	// configuration.
	if !mihomoOutboundSubscriptionImportMu.TryLock() {
		return nil, ErrMihomoSubscriptionImportBusy
	}
	defer mihomoOutboundSubscriptionImportMu.Unlock()
	if err := ensureMihomoConfigRevision(request.ExpectedRevision); err != nil {
		return nil, err
	}

	db := database.GetDB()
	if db == nil {
		return nil, common.NewError("database is not ready")
	}

	result := &MihomoDNSPatchResult{}
	err := db.Transaction(func(tx *gorm.DB) error {
		current, err := loadMihomoConfigForDNSPatch(tx)
		if err != nil {
			return err
		}
		updated, err := applyMihomoDNSPatch(current, request)
		if err != nil {
			return err
		}
		current, err = sanitizeMihomoConfigJSON(current)
		if err != nil {
			return err
		}

		result.Config = updated
		if bytes.Equal(current, updated) {
			return nil
		}

		// Use the same route/reference validation as the general Mihomo
		// config save. A DNS-only request must not preserve a malformed route
		// that would later be silently changed by the YAML renderer.
		if err := (&MihomoConfigService{}).SaveConfig(tx, updated); err != nil {
			return fmt.Errorf("invalid mihomo runtime config: %w", err)
		}
		// Render against the same transaction so invalid DNS changes cannot be
		// committed while the active server.yaml remains on the old version.
		if err := NewMihomoManagerService().ValidateServerConfig(tx); err != nil {
			return fmt.Errorf("invalid mihomo runtime config: %w", err)
		}
		if err := recordChange(tx, model.Changes{
			DateTime: time.Now().Unix(),
			Actor:    actor,
			Key:      "mihomo_config",
			Action:   "dns-patch",
			Obj:      buildMihomoDNSPatchAudit(request),
		}); err != nil {
			return err
		}
		result.Changed = true
		return nil
	})
	if err != nil {
		return nil, err
	}

	if result.Changed || request.RetryRuntime {
		markMihomoLastUpdate(time.Now().Unix())
		result.Revision = CurrentMihomoConfigRevisionForPolling()
		if err := NewMihomoManagerService().RegenerateServerConfig(); err != nil {
			runtimeErr := fmt.Errorf("regenerate mihomo server config after DNS patch failed: %w", err)
			logger.Warning(runtimeErr)
			return result, &CommittedSaveError{Err: runtimeErr}
		}
	}
	if result.Revision == 0 {
		result.Revision = CurrentMihomoConfigRevisionForPolling()
	}
	return result, nil
}

func loadMihomoConfigForDNSPatch(tx *gorm.DB) (json.RawMessage, error) {
	var setting model.Setting
	err := tx.Where("key = ?", mihomoConfigSettingKey).First(&setting).Error
	if err == nil {
		return json.RawMessage(setting.Value), nil
	}
	if !database.IsNotFound(err) {
		return nil, err
	}
	value, err := (&SettingService{}).defaultSettingValue(mihomoConfigSettingKey)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(value), nil
}

func applyMihomoDNSPatch(current json.RawMessage, request MihomoDNSPatchRequest) (json.RawMessage, error) {
	var document map[string]interface{}
	if err := json.Unmarshal(current, &document); err != nil {
		return nil, err
	}
	if document == nil {
		document = make(map[string]interface{})
	}

	if request.IPv6 == nil {
		delete(document, "ipv6")
	} else {
		document["ipv6"] = *request.IPv6
	}
	if request.TCPConcurrent == nil {
		delete(document, "tcp-concurrent")
	} else {
		document["tcp-concurrent"] = *request.TCPConcurrent
	}

	if bytes.Equal(bytes.TrimSpace(request.DNS), []byte("null")) {
		delete(document, "dns")
	} else {
		var dns map[string]interface{}
		if err := json.Unmarshal(request.DNS, &dns); err != nil {
			return nil, fmt.Errorf("dns 格式无效: %w", err)
		}
		if err := validateMihomoDNSConfig(dns); err != nil {
			return nil, err
		}
		document["dns"] = dns
	}

	data, err := json.Marshal(document)
	if err != nil {
		return nil, err
	}
	return sanitizeMihomoConfigJSON(data)
}

func buildMihomoDNSPatchAudit(request MihomoDNSPatchRequest) json.RawMessage {
	audit := map[string]interface{}{
		"ipv6":          request.IPv6,
		"tcpConcurrent": request.TCPConcurrent,
	}
	if bytes.Equal(bytes.TrimSpace(request.DNS), []byte("null")) {
		audit["dns"] = map[string]interface{}{"cleared": true}
		data, _ := json.Marshal(audit)
		return data
	}

	var dns map[string]interface{}
	_ = json.Unmarshal(request.DNS, &dns)
	addressCount := 0
	presentKeys := make([]string, 0, len(mihomoSupportedDNSListKeys))
	for _, key := range mihomoSupportedDNSListKeys {
		values := sanitizeMihomoDNSStringList(dns[key])
		if len(values) == 0 {
			continue
		}
		addressCount += len(values)
		presentKeys = append(presentKeys, key)
	}
	audit["dns"] = map[string]interface{}{
		"cleared":      false,
		"addressCount": addressCount,
		"keys":         presentKeys,
	}
	data, _ := json.Marshal(audit)
	return data
}
