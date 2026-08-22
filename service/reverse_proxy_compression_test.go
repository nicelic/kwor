package service

import (
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestReverseProxyCompressionStorageKeepsLegacyAndExplicitEmptyStates(t *testing.T) {
	legacy := reverseProxyStoredCompressionAlgorithms("")
	if len(legacy) != len(reverseProxyCompressionAlgorithmOrder) {
		t.Fatalf("legacy empty storage returned %v, want all algorithms", legacy)
	}
	if got := reverseProxyCompressionStorageValue(false, nil); got != reverseProxyCompressionDisabledStorageValue {
		t.Fatalf("disabled storage = %q, want %q", got, reverseProxyCompressionDisabledStorageValue)
	}
	if got := reverseProxyCompressionStorageValue(true, []string{}); got != reverseProxyCompressionEmptyStorageValue {
		t.Fatalf("explicit empty storage = %q, want %q", got, reverseProxyCompressionEmptyStorageValue)
	}
	enabled, algorithms := reverseProxyCompressionSettingsFromModel(true, reverseProxyCompressionEmptyStorageValue)
	if !enabled || len(algorithms) != 0 {
		t.Fatalf("explicit empty settings = (%t, %v), want (true, [])", enabled, algorithms)
	}
	enabled, algorithms = reverseProxyCompressionSettingsFromModel(true, reverseProxyCompressionDisabledStorageValue)
	if enabled || len(algorithms) != 0 {
		t.Fatalf("disabled settings = (%t, %v), want (false, [])", enabled, algorithms)
	}
}

func TestReverseProxyCompressionHeadersRespectSelectedSubset(t *testing.T) {
	rule := &model.ReverseProxyRule{
		ListenProtocol:              "http",
		ListenCompressionEnabled:    true,
		ListenCompressionAlgorithms: `["br","gzip"]`,
		TargetProtocol:              "http",
		TargetCompressionEnabled:    true,
		TargetCompressionAlgorithms: `["s2","deflate"]`,
	}
	if got, want := reverseProxyListenAcceptEncoding(rule), "br;q=1.000, gzip;q=0.999"; got != want {
		t.Fatalf("listen Accept-Encoding = %q, want %q", got, want)
	}
	if got, want := reverseProxyTargetAcceptEncoding(rule), "s2;q=1.000, deflate;q=0.999"; got != want {
		t.Fatalf("target Accept-Encoding = %q, want %q", got, want)
	}
}
