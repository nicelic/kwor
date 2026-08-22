package model

import (
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
)

func TestMihomoTlsSanitize_KeepsServerSHA256AndStripsClientSHA256(t *testing.T) {
	raw := []byte(`{
		"id": 1,
		"name": "tls-a",
		"server": {
			"min_version": "1.2",
			"client_authentication": "require-and-verify",
			"client_certificate_path": "/tmp/client.pem",
			"client_certificate_public_key_sha256": ["server-hash"]
		},
		"client": {
			"enabled": true,
			"fingerprint": "AA:BB",
			"include_server_certificate": false,
			"mihomo_use_fingerprint": true,
			"tls_store": "mozilla",
			"certificate_path": "/tmp/cert.pem",
			"certificate_public_key_sha256": ["client-hash"]
		}
	}`)

	var tls MihomoTls
	if err := json.Unmarshal(raw, &tls); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	tls.Sanitize()

	var server map[string]interface{}
	if err := json.Unmarshal(tls.Server, &server); err != nil {
		t.Fatalf("decode server failed: %v", err)
	}
	if _, exists := server["min_version"]; exists {
		t.Fatalf("unexpected min_version in sanitized server payload: %#v", server["min_version"])
	}
	if _, exists := server["client_authentication"]; exists {
		t.Fatalf("unexpected client_authentication in sanitized server payload: %#v", server["client_authentication"])
	}
	if _, exists := server["client_certificate_path"]; exists {
		t.Fatalf("unexpected client_certificate_path in sanitized server payload: %#v", server["client_certificate_path"])
	}
	if _, exists := server["client_certificate_public_key_sha256"]; exists {
		t.Fatalf("unexpected client_certificate_public_key_sha256 in sanitized server payload: %#v", server["client_certificate_public_key_sha256"])
	}

	var client map[string]interface{}
	if err := json.Unmarshal(tls.Client, &client); err != nil {
		t.Fatalf("decode client failed: %v", err)
	}
	for _, key := range []string{"mihomo_use_fingerprint", "tls_store", "certificate_path"} {
		if _, exists := client[key]; exists {
			t.Fatalf("unexpected %s in sanitized client payload: %#v", key, client[key])
		}
	}
	if got, _ := client["fingerprint"].(string); got != "AA:BB" {
		t.Fatalf("expected fingerprint to remain, got %#v", client["fingerprint"])
	}
	if got, ok := client["include_server_certificate"].(bool); !ok || got {
		t.Fatalf("expected include_server_certificate=false to remain, got %#v", client["include_server_certificate"])
	}
	if got, ok := client["certificate_public_key_sha256"].([]interface{}); !ok || len(got) != 1 || got[0] != "client-hash" {
		t.Fatalf("expected certificate_public_key_sha256 to remain, got %#v", client["certificate_public_key_sha256"])
	}
}

func TestNormalizeStoredBandwidthValueRejectsFractionalNumbers(t *testing.T) {
	if _, ok := normalizeStoredBandwidthValue(float64(10.5)); ok {
		t.Fatal("fractional bandwidth must not be truncated")
	}
	if value, ok := normalizeStoredBandwidthValue(float64(10)); !ok || value != 10 {
		t.Fatalf("integral bandwidth = (%d, %v), want (10, true)", value, ok)
	}
}

func TestMihomoTlsSanitizeSynchronizesRestlsScriptKeepsRateLimitAndShadowTLSShape(t *testing.T) {
	tls := MihomoTls{
		Mode:   MihomoTlsModeRestls,
		Server: json.RawMessage(`{"res_tls":{"dest":"edge.example.com:443","password":"p","restls_script":"server-script","rate_limit":100}}`),
		Client: json.RawMessage(`{"restls_opts":{"password":"p","version_hint":"tls13","restls_script":"client-script"}}`),
	}
	tls.Sanitize()
	var server, client map[string]interface{}
	if err := json.Unmarshal(tls.Server, &server); err != nil {
		t.Fatalf("decode server failed: %v", err)
	}
	if err := json.Unmarshal(tls.Client, &client); err != nil {
		t.Fatalf("decode client failed: %v", err)
	}
	resTLS := server["res_tls"].(map[string]interface{})
	restlsOpts := client["restls_opts"].(map[string]interface{})
	if resTLS["restls_script"] != "server-script" || restlsOpts["restls_script"] != "server-script" {
		t.Fatalf("restls script was not synchronized: server=%#v client=%#v", resTLS["restls_script"], restlsOpts["restls_script"])
	}
	if got, _ := resTLS["rate_limit"].(float64); got != 100 {
		t.Fatalf("Restls rate_limit = %#v, want 100", resTLS["rate_limit"])
	}

	shadowV2 := MihomoTls{
		Mode:   MihomoTlsModeShadowTLS,
		Server: json.RawMessage(`{"shadow_tls":{"version":2,"password":"server-pass","users":[{"name":"stale","password":"stale-pass"}],"handshake":{"dest":"edge.example.com:443"}}}`),
		Client: json.RawMessage(`{"shadow_tls_opts":{"version":2,"password":"server-pass"}}`),
	}
	shadowV2.Sanitize()
	var shadowV2Server map[string]interface{}
	if err := json.Unmarshal(shadowV2.Server, &shadowV2Server); err != nil {
		t.Fatalf("decode ShadowTLS v2 server failed: %v", err)
	}
	shadowV2Config := shadowV2Server["shadow_tls"].(map[string]interface{})
	if _, exists := shadowV2Config["users"]; exists {
		t.Fatalf("ShadowTLS v2 users must be removed: %#v", shadowV2Config)
	}

	shadowV3 := MihomoTls{
		Mode:   MihomoTlsModeShadowTLS,
		Server: json.RawMessage(`{"shadow_tls":{"version":3,"password":"stale-pass","users":[{"name":"u","password":"p"}],"handshake":{"dest":"edge.example.com:443"}}}`),
		Client: json.RawMessage(`{"shadow_tls_opts":{"version":3,"password":"p"}}`),
	}
	shadowV3.Sanitize()
	var shadowV3Server map[string]interface{}
	if err := json.Unmarshal(shadowV3.Server, &shadowV3Server); err != nil {
		t.Fatalf("decode ShadowTLS v3 server failed: %v", err)
	}
	shadowV3Config := shadowV3Server["shadow_tls"].(map[string]interface{})
	if _, exists := shadowV3Config["password"]; exists {
		t.Fatalf("ShadowTLS v3 password must be removed: %#v", shadowV3Config)
	}

	shadow := MihomoTls{
		Mode:   MihomoTlsModeShadowTLS,
		Server: json.RawMessage(`{"shadow_tls":{"version":3,"wildcard_sni":true,"handshake_for_server_name":true,"handshake":{"dest":"edge.example.com:443"}}}`),
		Client: json.RawMessage(`{"shadow_tls_opts":{"version":3,"password":"p"}}`),
	}
	shadow.Sanitize()
	var shadowServer map[string]interface{}
	if err := json.Unmarshal(shadow.Server, &shadowServer); err != nil {
		t.Fatalf("decode shadow server failed: %v", err)
	}
	shadowConfig := shadowServer["shadow_tls"].(map[string]interface{})
	if _, exists := shadowConfig["wildcard_sni"]; exists {
		t.Fatalf("invalid wildcard_sni should be removed: %#v", shadowConfig["wildcard_sni"])
	}
	if _, exists := shadowConfig["handshake_for_server_name"]; exists {
		t.Fatalf("invalid handshake_for_server_name should be removed: %#v", shadowConfig["handshake_for_server_name"])
	}
}

func TestMihomoTlsSanitizeRemovesShadowTLSV1ClientPassword(t *testing.T) {
	tls := MihomoTls{
		Mode:   MihomoTlsModeShadowTLS,
		Server: json.RawMessage(`{"shadow_tls":{"version":1,"handshake":{"dest":"edge.example.com:443"}}}`),
		Client: json.RawMessage(`{"shadow_tls_opts":{"version":1,"password":"stale-password"}}`),
	}
	tls.Sanitize()

	var client map[string]interface{}
	if err := json.Unmarshal(tls.Client, &client); err != nil {
		t.Fatalf("decode client failed: %v", err)
	}
	opts, ok := client["shadow_tls_opts"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected ShadowTLS client options, got %#v", client["shadow_tls_opts"])
	}
	if _, exists := opts["password"]; exists {
		t.Fatalf("ShadowTLS v1 client password must be removed: %#v", opts)
	}
}

func TestMihomoTlsSanitizeRemovesUnsupportedShadowTLSVersionFields(t *testing.T) {
	for _, version := range []int{1, 2} {
		t.Run("v"+strconv.Itoa(version), func(t *testing.T) {
			tls := MihomoTls{
				Mode:   MihomoTlsModeShadowTLS,
				Server: json.RawMessage(fmt.Sprintf(`{"shadow_tls":{"version":%d,"strict_mode":true,"wildcard_sni":"all","handshake_for_server_name":{"a.example.com":{"dest":"a.example.com:443"}},"handshake":{"dest":"edge.example.com:443"}}}`, version)),
				Client: json.RawMessage(fmt.Sprintf(`{"shadow_tls_opts":{"version":%d}}`, version)),
			}
			tls.Sanitize()
			var server map[string]interface{}
			if err := json.Unmarshal(tls.Server, &server); err != nil {
				t.Fatalf("decode server failed: %v", err)
			}
			wrapper := server["shadow_tls"].(map[string]interface{})
			unsupportedKeys := []string{"strict_mode", "wildcard_sni"}
			if version == 1 {
				unsupportedKeys = append(unsupportedKeys, "handshake_for_server_name")
			}
			for _, key := range unsupportedKeys {
				if _, exists := wrapper[key]; exists {
					t.Fatalf("ShadowTLS v%d must remove unsupported %s: %#v", version, key, wrapper[key])
				}
			}
		})
	}
}

func TestMihomoTlsSanitizeMovesWrapperNestedSNIToClientTLS(t *testing.T) {
	tls := MihomoTls{
		Mode:   MihomoTlsModeRestls,
		Server: json.RawMessage(`{"res_tls":{"dest":"edge.example.com:443","password":"p"}}`),
		Client: json.RawMessage(`{"restls_opts":{"password":"p","version_hint":"tls13","server_name":"cdn.example.com"}}`),
	}

	tls.Sanitize()
	var client map[string]interface{}
	if err := json.Unmarshal(tls.Client, &client); err != nil {
		t.Fatalf("decode client failed: %v", err)
	}
	if got, _ := client["server_name"].(string); got != "cdn.example.com" {
		t.Fatalf("expected wrapper SNI at client TLS level, got %#v", client["server_name"])
	}
	opts, ok := client["restls_opts"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected restls opts, got %#v", client["restls_opts"])
	}
	if _, exists := opts["server_name"]; exists {
		t.Fatalf("nested wrapper SNI must be removed: %#v", opts)
	}
}

func TestMihomoTlsSanitizeNormalizesWrapperDestinationsAndDerivesSNI(t *testing.T) {
	tests := []struct {
		name       string
		mode       string
		server     string
		client     string
		wrapperKey string
		wantDest   string
		wantSNI    string
		customSNI  string
	}{
		{
			name:       "shadowtls domain",
			mode:       MihomoTlsModeShadowTLS,
			server:     `{"shadow_tls":{"version":3,"users":[{"name":"u","password":"p"}],"handshake":{"dest":"edge.example.com"}}}`,
			client:     `{"shadow_tls_opts":{"version":3,"password":"p"}}`,
			wrapperKey: "shadow_tls",
			wantDest:   "edge.example.com:443",
			wantSNI:    "edge.example.com",
		},
		{
			name:       "restls IPv4 custom port",
			mode:       MihomoTlsModeRestls,
			server:     `{"res_tls":{"dest":"198.51.100.10:8443","password":"p"}}`,
			client:     `{"restls_opts":{"password":"p","version_hint":"tls13"}}`,
			wrapperKey: "res_tls",
			wantDest:   "198.51.100.10:8443",
			wantSNI:    "198.51.100.10",
		},
		{
			name:       "jls IPv6",
			mode:       MihomoTlsModeJLS,
			server:     `{"jls_config":{"users":[{"username":"u","password":"p"}],"dest":"[2001:db8::10]"}}`,
			client:     `{"jls_opts":{"username":"u","password":"p"}}`,
			wrapperKey: "jls_config",
			wantDest:   "[2001:db8::10]:443",
			wantSNI:    "2001:db8::10",
		},
		{
			name:       "restls preserves custom SNI",
			mode:       MihomoTlsModeRestls,
			server:     `{"res_tls":{"dest":"edge.example.com","password":"p"}}`,
			client:     `{"server_name":"cdn.example.com","restls_opts":{"password":"p","version_hint":"tls13"}}`,
			wrapperKey: "res_tls",
			wantDest:   "edge.example.com:443",
			wantSNI:    "cdn.example.com",
			customSNI:  "cdn.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tls := MihomoTls{
				Mode:   tt.mode,
				Server: json.RawMessage(tt.server),
				Client: json.RawMessage(tt.client),
			}
			tls.Sanitize()

			var server, client map[string]interface{}
			if err := json.Unmarshal(tls.Server, &server); err != nil {
				t.Fatalf("decode server failed: %v", err)
			}
			if err := json.Unmarshal(tls.Client, &client); err != nil {
				t.Fatalf("decode client failed: %v", err)
			}
			wrapper, ok := server[tt.wrapperKey].(map[string]interface{})
			if !ok {
				t.Fatalf("missing %s wrapper: %#v", tt.wrapperKey, server)
			}
			gotDest := wrapper["dest"]
			if tt.mode == MihomoTlsModeShadowTLS {
				handshake, ok := wrapper["handshake"].(map[string]interface{})
				if !ok {
					t.Fatalf("missing ShadowTLS handshake: %#v", wrapper)
				}
				gotDest = handshake["dest"]
			}
			if gotDest != tt.wantDest {
				t.Fatalf("destination = %#v, want %q", gotDest, tt.wantDest)
			}
			if got, _ := client["server_name"].(string); got != tt.wantSNI {
				t.Fatalf("client SNI = %#v, want %q", client["server_name"], tt.wantSNI)
			}
			if tt.mode == MihomoTlsModeJLS {
				if got, _ := wrapper["sni"].(string); got != tt.wantSNI {
					t.Fatalf("JLS listener SNI = %#v, want %q", wrapper["sni"], tt.wantSNI)
				}
			}
			if tt.customSNI != "" && client["server_name"] != tt.customSNI {
				t.Fatalf("custom SNI was overwritten: %#v", client["server_name"])
			}
		})
	}
}

func TestMihomoTlsSanitizeSynchronizesJLSSNIWithClientTLS(t *testing.T) {
	tls := MihomoTls{
		Mode:   MihomoTlsModeJLS,
		Server: json.RawMessage(`{"jls_config":{"dest":"edge.example.com:443","sni":"server.example.com","users":[{"username":"u","password":"p"}]}}`),
		Client: json.RawMessage(`{"server_name":"client.example.com","jls_opts":{"username":"u","password":"p"}}`),
	}
	tls.Sanitize()

	var server, client map[string]interface{}
	if err := json.Unmarshal(tls.Server, &server); err != nil {
		t.Fatalf("decode server failed: %v", err)
	}
	if err := json.Unmarshal(tls.Client, &client); err != nil {
		t.Fatalf("decode client failed: %v", err)
	}
	config, ok := server["jls_config"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected jls_config payload, got %#v", server["jls_config"])
	}
	if got, _ := config["sni"].(string); got != "client.example.com" {
		t.Fatalf("JLS server SNI = %#v, want client.example.com", config["sni"])
	}
	if got, _ := client["server_name"].(string); got != "client.example.com" {
		t.Fatalf("JLS client SNI = %#v, want client.example.com", client["server_name"])
	}
}

func TestMihomoTlsSanitizeDerivesJLSSNIWhenClientSNIIsBlank(t *testing.T) {
	tls := MihomoTls{
		Mode:   MihomoTlsModeJLS,
		Server: json.RawMessage(`{"jls_config":{"dest":"edge.example.com:443","sni":"server.example.com","users":[{"username":"u","password":"p"}]}}`),
		Client: json.RawMessage(`{"server_name":"","jls_opts":{"username":"u","password":"p"}}`),
	}
	tls.Sanitize()

	var server, client map[string]interface{}
	if err := json.Unmarshal(tls.Server, &server); err != nil {
		t.Fatalf("decode server failed: %v", err)
	}
	if err := json.Unmarshal(tls.Client, &client); err != nil {
		t.Fatalf("decode client failed: %v", err)
	}
	config, ok := server["jls_config"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected jls_config payload, got %#v", server["jls_config"])
	}
	if got, _ := config["sni"].(string); got != "edge.example.com" {
		t.Fatalf("blank JLS SNI must derive from destination, got %#v", config["sni"])
	}
	if got, _ := client["server_name"].(string); got != "edge.example.com" {
		t.Fatalf("blank JLS client SNI must derive from destination, got %#v", client["server_name"])
	}
}

func TestMihomoTlsSanitizeDerivesShadowTLSAndRestlsSNIWhenClientSNIIsBlank(t *testing.T) {
	tests := []struct {
		name   string
		mode   string
		server string
		client string
	}{
		{
			name:   "ShadowTLS",
			mode:   MihomoTlsModeShadowTLS,
			server: `{"shadow_tls":{"version":3,"users":[{"name":"u","password":"p"}],"handshake":{"dest":"edge.example.com:443"}}}`,
			client: `{"server_name":"","shadow_tls_opts":{"version":3,"password":"p"}}`,
		},
		{
			name:   "Restls",
			mode:   MihomoTlsModeRestls,
			server: `{"res_tls":{"dest":"edge.example.com:443","password":"p"}}`,
			client: `{"server_name":"","restls_opts":{"password":"p","version_hint":"tls13"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tls := MihomoTls{
				Mode:   tt.mode,
				Server: json.RawMessage(tt.server),
				Client: json.RawMessage(tt.client),
			}
			tls.Sanitize()

			var client map[string]interface{}
			if err := json.Unmarshal(tls.Client, &client); err != nil {
				t.Fatalf("decode client failed: %v", err)
			}
			if got, _ := client["server_name"].(string); got != "edge.example.com" {
				t.Fatalf("blank %s SNI must derive from destination, got %#v", tt.name, client["server_name"])
			}
		})
	}
}

func TestMihomoTlsSanitizeMovesLegacyWrapperServerSNIAndDropsNullNumericOptions(t *testing.T) {
	tls := MihomoTls{
		Mode:   MihomoTlsModeRestls,
		Server: json.RawMessage(`{"server_name":"legacy.example.com","res_tls":{"dest":"edge.example.com:443","password":"p","min_record_len":null,"rate_limit":null}}`),
		Client: json.RawMessage(`{"restls_opts":{"password":"p","version_hint":"tls13"}}`),
	}
	tls.Sanitize()

	var server, client map[string]interface{}
	if err := json.Unmarshal(tls.Server, &server); err != nil {
		t.Fatalf("decode server failed: %v", err)
	}
	if err := json.Unmarshal(tls.Client, &client); err != nil {
		t.Fatalf("decode client failed: %v", err)
	}
	if _, exists := server["server_name"]; exists {
		t.Fatalf("legacy wrapper server SNI must be removed from server payload: %#v", server)
	}
	if got, _ := client["server_name"].(string); got != "legacy.example.com" {
		t.Fatalf("expected legacy wrapper SNI in client payload, got %#v", client["server_name"])
	}
	resTLS, ok := server["res_tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected res_tls payload, got %#v", server["res_tls"])
	}
	for _, key := range []string{"min_record_len", "rate_limit"} {
		if _, exists := resTLS[key]; exists {
			t.Fatalf("null Restls option %s must be omitted: %#v", key, resTLS)
		}
	}

	jls := MihomoTls{
		Mode:   MihomoTlsModeJLS,
		Server: json.RawMessage(`{"jls_config":{"dest":"edge.example.com:443","users":[{"username":"u","password":"p"}],"rate_limit":null}}`),
		Client: json.RawMessage(`{"jls_opts":{"username":"u","password":"p"}}`),
	}
	jls.Sanitize()
	var jlsServer map[string]interface{}
	if err := json.Unmarshal(jls.Server, &jlsServer); err != nil {
		t.Fatalf("decode JLS server failed: %v", err)
	}
	jlsConfig, ok := jlsServer["jls_config"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected jls_config payload, got %#v", jlsServer["jls_config"])
	}
	if _, exists := jlsConfig["rate_limit"]; exists {
		t.Fatalf("null JLS rate_limit must be omitted: %#v", jlsConfig)
	}
}

func TestMihomoTlsSanitizeDoesNotPromoteInactiveWrapperSNI(t *testing.T) {
	tls := MihomoTls{
		Mode:   MihomoTlsModeTLS,
		Server: json.RawMessage(`{"enabled":true}`),
		Client: json.RawMessage(`{"shadow_tls_opts":{"version":3,"password":"p","server_name":"stale.example.com"}}`),
	}

	tls.Sanitize()
	var client map[string]interface{}
	if err := json.Unmarshal(tls.Client, &client); err != nil {
		t.Fatalf("decode client failed: %v", err)
	}
	if _, exists := client["server_name"]; exists {
		t.Fatalf("inactive wrapper SNI must not be promoted: %#v", client["server_name"])
	}
	if _, exists := client["shadow_tls_opts"]; exists {
		t.Fatalf("inactive wrapper options must be removed: %#v", client["shadow_tls_opts"])
	}
}

func TestMihomoTlsSanitizeClearsCertificateBindingForReality(t *testing.T) {
	tls := MihomoTls{
		CertificateRecordID: 42,
		Mode:                MihomoTlsModeReality,
		Server:              json.RawMessage(`{"reality":{"enabled":true},"certificate_record":"stale"}`),
		Client:              json.RawMessage(`{"reality":{"enabled":true}}`),
	}
	tls.Sanitize()
	if tls.CertificateRecordID != 0 {
		t.Fatalf("expected Reality certificate binding to be cleared, got %d", tls.CertificateRecordID)
	}
}

func TestMihomoTlsSanitizeKeepsClientECHForTLSOnly(t *testing.T) {
	tls := MihomoTls{
		Mode:   MihomoTlsModeTLS,
		Server: json.RawMessage(`{"enabled":true,"ech":{"enabled":true,"key":["server-key"]}}`),
		Client: json.RawMessage(`{
			"ech":{"enabled":true,"config":["client-config"]},
			"fragment":true,
			"fragment_fallback_delay":"500ms",
			"record_fragment":true
		}`),
	}
	tls.Sanitize()

	var client map[string]interface{}
	if err := json.Unmarshal(tls.Client, &client); err != nil {
		t.Fatalf("decode TLS client failed: %v", err)
	}
	if _, ok := client["ech"].(map[string]interface{}); !ok {
		t.Fatalf("TLS client ECH must survive sanitization: %#v", client)
	}
	for _, key := range []string{"fragment", "fragment_fallback_delay", "record_fragment"} {
		if _, exists := client[key]; exists {
			t.Fatalf("unsupported Mihomo TLS client field %q survived: %#v", key, client)
		}
	}

	tls.Mode = MihomoTlsModeReality
	tls.Sanitize()
	client = nil
	if err := json.Unmarshal(tls.Client, &client); err != nil {
		t.Fatalf("decode Reality client failed: %v", err)
	}
	if _, exists := client["ech"]; exists {
		t.Fatalf("Reality client ECH must be removed: %#v", client)
	}
}

func TestMihomoTlsSanitizeDropsInvalidRealityHandshakePort(t *testing.T) {
	tls := MihomoTls{
		Mode: MihomoTlsModeReality,
		Server: json.RawMessage(`{
			"reality": {
				"enabled": true,
				"handshake": {
					"server": "addons.mozilla.org",
					"server_port": 443.5
				}
			}
		}`),
	}
	tls.Sanitize()

	var server map[string]interface{}
	if err := json.Unmarshal(tls.Server, &server); err != nil {
		t.Fatalf("decode sanitized Reality server: %v", err)
	}
	reality, ok := server["reality"].(map[string]interface{})
	if !ok {
		t.Fatalf("Reality config missing: %#v", server)
	}
	handshake, ok := reality["handshake"].(map[string]interface{})
	if !ok {
		t.Fatalf("Reality handshake missing: %#v", reality)
	}
	if _, exists := handshake["server_port"]; exists {
		t.Fatalf("invalid Reality handshake port survived: %#v", handshake)
	}
}

func TestMihomoOutboundSanitize_StripsUnsupportedDialFieldsAndPreservesAnyTLSReality(t *testing.T) {
	raw := []byte(`{
		"type": "anytls",
		"tag": "node-a",
		"inet4_bind_address": "127.0.0.1",
		"inet6_bind_address": "::1",
		"reuse_addr": true,
		"udp_fragment": true,
		"connect_timeout": "5s",
		"domain_resolver": "dns-out",
		"tls": {
			"enabled": true,
			"fragment": true,
			"fragment_fallback_delay": "500ms",
			"record_fragment": true,
			"utls": {
				"enabled": true,
				"fingerprint": "chrome"
			},
			"reality": {
				"enabled": true,
				"public_key": "pub-key",
				"short_id": "short-id"
			}
		}
	}`)

	var outbound MihomoOutbound
	if err := outbound.UnmarshalJSON(raw); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	encoded, err := outbound.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	for _, key := range []string{
		"inet4_bind_address",
		"inet6_bind_address",
		"reuse_addr",
		"udp_fragment",
		"connect_timeout",
		"domain_resolver",
	} {
		if _, exists := payload[key]; exists {
			t.Fatalf("unexpected %s in sanitized payload: %#v", key, payload[key])
		}
	}

	tlsMap, ok := payload["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected tls map, got %#v", payload["tls"])
	}
	reality, exists := tlsMap["reality"].(map[string]interface{})
	if !exists || reality["public_key"] != "pub-key" || reality["short_id"] != "short-id" {
		t.Fatalf("expected Reality to remain in anytls payload: %#v", tlsMap["reality"])
	}
	if _, exists := tlsMap["utls"]; !exists {
		t.Fatalf("expected utls to remain for anytls: %#v", tlsMap)
	}
	for _, key := range []string{"fragment", "fragment_fallback_delay", "record_fragment"} {
		if _, exists := tlsMap[key]; exists {
			t.Fatalf("unsupported Mihomo TLS field %q survived: %#v", key, tlsMap)
		}
	}
}

func TestMihomoOutboundSanitize_StripsUnsupportedUTLSForHysteria2(t *testing.T) {
	raw := []byte(`{
		"type": "hysteria2",
		"tag": "node-a",
		"tls": {
			"enabled": true,
			"utls": {
				"enabled": true,
				"fingerprint": "chrome"
			}
		}
	}`)

	var outbound MihomoOutbound
	if err := outbound.UnmarshalJSON(raw); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	encoded, err := outbound.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	tlsMap, ok := payload["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected tls map, got %#v", payload["tls"])
	}
	if _, exists := tlsMap["utls"]; exists {
		t.Fatalf("unexpected utls in hysteria2 payload: %#v", tlsMap["utls"])
	}
}

func TestMihomoOutboundSanitize_StripsUnsupportedGroupHelperFields(t *testing.T) {
	tests := []struct {
		name         string
		raw          []byte
		expectedGone []string
		expectedKeep []string
	}{
		{
			name: "selector strips default helper and unsupported mihomo group fields",
			raw: []byte(`{
				"type": "selector",
				"tag": "group-a",
				"outbounds": ["node-a", "DIRECT"],
				"default": "node-a",
				"url": "https://cp.cloudflare.com/generate_204",
				"interval": "300s",
				"tolerance": 150,
				"idle_timeout": "30m",
				"interrupt_exist_connections": true
			}`),
			expectedGone: []string{"default", "url", "interval", "tolerance", "idle_timeout", "interrupt_exist_connections"},
			expectedKeep: []string{"outbounds"},
		},
		{
			name: "urltest keeps supported probe fields but strips stale helper fields",
			raw: []byte(`{
				"type": "urltest",
				"tag": "group-b",
				"outbounds": ["node-a"],
				"default": "node-a",
				"url": "https://cp.cloudflare.com/generate_204",
				"interval": "300s",
				"tolerance": 150,
				"idle_timeout": "30m",
				"interrupt_exist_connections": true
			}`),
			expectedGone: []string{"default", "idle_timeout", "interrupt_exist_connections"},
			expectedKeep: []string{"url", "interval", "tolerance", "outbounds"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var outbound MihomoOutbound
			if err := outbound.UnmarshalJSON(tt.raw); err != nil {
				t.Fatalf("unmarshal failed: %v", err)
			}

			encoded, err := outbound.MarshalJSON()
			if err != nil {
				t.Fatalf("marshal failed: %v", err)
			}

			var payload map[string]interface{}
			if err := json.Unmarshal(encoded, &payload); err != nil {
				t.Fatalf("decode failed: %v", err)
			}

			for _, key := range tt.expectedGone {
				if _, exists := payload[key]; exists {
					t.Fatalf("unexpected %s in sanitized payload: %#v", key, payload[key])
				}
			}
			for _, key := range tt.expectedKeep {
				if _, exists := payload[key]; !exists {
					t.Fatalf("expected %s to remain in sanitized payload: %#v", key, payload)
				}
			}
		})
	}
}
