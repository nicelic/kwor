package service

import (
	"fmt"
	"testing"
	"time"
)

func TestComputePeriodTag_ResetDay31SwitchesAtStartOfConfiguredDay(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*60*60)

	before := time.Date(2026, time.May, 30, 23, 59, 59, 0, loc)
	after := time.Date(2026, time.May, 31, 0, 0, 0, 0, loc)

	beforeTag := computePeriodTag(31, before)
	afterTag := computePeriodTag(31, after)
	if beforeTag == afterTag {
		t.Fatalf("expected period tag change at boundary, got same tag: %q", beforeTag)
	}

	expectedBeforeBoundary := computeClientMonthlyResetBoundary(31, 2026, time.April, loc)
	expectedAfterBoundary := computeClientMonthlyResetBoundary(31, 2026, time.May, loc)
	if beforeTag != fmt.Sprintf("boundary:%d", expectedBeforeBoundary.Unix()) {
		t.Fatalf("unexpected tag before boundary: got %q", beforeTag)
	}
	if afterTag != fmt.Sprintf("boundary:%d", expectedAfterBoundary.Unix()) {
		t.Fatalf("unexpected tag after boundary: got %q", afterTag)
	}
}

func TestComputePeriodTag_ResetDay30SwitchesAtStartOfConfiguredDay(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*60*60)

	before := time.Date(2026, time.May, 29, 23, 59, 59, 0, loc)
	after := time.Date(2026, time.May, 30, 0, 0, 0, 0, loc)

	beforeTag := computePeriodTag(30, before)
	afterTag := computePeriodTag(30, after)
	if beforeTag == afterTag {
		t.Fatalf("expected period tag change at boundary, got same tag: %q", beforeTag)
	}

	expectedBeforeBoundary := computeClientMonthlyResetBoundary(30, 2026, time.April, loc)
	expectedAfterBoundary := computeClientMonthlyResetBoundary(30, 2026, time.May, loc)
	if beforeTag != fmt.Sprintf("boundary:%d", expectedBeforeBoundary.Unix()) {
		t.Fatalf("unexpected tag before boundary: got %q", beforeTag)
	}
	if afterTag != fmt.Sprintf("boundary:%d", expectedAfterBoundary.Unix()) {
		t.Fatalf("unexpected tag after boundary: got %q", afterTag)
	}
}

func TestApplyPeriodResetIfNeeded_ResetsOnlyWhenBoundaryChanges(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*60*60)
	resetDay := 30

	before := time.Date(2026, time.May, 29, 12, 0, 0, 0, loc)
	boundary := time.Date(2026, time.May, 30, 0, 0, 0, 0, loc)

	state := trafficOverviewRuntimeState{
		PeriodTag:      computePeriodTag(resetDay, before),
		PeriodBaseUp:   100,
		PeriodBaseDown: 200,
		PeriodResetDay: resetDay,
	}

	changed, err := applyPeriodResetIfNeeded(&state, resetDay, 500, 700, before)
	if err != nil {
		t.Fatalf("unexpected error before boundary: %v", err)
	}
	if changed {
		t.Fatalf("did not expect reset before boundary")
	}
	if state.PeriodBaseUp != 100 || state.PeriodBaseDown != 200 {
		t.Fatalf("period bases should not change before boundary")
	}

	changed, err = applyPeriodResetIfNeeded(&state, resetDay, 800, 900, boundary)
	if err != nil {
		t.Fatalf("unexpected error on boundary: %v", err)
	}
	if !changed {
		t.Fatalf("expected reset on boundary")
	}
	if state.PeriodBaseUp != 800 || state.PeriodBaseDown != 900 {
		t.Fatalf("period bases were not updated on boundary")
	}
}

func TestApplyPeriodResetIfNeeded_EnablingResetDayKeepsCurrentUsage(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*60*60)
	now := time.Date(2026, time.June, 22, 12, 0, 0, 0, loc)

	state := trafficOverviewRuntimeState{
		PeriodBaseUp:   100,
		PeriodBaseDown: 200,
	}

	changed, err := applyPeriodResetIfNeeded(&state, 17, 500, 700, now)
	if err != nil {
		t.Fatalf("unexpected error when enabling reset day: %v", err)
	}
	if !changed {
		t.Fatalf("expected state alignment when enabling reset day")
	}
	if state.PeriodBaseUp != 100 || state.PeriodBaseDown != 200 {
		t.Fatalf("period bases should stay unchanged when enabling reset day")
	}
	if state.PeriodResetDay != 17 {
		t.Fatalf("period reset day = %d, want 17", state.PeriodResetDay)
	}
	if state.PeriodTag != computePeriodTag(17, now) {
		t.Fatalf("unexpected period tag after enabling reset day: %q", state.PeriodTag)
	}
}

func TestApplyPeriodResetIfNeeded_ChangingResetDayKeepsCurrentUsage(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*60*60)
	now := time.Date(2026, time.June, 22, 12, 0, 0, 0, loc)

	state := trafficOverviewRuntimeState{
		PeriodBaseUp:   100,
		PeriodBaseDown: 200,
		PeriodTag:      computePeriodTag(17, now),
		PeriodResetDay: 17,
	}

	changed, err := applyPeriodResetIfNeeded(&state, 25, 500, 700, now)
	if err != nil {
		t.Fatalf("unexpected error when changing reset day: %v", err)
	}
	if !changed {
		t.Fatalf("expected state alignment when changing reset day")
	}
	if state.PeriodBaseUp != 100 || state.PeriodBaseDown != 200 {
		t.Fatalf("period bases should stay unchanged when changing reset day")
	}
	if state.PeriodResetDay != 25 {
		t.Fatalf("period reset day = %d, want 25", state.PeriodResetDay)
	}
	if state.PeriodTag != computePeriodTag(25, now) {
		t.Fatalf("unexpected period tag after changing reset day: %q", state.PeriodTag)
	}
}

func TestVnstatManagementSupportForRuntimeBlocksDocker(t *testing.T) {
	canManage, hint := vnstatManagementSupportForRuntime("linux", true)
	if canManage {
		t.Fatal("expected docker runtime to disable vnstat management")
	}
	if hint == "" {
		t.Fatal("expected docker runtime hint")
	}

	canManage, hint = vnstatManagementSupportForRuntime("linux", false)
	if !canManage {
		t.Fatalf("expected host runtime to allow vnstat management, got hint=%q", hint)
	}
}

func TestVnstatManagementSupportForRuntimeRejectsNonLinux(t *testing.T) {
	canManage, hint := vnstatManagementSupportForRuntime("windows", false)
	if canManage {
		t.Fatal("expected non-linux runtime to disable vnstat management")
	}
	if hint == "" {
		t.Fatal("expected non-linux runtime hint")
	}
}

func TestComputePeriodTag_ResetDay1SwitchesAtStartOfMonth(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*60*60)

	before := time.Date(2026, time.April, 30, 23, 59, 59, 0, loc)
	after := time.Date(2026, time.May, 1, 0, 0, 0, 0, loc)

	beforeTag := computePeriodTag(1, before)
	afterTag := computePeriodTag(1, after)
	if beforeTag == afterTag {
		t.Fatalf("expected period tag change at boundary, got same tag: %q", beforeTag)
	}

	expectedBeforeBoundary := computeClientMonthlyResetBoundary(1, 2026, time.April, loc)
	expectedAfterBoundary := computeClientMonthlyResetBoundary(1, 2026, time.May, loc)
	if beforeTag != fmt.Sprintf("boundary:%d", expectedBeforeBoundary.Unix()) {
		t.Fatalf("unexpected tag before boundary: got %q", beforeTag)
	}
	if afterTag != fmt.Sprintf("boundary:%d", expectedAfterBoundary.Unix()) {
		t.Fatalf("unexpected tag after boundary: got %q", afterTag)
	}
}

func TestComputePeriodTag_ResetDay25SwitchesAtStartOfConfiguredDay(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*60*60)

	before := time.Date(2026, time.July, 24, 23, 59, 59, 0, loc)
	after := time.Date(2026, time.July, 25, 0, 0, 0, 0, loc)

	beforeTag := computePeriodTag(25, before)
	afterTag := computePeriodTag(25, after)
	if beforeTag == afterTag {
		t.Fatalf("expected period tag change at boundary, got same tag: %q", beforeTag)
	}

	expectedBeforeBoundary := computeClientMonthlyResetBoundary(25, 2026, time.June, loc)
	expectedAfterBoundary := computeClientMonthlyResetBoundary(25, 2026, time.July, loc)
	if beforeTag != fmt.Sprintf("boundary:%d", expectedBeforeBoundary.Unix()) {
		t.Fatalf("unexpected tag before boundary: got %q", beforeTag)
	}
	if afterTag != fmt.Sprintf("boundary:%d", expectedAfterBoundary.Unix()) {
		t.Fatalf("unexpected tag after boundary: got %q", afterTag)
	}
}

func TestComputePeriodTag_ResetDay31FallsBackForShortMonth(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*60*60)

	before := time.Date(2025, time.February, 27, 23, 59, 59, 0, loc)
	after := time.Date(2025, time.February, 28, 0, 0, 0, 0, loc)

	beforeTag := computePeriodTag(31, before)
	afterTag := computePeriodTag(31, after)
	if beforeTag == afterTag {
		t.Fatalf("expected period tag change at short-month boundary, got same tag: %q", beforeTag)
	}

	expectedBeforeBoundary := computeClientMonthlyResetBoundary(31, 2025, time.January, loc)
	expectedAfterBoundary := computeClientMonthlyResetBoundary(31, 2025, time.February, loc)
	if beforeTag != fmt.Sprintf("boundary:%d", expectedBeforeBoundary.Unix()) {
		t.Fatalf("unexpected tag before boundary: got %q", beforeTag)
	}
	if afterTag != fmt.Sprintf("boundary:%d", expectedAfterBoundary.Unix()) {
		t.Fatalf("unexpected tag after boundary: got %q", afterTag)
	}
}

func TestShouldResetClientTrafficMonthly_TriggersAtConfiguredDayMidnight(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*60*60)

	beforeBoundary := time.Date(2026, time.July, 4, 23, 59, 59, 0, loc)
	boundary := time.Date(2026, time.July, 5, 0, 0, 0, 0, loc)
	lastReset := time.Date(2026, time.June, 5, 0, 0, 0, 0, loc).Unix()

	if shouldResetClientTrafficMonthly(lastReset, 5, beforeBoundary) {
		t.Fatal("did not expect reset before configured day boundary")
	}
	if !shouldResetClientTrafficMonthly(lastReset, 5, boundary) {
		t.Fatal("expected reset at configured day boundary")
	}
}

func TestNextClientMonthlyResetBoundary_UsesCurrentMonthBeforeBoundary(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*60*60)
	now := time.Date(2026, time.July, 4, 23, 59, 59, 0, loc)

	nextBoundary, ok := nextClientMonthlyResetBoundary(5, now)
	if !ok {
		t.Fatal("expected next boundary to exist")
	}

	expected := time.Date(2026, time.July, 5, 0, 0, 0, 0, loc)
	if !nextBoundary.Equal(expected) {
		t.Fatalf("next boundary = %v, want %v", nextBoundary, expected)
	}
}

func TestNextClientMonthlyResetBoundary_UsesNextMonthAtBoundary(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*60*60)
	now := time.Date(2026, time.July, 5, 0, 0, 0, 0, loc)

	nextBoundary, ok := nextClientMonthlyResetBoundary(5, now)
	if !ok {
		t.Fatal("expected next boundary to exist")
	}

	expected := time.Date(2026, time.August, 5, 0, 0, 0, 0, loc)
	if !nextBoundary.Equal(expected) {
		t.Fatalf("next boundary = %v, want %v", nextBoundary, expected)
	}
}

func TestNextClientMonthlyResetBoundary_FallsBackToMonthEnd(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*60*60)
	now := time.Date(2025, time.February, 28, 0, 0, 0, 0, loc)

	nextBoundary, ok := nextClientMonthlyResetBoundary(31, now)
	if !ok {
		t.Fatal("expected next boundary to exist")
	}

	expected := time.Date(2025, time.March, 31, 0, 0, 0, 0, loc)
	if !nextBoundary.Equal(expected) {
		t.Fatalf("next boundary = %v, want %v", nextBoundary, expected)
	}
}
