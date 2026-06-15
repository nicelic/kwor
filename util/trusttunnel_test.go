package util

import "testing"

func TestResolveTrustTunnelUDP_PrefersExplicitBool(t *testing.T) {
	udp, ok := ResolveTrustTunnelUDP(map[string]interface{}{
		"udp":     false,
		"network": []interface{}{"udp"},
	})
	if !ok {
		t.Fatalf("expected explicit udp field to be detected")
	}
	if udp {
		t.Fatalf("expected explicit udp=false to win over legacy network")
	}
}

func TestSanitizeTrustTunnelOutbound_NormalizesLegacyFields(t *testing.T) {
	outbound := map[string]interface{}{
		"network":               []interface{}{"tcp", "udp"},
		"uuid":                  "tt-secret",
		"health-check":          true,
		"username":              "alice",
		"password":              "secret",
		"server":                "1.1.1.1",
		"server_port":           443,
		"congestion_controller": "bbr",
	}

	SanitizeTrustTunnelOutbound(outbound)

	if got, _ := outbound["udp"].(bool); !got {
		t.Fatalf("expected udp=true after sanitize, got %#v", outbound["udp"])
	}
	if got, _ := outbound["health_check"].(bool); !got {
		t.Fatalf("expected health_check=true after sanitize, got %#v", outbound["health_check"])
	}
	if _, exists := outbound["network"]; exists {
		t.Fatalf("expected network to be removed, got %#v", outbound["network"])
	}
	if _, exists := outbound["uuid"]; exists {
		t.Fatalf("expected uuid to be removed, got %#v", outbound["uuid"])
	}
	if _, exists := outbound["health-check"]; exists {
		t.Fatalf("expected health-check alias to be removed, got %#v", outbound["health-check"])
	}
}

func TestApplyTrustTunnelReuseOptions_NormalizesAliases(t *testing.T) {
	outbound := map[string]interface{}{}
	ApplyTrustTunnelReuseOptions(outbound, map[string]interface{}{
		"max_connections": 1,
		"min-streams":     "0",
		"max_streams":     float64(0),
	})

	if got, _ := outbound["max-connections"].(int); got != 1 {
		t.Fatalf("expected max-connections=1, got %#v", outbound["max-connections"])
	}
	if got, _ := outbound["min-streams"].(int); got != 0 {
		t.Fatalf("expected min-streams=0, got %#v", outbound["min-streams"])
	}
	if got, _ := outbound["max-streams"].(int); got != 0 {
		t.Fatalf("expected max-streams=0, got %#v", outbound["max-streams"])
	}
}

func TestResolveTrustTunnelReuseOption_RejectsNegativeOrNonInteger(t *testing.T) {
	if _, ok := ResolveTrustTunnelReuseOption(map[string]interface{}{"max_connections": -1}, "max_connections"); ok {
		t.Fatalf("expected negative value to be rejected")
	}
	if _, ok := ResolveTrustTunnelReuseOption(map[string]interface{}{"max_connections": "1.5"}, "max_connections"); ok {
		t.Fatalf("expected non-integer string to be rejected")
	}
	if value, ok := ResolveTrustTunnelReuseOption(map[string]interface{}{"max_connections": "3"}, "max_connections"); !ok || value != 3 {
		t.Fatalf("expected integer string to parse, got (%v, %v)", value, ok)
	}
}
