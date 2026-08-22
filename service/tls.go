package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util/common"

	"gorm.io/gorm"
)

type TlsService struct {
	InboundService
}

const maxDefaultTLSJSONBytes = 2 * 1024 * 1024

// TlsSaveImpact keeps the post-commit work limited to the records whose
// effective runtime or generated client projections actually changed.
type TlsSaveImpact struct {
	TLSID                         uint
	RuntimeConfigChanged          bool
	SubscriptionProjectionChanged bool
	RefreshClientAndInboundData   bool
	PathBindingsChanged           bool
}

func (s *TlsService) GetAll() ([]model.Tls, error) {
	db := database.GetDB()
	tlsConfig := []model.Tls{}
	err := db.Model(model.Tls{}).Scan(&tlsConfig).Error
	if err != nil {
		return nil, err
	}

	return tlsConfig, nil
}

func (s *TlsService) Save(tx *gorm.DB, action string, data json.RawMessage, hostname string) (TlsSaveImpact, error) {
	impact := TlsSaveImpact{}
	var err error

	switch action {
	case "new", "edit":
		if len(data) > maxDefaultTLSJSONBytes {
			return impact, common.NewErrorf("sing-box TLS payload exceeds the %d byte limit", maxDefaultTLSJSONBytes)
		}
		var tls model.Tls
		err = json.Unmarshal(data, &tls)
		if err != nil {
			return impact, err
		}
		if action == "new" {
			// A create payload can come from a cloned editor record; never reuse
			// its primary key with GORM Save.
			tls.Id = 0
		}
		if err = validateDefaultTLSJSONShape(tls.Server, "server"); err != nil {
			return impact, err
		}
		if err = validateDefaultTLSJSONShape(tls.Client, "client"); err != nil {
			return impact, err
		}
		if err = sanitizeStoredTLSRecord(&tls); err != nil {
			return impact, err
		}
		if err = validateDefaultTLSRecordSize(&tls); err != nil {
			return impact, err
		}

		previous := model.Tls{}
		if action == "edit" {
			if tls.Id == 0 {
				return impact, common.NewError("sing-box TLS id is required for edit")
			}
			if err = tx.Where("id = ?", tls.Id).First(&previous).Error; err != nil {
				return impact, err
			}
			if err = sanitizeStoredTLSRecord(&previous); err != nil {
				return impact, err
			}
		}
		err = tx.Save(&tls).Error
		if err != nil {
			return impact, err
		}
		impact.TLSID = tls.Id
		if action == "new" {
			impact.PathBindingsChanged = tlsRawHasPathBindings(tls.Server) || tlsRawHasPathBindings(tls.Client)
		}
		if action == "edit" {
			serverChanged := !bytes.Equal(previous.Server, tls.Server)
			clientChanged := !bytes.Equal(previous.Client, tls.Client)
			impact.PathBindingsChanged = tlsPathBindingsChanged(previous.Server, tls.Server) ||
				tlsPathBindingsChanged(previous.Client, tls.Client)
			if !serverChanged && !clientChanged {
				break
			}

			var inbounds []model.Inbound
			err = tx.Model(model.Inbound{}).Preload("Tls").Where("tls_id = ?", tls.Id).Find(&inbounds).Error
			if err != nil {
				return impact, err
			}
			if len(inbounds) > 0 {
				err = s.ClientService.UpdateLinksByInboundChange(tx, &inbounds, hostname, "")
				if err != nil {
					return impact, err
				}
				var inboundIds []uint
				for _, inbound := range inbounds {
					inboundIds = append(inboundIds, inbound.Id)
				}
				err = s.InboundService.UpdateOutJsons(tx, inboundIds, hostname)
				if err != nil {
					return impact, common.NewError("unable to update out_json of inbounds: ", err.Error())
				}
				impact.SubscriptionProjectionChanged = true
				impact.RefreshClientAndInboundData = true
			}
			if serverChanged {
				var serviceCount int64
				if err = tx.Model(model.Service{}).Where("tls_id = ?", tls.Id).Count(&serviceCount).Error; err != nil {
					return impact, err
				}
				impact.RuntimeConfigChanged = len(inbounds) > 0 || serviceCount > 0
			}
		}
	case "del":
		var id uint
		err = json.Unmarshal(data, &id)
		if err != nil {
			return impact, err
		}
		var inboundCount int64
		err = tx.Model(model.Inbound{}).Where("tls_id = ?", id).Count(&inboundCount).Error
		if err != nil {
			return impact, err
		}
		var serviceCount int64
		err = tx.Model(model.Service{}).Where("tls_id = ?", id).Count(&serviceCount).Error
		if err != nil {
			return impact, err
		}
		if inboundCount > 0 || serviceCount > 0 {
			return impact, common.NewError("tls in use")
		}
		err = tx.Where("id = ?", id).Delete(model.Tls{}).Error
		if err != nil {
			return impact, err
		}
		impact.PathBindingsChanged = true
	default:
		return impact, common.NewErrorf("unknown action: %s", action)
	}

	return impact, nil
}

func validateDefaultTLSRecordSize(tls *model.Tls) error {
	if tls == nil {
		return common.NewError("sing-box TLS is required")
	}
	if len(tls.Server) > maxDefaultTLSJSONBytes || len(tls.Client) > maxDefaultTLSJSONBytes {
		return common.NewErrorf("sing-box TLS server/client configuration exceeds the %d byte limit", maxDefaultTLSJSONBytes)
	}
	if len(tls.Server)+len(tls.Client) > maxDefaultTLSJSONBytes {
		return fmt.Errorf("sing-box TLS configuration exceeds the %d byte limit", maxDefaultTLSJSONBytes)
	}
	return nil
}

func validateDefaultTLSJSONShape(raw json.RawMessage, field string) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}

	var value interface{}
	if err := json.Unmarshal(trimmed, &value); err != nil {
		return common.NewErrorf("sing-box TLS %s JSON is invalid: %v", field, err)
	}
	if _, ok := value.(map[string]interface{}); !ok {
		return common.NewErrorf("sing-box TLS %s must be a JSON object", field)
	}
	return nil
}

func sanitizeStoredTLSRecord(tls *model.Tls) error {
	server, err := sanitizeStoredTLSJSON(tls.Server, "client_certificate_public_key_sha256", "client_certificate", "client_certificate_path")
	if err != nil {
		return err
	}
	client, err := sanitizeStoredTLSJSON(tls.Client, "certificate_public_key_sha256", "certificate", "certificate_path")
	if err != nil {
		return err
	}

	tls.Server = server
	tls.Client = client
	return nil
}

func sanitizeStoredTLSJSON(raw json.RawMessage, hashKey string, conflictingKeys ...string) (json.RawMessage, error) {
	if len(raw) == 0 {
		return raw, nil
	}

	payload := map[string]interface{}{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}

	if hasNonEmptyStringSlice(payload[hashKey]) {
		for _, key := range conflictingKeys {
			delete(payload, key)
		}
	}
	sanitizeStoredTLSOptionalFields(payload)

	sanitized, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, err
	}
	return sanitized, nil
}

// sanitizeStoredTLSOptionalFields keeps optional SNI/ALPN fields absent when
// their editor controls are enabled but the user supplied no value.
func sanitizeStoredTLSOptionalFields(payload map[string]interface{}) {
	if payload == nil {
		return
	}
	if raw, exists := payload["server_name"]; exists {
		value, ok := raw.(string)
		if !ok || strings.TrimSpace(value) == "" {
			delete(payload, "server_name")
		} else {
			payload["server_name"] = strings.TrimSpace(value)
		}
	}
	if raw, exists := payload["alpn"]; exists {
		values, ok := raw.([]interface{})
		if !ok {
			delete(payload, "alpn")
			return
		}
		clean := make([]interface{}, 0, len(values))
		for _, item := range values {
			value, ok := item.(string)
			value = strings.TrimSpace(value)
			if ok && value != "" {
				clean = append(clean, value)
			}
		}
		if len(clean) == 0 {
			delete(payload, "alpn")
		} else {
			payload["alpn"] = clean
		}
	}
}

func hasNonEmptyStringSlice(value interface{}) bool {
	switch v := value.(type) {
	case []interface{}:
		for _, item := range v {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				return true
			}
		}
	case []string:
		for _, item := range v {
			if strings.TrimSpace(item) != "" {
				return true
			}
		}
	}
	return false
}

func tlsRawHasPathBindings(raw json.RawMessage) bool {
	return len(tlsPathBindingIdentitySet(raw)) > 0
}

func tlsPathBindingsChanged(previous json.RawMessage, current json.RawMessage) bool {
	previousSet := tlsPathBindingIdentitySet(previous)
	currentSet := tlsPathBindingIdentitySet(current)
	if len(previousSet) != len(currentSet) {
		return true
	}
	for identity := range previousSet {
		if _, exists := currentSet[identity]; !exists {
			return true
		}
	}
	return false
}

func tlsPathBindingIdentitySet(raw json.RawMessage) map[string]struct{} {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}

	var payload interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}

	entries := collectTLSPathEntries(payload)
	if len(entries) == 0 {
		return nil
	}
	identities := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		identities[entry.Key+"\x00"+entry.Path] = struct{}{}
	}
	return identities
}
