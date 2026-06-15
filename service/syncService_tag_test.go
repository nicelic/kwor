package service

import "testing"

func TestBuildClientSubTag(t *testing.T) {
	if got := buildClientSubTag("hy1_hk1", "hk1"); got != "hy1_hk1_hk1" {
		t.Fatalf("unexpected sub tag: %s", got)
	}
	if got := buildClientSubTag("  hy1_hk1  ", "  hk1  "); got != "hy1_hk1_hk1" {
		t.Fatalf("unexpected trimmed sub tag: %s", got)
	}
	if got := buildClientSubTag("hy1_hk1", ""); got != "hy1_hk1" {
		t.Fatalf("unexpected fallback tag: %s", got)
	}
}

func TestBuildManagedClientSubTag(t *testing.T) {
	if got := buildManagedClientSubTag("hy1_hk1", "hk1"); got != "s_hy1_hk1_hk1" {
		t.Fatalf("unexpected managed sub tag: %s", got)
	}
	if got := buildManagedClientSubTag("hy1_hk1", ""); got != "s_hy1_hk1" {
		t.Fatalf("unexpected managed fallback tag: %s", got)
	}
}

func TestBuildLegacySubTags(t *testing.T) {
	tags := buildLegacySubTags([]string{"hk1", "hk1", "hk2"}, "hy1_hk1")
	lookup := map[string]bool{}
	for _, tag := range tags {
		lookup[tag] = true
	}

	expected := []string{
		"hy1_hk1_hk1",
		"hk1-hy1_hk1",
		"hk1",
		"hy1_hk1_hk2",
		"hk2-hy1_hk1",
		"hk2",
		"hy1_hk1",
	}
	for _, tag := range expected {
		if !lookup[tag] {
			t.Fatalf("expected legacy tag %s not found in %#v", tag, tags)
		}
	}
}

func TestBuildManagedLegacySubTags(t *testing.T) {
	tags := buildManagedLegacySubTags([]string{"hk1", "hk2"}, "hy1_hk1")
	lookup := map[string]bool{}
	for _, tag := range tags {
		lookup[tag] = true
	}

	expected := []string{
		"s_hy1_hk1_hk1",
		"s_hy1_hk1_hk2",
		"s_hy1_hk1",
		"sm_hy1_hk1_hk1",
		"sm_hy1_hk1_hk2",
		"sm_hy1_hk1",
		"hy1_hk1_hk1",
		"hk1-hy1_hk1",
		"hk1",
		"hy1_hk1_hk2",
		"hk2-hy1_hk1",
		"hk2",
		"hy1_hk1",
	}
	for _, tag := range expected {
		if !lookup[tag] {
			t.Fatalf("expected managed legacy tag %s not found in %#v", tag, tags)
		}
	}
}

func TestRewriteOutboundTagReferences(t *testing.T) {
	outbound := map[string]interface{}{
		"type":   "shadowsocks",
		"tag":    "stls_hk1",
		"detour": "stls_hk1-out",
	}

	rewriteOutboundTagReferences(outbound, "stls_hk1", "stls_hk1_hk1")

	if got := outbound["tag"]; got != "stls_hk1_hk1" {
		t.Fatalf("tag rewrite failed: %#v", got)
	}
	if got := outbound["detour"]; got != "stls_hk1_hk1-out" {
		t.Fatalf("detour rewrite failed: %#v", got)
	}
}
