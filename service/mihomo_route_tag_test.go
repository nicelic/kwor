package service

import (
	"encoding/json"
	"testing"
)

func TestDeriveEffectiveMihomoInboundRouteTagFromRaw(t *testing.T) {
	t.Run("shadowtls keeps original tag even with ss_config", func(t *testing.T) {
		got := deriveEffectiveMihomoInboundRouteTagFromRaw("stls-in", "shadowtls", json.RawMessage(`{
			"ss_config":{"method":"2022-blake3-aes-128-gcm","password":"ss-pass"}
		}`))
		if got != "stls-in" {
			t.Fatalf("route tag = %q, want %q", got, "stls-in")
		}
	})

	t.Run("detour still overrides route tag", func(t *testing.T) {
		got := deriveEffectiveMihomoInboundRouteTagFromRaw("stls-in", "shadowtls", json.RawMessage(`{
			"detour":"proxy-out",
			"ss_config":{"method":"2022-blake3-aes-128-gcm","password":"ss-pass"}
		}`))
		if got != "proxy-out" {
			t.Fatalf("route tag = %q, want %q", got, "proxy-out")
		}
	})
}
