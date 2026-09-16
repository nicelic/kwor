package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func installPanelTimeTestHooks(t *testing.T, detector func() string) {
	t.Helper()

	oldDetector := panelTimeSystemLocationDetector
	oldNow := panelTimeNow
	panelTimeSystemLocationDetector = detector
	InvalidatePanelTimeLocationCache()

	t.Cleanup(func() {
		panelTimeSystemLocationDetector = oldDetector
		panelTimeNow = oldNow
		InvalidatePanelTimeLocationCache()
	})
}

func TestEnsurePanelTimeLocationUsesDetectedSystemLocation(t *testing.T) {
	settingService := initTimeLocationSettingTestDB(t)
	if err := settingService.ResetSettings(); err != nil {
		t.Fatalf("clear settings failed: %v", err)
	}
	installPanelTimeTestHooks(t, func() string {
		return "Asia/Shanghai"
	})

	got, err := settingService.EnsurePanelTimeLocation()
	if err != nil {
		t.Fatalf("EnsurePanelTimeLocation failed: %v", err)
	}
	if got != "Asia/Shanghai" {
		t.Fatalf("timezone=%q want Asia/Shanghai", got)
	}
	stored, exists, err := settingService.storedPanelTimeLocation()
	if err != nil || !exists || stored != "Asia/Shanghai" {
		t.Fatalf("stored timezone=%q exists=%v err=%v", stored, exists, err)
	}
}

func TestEnsurePanelTimeLocationFallsBackToUTCWhenSystemInvalid(t *testing.T) {
	settingService := initTimeLocationSettingTestDB(t)
	if err := settingService.ResetSettings(); err != nil {
		t.Fatalf("clear settings failed: %v", err)
	}
	installPanelTimeTestHooks(t, func() string {
		return "Invalid/Nonexistent_Timezone"
	})

	got, err := settingService.EnsurePanelTimeLocation()
	if err != nil {
		t.Fatalf("EnsurePanelTimeLocation failed: %v", err)
	}
	if got != "UTC" {
		t.Fatalf("timezone=%q want UTC", got)
	}
	context, err := settingService.GetPanelTimeContext()
	if err != nil {
		t.Fatalf("GetPanelTimeContext failed: %v", err)
	}
	if !context.Selectable || context.TimeLocation != "UTC" {
		t.Fatalf("UTC timezone should be valid and selectable: %#v", context)
	}
}

func TestEnsurePanelTimeLocationCoalescesConcurrentRecovery(t *testing.T) {
	settingService := initTimeLocationSettingTestDB(t)
	if err := settingService.ResetSettings(); err != nil {
		t.Fatalf("clear settings failed: %v", err)
	}
	installPanelTimeTestHooks(t, func() string {
		return "Asia/Shanghai"
	})

	const workers = 8
	start := make(chan struct{})
	errs := make(chan error, workers)
	var waitGroup sync.WaitGroup
	for i := 0; i < workers; i++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			_, err := settingService.EnsurePanelTimeLocation()
			errs <- err
		}()
	}
	close(start)
	waitGroup.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent recovery failed: %v", err)
		}
	}

	var rows int64
	if err := database.GetDB().Model(&model.Setting{}).Where("key = ?", "timeLocation").Count(&rows).Error; err != nil {
		t.Fatalf("count timezone rows failed: %v", err)
	}
	if rows != 1 {
		t.Fatalf("timeLocation rows=%d want 1", rows)
	}
}

func TestInitializePanelTimeOnStartupKeepsExistingValidValue(t *testing.T) {
	settingService := initTimeLocationSettingTestDB(t)
	if err := settingService.SaveSetting("timeLocation", "Asia/Tokyo"); err != nil {
		t.Fatalf("save existing timezone failed: %v", err)
	}
	installPanelTimeTestHooks(t, func() string {
		return "UTC"
	})

	if err := settingService.InitializePanelTimeOnStartup(); err != nil {
		t.Fatalf("startup initialization should retain valid database value: %v", err)
	}
	got, exists, err := settingService.storedPanelTimeLocation()
	if err != nil || !exists || got != "Asia/Tokyo" {
		t.Fatalf("stored timezone=%q exists=%v err=%v", got, exists, err)
	}
}

func TestSettingSaveNeverPersistsTemporarySystemTimeZoneField(t *testing.T) {
	settingService := initTimeLocationSettingTestDB(t)
	payload, err := json.Marshal(map[string]string{
		"systemTimeLocation": "Asia/Shanghai",
	})
	if err != nil {
		t.Fatalf("marshal payload failed: %v", err)
	}
	if err := settingService.Save(database.GetDB(), payload); err != nil {
		t.Fatalf("settings save failed: %v", err)
	}
	if _, err := settingService.getSetting("systemTimeLocation"); !database.IsNotFound(err) {
		t.Fatalf("temporary system timezone was persisted, err=%v", err)
	}
}

func TestSettingSaveKeepsPanelTimeCacheUntilTransactionCommit(t *testing.T) {
	settingService := initTimeLocationSettingTestDB(t)
	if err := settingService.SaveSetting("timeLocation", "UTC"); err != nil {
		t.Fatalf("seed panel timezone failed: %v", err)
	}
	if _, err := settingService.GetPanelTimeLocation(); err != nil {
		t.Fatalf("prime panel timezone cache failed: %v", err)
	}

	tx := database.GetDB().Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction failed: %v", tx.Error)
	}
	t.Cleanup(func() { tx.Rollback() })
	payload, err := json.Marshal(map[string]string{"timeLocation": "Asia/Tokyo"})
	if err != nil {
		t.Fatalf("marshal payload failed: %v", err)
	}
	if err := settingService.Save(tx, payload); err != nil {
		t.Fatalf("save timezone in transaction failed: %v", err)
	}

	name, _, cached := cachedPanelLocation()
	if !cached || name != "UTC" {
		t.Fatalf("uncommitted timezone save invalidated cache: name=%q cached=%v", name, cached)
	}
}

func TestLocalTimeValidation(t *testing.T) {
	if err := ValidatePanelTimeZoneLocal("UTC"); err != nil {
		t.Fatalf("ValidatePanelTimeZoneLocal(UTC) failed: %v", err)
	}
	if err := ValidatePanelTimeZoneLocal("Asia/Shanghai"); err != nil {
		t.Fatalf("ValidatePanelTimeZoneLocal(Asia/Shanghai) failed: %v", err)
	}
	if err := ValidatePanelTimeZoneLocal("Invalid/Nonexistent"); err == nil {
		t.Fatal("ValidatePanelTimeZoneLocal(Invalid/Nonexistent) should fail")
	}
}

func TestPanelNowUsesPanelCalendarLocation(t *testing.T) {
	settingService := initTimeLocationSettingTestDB(t)
	if err := settingService.SaveSetting("timeLocation", "Pacific/Auckland"); err != nil {
		t.Fatalf("save timezone failed: %v", err)
	}
	installPanelTimeTestHooks(t, func() string { return "UTC" })
	fixed := time.Date(2026, time.July, 22, 12, 30, 0, 0, time.UTC)
	panelTimeNow = func() time.Time { return fixed }

	now := PanelNow()
	if now.Location().String() != "Pacific/Auckland" {
		t.Fatalf("location=%q want Pacific/Auckland", now.Location())
	}
	if now.Day() != 23 {
		t.Fatalf("calendar date boundary did not use panel timezone: %s", now)
	}
}

func TestSystemTimeZoneStatusHidesValueWithoutPermission(t *testing.T) {
	oldIsLinux := systemTimeZoneIsLinux
	oldGeteuid := systemTimeZoneGeteuid
	oldDetector := systemTimeZoneDetector
	systemTimeZoneIsLinux = func() bool { return true }
	systemTimeZoneGeteuid = func() int { return 1000 }
	systemTimeZoneDetector = func() string { return "Asia/Shanghai" }
	t.Cleanup(func() {
		systemTimeZoneIsLinux = oldIsLinux
		systemTimeZoneGeteuid = oldGeteuid
		systemTimeZoneDetector = oldDetector
	})

	status := GetSystemTimeZoneStatus()
	if status.CanModify || status.Displayable || status.TimeLocation != "" {
		t.Fatalf("permission-limited status leaked a selectable timezone: %#v", status)
	}
}

func TestSetSystemTimeLocationUsesFileFallbackWhenTimedatectlUnavailable(t *testing.T) {
	oldIsLinux := systemTimeZoneIsLinux
	oldGeteuid := systemTimeZoneGeteuid
	oldDetector := systemTimeZoneDetector
	oldCommand := systemTimeZoneCommandRunner
	oldFileApplier := systemTimeZoneFileApplier
	systemTimeZoneIsLinux = func() bool { return true }
	systemTimeZoneGeteuid = func() int { return 0 }
	systemTimeZoneDetector = func() string { return "UTC" }
	systemTimeZoneCommandRunner = func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name != "timedatectl" {
			t.Fatalf("unexpected command %q", name)
		}
		return []byte("systemd unavailable"), errors.New("not found")
	}
	called := ""
	systemTimeZoneFileApplier = func(location string) error {
		called = location
		return nil
	}
	t.Cleanup(func() {
		systemTimeZoneIsLinux = oldIsLinux
		systemTimeZoneGeteuid = oldGeteuid
		systemTimeZoneDetector = oldDetector
		systemTimeZoneCommandRunner = oldCommand
		systemTimeZoneFileApplier = oldFileApplier
	})

	if err := SetSystemTimeLocation("Asia/Shanghai"); err != nil {
		t.Fatalf("SetSystemTimeLocation failed: %v", err)
	}
	if called != "Asia/Shanghai" {
		t.Fatalf("fallback location=%q", called)
	}
}

func TestSetSystemTimeLocationRestoresPreviousZoneWhenFileFallbackFails(t *testing.T) {
	oldIsLinux := systemTimeZoneIsLinux
	oldGeteuid := systemTimeZoneGeteuid
	oldDetector := systemTimeZoneDetector
	oldCommand := systemTimeZoneCommandRunner
	oldFileApplier := systemTimeZoneFileApplier
	systemTimeZoneIsLinux = func() bool { return true }
	systemTimeZoneGeteuid = func() int { return 0 }
	systemTimeZoneDetector = func() string { return "UTC" }
	systemTimeZoneCommandRunner = func(context.Context, string, ...string) ([]byte, error) {
		return nil, errors.New("timedatectl unavailable")
	}
	calls := make([]string, 0, 2)
	systemTimeZoneFileApplier = func(location string) error {
		calls = append(calls, location)
		if location == "Asia/Shanghai" {
			return errors.New("write timezone file failed")
		}
		return nil
	}
	t.Cleanup(func() {
		systemTimeZoneIsLinux = oldIsLinux
		systemTimeZoneGeteuid = oldGeteuid
		systemTimeZoneDetector = oldDetector
		systemTimeZoneCommandRunner = oldCommand
		systemTimeZoneFileApplier = oldFileApplier
	})

	if err := SetSystemTimeLocation("Asia/Shanghai"); err == nil {
		t.Fatal("expected fallback failure")
	}
	if len(calls) != 2 || calls[0] != "Asia/Shanghai" || calls[1] != "UTC" {
		t.Fatalf("fallback/restore calls=%v", calls)
	}
}

func TestRestoreSystemTimeLocationAllowsValidZoneOutsideSelector(t *testing.T) {
	oldIsLinux := systemTimeZoneIsLinux
	oldGeteuid := systemTimeZoneGeteuid
	oldDetector := systemTimeZoneDetector
	oldCommand := systemTimeZoneCommandRunner
	systemTimeZoneIsLinux = func() bool { return true }
	systemTimeZoneGeteuid = func() int { return 0 }
	systemTimeZoneDetector = func() string { return "UTC" }
	called := ""
	systemTimeZoneCommandRunner = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		called = strings.Join(args, " ")
		return nil, nil
	}
	t.Cleanup(func() {
		systemTimeZoneIsLinux = oldIsLinux
		systemTimeZoneGeteuid = oldGeteuid
		systemTimeZoneDetector = oldDetector
		systemTimeZoneCommandRunner = oldCommand
	})

	if err := RestoreSystemTimeLocation("Europe/Copenhagen"); err != nil {
		t.Fatalf("restore non-list IANA zone failed: %v", err)
	}
	if called != "set-timezone Europe/Copenhagen" {
		t.Fatalf("unexpected restore command: %q", called)
	}
}

type panelTimeScheduleReloaderStub struct {
	calls int
	err   error
}

func (s *panelTimeScheduleReloaderStub) ReloadPanelTimeSchedule() error {
	s.calls++
	return s.err
}

func TestReloadPanelTimeScheduleUsesRegisteredRuntime(t *testing.T) {
	stub := &panelTimeScheduleReloaderStub{}
	RegisterPanelTimeScheduleReloader(stub)
	t.Cleanup(func() { RegisterPanelTimeScheduleReloader(nil) })

	if err := ReloadPanelTimeSchedule(); err != nil {
		t.Fatalf("ReloadPanelTimeSchedule failed: %v", err)
	}
	if stub.calls != 1 {
		t.Fatalf("reload calls=%d want 1", stub.calls)
	}
}
