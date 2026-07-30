package service

import (
	"strings"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util/common"

	"gorm.io/gorm"
)

const subscriptionInitialBaselineVersion = 1

type SubscriptionInitialResetResult struct {
	Revision    uint64            `json:"revision"`
	Kind        string            `json:"kind"`
	ChangedKeys []string          `json:"changedKeys"`
	Values      map[string]string `json:"values"`
	Warnings    []string          `json:"warnings,omitempty"`
}

func defaultSubscriptionInitialState() model.SubscriptionInitialState {
	return model.SubscriptionInitialState{
		Id:                    1,
		JSONExtension:         "",
		ClashExtension:        "",
		ServerTLSStoreEnabled: "true",
		ServerTLSStore:        "chrome",
		ClientTLSStoreEnabled: "true",
		ClientTLSStore:        "chrome",
		BaselineVersion:       subscriptionInitialBaselineVersion,
	}
}

// ensureSubscriptionInitialState creates an immutable baseline without
// touching the current user-editable subscription settings.
func (s *SettingService) ensureSubscriptionInitialState() error {
	db := database.GetDB()
	if db == nil {
		return common.NewError("database is not ready")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		_, err := ensureSubscriptionInitialState(tx)
		return err
	})
}

func ensureSubscriptionInitialState(tx *gorm.DB) (model.SubscriptionInitialState, error) {
	if tx == nil {
		return model.SubscriptionInitialState{}, common.NewError("database transaction is not ready")
	}
	state := model.SubscriptionInitialState{Id: 1}
	err := tx.Where("id = ?", state.Id).First(&state).Error
	if database.IsNotFound(err) {
		state = defaultSubscriptionInitialState()
		if createErr := tx.Create(&state).Error; createErr != nil {
			return model.SubscriptionInitialState{}, createErr
		}
		return state, nil
	}
	if err != nil {
		return model.SubscriptionInitialState{}, err
	}
	return state, nil
}

func (s *SettingService) subscriptionInitialResetChanges(kind string) (string, map[string]string, error) {
	normalizedKind := strings.ToLower(strings.TrimSpace(kind))
	if normalizedKind != "json" && normalizedKind != "clash" {
		return "", nil, common.NewError("订阅扩展类型必须是 json 或 clash")
	}

	db := database.GetDB()
	if db == nil {
		return "", nil, common.NewError("database is not ready")
	}
	state := model.SubscriptionInitialState{}
	err := db.Transaction(func(tx *gorm.DB) error {
		var loadErr error
		state, loadErr = ensureSubscriptionInitialState(tx)
		return loadErr
	})
	if err != nil {
		return "", nil, err
	}

	if normalizedKind == "json" {
		return normalizedKind, map[string]string{
			"subJsonExt":            state.JSONExtension,
			"serverTlsStoreEnabled": state.ServerTLSStoreEnabled,
			"serverTlsStore":        state.ServerTLSStore,
			"clientTlsStoreEnabled": state.ClientTLSStoreEnabled,
			"clientTlsStore":        state.ClientTLSStore,
		}, nil
	}
	return normalizedKind, map[string]string{"subClashExt": state.ClashExtension}, nil
}

func (s *ConfigService) ResetSubscriptionToInitialState(kind string, expectedRevision uint64, actor string) (*SubscriptionInitialResetResult, error) {
	normalizedKind, changes, err := s.SettingService.subscriptionInitialResetChanges(kind)
	if err != nil {
		return nil, err
	}

	patch, err := s.SaveSettingsPatch(SettingsPatchRequest{
		ExpectedRevision: expectedRevision,
		Changes:          changes,
		AuditAction:      "subscription-initial-reset",
	}, actor)
	if err != nil {
		return nil, err
	}

	return &SubscriptionInitialResetResult{
		Revision:    patch.Revision,
		Kind:        normalizedKind,
		ChangedKeys: append([]string{}, patch.ChangedKeys...),
		Values:      changes,
		Warnings:    append([]string(nil), patch.Warnings...),
	}, nil
}
