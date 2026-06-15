package service

import "testing"

func TestExtractProxyOutboundsRawWithoutConversion_PreservesRawFields(t *testing.T) {
	jsonData := []byte(`{
		"outbounds": [
			{
				"type": "shadowsocks",
				"tag": "ss-node",
				"server": "1.1.1.1",
				"server_port": 443,
				"method": "aes-128-gcm",
				"password": "pwd",
				"detour": "stls-node-out"
			},
			{
				"type": "shadowtls",
				"tag": "stls-node-out",
				"server": "2.2.2.2",
				"server_port": 443,
				"version": 3,
				"tls": {
					"enabled": true,
					"server_name": "example.com",
					"tls_store": "system",
					"store": "mozilla"
				}
			},
			{
				"type": "vmess",
				"tag": "vmess-node",
				"server": "3.3.3.3",
				"server_port": 443,
				"uuid": "11111111-1111-1111-1111-111111111111",
				"tls": {
					"enabled": true,
					"client_fingerprint": "chrome",
					"minVersion": "1.2",
					"maxVersion": "1.3",
					"tls_store": "system",
					"store": "mozilla"
				}
			},
			{
				"type": "selector",
				"tag": "auto",
				"outbounds": ["ss-node", "vmess-node"]
			}
		]
	}`)

	outbounds, err := extractProxyOutboundsRawWithoutConversion(jsonData)
	if err != nil {
		t.Fatalf("extractProxyOutboundsRawWithoutConversion failed: %v", err)
	}
	if len(outbounds) != 3 {
		t.Fatalf("expected 3 proxy outbounds, got %d", len(outbounds))
	}

	ss := findOutboundByTag(outbounds, "ss-node")
	if ss == nil {
		t.Fatalf("expected ss-node outbound")
	}
	if got, _ := ss["detour"].(string); got != "stls-node-out" {
		t.Fatalf("expected detour=stls-node-out, got %#v", ss["detour"])
	}
	if _, exists := ss["ss_config"]; exists {
		t.Fatalf("unexpected merged ss_config in raw shadowsocks outbound: %#v", ss["ss_config"])
	}

	stls := findOutboundByTag(outbounds, "stls-node-out")
	if stls == nil {
		t.Fatalf("expected stls-node-out outbound")
	}
	if got, _ := stls["type"].(string); got != "shadowtls" {
		t.Fatalf("expected shadowtls type, got %#v", stls["type"])
	}

	vmess := findOutboundByTag(outbounds, "vmess-node")
	if vmess == nil {
		t.Fatalf("expected vmess-node outbound")
	}
	tlsMap, ok := vmess["tls"].(map[string]interface{})
	if !ok || tlsMap == nil {
		t.Fatalf("expected vmess tls map, got %#v", vmess["tls"])
	}
	if got, _ := tlsMap["client_fingerprint"].(string); got != "chrome" {
		t.Fatalf("expected client_fingerprint=chrome, got %#v", tlsMap["client_fingerprint"])
	}
	if got, _ := tlsMap["minVersion"].(string); got != "1.2" {
		t.Fatalf("expected minVersion=1.2, got %#v", tlsMap["minVersion"])
	}
	if got, _ := tlsMap["maxVersion"].(string); got != "1.3" {
		t.Fatalf("expected maxVersion=1.3, got %#v", tlsMap["maxVersion"])
	}
	if got, _ := tlsMap["tls_store"].(string); got != "system" {
		t.Fatalf("expected tls_store=system, got %#v", tlsMap["tls_store"])
	}
	if got, _ := tlsMap["store"].(string); got != "mozilla" {
		t.Fatalf("expected store=mozilla, got %#v", tlsMap["store"])
	}
	if _, exists := tlsMap["utls"]; exists {
		t.Fatalf("unexpected utls injected into raw tls: %#v", tlsMap["utls"])
	}
	if _, exists := tlsMap["min_version"]; exists {
		t.Fatalf("unexpected min_version normalized key in raw tls: %#v", tlsMap["min_version"])
	}
	if _, exists := tlsMap["max_version"]; exists {
		t.Fatalf("unexpected max_version normalized key in raw tls: %#v", tlsMap["max_version"])
	}
}

func TestExtractProxyOutboundsRawWithoutConversion_FiltersInvalidAndNonProxy(t *testing.T) {
	jsonData := []byte(`{
		"outbounds": [
			{"type": "direct", "tag": "direct"},
			{"type": "vmess", "tag": "  ", "server": "1.1.1.1", "server_port": 443},
			{"type": "vless", "tag": "valid-node", "server": "2.2.2.2", "server_port": 443}
		]
	}`)

	outbounds, err := extractProxyOutboundsRawWithoutConversion(jsonData)
	if err != nil {
		t.Fatalf("extractProxyOutboundsRawWithoutConversion failed: %v", err)
	}
	if len(outbounds) != 1 {
		t.Fatalf("expected 1 outbound, got %d (%#v)", len(outbounds), outbounds)
	}
	if got, _ := outbounds[0]["tag"].(string); got != "valid-node" {
		t.Fatalf("expected tag=valid-node, got %#v", outbounds[0]["tag"])
	}
}

func TestExtractProxyOutboundsRawWithoutConversion_KeepsShadowTLSType(t *testing.T) {
	jsonData := []byte(`{
		"outbounds": [
			{
				"type": "shadowtls",
				"tag": "shadowtls-node",
				"server": "4.4.4.4",
				"server_port": 443,
				"version": 3,
				"ss_config": {
					"method": "2022-blake3-aes-128-gcm",
					"password": "pwd"
				}
			}
		]
	}`)

	outbounds, err := extractProxyOutboundsRawWithoutConversion(jsonData)
	if err != nil {
		t.Fatalf("extractProxyOutboundsRawWithoutConversion failed: %v", err)
	}
	if len(outbounds) != 1 {
		t.Fatalf("expected 1 outbound, got %d", len(outbounds))
	}

	outbound := outbounds[0]
	if got, _ := outbound["type"].(string); got != "shadowtls" {
		t.Fatalf("expected type=shadowtls, got %#v", outbound["type"])
	}
	if _, exists := outbound["detour"]; exists {
		t.Fatalf("unexpected detour in imported shadowtls: %#v", outbound["detour"])
	}
	if _, exists := outbound["method"]; exists {
		t.Fatalf("unexpected shadowsocks method field at top-level: %#v", outbound["method"])
	}
	ssConfig, ok := outbound["ss_config"].(map[string]interface{})
	if !ok || ssConfig == nil {
		t.Fatalf("expected ss_config map, got %#v", outbound["ss_config"])
	}
	if got, _ := ssConfig["method"].(string); got != "2022-blake3-aes-128-gcm" {
		t.Fatalf("expected ss_config.method preserved, got %#v", ssConfig["method"])
	}
}

func findOutboundByTag(outbounds []map[string]interface{}, tag string) map[string]interface{} {
	for _, outbound := range outbounds {
		if got, _ := outbound["tag"].(string); got == tag {
			return outbound
		}
	}
	return nil
}
