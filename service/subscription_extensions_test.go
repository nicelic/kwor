package service

import (
	"encoding/json"
	"testing"
)

func TestClashSubscriptionDefaultsAndExplicitValues(t *testing.T) {
	defaults, err := ParseSubClashExtension("")
	if err != nil {
		t.Fatalf("parse canonical Clash subscription extension: %v", err)
	}
	if mode, _ := defaults["find-process-mode"].(string); mode != "always" {
		t.Fatalf("canonical find-process-mode = %q, want %q", mode, "always")
	}
	profile, ok := defaults["profile"].(map[string]interface{})
	if !ok {
		t.Fatalf("canonical profile has unexpected type: %T", defaults["profile"])
	}
	if storeFakeIP, _ := profile["store-fake-ip"].(bool); storeFakeIP {
		t.Fatal("canonical store-fake-ip = true, want false")
	}
	dns, ok := defaults["dns"].(map[string]interface{})
	if !ok {
		t.Fatalf("canonical dns has unexpected type: %T", defaults["dns"])
	}
	if useHosts, exists := dns["use-hosts"]; !exists || useHosts != false {
		t.Fatalf("canonical use-hosts = %#v, want false", useHosts)
	}

	explicit, err := ParseSubClashExtension(`find-process-mode: strict
profile:
  store-fake-ip: true
dns:
  use-hosts: true
`)
	if err != nil {
		t.Fatalf("parse explicit Clash subscription values: %v", err)
	}
	if mode, _ := explicit["find-process-mode"].(string); mode != "strict" {
		t.Fatalf("explicit find-process-mode = %q, want %q", mode, "strict")
	}
	explicitProfile, ok := explicit["profile"].(map[string]interface{})
	if !ok {
		t.Fatalf("explicit profile has unexpected type: %T", explicit["profile"])
	}
	if storeFakeIP, _ := explicitProfile["store-fake-ip"].(bool); !storeFakeIP {
		t.Fatal("explicit store-fake-ip = false, want true")
	}
	explicitDNS, ok := explicit["dns"].(map[string]interface{})
	if !ok {
		t.Fatalf("explicit dns has unexpected type: %T", explicit["dns"])
	}
	if useHosts, ok := explicitDNS["use-hosts"].(bool); !ok || !useHosts {
		t.Fatalf("explicit use-hosts = %#v, want true", explicitDNS["use-hosts"])
	}
}

func TestNormalizeSubJSONExtensionCanonicalizesDNSHTTPPaths(t *testing.T) {
	normalized, err := NormalizeSubscriptionExtension("subJsonExt", `{
  "dns": {
    "servers": [
      {"tag":"doh-default","type":"https","server":"dns.example","server_port":443},
      {"tag":"doh-legacy","type":"h3","server":"dns.example","server_port":443,"path":"dns-query"},
      {"tag":"doh-custom","type":"https","server":"dns.example","server_port":443,"path":" custom-doh "},
      {"tag":"dot","type":"tls","server":"dns.example","server_port":853,"path":"/dns-query"}
    ]
  }
}`)
	if err != nil {
		t.Fatalf("normalize JSON subscription extension: %v", err)
	}

	root := map[string]interface{}{}
	if err := json.Unmarshal([]byte(normalized), &root); err != nil {
		t.Fatalf("decode normalized JSON subscription extension: %v", err)
	}
	dns := root["dns"].(map[string]interface{})
	servers := dns["servers"].([]interface{})
	byTag := make(map[string]map[string]interface{}, len(servers))
	for _, raw := range servers {
		server := raw.(map[string]interface{})
		byTag[server["tag"].(string)] = server
	}

	for tag, expectedPath := range map[string]string{
		"doh-default": "/dns-query",
		"doh-legacy":  "/dns-query",
		"doh-custom":  "/custom-doh",
	} {
		if path, _ := byTag[tag]["path"].(string); path != expectedPath {
			t.Fatalf("%s path = %q, want %q", tag, path, expectedPath)
		}
	}
	if _, exists := byTag["dot"]["path"]; exists {
		t.Fatalf("DoT path must be removed: %#v", byTag["dot"])
	}
}

func TestSubscriptionResourceBounds(t *testing.T) {
	makeRows := func(count int) []interface{} {
		rows := make([]interface{}, count)
		for index := range rows {
			rows[index] = map[string]interface{}{"values": []interface{}{"example.com"}}
		}
		return rows
	}

	jsonRoot := map[string]interface{}{
		"_uiConfig": map[string]interface{}{
			"ruleRows": makeRows(SubscriptionJSONMaxEditorRuleRows),
		},
	}
	if err := ValidateSubJSONExtension(jsonRoot); err != nil {
		t.Fatalf("JSON resource limit should include %d rows: %v", SubscriptionJSONMaxEditorRuleRows, err)
	}
	jsonRoot["_uiConfig"].(map[string]interface{})["ruleRows"] = makeRows(SubscriptionJSONMaxEditorRuleRows + 1)
	if err := ValidateSubJSONExtension(jsonRoot); err == nil {
		t.Fatalf("JSON resource limit accepted %d rows", SubscriptionJSONMaxEditorRuleRows+1)
	}

	clashRoot := map[string]interface{}{
		"_uiConfig": map[string]interface{}{
			"clashRuleRows": makeRows(SubscriptionClashMaxEditorRuleRows),
		},
		"rules": makeRows(SubscriptionClashMaxRules),
	}
	if err := ValidateSubClashExtension(clashRoot); err != nil {
		t.Fatalf("Clash resource limits should allow 800 editor rows and 2048 rules: %v", err)
	}
	clashRoot["rules"] = makeRows(SubscriptionClashMaxRules + 1)
	if err := ValidateSubClashExtension(clashRoot); err == nil {
		t.Fatalf("Clash rules limit accepted %d rules", SubscriptionClashMaxRules+1)
	}
}
