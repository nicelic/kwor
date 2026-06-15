package service

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestParseClientInboundIDsForLimit(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []uint
	}{
		{
			name: "plain uint list",
			raw:  `[1,2,3]`,
			want: []uint{1, 2, 3},
		},
		{
			name: "mixed with duplicates and invalid",
			raw:  `[1, "2", 2, 0, "x", 3.5, 3]`,
			want: []uint{1, 2, 3},
		},
		{
			name: "empty",
			raw:  `[]`,
			want: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseClientInboundIDsForLimit(json.RawMessage(tc.raw))
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("unexpected ids: got=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestMbpsToBytesPerSecond(t *testing.T) {
	if got := mbpsToBytesPerSecond(200); got != 25000000 {
		t.Fatalf("unexpected conversion: got=%d want=25000000", got)
	}
	if got := mbpsToBytesPerSecond(0); got != 0 {
		t.Fatalf("unexpected zero conversion: got=%d", got)
	}
}
