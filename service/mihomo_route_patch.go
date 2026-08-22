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

// MihomoRoutePatchRequest contains only the configuration owned by the
// Mihomo route page. An omitted Sniffer field preserves the latest stored
// value; an explicit null removes it.
type MihomoRoutePatchRequest struct {
	ExpectedRevision *int64          `json:"expectedRevision"`
	Route            json.RawMessage `json:"route"`
	Sniffer          json.RawMessage `json:"sniffer"`
	// RetryRuntime asks the service to rewrite server.yaml even when the
	// normalized database snapshot is unchanged. This is needed after a
	// previous post-commit runtime-file failure.
	RetryRuntime bool `json:"retryRuntime,omitempty"`
}

type MihomoRoutePatchResult struct {
	Config   json.RawMessage `json:"config"`
	Changed  bool            `json:"changed"`
	Revision int64           `json:"revision"`
}

func (s *ConfigService) SaveMihomoRoutePatch(request MihomoRoutePatchRequest, actor, hostname string) (*MihomoRoutePatchResult, error) {
	if len(bytes.TrimSpace(request.Route)) == 0 || bytes.Equal(bytes.TrimSpace(request.Route), []byte("null")) {
		return nil, common.NewError("route is required")
	}
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

	result := &MihomoRoutePatchResult{}
	err := db.Transaction(func(tx *gorm.DB) error {
		current, err := loadMihomoConfigForDNSPatch(tx)
		if err != nil {
			return err
		}
		updated, err := applyMihomoRoutePatch(current, request)
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
		if err := (&MihomoConfigService{}).SaveConfig(tx, updated); err != nil {
			return err
		}
		if err := NewMihomoManagerService().ValidateServerConfig(tx); err != nil {
			return fmt.Errorf("invalid mihomo runtime config: %w", err)
		}
		if err := recordChange(tx, model.Changes{
			DateTime: time.Now().Unix(),
			Actor:    actor,
			Key:      mihomoConfigSettingKey,
			Action:   "route-patch",
			Obj:      buildMihomoConfigChangeAudit(updated),
		}); err != nil {
			return err
		}
		result.Changed = true
		return nil
	})
	if err != nil {
		return nil, err
	}

	if !result.Changed && !request.RetryRuntime {
		result.Revision = CurrentMihomoConfigRevisionForPolling()
		return result, nil
	}

	if result.Changed {
		markMihomoLastUpdate(time.Now().Unix())
	}
	result.Revision = CurrentMihomoConfigRevisionForPolling()
	if err := NewMihomoManagerService().RegenerateServerConfig(); err != nil {
		runtimeErr := fmt.Errorf("regenerate mihomo server config after route patch failed: %w", err)
		logger.Warning(runtimeErr)
		return result, &CommittedSaveError{Err: runtimeErr}
	}
	if err := s.syncAutoManagedMihomoClients(hostname); err != nil {
		logger.Warning("sync managed mihomo clients after route patch failed: ", err)
	}
	return result, nil
}

func applyMihomoRoutePatch(current json.RawMessage, request MihomoRoutePatchRequest) (json.RawMessage, error) {
	var document map[string]interface{}
	if err := json.Unmarshal(current, &document); err != nil {
		return nil, err
	}
	if document == nil {
		document = make(map[string]interface{})
	}

	var route map[string]interface{}
	if err := json.Unmarshal(request.Route, &route); err != nil || route == nil {
		if err != nil {
			return nil, fmt.Errorf("route format is invalid: %w", err)
		}
		return nil, common.NewError("route must be an object")
	}
	document["route"] = route

	if len(bytes.TrimSpace(request.Sniffer)) > 0 {
		snifferRaw := bytes.TrimSpace(request.Sniffer)
		if bytes.Equal(snifferRaw, []byte("null")) {
			delete(document, "sniffer")
		} else {
			var sniffer interface{}
			if err := json.Unmarshal(snifferRaw, &sniffer); err != nil {
				return nil, fmt.Errorf("sniffer format is invalid: %w", err)
			}
			switch sniffer.(type) {
			case bool, map[string]interface{}:
				document["sniffer"] = sniffer
			default:
				return nil, common.NewError("sniffer must be an object, boolean, or null")
			}
		}
	}

	data, err := json.Marshal(document)
	if err != nil {
		return nil, err
	}
	return sanitizeMihomoConfigJSON(data)
}
