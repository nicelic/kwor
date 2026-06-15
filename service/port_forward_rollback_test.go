package service

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func openPortForwardRollbackTestDB(t *testing.T) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "port-forward-rollback.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("init db failed: %v", err)
	}

	sqlDB, err := database.GetDB().DB()
	if err == nil && sqlDB != nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}
}

func createPortForwardRollbackRule(t *testing.T, targetPort int) model.PortForwardRule {
	t.Helper()

	row := model.PortForwardRule{
		Name:           "rollback-rule",
		Description:    "rollback-test",
		Enabled:        false,
		Family:         portForwardFamilyIPv4,
		Protocol:       portForwardProtocolTCP,
		LocalPortMode:  portForwardLocalPortModeSingle,
		LocalPortSpec:  "19991",
		LocalPortStart: 19991,
		LocalPortCount: 1,
		LocalPortEnd:   19991,
		TargetIP:       portForwardLoopbackIPv4,
		TargetPort:     targetPort,
	}
	if err := database.GetDB().Create(&row).Error; err != nil {
		t.Fatalf("create rule failed: %v", err)
	}
	return row
}

func loadPortForwardRollbackRule(t *testing.T, id uint) model.PortForwardRule {
	t.Helper()

	var row model.PortForwardRule
	if err := database.GetDB().Where("id = ?", id).First(&row).Error; err != nil {
		t.Fatalf("load rule %d failed: %v", id, err)
	}
	return row
}

func TestPortForwardUpsertRule_ReconcileFailureRollsBackRule(t *testing.T) {
	openPortForwardRollbackTestDB(t)

	originalReconcile := portForwardReconcileLocked
	t.Cleanup(func() {
		portForwardReconcileLocked = originalReconcile
	})

	reconcileCalls := 0
	portForwardReconcileLocked = func(_ *PortForwardService, _ time.Duration) error {
		reconcileCalls++
		if reconcileCalls == 1 {
			return errors.New("forced reconcile failure")
		}
		return nil
	}

	original := createPortForwardRollbackRule(t, 3000)
	payload := PortForwardRulePayload{
		ID:             original.Id,
		Name:           "rollback-rule-updated",
		Description:    "rollback-test-updated",
		Enabled:        false,
		Family:         portForwardFamilyIPv4,
		Protocol:       portForwardProtocolTCP,
		LocalPortMode:  portForwardLocalPortModeSingle,
		LocalPortStart: 19991,
		LocalPortCount: 1,
		LocalPortEnd:   19991,
		TargetIP:       portForwardLoopbackIPv4,
		TargetPort:     4000,
		RateLimitMbps:  0,
	}

	err := (&PortForwardService{}).UpsertRule(payload)
	if err == nil {
		t.Fatalf("expected upsert rollback error, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "rollback") {
		t.Fatalf("expected rollback hint in error, got %v", err)
	}

	current := loadPortForwardRollbackRule(t, original.Id)
	if current.TargetPort != original.TargetPort {
		t.Fatalf("target port should be rolled back: got %d want %d", current.TargetPort, original.TargetPort)
	}
	if current.Name != original.Name {
		t.Fatalf("name should be rolled back: got %q want %q", current.Name, original.Name)
	}
	if reconcileCalls < 2 {
		t.Fatalf("expected rollback reconcile to run, got %d calls", reconcileCalls)
	}
}

func TestPortForwardDeleteRule_ReconcileFailureRollsBackRow(t *testing.T) {
	openPortForwardRollbackTestDB(t)

	originalReconcile := portForwardReconcileLocked
	t.Cleanup(func() {
		portForwardReconcileLocked = originalReconcile
	})

	reconcileCalls := 0
	portForwardReconcileLocked = func(_ *PortForwardService, _ time.Duration) error {
		reconcileCalls++
		if reconcileCalls == 1 {
			return errors.New("forced reconcile failure")
		}
		return nil
	}

	row := createPortForwardRollbackRule(t, 3200)
	err := (&PortForwardService{}).DeleteRule(row.Id)
	if err == nil {
		t.Fatalf("expected delete rollback error, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "rollback") {
		t.Fatalf("expected rollback hint in error, got %v", err)
	}
	restored := loadPortForwardRollbackRule(t, row.Id)
	if restored.TargetPort != row.TargetPort {
		t.Fatalf("deleted rule should be restored: got %d want %d", restored.TargetPort, row.TargetPort)
	}
	if reconcileCalls < 2 {
		t.Fatalf("expected rollback reconcile to run, got %d calls", reconcileCalls)
	}
}
