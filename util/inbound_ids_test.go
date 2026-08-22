package util

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestParseInboundIDsRejectsPartialAndFractionalLegacyValues(t *testing.T) {
	ids, err := ParseInboundIDs(json.RawMessage(`["12",12,"12x","12.5","0","-1",12.5,13]`))
	if err != nil {
		t.Fatalf("ParseInboundIDs returned an error: %v", err)
	}
	if !reflect.DeepEqual(ids, []uint{12, 13}) {
		t.Fatalf("ParseInboundIDs = %#v, want []uint{12, 13}", ids)
	}
}
