package service

import (
	"encoding/json"
	"testing"
)

func TestGetChainRuleBytesByHandlesFromJSON(t *testing.T) {
	values, err := getChainRuleBytesByHandlesFromJSON([]byte(`{
  "nftables": [
    {"rule": {"handle": 11, "expr": [{"counter": {"bytes": 123}}]}},
    {"rule": {"handle": 22, "expr": [{"counter": {"bytes": 456}}]}}
  ]
}`), nftChainIn, map[int]struct{}{11: {}, 22: {}})
	if err != nil {
		t.Fatalf("parse json counters failed: %v", err)
	}
	if values[11] != 123 || values[22] != 456 {
		t.Fatalf("unexpected json counters: %#v", values)
	}
}

func TestGetChainRuleBytesByHandlesFromText(t *testing.T) {
	values, err := getChainRuleBytesByHandlesFromText([]byte(`
tcp dport 443 counter packets 2 bytes 321 comment "kwor" # handle 31
udp dport 8443 counter packets 4 bytes 654 comment "kwor" # handle 32
`), nftChainIn, map[int]struct{}{31: {}, 32: {}})
	if err != nil {
		t.Fatalf("parse text counters failed: %v", err)
	}
	if values[31] != 321 || values[32] != 654 {
		t.Fatalf("unexpected text counters: %#v", values)
	}
}

func TestParseNftClientInboundIDs_NumberArray(t *testing.T) {
	ids := parseNftClientInboundIDs(json.RawMessage(`[1,2,3,2]`))
	assertUintSliceEqual(t, ids, []uint{1, 2, 3})
}

func TestParseNftClientInboundIDs_StringArray(t *testing.T) {
	ids := parseNftClientInboundIDs(json.RawMessage(`["1"," 2 ","x","0","-1"]`))
	assertUintSliceEqual(t, ids, []uint{1, 2})
}

func TestParseNftClientInboundIDs_MixedArray(t *testing.T) {
	ids := parseNftClientInboundIDs(json.RawMessage(`[1,"2",3.0,3.1,null]`))
	assertUintSliceEqual(t, ids, []uint{1, 2, 3})
}

func assertUintSliceEqual(t *testing.T, got []uint, expected []uint) {
	t.Helper()

	if len(got) != len(expected) {
		t.Fatalf("length mismatch, got=%v expected=%v", got, expected)
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Fatalf("value mismatch at %d, got=%v expected=%v", i, got, expected)
		}
	}
}
