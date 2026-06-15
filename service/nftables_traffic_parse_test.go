package service

import (
	"encoding/json"
	"testing"
)

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
