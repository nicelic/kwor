package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/alireza0/s-ui/service"
	"github.com/alireza0/s-ui/util/common"

	"github.com/gin-gonic/gin"
)

type settingsPatchAPIRequest struct {
	ExpectedRevision           uint64            `json:"expectedRevision"`
	Changes                    map[string]string `json:"changes"`
	SystemTimeLocation         string            `json:"systemTimeLocation,omitempty"`
	ConfirmTrafficHistoryClear bool              `json:"confirmTrafficHistoryClear"`
}

type subscriptionInitialResetAPIRequest struct {
	ExpectedRevision uint64 `json:"expectedRevision"`
	Kind             string `json:"kind"`
}

var settingsSystemTimeZoneSaveMu sync.Mutex

func (a *ApiService) GetSettingsSnapshot(c *gin.Context) {
	includeExtensions := strings.TrimSpace(c.Query("includeExtensions")) != "false" && strings.TrimSpace(c.Query("includeExtensions")) != "0"
	snapshot, err := a.SettingService.GetSettingsSnapshot(includeExtensions)
	jsonObj(c, snapshot, err)
}

func (a *ApiService) GetSubscriptionSettingsSnapshot(c *gin.Context) {
	snapshot, err := a.SettingService.GetSubscriptionSettingsSnapshot(c.Query("kind"))
	jsonObj(c, snapshot, err)
}

func (a *ApiService) SaveSettingsPatch(c *gin.Context, actor string) {
	request := settingsPatchAPIRequest{}
	if err := c.ShouldBindJSON(&request); err != nil {
		jsonMsg(c, "", err)
		return
	}

	result, err := a.saveSettingsPatch(
		request.ExpectedRevision,
		request.Changes,
		request.SystemTimeLocation,
		request.ConfirmTrafficHistoryClear,
		actor,
	)
	if err != nil {
		writeSettingsPatchError(c, err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) ResetSubscriptionToInitialState(c *gin.Context, actor string) {
	request := subscriptionInitialResetAPIRequest{}
	if err := c.ShouldBindJSON(&request); err != nil {
		jsonMsg(c, "", err)
		return
	}
	result, err := a.ConfigService.ResetSubscriptionToInitialState(request.Kind, request.ExpectedRevision, actor)
	if err != nil {
		writeSettingsPatchError(c, err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) saveLegacySettings(c *gin.Context, actor string, raw string) {
	revisionText := strings.TrimSpace(c.Request.FormValue("expectedRevision"))
	if revisionText == "" {
		jsonMsg(c, "save", common.NewError("保存设置必须携带 expectedRevision"))
		return
	}
	expectedRevision, err := strconv.ParseUint(revisionText, 10, 64)
	if err != nil {
		jsonMsg(c, "save", common.NewError("expectedRevision 必须是非负整数"))
		return
	}

	changes := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &changes); err != nil {
		jsonMsg(c, "save", err)
		return
	}
	systemTimeLocation := changes["systemTimeLocation"]
	delete(changes, "systemTimeLocation")
	confirmTrafficHistoryClear := false
	if confirmation := strings.TrimSpace(c.Request.FormValue("confirmTrafficHistoryClear")); confirmation != "" {
		parsed, parseErr := strconv.ParseBool(confirmation)
		if parseErr != nil {
			jsonMsg(c, "save", common.NewError("confirmTrafficHistoryClear 必须是 true 或 false"))
			return
		}
		confirmTrafficHistoryClear = parsed
	}
	result, err := a.saveSettingsPatch(expectedRevision, changes, systemTimeLocation, confirmTrafficHistoryClear, actor)
	if err != nil {
		writeSettingsPatchError(c, err)
		return
	}

	snapshot, err := a.SettingService.GetSettingsSnapshot()
	if err != nil {
		jsonMsg(c, "save", err)
		return
	}
	jsonObj(c, gin.H{
		"settings":    snapshot.Values,
		"revision":    result.Revision,
		"changedKeys": result.ChangedKeys,
		"warnings":    result.Warnings,
	}, nil)
}

func (a *ApiService) saveSettingsPatch(expectedRevision uint64, changes map[string]string, systemTimeLocation string, confirmTrafficHistoryClear bool, actor string) (*service.SettingsPatchResult, error) {
	requiresSystemTimeLock := strings.TrimSpace(systemTimeLocation) != ""
	if requiresSystemTimeLock {
		settingsSystemTimeZoneSaveMu.Lock()
		defer settingsSystemTimeZoneSaveMu.Unlock()
		// Check before touching the host. The patch transaction below still
		// performs its own CAS, because ordinary settings can change while a
		// timezone remote validation is in progress.
		if err := a.SettingService.CheckSettingsRevision(expectedRevision); err != nil {
			return nil, err
		}
	}

	prepared, rollback, systemTimeChanged, err := a.prepareSettingsTimeZoneSave(changes, systemTimeLocation)
	if err != nil {
		return nil, err
	}
	result, err := a.ConfigService.SaveSettingsPatch(service.SettingsPatchRequest{
		ExpectedRevision:           expectedRevision,
		Changes:                    prepared,
		ConfirmTrafficHistoryClear: confirmTrafficHistoryClear,
		ForceRevision:              systemTimeChanged,
		SystemTimeChanged:          systemTimeChanged,
	}, actor)
	if err != nil {
		rollbackSettingsSystemTimeZone(&err, rollback)
		return nil, err
	}
	return result, nil
}

func rollbackSettingsSystemTimeZone(target *error, rollback func() error) {
	if target == nil || *target == nil || rollback == nil {
		return
	}
	if rollbackErr := rollback(); rollbackErr != nil {
		*target = fmt.Errorf("%w；恢复 Linux 系统时区失败：%v", *target, rollbackErr)
	}
}

func writeSettingsPatchError(c *gin.Context, err error) {
	if err == nil {
		return
	}
	var conflict *service.SettingsRevisionConflictError
	if errors.As(err, &conflict) {
		jsonMsgObj(c, "", gin.H{
			"code":            "revision_conflict",
			"currentRevision": conflict.CurrentRevision,
		}, err)
		return
	}
	jsonMsg(c, "", err)
}
