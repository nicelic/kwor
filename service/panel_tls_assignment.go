package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

const (
	panelAssignedCertificateRecordIDPanelKey  = "panelAssignedCertificateRecordID"
	panelAssignedCertificateRecordIDSubKey    = "subAssignedCertificateRecordID"
	panelAssignedCertificateRecordIDsPanelKey = "panelAssignedCertificateRecordIDs"
	panelAssignedCertificateRecordIDsSubKey   = "subAssignedCertificateRecordIDs"
	panelTLSDrainGracePeriod                  = 6 * time.Minute
	panelTLSUnapplyDrainGracePeriod           = 30 * time.Second
)

func PanelTLSDrainGracePeriod() time.Duration {
	return panelTLSDrainGracePeriod
}

func PanelTLSUnapplyDrainGracePeriod() time.Duration {
	return panelTLSUnapplyDrainGracePeriod
}

func panelAssignedCertificateRecordIDKey(target PanelSelfSignedTarget) string {
	if target == PanelSelfSignedTargetSub {
		return panelAssignedCertificateRecordIDSubKey
	}
	return panelAssignedCertificateRecordIDPanelKey
}

func panelAssignedCertificateRecordIDsKey(target PanelSelfSignedTarget) string {
	if target == PanelSelfSignedTargetSub {
		return panelAssignedCertificateRecordIDsSubKey
	}
	return panelAssignedCertificateRecordIDsPanelKey
}

func parseAssignedCertificateRecordIDs(raw string) ([]uint, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []uint{}, true
	}
	parsed := make([]uint, 0)
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return []uint{}, false
	}
	return parsed, true
}

func normalizeAssignedCertificateRecordIDs(ids []uint) []uint {
	if len(ids) == 0 {
		return []uint{}
	}
	seen := make(map[uint]struct{}, len(ids))
	result := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func filterExistingCertificateRecordIDs(ids []uint) ([]uint, error) {
	normalized := normalizeAssignedCertificateRecordIDs(ids)
	if len(normalized) == 0 {
		return []uint{}, nil
	}

	rows := make([]model.CertificateRecord, 0, len(normalized))
	if err := database.GetDB().
		Model(&model.CertificateRecord{}).
		Select("id").
		Where("id IN ?", normalized).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	existing := make(map[uint]struct{}, len(rows))
	for i := range rows {
		existing[rows[i].Id] = struct{}{}
	}

	result := make([]uint, 0, len(normalized))
	for _, id := range normalized {
		if _, ok := existing[id]; !ok {
			continue
		}
		result = append(result, id)
	}
	return result, nil
}

func readLegacyAssignedCertificateRecordID(settingService *SettingService, target PanelSelfSignedTarget) (uint, error) {
	if settingService == nil {
		return 0, nil
	}
	raw, err := settingService.getString(panelAssignedCertificateRecordIDKey(target))
	if err != nil {
		return 0, err
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	parsed, parseErr := strconv.ParseUint(raw, 10, 64)
	if parseErr != nil {
		return 0, nil
	}
	return uint(parsed), nil
}

func persistAssignedCertificateRecordIDs(settingService *SettingService, target PanelSelfSignedTarget, ids []uint) error {
	if settingService == nil {
		return nil
	}
	normalized := normalizeAssignedCertificateRecordIDs(ids)
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return err
	}
	if err := settingService.SaveSetting(panelAssignedCertificateRecordIDsKey(target), string(encoded)); err != nil {
		return err
	}

	legacyID := uint(0)
	if len(normalized) > 0 {
		legacyID = normalized[0]
	}
	return settingService.SaveSetting(panelAssignedCertificateRecordIDKey(target), strconv.FormatUint(uint64(legacyID), 10))
}

func GetAssignedCertificateRecordIDs(settingService *SettingService, target PanelSelfSignedTarget) ([]uint, error) {
	if settingService == nil {
		return []uint{}, nil
	}

	rawList, err := settingService.getString(panelAssignedCertificateRecordIDsKey(target))
	if err != nil {
		return nil, err
	}
	parsedFromList, parsedListOK := parseAssignedCertificateRecordIDs(rawList)
	filteredFromList, err := filterExistingCertificateRecordIDs(parsedFromList)
	if err != nil {
		return nil, err
	}

	legacyID, err := readLegacyAssignedCertificateRecordID(settingService, target)
	if err != nil {
		return nil, err
	}
	resolved := filteredFromList
	if len(resolved) == 0 && legacyID > 0 {
		filteredLegacy, legacyFilterErr := filterExistingCertificateRecordIDs([]uint{legacyID})
		if legacyFilterErr != nil {
			return nil, legacyFilterErr
		}
		resolved = filteredLegacy
	}

	canonicalListRawBytes, marshalErr := json.Marshal(resolved)
	if marshalErr != nil {
		return nil, marshalErr
	}
	canonicalListRaw := string(canonicalListRawBytes)
	currentListRaw := strings.TrimSpace(rawList)

	expectedLegacyID := uint(0)
	if len(resolved) > 0 {
		expectedLegacyID = resolved[0]
	}

	needsWriteback := !parsedListOK || currentListRaw != canonicalListRaw || legacyID != expectedLegacyID
	if needsWriteback {
		if persistErr := persistAssignedCertificateRecordIDs(settingService, target, resolved); persistErr != nil {
			return nil, persistErr
		}
	}
	return resolved, nil
}

func SetAssignedCertificateRecordIDs(settingService *SettingService, target PanelSelfSignedTarget, ids []uint) error {
	if settingService == nil {
		return nil
	}
	filtered, err := filterExistingCertificateRecordIDs(ids)
	if err != nil {
		return err
	}
	return persistAssignedCertificateRecordIDs(settingService, target, filtered)
}

func GetAssignedCertificateRecordID(settingService *SettingService, target PanelSelfSignedTarget) (uint, error) {
	ids, err := GetAssignedCertificateRecordIDs(settingService, target)
	if err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	return ids[0], nil
}

func SetAssignedCertificateRecordID(settingService *SettingService, target PanelSelfSignedTarget, id uint) error {
	if id == 0 {
		return SetAssignedCertificateRecordIDs(settingService, target, []uint{})
	}
	return SetAssignedCertificateRecordIDs(settingService, target, []uint{id})
}

func certificateAssignedRecordMatches(target PanelSelfSignedTarget, recordID uint) bool {
	if recordID == 0 {
		return false
	}
	settingService := &SettingService{}
	assignedIDs, err := GetAssignedCertificateRecordIDs(settingService, target)
	if err != nil {
		return false
	}
	return slices.Contains(assignedIDs, recordID)
}

func assignedTargetsForCertificateRecord(recordID uint) ([]PanelSelfSignedTarget, error) {
	if recordID == 0 {
		return nil, nil
	}

	settingService := &SettingService{}
	targets := make([]PanelSelfSignedTarget, 0, 2)
	for _, target := range []PanelSelfSignedTarget{PanelSelfSignedTargetPanel, PanelSelfSignedTargetSub} {
		assignedIDs, err := GetAssignedCertificateRecordIDs(settingService, target)
		if err != nil {
			return nil, err
		}
		if slices.Contains(assignedIDs, recordID) {
			targets = append(targets, target)
		}
	}
	return targets, nil
}

func ApplyPanelTLSRuntimeSettingsForRecord(recordID uint) error {
	targets, err := assignedTargetsForCertificateRecord(recordID)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return nil
	}

	errMessages := make([]string, 0, len(targets))
	for _, target := range targets {
		if applyErr := ApplyPanelTLSRuntimeSettings(target); applyErr != nil {
			errMessages = append(errMessages, fmt.Sprintf("%s: %v", target, applyErr))
		}
	}
	if len(errMessages) == 0 {
		return nil
	}
	return errors.New(strings.Join(errMessages, "; "))
}
