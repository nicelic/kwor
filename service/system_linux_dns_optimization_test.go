package service

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/alireza0/s-ui/database"
)

func TestExtractActiveLinuxNameServers(t *testing.T) {
	content := "#nameserver 1.1.1.1\n  # nameserver 9.9.9.9\nnameserver 8.8.8.8\nnameserver 1.1.1.1 # keep\n"
	got := extractActiveLinuxNameServers(content)
	want := []string{"8.8.8.8", "1.1.1.1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("extractActiveLinuxNameServers = %#v, want %#v", got, want)
	}
}

func TestReplaceActiveLinuxNameServers(t *testing.T) {
	content := "# head\nnameserver 1.1.1.1\noptions edns0\n#nameserver 9.9.9.9\nnameserver 8.8.8.8\n"
	got := replaceActiveLinuxNameServers(content, []string{"4.4.4.4"})
	want := "# head\nnameserver 4.4.4.4\noptions edns0\n#nameserver 9.9.9.9\n"
	if got != want {
		t.Fatalf("replaceActiveLinuxNameServers = %q, want %q", got, want)
	}
}

func TestReplaceActiveLinuxNameServersWhenNoActive(t *testing.T) {
	content := "# only comments\noptions rotate\n"
	got := replaceActiveLinuxNameServers(content, []string{"1.1.1.1", "8.8.8.8"})
	want := "# only comments\noptions rotate\nnameserver 1.1.1.1\nnameserver 8.8.8.8\n"
	if got != want {
		t.Fatalf("replaceActiveLinuxNameServers(no-active) = %q, want %q", got, want)
	}
}

func TestNormalizeLinuxNameServerInput(t *testing.T) {
	got := normalizeLinuxNameServerInput("1.1.1.1, 8.8.8.8\n9.9.9.9")
	want := []string{"1.1.1.1", "8.8.8.8", "9.9.9.9"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeLinuxNameServerInput = %#v, want %#v", got, want)
	}
}

func TestNormalizeLinuxNameServerInputText(t *testing.T) {
	got := normalizeLinuxNameServerInputText("1.1.1.1   1.0.0.1\r\n\r\n2606:4700:4700::1111   2606:4700:4700::1001")
	want := "1.1.1.1 1.0.0.1\n2606:4700:4700::1111 2606:4700:4700::1001"
	if got != want {
		t.Fatalf("normalizeLinuxNameServerInputText = %q, want %q", got, want)
	}
}

func TestBuildLinuxDNSNameServersInputFromContent(t *testing.T) {
	content := "# comment\nnameserver 1.1.1.1\nnameserver 1.0.0.1\noptions edns0\nnameserver 2606:4700:4700::1111\n"
	got := buildLinuxDNSNameServersInputFromContent(content, []string{"1.1.1.1", "1.0.0.1", "2606:4700:4700::1111"})
	want := "1.1.1.1\n1.0.0.1\n2606:4700:4700::1111"
	if got != want {
		t.Fatalf("buildLinuxDNSNameServersInputFromContent = %q, want %q", got, want)
	}
}

func TestBuildLinuxDNSNameServersInputFromContentFallback(t *testing.T) {
	got := buildLinuxDNSNameServersInputFromContent("", []string{"1.1.1.1", "1.0.0.1"})
	want := "1.1.1.1 1.0.0.1"
	if got != want {
		t.Fatalf("buildLinuxDNSNameServersInputFromContent fallback = %q, want %q", got, want)
	}
}

func initSystemLinuxDNSTestSettingService(t *testing.T) *SettingService {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "system-linux-dns-settings.db")
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

func TestResolveManagedLinuxDNSNameServersInputPreservesSavedLayout(t *testing.T) {
	settingService := initSystemLinuxDNSTestSettingService(t)
	svc := &SystemLinuxDNSOptimizationService{SettingService: *settingService}

	if err := settingService.setString(systemLinuxDNSNameServersInputKey, "1.1.1.1 1.0.0.1"); err != nil {
		t.Fatalf("set saved dns input failed: %v", err)
	}

	content := "nameserver 1.1.1.1\nnameserver 1.0.0.1\n"
	got, err := svc.resolveManagedLinuxDNSNameServersInput(content, []string{"1.1.1.1", "1.0.0.1"})
	if err != nil {
		t.Fatalf("resolveManagedLinuxDNSNameServersInput failed: %v", err)
	}
	if got != "1.1.1.1 1.0.0.1" {
		t.Fatalf("resolved input = %q, want %q", got, "1.1.1.1 1.0.0.1")
	}
}

func TestResolveManagedLinuxDNSNameServersInputClearsMismatchedSavedLayout(t *testing.T) {
	settingService := initSystemLinuxDNSTestSettingService(t)
	svc := &SystemLinuxDNSOptimizationService{SettingService: *settingService}

	if err := settingService.setString(systemLinuxDNSNameServersInputKey, "1.1.1.1 1.0.0.1 9.9.9.9"); err != nil {
		t.Fatalf("set saved dns input failed: %v", err)
	}

	content := "nameserver 1.1.1.1\nnameserver 1.0.0.1\n"
	got, err := svc.resolveManagedLinuxDNSNameServersInput(content, []string{"1.1.1.1", "1.0.0.1"})
	if err != nil {
		t.Fatalf("resolveManagedLinuxDNSNameServersInput failed: %v", err)
	}
	want := "1.1.1.1\n1.0.0.1"
	if got != want {
		t.Fatalf("resolved input = %q, want %q", got, want)
	}

	saved, err := settingService.getString(systemLinuxDNSNameServersInputKey)
	if err != nil {
		t.Fatalf("get saved dns input failed: %v", err)
	}
	if saved != want {
		t.Fatalf("saved input = %q, want %q", saved, want)
	}
}
