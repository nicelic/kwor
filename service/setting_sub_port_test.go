package service

import (
	"path/filepath"
	"strconv"
	"testing"

	"github.com/alireza0/s-ui/database"
)

func initSubPortSettingTestDB(t *testing.T) *SettingService {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "sub-port-settings.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	if sqlDB, err := database.GetDB().DB(); err == nil && sqlDB != nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}

	return &SettingService{}
}

func TestBuildInitialRandomSubPortSequence_WrapsByTen(t *testing.T) {
	ports := buildInitialRandomSubPortSequence(64995)
	if len(ports) == 0 {
		t.Fatal("expected non-empty port sequence")
	}
	if ports[0] != 64990 {
		t.Fatalf("first port = %d, want 64990", ports[0])
	}
	if len(ports) < 3 {
		t.Fatalf("sequence too short: %d", len(ports))
	}
	if ports[1] != 65000 {
		t.Fatalf("second port = %d, want 65000", ports[1])
	}
	if ports[2] != 25000 {
		t.Fatalf("third port = %d, want 25000", ports[2])
	}
}

func TestChooseInitialRandomSubPortFromStart_SkipsUnavailablePortsByTen(t *testing.T) {
	checks := 0
	port, err := chooseInitialRandomSubPortFromStart(29003, func(candidate int) bool {
		checks++
		return candidate == 29020
	})
	if err != nil {
		t.Fatalf("chooseInitialRandomSubPortFromStart failed: %v", err)
	}
	if port != 29020 {
		t.Fatalf("selected port = %d, want 29020", port)
	}
	if checks != 3 {
		t.Fatalf("checks = %d, want 3", checks)
	}
}

func TestSettingServiceGetAllSetting_GeneratesInitialRandomSubPort(t *testing.T) {
	settingService := initSubPortSettingTestDB(t)

	settings, err := settingService.GetAllSetting()
	if err != nil {
		t.Fatalf("GetAllSetting failed: %v", err)
	}

	subPortText := (*settings)["subPort"]
	subPort, err := strconv.Atoi(subPortText)
	if err != nil {
		t.Fatalf("subPort %q is not numeric: %v", subPortText, err)
	}
	if subPort < initialRandomSubPortMin || subPort > initialRandomSubPortMax {
		t.Fatalf("subPort = %d, want within %d-%d", subPort, initialRandomSubPortMin, initialRandomSubPortMax)
	}
	if (subPort-initialRandomSubPortMin)%initialRandomSubPortStep != 0 {
		t.Fatalf("subPort = %d, want step %d alignment", subPort, initialRandomSubPortStep)
	}

	storedPort, err := settingService.GetSubPort()
	if err != nil {
		t.Fatalf("GetSubPort failed: %v", err)
	}
	if storedPort != subPort {
		t.Fatalf("stored subPort mismatch: got=%d want=%d", storedPort, subPort)
	}
}

func TestSettingServiceGetAllSetting_PreservesExistingSubPort(t *testing.T) {
	settingService := initSubPortSettingTestDB(t)
	if err := settingService.SetSubPort(58733); err != nil {
		t.Fatalf("SetSubPort failed: %v", err)
	}

	settings, err := settingService.GetAllSetting()
	if err != nil {
		t.Fatalf("GetAllSetting failed: %v", err)
	}

	if got := (*settings)["subPort"]; got != "58733" {
		t.Fatalf("subPort = %q, want %q", got, "58733")
	}
}

func TestSettingServiceGetAllSetting_RegeneratesBlankSubPort(t *testing.T) {
	settingService := initSubPortSettingTestDB(t)
	if err := settingService.SaveSetting("subPort", "   "); err != nil {
		t.Fatalf("SaveSetting failed: %v", err)
	}

	settings, err := settingService.GetAllSetting()
	if err != nil {
		t.Fatalf("GetAllSetting failed: %v", err)
	}

	subPortText := (*settings)["subPort"]
	subPort, err := strconv.Atoi(subPortText)
	if err != nil {
		t.Fatalf("subPort %q is not numeric: %v", subPortText, err)
	}
	if subPort < initialRandomSubPortMin || subPort > initialRandomSubPortMax {
		t.Fatalf("subPort = %d, want within %d-%d", subPort, initialRandomSubPortMin, initialRandomSubPortMax)
	}
}

func TestSettingServiceGetAllSetting_RegeneratesInvalidSubPort(t *testing.T) {
	settingService := initSubPortSettingTestDB(t)
	if err := settingService.SaveSetting("subPort", "bad-port"); err != nil {
		t.Fatalf("SaveSetting failed: %v", err)
	}

	settings, err := settingService.GetAllSetting()
	if err != nil {
		t.Fatalf("GetAllSetting failed: %v", err)
	}

	subPortText := (*settings)["subPort"]
	subPort, err := strconv.Atoi(subPortText)
	if err != nil {
		t.Fatalf("subPort %q is not numeric: %v", subPortText, err)
	}
	if subPort < initialRandomSubPortMin || subPort > initialRandomSubPortMax {
		t.Fatalf("subPort = %d, want within %d-%d", subPort, initialRandomSubPortMin, initialRandomSubPortMax)
	}
}
