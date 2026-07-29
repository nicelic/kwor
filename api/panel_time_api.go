package api

import (
	"fmt"
	"strings"

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

// prepareSettingsTimeZoneSave validates an actual timezone change before the
// database or host is modified. The caller serializes system timezone saves
// and uses the rollback only when the following SQLite CAS fails.
func (a *ApiService) prepareSettingsTimeZoneSave(changes map[string]string, systemTimeLocation string) (map[string]string, func() error, bool, error) {
	settings := make(map[string]string, len(changes))
	for key, value := range changes {
		settings[key] = value
	}

	panelRequested, panelRequestedPresent := settings["timeLocation"]
	panelChanged := false
	if panelRequestedPresent {
		normalized, err := service.NormalizePanelTimeLocation(panelRequested)
		if err != nil {
			return nil, nil, false, err
		}
		settings["timeLocation"] = normalized
		current, err := a.SettingService.GetPanelTimeLocation()
		if err != nil {
			return nil, nil, false, err
		}
		panelChanged = current == nil || current.String() != normalized
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
		previousSystemTimeLocation = service.GetCurrentSystemTimeLocation()
		if previousSystemTimeLocation == "" {
			return nil, nil, false, fmt.Errorf("无法读取当前 Linux 系统时区，为保证失败可回滚，已拒绝修改")
		}
		systemChanged = previousSystemTimeLocation != normalizedSystemRequested
	}

	// A remote request is deliberately made only for a real user mutation. A
	// regular settings save with the same timezone does not create background
	// work or network traffic.
	validations := make(map[string]struct{}, 2)
	if panelChanged {
		validations[settings["timeLocation"]] = struct{}{}
	}
	if systemChanged {
		validations[normalizedSystemRequested] = struct{}{}
	}
	for location := range validations {
		if err := service.ValidatePanelTimeZoneRemote(location); err != nil {
			return nil, nil, false, err
		}
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
