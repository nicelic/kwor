package service

import (
	"path/filepath"
	"sync"
	"sync/atomic"
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

func TestFirewallOverviewCoalescesConcurrentDiagnostics(t *testing.T) {
	openSettingsOverviewTestDB(t)
	originalSupported := firewallSupportedFn
	var calls atomic.Int32
	started := make(chan struct{})
	firewallSupportedFn = func() bool {
		if calls.Add(1) == 1 {
			close(started)
		}
		time.Sleep(80 * time.Millisecond)
		return false
	}
	t.Cleanup(func() { firewallSupportedFn = originalSupported })

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := (&FirewallService{}).GetOverview()
		errs <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first firewall overview did not start")
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := (&FirewallService{}).GetOverview()
		errs <- err
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("firewall overview failed: %v", err)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("concurrent overview diagnostics=%d want=1", got)
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

func TestPortForwardRuntimeOverviewDoesNotWaitForSQLite(t *testing.T) {
	openSettingsOverviewTestDB(t)

	tx := database.GetDB().Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction: %v", tx.Error)
	}
	defer func() { _ = tx.Rollback().Error }()
	if err := tx.Exec("SELECT 1").Error; err != nil {
		t.Fatalf("hold sqlite connection: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := (&PortForwardService{}).GetRuntimeOverview()
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("port-forward runtime overview failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("port-forward runtime overview waited for SQLite")
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
