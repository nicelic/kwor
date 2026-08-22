package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestValidateMihomoTLSRecordSizeRejectsOversizedConfig(t *testing.T) {
	tls := &model.MihomoTls{
		Server: json.RawMessage(strings.Repeat("x", maxMihomoTLSJSONBytes+1)),
		Client: json.RawMessage(`{}`),
	}
	if err := validateMihomoTLSRecordSize(tls); err == nil {
		t.Fatal("expected oversized Mihomo TLS configuration to be rejected")
	}
}

func TestValidateMihomoTLSJSONShapeRejectsNonObject(t *testing.T) {
	for _, raw := range []json.RawMessage{json.RawMessage(`[]`), json.RawMessage(`"tls"`), json.RawMessage(`123`)} {
		if err := validateMihomoTLSJSONShape(raw, "server"); err == nil {
			t.Fatalf("expected non-object JSON to be rejected: %s", raw)
		}
	}
}

func TestValidateMihomoTLSJSONShapeAllowsEmptyAndNull(t *testing.T) {
	for _, raw := range []json.RawMessage{nil, json.RawMessage(`null`), json.RawMessage(`{}`)} {
		if err := validateMihomoTLSJSONShape(raw, "server"); err != nil {
			t.Fatalf("expected empty/object JSON to be accepted: %s: %v", raw, err)
		}
	}
}

func TestMihomoTlsSanitizeKeepsOnlyOneWrapperUser(t *testing.T) {
	tests := []struct {
		name   string
		mode   string
		server string
		key    string
	}{
		{
			name:   "shadowtls",
			mode:   model.MihomoTlsModeShadowTLS,
			server: `{"shadow_tls":{"version":3,"users":[{"name":"one","password":"p"},{"name":"two","password":"q"}]}}`,
			key:    "shadow_tls",
		},
		{
			name:   "jls",
			mode:   model.MihomoTlsModeJLS,
			server: `{"jls_config":{"users":[{"username":"one","password":"p"},{"username":"two","password":"q"}]}}`,
			key:    "jls_config",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &model.MihomoTls{Mode: tt.mode, Server: json.RawMessage(tt.server), Client: json.RawMessage(`{}`)}
			config.Sanitize()
			var server map[string]interface{}
			if err := json.Unmarshal(config.Server, &server); err != nil {
				t.Fatalf("decode sanitized server: %v", err)
			}
			wrapper, ok := server[tt.key].(map[string]interface{})
			if !ok {
				t.Fatalf("missing wrapper %q: %#v", tt.key, server)
			}
			users, ok := wrapper["users"].([]interface{})
			if !ok || len(users) != 1 {
				t.Fatalf("expected one wrapper user, got %#v", wrapper["users"])
			}
		})
	}
}

func TestMihomoTlsSanitizeRemovesEmptyRootSNIAndALPN(t *testing.T) {
	config := &model.MihomoTls{
		Mode:   model.MihomoTlsModeTLS,
		Server: json.RawMessage(`{"server_name":"  ","alpn":[]}`),
		Client: json.RawMessage(`{"server_name":"","alpn":[" ",""]}`),
	}
	config.Sanitize()

	for name, raw := range map[string]json.RawMessage{"server": config.Server, "client": config.Client} {
		payload := map[string]interface{}{}
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("decode %s payload: %v", name, err)
		}
		if _, exists := payload["server_name"]; exists {
			t.Fatalf("%s payload kept empty server_name: %#v", name, payload)
		}
		if _, exists := payload["alpn"]; exists {
			t.Fatalf("%s payload kept empty alpn: %#v", name, payload)
		}
	}
}

func TestMihomoTlsSanitizePreservesJLSNestedALPN(t *testing.T) {
	config := &model.MihomoTls{
		Mode:   model.MihomoTlsModeJLS,
		Server: json.RawMessage(`{"server_name":"","alpn":[],"jls_config":{"dest":"edge.example.com:443","alpn":["h2"],"users":[{"username":"u","password":"p"}]}}`),
		Client: json.RawMessage(`{"jls_opts":{"username":"u","password":"p"}}`),
	}
	config.Sanitize()

	server := map[string]interface{}{}
	if err := json.Unmarshal(config.Server, &server); err != nil {
		t.Fatalf("decode server payload: %v", err)
	}
	if _, exists := server["alpn"]; exists {
		t.Fatalf("root empty alpn should be removed: %#v", server)
	}
	wrapper, ok := server["jls_config"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing jls_config: %#v", server)
	}
	alpn, ok := wrapper["alpn"].([]interface{})
	if !ok || len(alpn) != 1 || alpn[0] != "h2" {
		t.Fatalf("JLS nested alpn was not preserved: %#v", wrapper["alpn"])
	}
}

func TestValidateDefaultTLSRecordSizeRejectsOversizedConfig(t *testing.T) {
	tls := &model.Tls{
		Server: json.RawMessage(strings.Repeat("x", maxDefaultTLSJSONBytes+1)),
		Client: json.RawMessage(`{}`),
	}
	if err := validateDefaultTLSRecordSize(tls); err == nil {
		t.Fatal("expected oversized sing-box TLS configuration to be rejected")
	}
}

func TestValidateDefaultTLSJSONShapeRejectsNonObject(t *testing.T) {
	for _, raw := range []json.RawMessage{json.RawMessage(`[]`), json.RawMessage(`"tls"`), json.RawMessage(`123`)} {
		if err := validateDefaultTLSJSONShape(raw, "server"); err == nil {
			t.Fatalf("expected non-object JSON to be rejected: %s", raw)
		}
	}
}

func TestValidateMihomoTLSModesMatchServerAndClient(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		server  string
		client  string
		wantErr bool
	}{
		{
			name:   "shadowtls v3",
			mode:   model.MihomoTlsModeShadowTLS,
			server: `{"shadow_tls":{"enable":true,"version":3,"users":[{"name":"u","password":"p"}],"handshake":{"dest":"www.example.com:443"}}}`,
			client: `{"shadow_tls_opts":{"version":3,"password":"p"}}`,
		},
		{
			name:   "shadowtls v1",
			mode:   model.MihomoTlsModeShadowTLS,
			server: `{"shadow_tls":{"enable":true,"version":1,"handshake":{"dest":"www.example.com:443"}}}`,
			client: `{"shadow_tls_opts":{"version":1}}`,
		},
		{
			name:   "shadowtls v2",
			mode:   model.MihomoTlsModeShadowTLS,
			server: `{"shadow_tls":{"enable":true,"version":2,"password":"p","handshake":{"dest":"www.example.com:443"}}}`,
			client: `{"shadow_tls_opts":{"version":2,"password":"p"}}`,
		},
		{
			name:    "shadowtls v2 password mismatch",
			mode:    model.MihomoTlsModeShadowTLS,
			server:  `{"shadow_tls":{"enable":true,"version":2,"password":"server","handshake":{"dest":"www.example.com:443"}}}`,
			client:  `{"shadow_tls_opts":{"version":2,"password":"client"}}`,
			wantErr: true,
		},
		{
			name:    "shadowtls v2 whitespace password mismatch",
			mode:    model.MihomoTlsModeShadowTLS,
			server:  `{"shadow_tls":{"enable":true,"version":2,"password":"p ","handshake":{"dest":"www.example.com:443"}}}`,
			client:  `{"shadow_tls_opts":{"version":2,"password":"p"}}`,
			wantErr: true,
		},
		{
			name:    "shadowtls v3 multiple users",
			mode:    model.MihomoTlsModeShadowTLS,
			server:  `{"shadow_tls":{"enable":true,"version":3,"users":[{"name":"u","password":"p"},{"name":"v","password":"q"}],"handshake":{"dest":"www.example.com:443"}}}`,
			client:  `{"shadow_tls_opts":{"version":3,"password":"p"}}`,
			wantErr: true,
		},
		{
			name:   "restls",
			mode:   model.MihomoTlsModeRestls,
			server: `{"res_tls":{"enable":true,"dest":"www.example.com:443","password":"p","min_record_len":64,"rate_limit":204800}}`,
			client: `{"restls_opts":{"password":"p","version_hint":"tls13"}}`,
		},
		{
			name:   "jls",
			mode:   model.MihomoTlsModeJLS,
			server: `{"jls_config":{"enable":true,"users":[{"username":"u","password":"p"}],"dest":"www.example.com:443","alpn":["h2"],"rate_limit":100}}`,
			client: `{"jls_opts":{"username":"u","password":"p"}}`,
		},
		{
			name:    "jls multiple users",
			mode:    model.MihomoTlsModeJLS,
			server:  `{"jls_config":{"enable":true,"users":[{"username":"u","password":"p"},{"username":"v","password":"q"}],"dest":"www.example.com:443"}}`,
			client:  `{"jls_opts":{"username":"u","password":"p"}}`,
			wantErr: true,
		},
		{
			name:   "shadowtls wildcard SNI without default handshake",
			mode:   model.MihomoTlsModeShadowTLS,
			server: `{"shadow_tls":{"enable":true,"version":3,"wildcard_sni":"all","users":[{"name":"u","password":"p"}],"handshake_for_server_name":{"www.example.com":{"dest":"www.example.com:443"}}}}`,
			client: `{"shadow_tls_opts":{"version":3,"password":"p"}}`,
		},
		{
			name:    "restls password mismatch",
			mode:    model.MihomoTlsModeRestls,
			server:  `{"res_tls":{"dest":"www.example.com:443","password":"server"}}`,
			client:  `{"restls_opts":{"password":"client","version_hint":"tls13"}}`,
			wantErr: true,
		},
		{
			name:    "shadowtls v3 password mismatch",
			mode:    model.MihomoTlsModeShadowTLS,
			server:  `{"shadow_tls":{"enable":true,"version":3,"users":[{"name":"u","password":"server"}],"handshake":{"dest":"www.example.com:443"}}}`,
			client:  `{"shadow_tls_opts":{"version":3,"password":"client"}}`,
			wantErr: true,
		},
		{
			name:    "jls unknown user",
			mode:    model.MihomoTlsModeJLS,
			server:  `{"jls_config":{"users":[{"username":"u","password":"p"}],"dest":"www.example.com:443"}}`,
			client:  `{"jls_opts":{"username":"other","password":"p"}}`,
			wantErr: true,
		},
		{
			name:    "jls whitespace credential mismatch",
			mode:    model.MihomoTlsModeJLS,
			server:  `{"jls_config":{"users":[{"username":"u ","password":"p"}],"dest":"www.example.com:443"}}`,
			client:  `{"jls_opts":{"username":"u","password":"p"}}`,
			wantErr: true,
		},
		{
			name:    "shadowtls invalid wildcard SNI",
			mode:    model.MihomoTlsModeShadowTLS,
			server:  `{"shadow_tls":{"version":3,"wildcard_sni":true,"users":[{"name":"u","password":"p"}],"handshake":{"dest":"www.example.com:443"}}}`,
			client:  `{"shadow_tls_opts":{"version":3,"password":"p"}}`,
			wantErr: true,
		},
		{
			name:    "restls invalid destination",
			mode:    model.MihomoTlsModeRestls,
			server:  `{"res_tls":{"dest":"www.example.com","password":"p"}}`,
			client:  `{"restls_opts":{"password":"p","version_hint":"tls13"}}`,
			wantErr: true,
		},
		{
			name:    "restls non-integer min record length",
			mode:    model.MihomoTlsModeRestls,
			server:  `{"res_tls":{"dest":"www.example.com:443","password":"p","min_record_len":"64"}}`,
			client:  `{"restls_opts":{"password":"p","version_hint":"tls13"}}`,
			wantErr: true,
		},
		{
			name:    "restls non-integer rate limit",
			mode:    model.MihomoTlsModeRestls,
			server:  `{"res_tls":{"dest":"www.example.com:443","password":"p","rate_limit":"204800"}}`,
			client:  `{"restls_opts":{"password":"p","version_hint":"tls13"}}`,
			wantErr: true,
		},
		{
			name:    "jls non-integer rate limit",
			mode:    model.MihomoTlsModeJLS,
			server:  `{"jls_config":{"users":[{"username":"u","password":"p"}],"dest":"www.example.com:443","rate_limit":"100"}}`,
			client:  `{"jls_opts":{"username":"u","password":"p"}}`,
			wantErr: true,
		},
		{
			name:    "shadowtls fractional version",
			mode:    model.MihomoTlsModeShadowTLS,
			server:  `{"shadow_tls":{"version":3.5,"users":[{"name":"u","password":"p"}],"handshake":{"dest":"www.example.com:443"}}}`,
			client:  `{"shadow_tls_opts":{"version":3,"password":"p"}}`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tls := &model.MihomoTls{Mode: tt.mode, Server: json.RawMessage(tt.server), Client: json.RawMessage(tt.client)}
			err := validateMihomoTLSMode(tls)
			if tt.wantErr && err == nil {
				t.Fatal("expected validation error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestValidateMihomoInboundTLSModeRestrictions(t *testing.T) {
	reality := &model.MihomoTls{Mode: model.MihomoTlsModeReality, Server: json.RawMessage(`{"reality":{"enabled":true}}`), Client: json.RawMessage(`{}`)}
	if err := validateMihomoInboundTLSMode(&model.MihomoInbound{Type: "anytls", Tls: reality}); err != nil {
		t.Fatalf("AnyTLS Reality should be accepted: %v", err)
	}
	shadow := &model.MihomoTls{Mode: model.MihomoTlsModeShadowTLS, Server: json.RawMessage(`{"shadow_tls":{}}`), Client: json.RawMessage(`{"shadow_tls_opts":{}}`)}
	if err := validateMihomoInboundTLSMode(&model.MihomoInbound{Type: "snell", Tls: shadow}); err == nil {
		t.Fatal("expected wrapper TLS on unsupported inbound to be rejected")
	}
	if err := validateMihomoInboundTLSMode(&model.MihomoInbound{Type: "shadowsocks", Tls: shadow}); err != nil {
		t.Fatalf("Shadowsocks wrapper TLS should be accepted: %v", err)
	}
}

func TestValidateMihomoTLSModeRejectsWrapperFieldShapeErrors(t *testing.T) {
	tests := []struct {
		name   string
		mode   string
		server string
		client string
	}{
		{
			name:   "shadowtls handshake proxy",
			mode:   model.MihomoTlsModeShadowTLS,
			server: `{"shadow_tls":{"version":3,"users":[{"name":"u","password":"p"}],"handshake":{"dest":"edge.example.com:443","proxy":123}}}`,
			client: `{"shadow_tls_opts":{"version":3,"password":"p"}}`,
		},
		{
			name:   "restls script",
			mode:   model.MihomoTlsModeRestls,
			server: `{"res_tls":{"dest":"edge.example.com:443","password":"p","restls_script":123}}`,
			client: `{"restls_opts":{"password":"p","version_hint":"tls13"}}`,
		},
		{
			name:   "jls alpn",
			mode:   model.MihomoTlsModeJLS,
			server: `{"jls_config":{"users":[{"username":"u","password":"p"}],"dest":"edge.example.com:443","alpn":"h2"}}`,
			client: `{"jls_opts":{"username":"u","password":"p"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMihomoTLSMode(&model.MihomoTls{
				Mode:   tt.mode,
				Server: json.RawMessage(tt.server),
				Client: json.RawMessage(tt.client),
			})
			if err == nil {
				t.Fatal("expected wrapper field shape validation error")
			}
		})
	}
}
