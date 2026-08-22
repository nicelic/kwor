package service

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database"
)

func TestDefaultSystemSysctlContentUsesConservativeTCPBufferMaximum(t *testing.T) {
	for _, forbidden := range []string{"360000000", "36000000 360000000"} {
		if strings.Contains(defaultSystemSysctlContent, forbidden) {
			t.Fatalf("default sysctl profile still contains %q", forbidden)
		}
	}
	for _, expected := range []string{
		"net.ipv4.tcp_rmem=4096 131072 16777216",
		"net.ipv4.tcp_wmem=4096 16384 16777216",
	} {
		if !strings.Contains(defaultSystemSysctlContent, expected) {
			t.Fatalf("default sysctl profile is missing %q", expected)
		}
	}
}

func TestLegacyDefaultSystemSysctlContentIsDistinctFromCurrentDefault(t *testing.T) {
	if normalizeManagedSysctlContent(legacyDefaultSystemSysctlContent) == normalizeManagedSysctlContent(defaultSystemSysctlContent) {
		t.Fatal("legacy sysctl profile must remain distinguishable for exact migration")
	}
}

func TestMigrateLegacyDefaultSystemSysctlContentOnlyMigratesExactDefault(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "sysctl-default.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("init sysctl db failed: %v", err)
	}
	if sqlDB, err := database.GetDB().DB(); err == nil && sqlDB != nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}

	svc := &SystemSysctlOptimizationService{}
	if err := svc.setString(systemSysctlContentKey, legacyDefaultSystemSysctlContent); err != nil {
		t.Fatalf("set legacy default: %v", err)
	}
	migrated, err := svc.migrateLegacyDefaultContentOnStartup()
	if err != nil || !migrated {
		t.Fatalf("legacy default migration = (%v, %v), want (true, nil)", migrated, err)
	}
	content, err := svc.getString(systemSysctlContentKey)
	if err != nil || content != defaultSystemSysctlContent {
		t.Fatalf("migrated content = %q, %v", content, err)
	}

	custom := legacyDefaultSystemSysctlContent + "net.ipv4.ip_forward=1\n"
	if err := svc.setString(systemSysctlContentKey, custom); err != nil {
		t.Fatalf("set custom content: %v", err)
	}
	migrated, err = svc.migrateLegacyDefaultContentOnStartup()
	if err != nil || migrated {
		t.Fatalf("custom content migration = (%v, %v), want (false, nil)", migrated, err)
	}
	content, err = svc.getString(systemSysctlContentKey)
	if err != nil || content != custom {
		t.Fatalf("custom content was changed: %q, %v", content, err)
	}
}
