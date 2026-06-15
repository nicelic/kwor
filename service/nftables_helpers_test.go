package service

import (
	"os"
	"testing"
	"time"
)

func TestParsePortRangeInputNormalizeAndMerge(t *testing.T) {
	ranges := parsePortRangeInput("3000-2000, 2001:2002, 65536, 0, 100, 100")
	if len(ranges) != 2 {
		t.Fatalf("unexpected range count: %d", len(ranges))
	}
	if ranges[0].start != 100 || ranges[0].end != 100 {
		t.Fatalf("unexpected first range: %+v", ranges[0])
	}
	if ranges[1].start != 2000 || ranges[1].end != 3000 {
		t.Fatalf("unexpected second range: %+v", ranges[1])
	}
}

func TestExcludePortsFromRanges(t *testing.T) {
	ranges := []portRange{{start: 1000, end: 1005}}
	excluded := map[int]struct{}{
		1001: {},
		1003: {},
	}
	allowed, skipped, sample := excludePortsFromRanges(ranges, excluded)
	if skipped != 2 {
		t.Fatalf("unexpected skipped count: %d", skipped)
	}
	if len(sample) != 2 || sample[0] != 1001 || sample[1] != 1003 {
		t.Fatalf("unexpected sample: %v", sample)
	}
	if len(allowed) != 3 {
		t.Fatalf("unexpected allowed count: %d", len(allowed))
	}
	if allowed[0] != (portRange{start: 1000, end: 1000}) {
		t.Fatalf("unexpected allowed[0]: %+v", allowed[0])
	}
	if allowed[1] != (portRange{start: 1002, end: 1002}) {
		t.Fatalf("unexpected allowed[1]: %+v", allowed[1])
	}
	if allowed[2] != (portRange{start: 1004, end: 1005}) {
		t.Fatalf("unexpected allowed[2]: %+v", allowed[2])
	}
}

func TestPortHopRangeToNftWithExclusions_ForceRedirectRange(t *testing.T) {
	got, skipped, sample := portHopRangeToNftWithExclusions("31000-31002", 0)
	if got != "31000-31002" {
		t.Fatalf("unexpected nft range: %s", got)
	}
	if skipped != 0 {
		t.Fatalf("unexpected skipped count: %d", skipped)
	}
	if len(sample) != 0 {
		t.Fatalf("unexpected skipped sample: %v", sample)
	}
}

func TestPortHopRangeToNftWithExclusions_ExcludeListenPortOnly(t *testing.T) {
	got, skipped, sample := portHopRangeToNftWithExclusions("31000-31002", 31001)
	if got != "31000, 31002" {
		t.Fatalf("unexpected nft range after listen exclusion: %s", got)
	}
	if skipped != 1 {
		t.Fatalf("unexpected skipped count: %d", skipped)
	}
	if len(sample) != 1 || sample[0] != 31001 {
		t.Fatalf("unexpected skipped sample: %v", sample)
	}
}

func TestRuleLineHasExactComment(t *testing.T) {
	line := `meta l4proto { tcp, udp } th dport 4458 counter comment "kwor_inbound_hy1_in" # handle 12`
	if !ruleLineHasExactComment(line, "kwor_inbound_hy1_in") {
		t.Fatal("expected exact comment match")
	}
	if ruleLineHasExactComment(line, "kwor_inbound_hy1") {
		t.Fatal("substring comment must not match")
	}
}

func TestParsePortHopInterval(t *testing.T) {
	if d, ok := parsePortHopInterval("30s"); !ok || d != 30*time.Second {
		t.Fatalf("unexpected interval parse for 30s: %v %v", d, ok)
	}
	if d, ok := parsePortHopInterval(" 1m "); !ok || d != time.Minute {
		t.Fatalf("unexpected interval parse for 1m: %v %v", d, ok)
	}
	if _, ok := parsePortHopInterval(""); ok {
		t.Fatal("empty interval should be invalid")
	}
	if _, ok := parsePortHopInterval("oops"); ok {
		t.Fatal("invalid interval should be rejected")
	}
}

func TestLoadNftTableName(t *testing.T) {
	original, had := os.LookupEnv("KWOR_NFT_TABLE")
	defer func() {
		if had {
			_ = os.Setenv("KWOR_NFT_TABLE", original)
		} else {
			_ = os.Unsetenv("KWOR_NFT_TABLE")
		}
	}()

	_ = os.Unsetenv("KWOR_NFT_TABLE")
	if got := loadNftTableName(); got != "kwor" {
		t.Fatalf("expected default table name, got: %s", got)
	}

	_ = os.Setenv("KWOR_NFT_TABLE", "kwor_a")
	if got := loadNftTableName(); got != "kwor_a" {
		t.Fatalf("expected custom table name, got: %s", got)
	}

	_ = os.Setenv("KWOR_NFT_TABLE", "bad table name")
	if got := loadNftTableName(); got != "kwor" {
		t.Fatalf("invalid table name should fallback to default, got: %s", got)
	}
}

func TestBuildNftPortSetArgsFromRanges(t *testing.T) {
	args := buildNftPortSetArgsFromRanges([]portRange{
		{start: 25000, end: 21000}, // reversed on purpose
		{start: 31100, end: 31100},
	})
	want := []string{"{", "21000-25000", ",", "31100", "}"}
	if len(args) != len(want) {
		t.Fatalf("unexpected args length: got=%v want=%v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args[%d]=%q want=%q full=%v", i, args[i], want[i], args)
		}
	}
}
