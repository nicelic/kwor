package service

import (
	"encoding/json"
	"testing"
)

func TestParseClientInboundIDs_NumberArray(t *testing.T) {
	ids, err := parseClientInboundIDs(json.RawMessage(`[1,2,3,2]`))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	assertUintSliceEqual(t, ids, []uint{1, 2, 3})
}

func TestParseClientInboundIDs_StringArray(t *testing.T) {
	ids, err := parseClientInboundIDs(json.RawMessage(`["1"," 2 ","x","0","-1"]`))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	assertUintSliceEqual(t, ids, []uint{1, 2})
}

func TestParseClientInboundIDs_MixedArray(t *testing.T) {
	ids, err := parseClientInboundIDs(json.RawMessage(`[1,"2",3.0,3.1,null]`))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	assertUintSliceEqual(t, ids, []uint{1, 2, 3})
}

func TestParseClientInboundIDs_InvalidJSON(t *testing.T) {
	_, err := parseClientInboundIDs(json.RawMessage(`[1,`))
	if err == nil {
		t.Fatal("expected parse error for invalid json")
	}
}
