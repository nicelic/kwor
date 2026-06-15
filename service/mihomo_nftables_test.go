package service

import (
	"encoding/json"
	"testing"
)

func TestParseMihomoInboundIDs_NumberArray(t *testing.T) {
	ids := parseMihomoInboundIDs(json.RawMessage(`[1,2,3,2]`))
	expected := []uint{1, 2, 3}
	assertUintSlicesEqual(t, ids, expected)
}

func TestParseMihomoInboundIDs_StringArray(t *testing.T) {
	ids := parseMihomoInboundIDs(json.RawMessage(`["1"," 2 ","x","0","-1"]`))
	expected := []uint{1, 2}
	assertUintSlicesEqual(t, ids, expected)
}

func TestParseMihomoInboundIDs_MixedArray(t *testing.T) {
	ids := parseMihomoInboundIDs(json.RawMessage(`[1,"2",3.0,3.1,null]`))
	expected := []uint{1, 2, 3}
	assertUintSlicesEqual(t, ids, expected)
}

func assertUintSlicesEqual(t *testing.T, got []uint, expected []uint) {
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
