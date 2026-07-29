package service

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/alireza0/s-ui/database"
)

func TestFirewallOverviewDoesNotWaitForReconcileLock(t *testing.T) {
	openSettingsOverviewTestDB(t)
	originalSupported := firewallSupportedFn
	firewallSupportedFn = func() bool { return false }
	t.Cleanup(func() { firewallSupportedFn = originalSupported })

	firewallStateMu.Lock()
	defer firewallStateMu.Unlock()
	done := make(chan error, 1)
	go func() {
		_, err := (&FirewallService{}).GetOverview()
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("firewall overview failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("firewall overview waited for the reconcile lock")
	}
}

func TestPortForwardOverviewDoesNotWaitForReconcileLock(t *testing.T) {
	openSettingsOverviewTestDB(t)

	portForwardStateMu.Lock()
	defer portForwardStateMu.Unlock()
	done := make(chan error, 1)
	go func() {
		_, err := (&PortForwardService{}).GetOverview()
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("port-forward overview failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("port-forward overview waited for the reconcile lock")
	}
}

func openSettingsOverviewTestDB(t *testing.T) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "settings-overview.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("init settings overview db failed: %v", err)
	}
	if sqlDB, err := database.GetDB().DB(); err == nil && sqlDB != nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
}
