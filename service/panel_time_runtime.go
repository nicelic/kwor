package service

import "sync"

// PanelTimeScheduleReloader is implemented by the app package. Keeping this
// small callback in service avoids a service -> app import cycle while making
// a saved panel timezone take effect for cron calendar schedules immediately.
type PanelTimeScheduleReloader interface {
	ReloadPanelTimeSchedule() error
}

var panelTimeScheduleRuntime struct {
	mu       sync.RWMutex
	reloader PanelTimeScheduleReloader
}

func RegisterPanelTimeScheduleReloader(reloader PanelTimeScheduleReloader) {
	panelTimeScheduleRuntime.mu.Lock()
	panelTimeScheduleRuntime.reloader = reloader
	panelTimeScheduleRuntime.mu.Unlock()
}

func ReloadPanelTimeSchedule() error {
	panelTimeScheduleRuntime.mu.RLock()
	reloader := panelTimeScheduleRuntime.reloader
	panelTimeScheduleRuntime.mu.RUnlock()
	if reloader == nil {
		return nil
	}
	return reloader.ReloadPanelTimeSchedule()
}
