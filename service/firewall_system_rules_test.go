package service

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func openFirewallSystemRuleTestDB(t *testing.T) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "firewall-system-rules.db")
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

func loadFirewallSystemRuleByKey(t *testing.T, key string) model.FirewallRule {
	t.Helper()

	var row model.FirewallRule
	if err := database.GetDB().
		Where("origin = ? AND system_key = ?", firewallOriginSystem, key).
		First(&row).Error; err != nil {
		t.Fatalf("load system firewall rule %s failed: %v", key, err)
	}
	return row
}

func TestFirewallSyncIfNeeded_DisabledStillRefreshesSystemReservedPorts(t *testing.T) {
	openFirewallSystemRuleTestDB(t)

	settingSvc := &SettingService{}
	if err := settingSvc.SetPort(18080); err != nil {
		t.Fatalf("set panel port failed: %v", err)
	}
	if err := settingSvc.SetSubPort(28080); err != nil {
		t.Fatalf("set sub port failed: %v", err)
	}

	firewallSvc := &FirewallService{}
	if err := firewallSvc.SetEnabled(false); err != nil {
		t.Fatalf("set firewall disabled failed: %v", err)
	}
	if err := firewallSvc.SyncIfNeeded(0); err != nil {
		t.Fatalf("sync firewall while disabled failed: %v", err)
	}

	sshRule := loadFirewallSystemRuleByKey(t, firewallSystemSSH)
	if sshRule.PortSpec != "22" {
		t.Fatalf("ssh port spec mismatch: got %q want %q", sshRule.PortSpec, "22")
	}
	panelRule := loadFirewallSystemRuleByKey(t, firewallSystemPanel)
	if panelRule.PortSpec != "18080" {
		t.Fatalf("panel port spec mismatch: got %q want %q", panelRule.PortSpec, "18080")
	}
	subRule := loadFirewallSystemRuleByKey(t, firewallSystemSub)
	if subRule.PortSpec != "28080" {
		t.Fatalf("sub port spec mismatch: got %q want %q", subRule.PortSpec, "28080")
	}

	previousPanelSeenAt := panelRule.LastSeenAt
	previousSubSeenAt := subRule.LastSeenAt
	time.Sleep(1100 * time.Millisecond)
	if err := firewallSvc.SyncIfNeeded(0); err != nil {
		t.Fatalf("second sync firewall while disabled failed: %v", err)
	}
	panelRule = loadFirewallSystemRuleByKey(t, firewallSystemPanel)
	subRule = loadFirewallSystemRuleByKey(t, firewallSystemSub)
	if panelRule.LastSeenAt != previousPanelSeenAt {
		t.Fatalf("panel rule should not be rewritten when ports unchanged: got %d want %d", panelRule.LastSeenAt, previousPanelSeenAt)
	}
	if subRule.LastSeenAt != previousSubSeenAt {
		t.Fatalf("sub rule should not be rewritten when ports unchanged: got %d want %d", subRule.LastSeenAt, previousSubSeenAt)
	}

	if err := settingSvc.SetPort(18081); err != nil {
		t.Fatalf("update panel port failed: %v", err)
	}
	if err := settingSvc.SetSubPort(28081); err != nil {
		t.Fatalf("update sub port failed: %v", err)
	}
	if err := firewallSvc.SyncIfNeeded(0); err != nil {
		t.Fatalf("sync firewall after port update failed: %v", err)
	}

	panelRule = loadFirewallSystemRuleByKey(t, firewallSystemPanel)
	subRule = loadFirewallSystemRuleByKey(t, firewallSystemSub)
	if panelRule.PortSpec != "18081" {
		t.Fatalf("panel port spec update mismatch: got %q want %q", panelRule.PortSpec, "18081")
	}
	if subRule.PortSpec != "28081" {
		t.Fatalf("sub port spec update mismatch: got %q want %q", subRule.PortSpec, "28081")
	}
	if panelRule.LastSeenAt <= previousPanelSeenAt {
		t.Fatalf("panel rule lastSeenAt should increase after port change: got %d <= %d", panelRule.LastSeenAt, previousPanelSeenAt)
	}
	if subRule.LastSeenAt <= previousSubSeenAt {
		t.Fatalf("sub rule lastSeenAt should increase after port change: got %d <= %d", subRule.LastSeenAt, previousSubSeenAt)
	}
}

func TestFirewallSystemRuleReserveToggle_PersistsAcrossSync(t *testing.T) {
	openFirewallSystemRuleTestDB(t)

	firewallSvc := &FirewallService{}
	if err := firewallSvc.SetEnabled(false); err != nil {
		t.Fatalf("set firewall disabled failed: %v", err)
	}
	if err := firewallSvc.SyncIfNeeded(0); err != nil {
		t.Fatalf("initial sync failed: %v", err)
	}

	if err := firewallSvc.SetSystemRuleReserved(firewallSystemSSH, false); err != nil {
		t.Fatalf("disable ssh reserved rule failed: %v", err)
	}
	if err := firewallSvc.SetSystemRuleReserved(firewallSystemSub, false); err != nil {
		t.Fatalf("disable sub reserved rule failed: %v", err)
	}
	if err := firewallSvc.SyncIfNeeded(0); err != nil {
		t.Fatalf("sync after disabling reserved rules failed: %v", err)
	}

	sshRule := loadFirewallSystemRuleByKey(t, firewallSystemSSH)
	subRule := loadFirewallSystemRuleByKey(t, firewallSystemSub)
	panelRule := loadFirewallSystemRuleByKey(t, firewallSystemPanel)
	if sshRule.Enabled {
		t.Fatalf("ssh reserved rule should stay disabled after sync")
	}
	if subRule.Enabled {
		t.Fatalf("sub reserved rule should stay disabled after sync")
	}
	if !panelRule.Enabled {
		t.Fatalf("panel reserved rule must remain enabled")
	}

	overview, err := firewallSvc.GetOverview()
	if err != nil {
		t.Fatalf("get firewall overview failed: %v", err)
	}
	if overview.DefaultPorts.SSHReserved {
		t.Fatalf("overview should report ssh reserved disabled")
	}
	if overview.DefaultPorts.SubReserved {
		t.Fatalf("overview should report sub reserved disabled")
	}
	if !overview.DefaultPorts.PanelReserved {
		t.Fatalf("overview should report panel reserved enabled")
	}
	if len(overview.DefaultPorts.Active) == 0 {
		t.Fatalf("active reserved ports should still include panel port")
	}

	if err := firewallSvc.SetSystemRuleReserved(firewallSystemSSH, true); err != nil {
		t.Fatalf("restore ssh reserved rule failed: %v", err)
	}
	sshRule = loadFirewallSystemRuleByKey(t, firewallSystemSSH)
	if !sshRule.Enabled {
		t.Fatalf("ssh reserved rule should be restored")
	}
}

func TestCleanupTemporaryFirewallRulesOnStartupRemovesTemporaryAndLegacyACMERules(t *testing.T) {
	openFirewallSystemRuleTestDB(t)

	now := time.Now().Unix()
	rows := []model.FirewallRule{
		{
			Name:              "ACME temp new",
			Description:       "Temporary ACME validation rule, auto removed after issue or renew",
			Enabled:           true,
			Origin:            firewallOriginTemporary,
			TemporaryType:     "acme",
			TemporaryExpireAt: now - 10,
			Direction:         firewallDirectionIngress,
			Family:            firewallFamilyDual,
			Protocol:          firewallProtocolTCPUDP,
			PortSpec:          "443",
		},
		{
			Name:        "ACME temporary allow 80",
			Description: "Temporary ACME validation rule, auto removed after issue or renew",
			Enabled:     true,
			Origin:      firewallOriginManual,
			Direction:   firewallDirectionIngress,
			Family:      firewallFamilyDual,
			Protocol:    firewallProtocolTCPUDP,
			PortSpec:    "80",
		},
	}
	if err := database.GetDB().Create(&rows).Error; err != nil {
		t.Fatalf("create temporary firewall rows failed: %v", err)
	}

	if err := (&FirewallService{}).CleanupTemporaryRulesOnStartup(); err != nil {
		t.Fatalf("cleanup temporary firewall rules failed: %v", err)
	}

	var count int64
	if err := database.GetDB().Model(&model.FirewallRule{}).Where("origin IN ?", []string{firewallOriginTemporary, firewallOriginManual}).Count(&count).Error; err != nil {
		t.Fatalf("count remaining temporary firewall rows failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected temporary firewall rules cleaned up, count=%d", count)
	}
}

func TestFirewallOverviewCountsTemporaryRulesSeparately(t *testing.T) {
	openFirewallSystemRuleTestDB(t)

	rows := []model.FirewallRule{
		{
			Name:          "manual",
			Enabled:       true,
			Origin:        firewallOriginManual,
			Direction:     firewallDirectionIngress,
			Family:        firewallFamilyDual,
			Protocol:      firewallProtocolTCP,
			PortSpec:      "443",
			TemporaryType: "",
		},
		{
			Name:              "temporary",
			Enabled:           true,
			Origin:            firewallOriginTemporary,
			TemporaryType:     "acme",
			TemporaryExpireAt: time.Now().Add(10 * time.Minute).Unix(),
			Direction:         firewallDirectionIngress,
			Family:            firewallFamilyDual,
			Protocol:          firewallProtocolTCPUDP,
			PortSpec:          "80",
		},
	}
	if err := database.GetDB().Create(&rows).Error; err != nil {
		t.Fatalf("create overview firewall rows failed: %v", err)
	}

	overview, err := (&FirewallService{}).GetOverview()
	if err != nil {
		t.Fatalf("get firewall overview failed: %v", err)
	}
	if overview.ManualCount != 1 {
		t.Fatalf("manual count mismatch: got=%d want=1", overview.ManualCount)
	}
	if overview.TemporaryCount != 1 {
		t.Fatalf("temporary count mismatch: got=%d want=1", overview.TemporaryCount)
	}

	foundTemporary := false
	for _, row := range overview.Rules {
		if row.Origin != firewallOriginTemporary {
			continue
		}
		foundTemporary = true
		if row.CanEdit || row.CanDelete {
			t.Fatalf("temporary firewall rule must be read-only: %#v", row)
		}
	}
	if !foundTemporary {
		t.Fatal("expected temporary firewall rule in overview")
	}
}
