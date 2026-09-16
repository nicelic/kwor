package api

import (
	"fmt"
	"strings"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/service"

	"github.com/gin-gonic/gin"
)

func (a *ApiService) GetPanelTimeContext(c *gin.Context) {
	context, err := a.SettingService.GetPanelTimeContext()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, context, nil)
}

func (a *ApiService) GetSystemTimeZone(c *gin.Context) {
	jsonObj(c, service.GetSystemTimeZoneStatus(), nil)
}

type systemTimeZoneUpdateRequest struct {
	TimeLocation string `json:"timeLocation" form:"timeLocation"`
}

// SetSystemTimeZone changes only the host OS timezone through an isolated endpoint.
func (a *ApiService) SetSystemTimeZone(c *gin.Context) {
	req := systemTimeZoneUpdateRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("无效的请求参数: %w", err))
		return
	}

	requested := strings.TrimSpace(req.TimeLocation)
	if requested == "" {
		jsonMsg(c, "", fmt.Errorf("时区不能为空"))
		return
	}

	status := service.GetSystemTimeZoneStatus()
	if !status.CanModify {
		reason := strings.TrimSpace(status.Reason)
		if reason == "" {
			reason = "当前面板进程没有修改系统时区的权限"
		}
		jsonMsg(c, "", fmt.Errorf("%s", reason))
		return
	}

	normalized, err := service.NormalizePanelTimeLocation(requested)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	if !service.IsSelectableTimeLocation(normalized) {
		jsonMsg(c, "", fmt.Errorf("系统时区只能选择面板提供的时区"))
		return
	}

	if err := service.SetSystemTimeLocation(normalized); err != nil {
		jsonMsg(c, "", err)
		return
	}

	// 记录审计日志
	db := database.GetDB()
	if db != nil {
		actor := GetLoginUser(c)
		if actor == "" {
			actor = "admin"
		}
		_ = db.Create(&model.Changes{
			DateTime: time.Now().Unix(),
			Actor:    actor,
			Key:      "settings",
			Action:   "system-timezone",
			Obj:      []byte(fmt.Sprintf(`{"timeLocation":%q}`, normalized)),
		}).Error
	}

	jsonObj(c, service.GetSystemTimeZoneStatus(), nil)
}

// prepareSettingsTimeZoneSave validates an actual timezone change before the
// database or host is modified.
func (a *ApiService) prepareSettingsTimeZoneSave(changes map[string]string, systemTimeLocation string) (map[string]string, func() error, bool, error) {
	settings := make(map[string]string, len(changes))
	for key, value := range changes {
		settings[key] = value
	}

	panelRequested, panelRequestedPresent := settings["timeLocation"]
	if panelRequestedPresent {
		normalized, err := service.NormalizePanelTimeLocation(panelRequested)
		if err != nil {
			return nil, nil, false, err
		}
		if err := service.ValidatePanelTimeZoneLocal(normalized); err != nil {
			return nil, nil, false, err
		}
		settings["timeLocation"] = normalized
	}

	systemChanged := false
	previousSystemTimeLocation := ""
	normalizedSystemRequested := ""
	if strings.TrimSpace(systemTimeLocation) != "" {
		status := service.GetSystemTimeZoneStatus()
		if !status.CanModify {
			reason := strings.TrimSpace(status.Reason)
			if reason == "" {
				reason = "当前面板进程没有修改系统时区的权限"
			}
			return nil, nil, false, fmt.Errorf("%s", reason)
		}

		var err error
		normalizedSystemRequested, err = service.NormalizePanelTimeLocation(systemTimeLocation)
		if err != nil {
			return nil, nil, false, err
		}
		if !service.IsSelectableTimeLocation(normalizedSystemRequested) {
			return nil, nil, false, fmt.Errorf("系统时区只能选择面板提供的时区")
		}
		if err := service.ValidatePanelTimeZoneLocal(normalizedSystemRequested); err != nil {
			return nil, nil, false, err
		}
		previousSystemTimeLocation = service.GetCurrentSystemTimeLocation()
		if previousSystemTimeLocation == "" {
			return nil, nil, false, fmt.Errorf("无法读取当前 Linux 系统时区，为保证失败可回滚，已拒绝修改")
		}
		systemChanged = previousSystemTimeLocation != normalizedSystemRequested
	}

	if !systemChanged {
		return settings, nil, false, nil
	}

	if err := service.SetSystemTimeLocation(normalizedSystemRequested); err != nil {
		return nil, nil, false, err
	}
	rollback := func() error {
		return service.RestoreSystemTimeLocation(previousSystemTimeLocation)
	}
	return settings, rollback, true, nil
}
