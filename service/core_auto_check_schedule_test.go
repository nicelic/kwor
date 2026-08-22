package service

import (
	"strconv"

	"strings"

	"testing"

	"time"
)

func installCoreAutoCheckTestNow(t *testing.T, now time.Time) {
	t.Helper()
	previous := panelTimeNow
	panelTimeNow = func() time.Time { return now }
	InvalidatePanelTimeLocationCache()
	t.Cleanup(func() {
		panelTimeNow = previous
		InvalidatePanelTimeLocationCache()
	})
}

func coreAutoCheckSettingValue(t *testing.T, settingSvc *SettingService, key string) string {
	t.Helper()
	value, err := settingSvc.getString(key)
	if err != nil {
		t.Fatalf("read %s: %v", key, err)
	}
	return value
}

func TestCoreAutoCheckTotalSwitchAndIndependentSettings(t *testing.T) {
	settingSvc := initTimeLocationSettingTestDB(t)
	installCoreAutoCheckTestNow(t, time.Date(2026, time.August, 11, 23, 59, 0, 0, time.UTC))

	tests := []struct {
		name             string
		enableCheck      func(bool) error
		enableAutoUpdate func(bool) error
		setInterval      func(int) error
		checkKey         string
		autoUpdateKey    string
		intervalKey      string
		firstCheckKey    string
		firstTimeZoneKey string
	}{
		{
			name:             "sing-box",
			enableCheck:      (&CoreManagerService{}).SetCoreAutoCheckEnabled,
			enableAutoUpdate: (&CoreManagerService{}).SetCoreAutoUpdateEnabled,
			setInterval:      (&CoreManagerService{}).SetCoreAutoCheckInterval,
			checkKey:         coreAutoCheckEnabledKey,
			autoUpdateKey:    coreAutoUpdateEnabledKey,
			intervalKey:      coreAutoCheckIntervalHoursKey,
			firstCheckKey:    coreAutoCheckFirstAtKey,
			firstTimeZoneKey: coreAutoCheckFirstTimeZoneKey,
		},
		{
			name:             "Mihomo",
			enableCheck:      (&MihomoCoreManagerService{}).SetCoreAutoCheckEnabled,
			enableAutoUpdate: (&MihomoCoreManagerService{}).SetCoreAutoUpdateEnabled,
			setInterval:      (&MihomoCoreManagerService{}).SetCoreAutoCheckInterval,
			checkKey:         mihomoCoreAutoCheckEnabledKey,
			autoUpdateKey:    mihomoCoreAutoUpdateEnabledKey,
			intervalKey:      mihomoCoreAutoCheckIntervalHoursKey,
			firstCheckKey:    mihomoCoreAutoCheckFirstAtKey,
			firstTimeZoneKey: mihomoCoreAutoCheckFirstTimeZoneKey,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := settingSvc.SaveSetting(test.checkKey, "false"); err != nil {
				t.Fatalf("reset auto check: %v", err)
			}
			if err := settingSvc.SaveSetting(test.autoUpdateKey, "false"); err != nil {
				t.Fatalf("reset auto update: %v", err)
			}

			if err := test.enableAutoUpdate(true); err == nil || !strings.Contains(err.Error(), "请先启用自动更新检查") {
				t.Fatalf("auto update should require total switch, err=%v", err)
			}
			if got := coreAutoCheckSettingValue(t, settingSvc, test.autoUpdateKey); got != "false" {
				t.Fatalf("auto update changed while total switch was off: %q", got)
			}

			if err := test.enableCheck(true); err != nil {
				t.Fatalf("enable total switch: %v", err)
			}
			if got := coreAutoCheckSettingValue(t, settingSvc, test.firstCheckKey); got != strconv.FormatInt(time.Date(2026, time.August, 12, 0, 0, 0, 0, time.UTC).Unix(), 10) {
				t.Fatalf("first check=%q, want next UTC midnight", got)
			}
			if got := coreAutoCheckSettingValue(t, settingSvc, test.firstTimeZoneKey); got != "UTC" {
				t.Fatalf("first check timezone=%q, want UTC", got)
			}

			if err := settingSvc.SaveSetting(test.autoUpdateKey, "true"); err != nil {
				t.Fatalf("seed enabled auto update: %v", err)
			}
			if err := test.setInterval(7); err != nil {
				t.Fatalf("save interval: %v", err)
			}
			if got := coreAutoCheckSettingValue(t, settingSvc, test.checkKey); got != "true" {
				t.Fatalf("interval save changed total switch: %q", got)
			}
			if got := coreAutoCheckSettingValue(t, settingSvc, test.autoUpdateKey); got != "true" {
				t.Fatalf("interval save changed auto update switch: %q", got)
			}
			if got := coreAutoCheckSettingValue(t, settingSvc, test.intervalKey); got != "7" {
				t.Fatalf("interval=%q, want 7", got)
			}

			if err := test.enableCheck(false); err != nil {
				t.Fatalf("disable total switch: %v", err)
			}
			if got := coreAutoCheckSettingValue(t, settingSvc, test.autoUpdateKey); got != "false" {
				t.Fatalf("disable total switch did not disable auto update: %q", got)
			}
			if got := coreAutoCheckSettingValue(t, settingSvc, test.firstCheckKey); got != "0" {
				t.Fatalf("disable total switch did not clear first check plan: %q", got)
			}
			if err := test.enableCheck(true); err != nil {
				t.Fatalf("re-enable total switch: %v", err)
			}
			if got := coreAutoCheckSettingValue(t, settingSvc, test.firstCheckKey); got != strconv.FormatInt(time.Date(2026, time.August, 12, 0, 0, 0, 0, time.UTC).Unix(), 10) {
				t.Fatalf("re-enable did not reset first check to the next midnight: %q", got)
			}
		})
	}
}

func TestCoreAutoCheckScheduleUsesPanelMidnightThenActualCheckInterval(t *testing.T) {
	settingSvc := initTimeLocationSettingTestDB(t)
	if err := settingSvc.SaveSetting("timeLocation", "UTC"); err != nil {
		t.Fatalf("save UTC timezone: %v", err)
	}
	installCoreAutoCheckTestNow(t, time.Date(2026, time.August, 11, 23, 59, 0, 0, time.UTC))

	if err := (&CoreManagerService{}).SetCoreAutoCheckEnabled(true); err != nil {
		t.Fatalf("enable auto check: %v", err)
	}
	schedule, err := loadCoreAutoCheckSchedule(settingSvc, singboxCoreAutoCheckSettingKeys)
	if err != nil {
		t.Fatalf("load first schedule: %v", err)
	}
	shouldRun, err := shouldRunCoreAutoCheckLocked(settingSvc, singboxCoreAutoCheckSettingKeys, schedule, false)
	if err != nil || shouldRun {
		t.Fatalf("check ran before first midnight: shouldRun=%v err=%v", shouldRun, err)
	}

	installCoreAutoCheckTestNow(t, time.Date(2026, time.August, 12, 0, 0, 0, 0, time.UTC))
	schedule, err = loadCoreAutoCheckSchedule(settingSvc, singboxCoreAutoCheckSettingKeys)
	if err != nil {
		t.Fatalf("reload first schedule: %v", err)
	}
	shouldRun, err = shouldRunCoreAutoCheckLocked(settingSvc, singboxCoreAutoCheckSettingKeys, schedule, false)
	if err != nil || !shouldRun {
		t.Fatalf("check did not run at first midnight: shouldRun=%v err=%v", shouldRun, err)
	}

	if err := settingSvc.SaveSetting(coreAutoCheckLastAtKey, strconv.FormatInt(time.Date(2026, time.August, 12, 0, 0, 0, 0, time.UTC).Unix(), 10)); err != nil {
		t.Fatalf("save last check: %v", err)
	}
	if err := (&CoreManagerService{}).SetCoreAutoCheckInterval(7); err != nil {
		t.Fatalf("save interval: %v", err)
	}
	installCoreAutoCheckTestNow(t, time.Date(2026, time.August, 12, 6, 59, 0, 0, time.UTC))
	schedule, _ = loadCoreAutoCheckSchedule(settingSvc, singboxCoreAutoCheckSettingKeys)
	shouldRun, err = shouldRunCoreAutoCheckLocked(settingSvc, singboxCoreAutoCheckSettingKeys, schedule, false)
	if err != nil || shouldRun {
		t.Fatalf("check ran before seven-hour interval: shouldRun=%v err=%v", shouldRun, err)
	}
	installCoreAutoCheckTestNow(t, time.Date(2026, time.August, 12, 7, 0, 0, 0, time.UTC))
	schedule, _ = loadCoreAutoCheckSchedule(settingSvc, singboxCoreAutoCheckSettingKeys)
	shouldRun, err = shouldRunCoreAutoCheckLocked(settingSvc, singboxCoreAutoCheckSettingKeys, schedule, false)
	if err != nil || !shouldRun {
		t.Fatalf("check did not run at seven-hour interval: shouldRun=%v err=%v", shouldRun, err)
	}
}

func TestCoreAutoCheckFirstRunFollowsChangedPanelTimeZone(t *testing.T) {
	settingSvc := initTimeLocationSettingTestDB(t)
	installCoreAutoCheckTestNow(t, time.Date(2026, time.August, 11, 23, 59, 0, 0, time.UTC))
	if err := (&MihomoCoreManagerService{}).SetCoreAutoCheckEnabled(true); err != nil {
		t.Fatalf("enable Mihomo auto check: %v", err)
	}

	if err := settingSvc.SaveSetting("timeLocation", "Asia/Tokyo"); err != nil {
		t.Fatalf("save changed timezone: %v", err)
	}
	schedule, err := loadCoreAutoCheckSchedule(settingSvc, mihomoCoreAutoCheckSettingKeys)
	if err != nil {
		t.Fatalf("load schedule: %v", err)
	}
	shouldRun, err := shouldRunCoreAutoCheckLocked(settingSvc, mihomoCoreAutoCheckSettingKeys, schedule, false)
	if err != nil || shouldRun {
		t.Fatalf("timezone change should only reschedule first run: shouldRun=%v err=%v", shouldRun, err)
	}
	if got := coreAutoCheckSettingValue(t, settingSvc, mihomoCoreAutoCheckFirstTimeZoneKey); got != "Asia/Tokyo" {
		t.Fatalf("first check timezone=%q, want Asia/Tokyo", got)
	}
	expected := time.Date(2026, time.August, 13, 0, 0, 0, 0, mustTimeLocation(t, "Asia/Tokyo")).Unix()
	if got := coreAutoCheckSettingValue(t, settingSvc, mihomoCoreAutoCheckFirstAtKey); got != strconv.FormatInt(expected, 10) {
		t.Fatalf("first check=%q, want %d", got, expected)
	}
}

func TestScheduledAutoUpdateRequiresTotalSwitch(t *testing.T) {
	settingSvc := initTimeLocationSettingTestDB(t)
	for _, test := range []struct {
		name          string
		checkKey      string
		autoUpdateKey string
		attemptKey    string
		run           func() error
	}{
		{"sing-box", coreAutoCheckEnabledKey, coreAutoUpdateEnabledKey, coreAutoUpdateLastAttemptKey, (&CoreManagerService{}).RunScheduledAutoUpdate},
		{"Mihomo", mihomoCoreAutoCheckEnabledKey, mihomoCoreAutoUpdateEnabledKey, mihomoCoreAutoUpdateLastAttemptKey, (&MihomoCoreManagerService{}).RunScheduledAutoUpdate},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := settingSvc.SaveSetting(test.checkKey, "false"); err != nil {
				t.Fatalf("disable total switch: %v", err)
			}
			if err := settingSvc.SaveSetting(test.autoUpdateKey, "true"); err != nil {
				t.Fatalf("enable auto update: %v", err)
			}
			if err := settingSvc.SaveSetting(test.attemptKey, "0"); err != nil {
				t.Fatalf("clear attempt: %v", err)
			}
			if err := test.run(); err != nil {
				t.Fatalf("scheduled update should skip when total switch is off: %v", err)
			}
			if got := coreAutoCheckSettingValue(t, settingSvc, test.attemptKey); got != "0" {
				t.Fatalf("scheduled update entered execution while total switch was off: attempt=%q", got)
			}
		})
	}
}

func mustTimeLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	location, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	return location
}
