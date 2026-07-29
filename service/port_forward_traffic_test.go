package service

import (
	"strings"
	"testing"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func createPortForwardTrafficTestRule(t *testing.T, port int) model.PortForwardRule {
	t.Helper()
	row := model.PortForwardRule{
		Name:           "traffic-rule",
		Enabled:        true,
		Family:         portForwardFamilyIPv4,
		Protocol:       portForwardProtocolTCP,
		LocalPortMode:  portForwardLocalPortModeSingle,
		LocalPortSpec:  "18080",
		LocalPortStart: 18080,
		LocalPortCount: 1,
		LocalPortEnd:   18080,
		TargetIP:       portForwardLoopbackIPv4,
		TargetPort:     port,
	}
	if err := database.GetDB().Create(&row).Error; err != nil {
		t.Fatalf("create traffic rule: %v", err)
	}
	return row
}

func TestPortForwardTrafficSyncKeepsRuleAndOverviewResetsIndependent(t *testing.T) {
	openPortForwardRollbackTestDB(t)
	row := createPortForwardTrafficTestRule(t, 8080)
	if err := database.GetDB().Create(&model.PortForwardRuleTrafficState{RuleId: row.Id}).Error; err != nil {
		t.Fatalf("create rule traffic state: %v", err)
	}
	if err := database.GetDB().Create(&model.PortForwardOverviewTrafficState{Id: portForwardTrafficOverviewStateID}).Error; err != nil {
		t.Fatalf("create overview traffic state: %v", err)
	}

	now := time.Date(2027, 5, 1, 12, 0, 0, 0, time.UTC)
	first, err := syncPortForwardTrafficStateRows([]model.PortForwardRule{row}, map[string]int64{
		portForwardCounterName(row.Id, "up"):   100,
		portForwardCounterName(row.Id, "down"): 200,
	}, now)
	if err != nil {
		t.Fatalf("first traffic sync: %v", err)
	}
	if got := first.Rules[row.Id].UsedUpBytes + first.Rules[row.Id].UsedDownBytes; got != 300 {
		t.Fatalf("first rule usage = %d, want 300", got)
	}
	if got := first.OverviewUp + first.OverviewDown; got != 300 {
		t.Fatalf("first overview usage = %d, want 300", got)
	}

	var state model.PortForwardRuleTrafficState
	if err := database.GetDB().Where("rule_id = ?", row.Id).First(&state).Error; err != nil {
		t.Fatalf("load rule traffic state: %v", err)
	}
	state.UsedUpBytes = 0
	state.UsedDownBytes = 0
	state.LastResetAt = now.Unix()
	if err := database.GetDB().Save(&state).Error; err != nil {
		t.Fatalf("reset rule traffic state: %v", err)
	}

	second, err := syncPortForwardTrafficStateRows([]model.PortForwardRule{row}, map[string]int64{
		portForwardCounterName(row.Id, "up"):   125,
		portForwardCounterName(row.Id, "down"): 260,
	}, now.Add(5*time.Second))
	if err != nil {
		t.Fatalf("second traffic sync: %v", err)
	}
	if got := second.Rules[row.Id].UsedUpBytes + second.Rules[row.Id].UsedDownBytes; got != 85 {
		t.Fatalf("rule reset usage = %d, want 85", got)
	}
	if got := second.OverviewUp + second.OverviewDown; got != 385 {
		t.Fatalf("overview must retain pre-reset usage, got %d want 385", got)
	}
}

func TestPortForwardTrafficMonthlyResetKeepsOverviewHistory(t *testing.T) {
	openPortForwardRollbackTestDB(t)
	row := createPortForwardTrafficTestRule(t, 8081)
	row.TrafficResetDay = 3
	if err := database.GetDB().Save(&row).Error; err != nil {
		t.Fatalf("set reset day: %v", err)
	}
	beforeBoundary := time.Date(2027, 5, 2, 12, 0, 0, 0, time.UTC)
	state := model.PortForwardRuleTrafficState{
		RuleId:                row.Id,
		LastUpBytes:           100,
		LastDownBytes:         100,
		UsedUpBytes:           600,
		UsedDownBytes:         400,
		OverviewLastUpBytes:   100,
		OverviewLastDownBytes: 100,
		AppliedResetDay:       3,
		ResetPeriodTag:        computePeriodTag(3, beforeBoundary),
	}
	if err := database.GetDB().Create(&state).Error; err != nil {
		t.Fatalf("create rule traffic state: %v", err)
	}
	if err := database.GetDB().Create(&model.PortForwardOverviewTrafficState{
		Id:            portForwardTrafficOverviewStateID,
		UsedUpBytes:   600,
		UsedDownBytes: 400,
	}).Error; err != nil {
		t.Fatalf("create overview traffic state: %v", err)
	}

	afterBoundary := time.Date(2027, 5, 4, 12, 0, 0, 0, time.UTC)
	runtime, err := syncPortForwardTrafficStateRows([]model.PortForwardRule{row}, map[string]int64{
		portForwardCounterName(row.Id, "up"):   150,
		portForwardCounterName(row.Id, "down"): 150,
	}, afterBoundary)
	if err != nil {
		t.Fatalf("monthly traffic sync: %v", err)
	}
	if got := runtime.Rules[row.Id].UsedUpBytes + runtime.Rules[row.Id].UsedDownBytes; got != 0 {
		t.Fatalf("monthly reset rule usage = %d, want 0", got)
	}
	if got := runtime.OverviewUp + runtime.OverviewDown; got != 1100 {
		t.Fatalf("overview should keep reset-period history, got %d want 1100", got)
	}
}

func TestPortForwardTrafficSyncHandlesCounterRebuildWithoutDuplicateBytes(t *testing.T) {
	openPortForwardRollbackTestDB(t)
	row := createPortForwardTrafficTestRule(t, 8082)
	state := model.PortForwardRuleTrafficState{
		RuleId:              row.Id,
		LastUpBytes:         100,
		UsedUpBytes:         100,
		OverviewLastUpBytes: 100,
	}
	if err := database.GetDB().Create(&state).Error; err != nil {
		t.Fatalf("create rule traffic state: %v", err)
	}
	if err := database.GetDB().Create(&model.PortForwardOverviewTrafficState{
		Id:          portForwardTrafficOverviewStateID,
		UsedUpBytes: 100,
	}).Error; err != nil {
		t.Fatalf("create overview traffic state: %v", err)
	}

	now := time.Date(2027, 5, 4, 12, 0, 0, 0, time.UTC)
	if _, err := syncPortForwardTrafficStateRows([]model.PortForwardRule{row}, map[string]int64{}, now); err != nil {
		t.Fatalf("sync after counter rebuild: %v", err)
	}
	runtime, err := syncPortForwardTrafficStateRows([]model.PortForwardRule{row}, map[string]int64{
		portForwardCounterName(row.Id, "up"): 25,
	}, now.Add(5*time.Second))
	if err != nil {
		t.Fatalf("sync rebuilt counter: %v", err)
	}
	if runtime.Rules[row.Id].UsedUpBytes != 125 || runtime.OverviewUp != 125 {
		t.Fatalf("rebuilt counter must add only new bytes, runtime=%#v", runtime)
	}
}

func TestPortForwardTrafficBlockUsesQuotaThenExpiry(t *testing.T) {
	row := model.PortForwardRule{
		TrafficLimitBytes: 100,
		TrafficExpiryDate: "2027-05-10",
	}
	state := model.PortForwardRuleTrafficState{UsedUpBytes: 60, UsedDownBytes: 40}
	quota, err := buildPortForwardTrafficRuleRuntime(row, state, time.Date(2027, 5, 4, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("build quota runtime: %v", err)
	}
	if !quota.LimitReached || quota.BlockReason != portForwardTrafficBlockQuota {
		t.Fatalf("quota runtime = %#v", quota)
	}

	expired, err := buildPortForwardTrafficRuleRuntime(row, state, time.Date(2027, 5, 10, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("build expired runtime: %v", err)
	}
	if !expired.Expired || expired.BlockReason != portForwardTrafficBlockExpiry {
		t.Fatalf("expiry must take precedence, got %#v", expired)
	}
}

func TestPortForwardTrafficBlockRulePrecedesTrafficCounter(t *testing.T) {
	withNftCapabilitiesTestGlobals(t)
	setNftCapabilitiesForTest(buildNftablesCapabilities("nftables v1.0.6", "6.1.0", nil))
	row := model.PortForwardRule{
		Id:            91,
		Family:        portForwardFamilyIPv4,
		Protocol:      portForwardProtocolTCP,
		LocalPortSpec: "443,8443",
		TargetIP:      "198.51.100.10",
		TargetPort:    443,
	}
	_, commands := buildManagedPortForwardRuleCommandsWithTrafficBlock(row, portForwardTrafficBlockQuota)
	lines := make([]string, 0, len(commands))
	for _, command := range commands {
		lines = append(lines, strings.Join(command, " "))
	}
	script := strings.Join(lines, "\n")
	blockAt := strings.Index(script, portForwardTrafficBlockComment(row.Id))
	downAt := strings.Index(script, portForwardRuleComment(row.Id, "down"))
	if blockAt < 0 || downAt < 0 || blockAt >= downAt {
		t.Fatalf("traffic block must precede counter:\n%s", script)
	}
	if !strings.Contains(script, "ct direction original drop") {
		t.Fatalf("traffic block must drop only original-direction traffic:\n%s", script)
	}
}

func TestPortForwardTrafficBlockParticipatesInIntegrityCheck(t *testing.T) {
	row := model.PortForwardRule{
		Id:       92,
		Family:   portForwardFamilyIPv4,
		Protocol: portForwardProtocolTCP,
		TargetIP: "198.51.100.10",
	}
	snapshot := portForwardNftSnapshot{tables: map[string]portForwardNftTableSnapshot{
		nftFamily: {
			known:  true,
			exists: true,
			chains: map[string]bool{
				portForwardForwardChain: true,
				portForwardInputChain:   true,
				portForwardOutputChain:  true,
			},
			commentCounts: map[string]map[string]int{
				portForwardForwardChain: {
					portForwardRuleComment(row.Id, "down"): 1,
					portForwardRuleComment(row.Id, "up"):   1,
					portForwardTrafficBlockComment(row.Id): 1,
				},
				portForwardInputChain:  {},
				portForwardOutputChain: {},
			},
			counters: map[string]int64{},
		},
	}}
	if !snapshot.filterCountersIntact(row, 1, portForwardLimitStateView{}, true) {
		t.Fatal("expected matching traffic block comment to be intact")
	}
	if snapshot.filterCountersIntact(row, 1, portForwardLimitStateView{}, false) {
		t.Fatal("unexpected traffic block comment must force a redraw")
	}
}
