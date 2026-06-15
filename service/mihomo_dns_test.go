package service

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestSanitizeMihomoDNSConfig(t *testing.T) {
	got := sanitizeMihomoDNSConfig(map[string]interface{}{
		"direct-nameserver": []interface{}{
			" tls://223.5.5.5:853 ",
			"",
			"tls://223.5.5.5:853",
		},
		"proxy-server-nameserver": []interface{}{"udp://223.5.5.5"},
		"nameserver":              []interface{}{"udp://8.8.8.8#节点选择", nil},
		"default-nameserver":      []interface{}{"223.5.5.5", "223.5.5.5"},
		"fallback":                []interface{}{123, "tcp://8.8.4.4#节点选择"},
		"ipv6":                    "true",
		"prefer-h3":               "true",
		"ipv6-timeout":            "50ms",
		"listen":                  "0.0.0.0:53",
		"fallback-filter": map[string]interface{}{
			"geoip": true,
		},
	})

	want := map[string]interface{}{
		"direct-nameserver":       []string{"tls://223.5.5.5:853"},
		"proxy-server-nameserver": []string{"udp://223.5.5.5"},
		"nameserver":              []string{"udp://8.8.8.8#节点选择"},
		"default-nameserver":      []string{"223.5.5.5"},
		"fallback":                []string{"tcp://8.8.4.4#节点选择"},
		"ipv6":                    true,
		"prefer-h3":               true,
		"ipv6-timeout":            50,
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sanitizeMihomoDNSConfig() = %#v, want %#v", got, want)
	}
}

func TestBuildMihomoDNSDocument(t *testing.T) {
	got := buildMihomoDNSDocument(map[string]interface{}{
		"nameserver": []interface{}{"udp://8.8.8.8#节点选择"},
	})

	want := map[string]interface{}{
		"enable":     true,
		"nameserver": []string{"udp://8.8.8.8#节点选择"},
		"ipv6":       false,
		"prefer-h3":  false,
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildMihomoDNSDocument() = %#v, want %#v", got, want)
	}
}

func TestBuildMihomoDNSDocumentWithIPv6Timeout(t *testing.T) {
	got := buildMihomoDNSDocument(map[string]interface{}{
		"nameserver":   []interface{}{"udp://8.8.8.8#节点选择"},
		"ipv6":         true,
		"ipv6-timeout": "100ms",
	})

	want := map[string]interface{}{
		"enable":       true,
		"nameserver":   []string{"udp://8.8.8.8#节点选择"},
		"ipv6":         true,
		"prefer-h3":    false,
		"ipv6-timeout": 100,
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildMihomoDNSDocument() = %#v, want %#v", got, want)
	}
}

func TestSanitizeMihomoConfigJSONDropsEmptyDNS(t *testing.T) {
	raw := json.RawMessage(`{
	  "dns": {
	    "listen": "0.0.0.0:53",
	    "fallback-filter": {
	      "geoip": true
	    }
	  },
	  "route": {
	    "rules": [],
	    "rule_set": []
	  }
	}`)

	sanitized, err := sanitizeMihomoConfigJSON(raw)
	if err != nil {
		t.Fatalf("sanitizeMihomoConfigJSON() error = %v", err)
	}

	var document map[string]interface{}
	if err := json.Unmarshal(sanitized, &document); err != nil {
		t.Fatalf("unmarshal sanitized config failed: %v", err)
	}

	if _, exists := document["dns"]; exists {
		t.Fatalf("expected empty dns block to be removed, got %#v", document["dns"])
	}
}

func TestSanitizeMihomoConfigJSONKeepsDNSPreferH3(t *testing.T) {
	raw := json.RawMessage(`{
	  "dns": {
	    "nameserver": ["https://dns.alidns.com/dns-query"],
	    "prefer-h3": "true"
	  },
	  "route": {
	    "rules": [],
	    "rule_set": []
	  }
	}`)

	sanitized, err := sanitizeMihomoConfigJSON(raw)
	if err != nil {
		t.Fatalf("sanitizeMihomoConfigJSON() error = %v", err)
	}

	var document map[string]interface{}
	if err := json.Unmarshal(sanitized, &document); err != nil {
		t.Fatalf("unmarshal sanitized config failed: %v", err)
	}

	dns, _ := document["dns"].(map[string]interface{})
	if dns == nil {
		t.Fatalf("expected dns block in sanitized config")
	}
	if got, ok := dns["prefer-h3"].(bool); !ok || !got {
		t.Fatalf("expected dns.prefer-h3=true, got %#v", dns["prefer-h3"])
	}
}

func TestSanitizeMihomoConfigJSONKeepsTopLevelIPv6(t *testing.T) {
	raw := json.RawMessage(`{
	  "ipv6": "true",
	  "route": {
	    "rules": [],
	    "rule_set": []
	  }
	}`)

	sanitized, err := sanitizeMihomoConfigJSON(raw)
	if err != nil {
		t.Fatalf("sanitizeMihomoConfigJSON() error = %v", err)
	}

	var document map[string]interface{}
	if err := json.Unmarshal(sanitized, &document); err != nil {
		t.Fatalf("unmarshal sanitized config failed: %v", err)
	}

	if got, ok := document["ipv6"].(bool); !ok || !got {
		t.Fatalf("expected top-level ipv6=true, got %#v", document["ipv6"])
	}
}

func TestSanitizeMihomoConfigJSONDropsInvalidTopLevelIPv6(t *testing.T) {
	raw := json.RawMessage(`{
	  "ipv6": "",
	  "route": {
	    "rules": [],
	    "rule_set": []
	  }
	}`)

	sanitized, err := sanitizeMihomoConfigJSON(raw)
	if err != nil {
		t.Fatalf("sanitizeMihomoConfigJSON() error = %v", err)
	}

	var document map[string]interface{}
	if err := json.Unmarshal(sanitized, &document); err != nil {
		t.Fatalf("unmarshal sanitized config failed: %v", err)
	}

	if _, exists := document["ipv6"]; exists {
		t.Fatalf("expected invalid top-level ipv6 to be removed, got %#v", document["ipv6"])
	}
}

func TestSanitizeMihomoConfigJSONKeepsTopLevelTCPConcurrent(t *testing.T) {
	raw := json.RawMessage(`{
	  "tcp-concurrent": "true",
	  "route": {
	    "rules": [],
	    "rule_set": []
	  }
	}`)

	sanitized, err := sanitizeMihomoConfigJSON(raw)
	if err != nil {
		t.Fatalf("sanitizeMihomoConfigJSON() error = %v", err)
	}

	var document map[string]interface{}
	if err := json.Unmarshal(sanitized, &document); err != nil {
		t.Fatalf("unmarshal sanitized config failed: %v", err)
	}

	if got, ok := document["tcp-concurrent"].(bool); !ok || !got {
		t.Fatalf("expected top-level tcp-concurrent=true, got %#v", document["tcp-concurrent"])
	}
}

func TestSanitizeMihomoConfigJSONDropsInvalidTopLevelTCPConcurrent(t *testing.T) {
	raw := json.RawMessage(`{
	  "tcp-concurrent": "",
	  "route": {
	    "rules": [],
	    "rule_set": []
	  }
	}`)

	sanitized, err := sanitizeMihomoConfigJSON(raw)
	if err != nil {
		t.Fatalf("sanitizeMihomoConfigJSON() error = %v", err)
	}

	var document map[string]interface{}
	if err := json.Unmarshal(sanitized, &document); err != nil {
		t.Fatalf("unmarshal sanitized config failed: %v", err)
	}

	if _, exists := document["tcp-concurrent"]; exists {
		t.Fatalf("expected invalid top-level tcp-concurrent to be removed, got %#v", document["tcp-concurrent"])
	}
}

func TestCopyMihomoGeneralConfigKeepsTopLevelTCPConcurrent(t *testing.T) {
	base := map[string]interface{}{
		"tcp-concurrent": true,
		"dns": map[string]interface{}{
			"nameserver": []interface{}{"udp://8.8.8.8"},
		},
	}

	document := copyMihomoGeneralConfig(base)
	if got, ok := document["tcp-concurrent"].(bool); !ok || !got {
		t.Fatalf("expected copied top-level tcp-concurrent=true, got %#v", document["tcp-concurrent"])
	}
	if _, exists := document["dns"]; exists {
		t.Fatalf("expected dns to be excluded from copyMihomoGeneralConfig result, got %#v", document["dns"])
	}
}

func TestSanitizeMihomoConfigJSONNormalizesRouteNoResolve(t *testing.T) {
	raw := json.RawMessage(`{
	  "route": {
	    "no-resolve": "false",
	    "rules": [],
	    "rule_set": []
	  }
	}`)

	sanitized, err := sanitizeMihomoConfigJSON(raw)
	if err != nil {
		t.Fatalf("sanitizeMihomoConfigJSON() error = %v", err)
	}

	var document map[string]interface{}
	if err := json.Unmarshal(sanitized, &document); err != nil {
		t.Fatalf("unmarshal sanitized config failed: %v", err)
	}

	route, _ := document["route"].(map[string]interface{})
	if route == nil {
		t.Fatalf("expected route block in sanitized config")
	}
	if got, ok := route["no_resolve"].(bool); !ok || got {
		t.Fatalf("expected route.no_resolve=false, got %#v", route["no_resolve"])
	}
	if _, exists := route["no-resolve"]; exists {
		t.Fatalf("expected legacy no-resolve key to be removed, got %#v", route["no-resolve"])
	}
}
