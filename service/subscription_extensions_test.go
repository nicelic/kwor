package service

import (
	"encoding/json"
	"testing"
)

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
