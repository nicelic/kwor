package service

import (
	"strconv"

	"strings"

	"time"
)

// coreAutoCheckSettingKeys keeps the sing-box and Mihomo schedules on the
// same calendar rules while preserving their independent persisted settings.
type coreAutoCheckSettingKeys struct {
	enabled            string
	intervalHours      string
	lastCheckedAt      string
	firstCheckAt       string
	firstCheckTimeZone string
}

type coreAutoCheckSchedule struct {
	Enabled            bool
	IntervalHours      int
	LastCheckedAt      int64
	FirstCheckAt       int64
	FirstCheckTimeZone string
}

func loadCoreAutoCheckSchedule(settingSvc *SettingService, keys coreAutoCheckSettingKeys) (coreAutoCheckSchedule, error) {
	enabled, err := settingSvc.getBool(keys.enabled)
	if err != nil {
		return coreAutoCheckSchedule{}, err
	}

	intervalRaw, err := settingSvc.getString(keys.intervalHours)
	if err != nil {
		return coreAutoCheckSchedule{}, err
	}
	lastRaw, err := settingSvc.getString(keys.lastCheckedAt)
	if err != nil {
		return coreAutoCheckSchedule{}, err
	}
	firstRaw, err := settingSvc.getString(keys.firstCheckAt)
	if err != nil {
		return coreAutoCheckSchedule{}, err
	}
	firstTimeZone, err := settingSvc.getString(keys.firstCheckTimeZone)
	if err != nil {
		return coreAutoCheckSchedule{}, err
	}

	return coreAutoCheckSchedule{
		Enabled:            enabled,
		IntervalHours:      normalizeCoreAutoCheckIntervalHours(intervalRaw),
		LastCheckedAt:      parseCoreAutoCheckUnixSetting(lastRaw),
		FirstCheckAt:       parseCoreAutoCheckUnixSetting(firstRaw),
		FirstCheckTimeZone: strings.TrimSpace(firstTimeZone),
	}, nil
}

func parseCoreAutoCheckUnixSetting(raw string) int64 {
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || value < 0 {
		return 0
	}
	return value
}

func nextPanelMidnight(now time.Time) time.Time {
	location := now.Location()
	if location == nil {
		location = time.UTC
		now = now.UTC()
	}
	return time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, location)
}

func scheduleCoreAutoCheckFirstRunLocked(settingSvc *SettingService, keys coreAutoCheckSettingKeys) error {
	now := PanelNow()
	if err := settingSvc.setString(keys.firstCheckAt, strconv.FormatInt(nextPanelMidnight(now).Unix(), 10)); err != nil {
		return err
	}
	return settingSvc.setString(keys.firstCheckTimeZone, now.Location().String())
}

func clearCoreAutoCheckFirstRunLocked(settingSvc *SettingService, keys coreAutoCheckSettingKeys) error {
	if err := settingSvc.setString(keys.firstCheckAt, "0"); err != nil {
		return err
	}
	return settingSvc.setString(keys.firstCheckTimeZone, "")
}

func ensureCoreAutoCheckFirstRunLocked(settingSvc *SettingService, keys coreAutoCheckSettingKeys, schedule coreAutoCheckSchedule) (int64, error) {
	now := PanelNow()
	currentTimeZone := now.Location().String()
	if schedule.FirstCheckAt > 0 && schedule.FirstCheckTimeZone == currentTimeZone {
		return schedule.FirstCheckAt, nil
	}
	if schedule.FirstCheckAt == 0 && schedule.LastCheckedAt > 0 {
		return 0, nil
	}
	if err := scheduleCoreAutoCheckFirstRunLocked(settingSvc, keys); err != nil {
		return 0, err
	}
	return nextPanelMidnight(now).Unix(), nil
}

func shouldRunCoreAutoCheckLocked(settingSvc *SettingService, keys coreAutoCheckSettingKeys, schedule coreAutoCheckSchedule, force bool) (bool, error) {
	if !schedule.Enabled {
		return false, nil
	}
	if force {
		return true, nil
	}

	now := PanelNow().Unix()
	if schedule.FirstCheckAt > 0 {
		firstCheckAt, err := ensureCoreAutoCheckFirstRunLocked(settingSvc, keys, schedule)
		if err != nil {
			return false, err
		}
		return now >= firstCheckAt, nil
	}
	if schedule.LastCheckedAt > 0 {
		nextDueAt := schedule.LastCheckedAt + int64(schedule.IntervalHours)*int64(time.Hour/time.Second)
		return now >= nextDueAt, nil
	}

	firstCheckAt, err := ensureCoreAutoCheckFirstRunLocked(settingSvc, keys, schedule)
	if err != nil {
		return false, err
	}
	return now >= firstCheckAt, nil
}

func ReschedulePendingCoreAutoChecksForPanelTimeZone() error {
	if err := (&CoreManagerService{}).ReschedulePendingCoreAutoCheckForPanelTimeZone(); err != nil {
		return err
	}
	return (&MihomoCoreManagerService{}).ReschedulePendingCoreAutoCheckForPanelTimeZone()
}
