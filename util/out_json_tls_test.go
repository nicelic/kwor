package util

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestReadPemFileRejectsOversizedMaterial(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oversized.pem")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", int(maxTLSPEMFileBytes)+1)), 0o600); err != nil {
		t.Fatalf("write oversized PEM file: %v", err)
	}

	if got := readPemFile(path); got != nil {
		t.Fatalf("expected oversized PEM file to be rejected, got %d lines", len(got))
	}
}

func TestPrepareTLSAndAddTLSTolerateIncompleteNestedObjects(t *testing.T) {
	tls := &model.Tls{
		Server: json.RawMessage(`{"reality":{"enabled":"true"},"ech":{"enabled":true}}`),
		Client: json.RawMessage(`{}`),
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("TLS projection panicked on incomplete nested objects: %v", recovered)
		}
	}()

	prepared := prepareTls(tls)
	if prepared == nil {
		t.Fatal("expected a non-nil prepared TLS object")
	}
	out := map[string]interface{}{}
	addTls(&out, tls)
	if _, ok := out["tls"]; !ok {
		t.Fatalf("expected TLS output to be present: %#v", out)
	}
}

func TestAddTLSProjectsMihomoWrapperClientOptions(t *testing.T) {
	tests := []struct {
		name       string
		mode       string
		server     string
		client     string
		key        string
		expectedKV map[string]interface{}
	}{
		{
			name:       "shadowtls",
			mode:       model.MihomoTlsModeShadowTLS,
			server:     `{"shadow_tls":{"enable":true,"version":3,"handshake":{"dest":"example.com:443"}}}`,
			client:     `{"server_name":"example.com","shadow_tls_opts":{"version":3,"password":"p"}}`,
			key:        "shadow_tls_opts",
			expectedKV: map[string]interface{}{"version": float64(3), "password": "p"},
		},
		{
			name:       "restls",
			mode:       model.MihomoTlsModeRestls,
			server:     `{"res_tls":{"enable":true,"dest":"example.com:443","password":"p"}}`,
			client:     `{"restls_opts":{"password":"p","version_hint":"tls13"}}`,
			key:        "restls_opts",
			expectedKV: map[string]interface{}{"password": "p", "version_hint": "tls13"},
		},
		{
			name:       "jls",
			mode:       model.MihomoTlsModeJLS,
			server:     `{"jls_config":{"enable":true,"dest":"example.com:443"}}`,
			client:     `{"jls_opts":{"username":"u","password":"p"}}`,
			key:        "jls_opts",
			expectedKV: map[string]interface{}{"username": "u", "password": "p"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tls := &model.Tls{Mode: tt.mode, Server: json.RawMessage(tt.server), Client: json.RawMessage(tt.client)}
			out := map[string]interface{}{}
			addTls(&out, tls, "vless")
			tlsMap, ok := out["tls"].(map[string]interface{})
			if !ok || tlsMap["enabled"] != true {
				t.Fatalf("expected enabled TLS map, got %#v", out["tls"])
			}
			opts, ok := tlsMap[tt.key].(map[string]interface{})
			if !ok {
				t.Fatalf("expected %s in client TLS map, got %#v", tt.key, tlsMap)
			}
			for key, expected := range tt.expectedKV {
				if opts[key] != expected {
					t.Fatalf("%s.%s = %#v, want %#v", tt.key, key, opts[key], expected)
				}
			}
		})
	}
}

func TestAddTLSProjectsWrapperSNIAtOuterTLSLevel(t *testing.T) {
	tls := &model.Tls{
		Mode:   model.MihomoTlsModeRestls,
		Server: json.RawMessage(`{"res_tls":{"enable":true,"dest":"edge.example.com:443","password":"p"}}`),
		Client: json.RawMessage(`{"server_name":"cdn.example.com","restls_opts":{"password":"p","version_hint":"tls13"}}`),
	}

	out := map[string]interface{}{}
	addTls(&out, tls, "vless")
	tlsMap, ok := out["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected TLS map, got %#v", out["tls"])
	}
	if got, _ := tlsMap["server_name"].(string); got != "cdn.example.com" {
		t.Fatalf("expected outer TLS server_name, got %#v", tlsMap["server_name"])
	}
	opts, ok := tlsMap["restls_opts"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected restls opts, got %#v", tlsMap["restls_opts"])
	}
	if _, exists := opts["server_name"]; exists {
		t.Fatalf("nested wrapper SNI must not be emitted: %#v", opts)
	}
}

func TestAddTLSSSWrapperUsesMihomoPluginSchema(t *testing.T) {
	tests := []struct {
		mode, plugin, server, client string
	}{
		{model.MihomoTlsModeShadowTLS, "shadow-tls", `{"shadow_tls":{"enable":true,"version":3,"users":[{"name":"u","password":"p"}],"handshake":{"dest":"cloud.tencent.com:443"}}}`, `{"shadow_tls_opts":{"version":3,"password":"p"},"utls":{"enabled":true,"fingerprint":"chrome"}}`},
		{model.MihomoTlsModeRestls, "restls", `{"res_tls":{"enable":true,"dest":"www.microsoft.com:443","password":"p","restls_script":"300?100"}}`, `{"restls_opts":{"password":"p","version_hint":"tls13"}}`},
		{model.MihomoTlsModeJLS, "jls", `{"jls_config":{"enable":true,"dest":"www.example.com:443","alpn":["h2"]}}`, `{"jls_opts":{"username":"u","password":"p"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.plugin, func(t *testing.T) {
			tls := &model.Tls{Mode: tt.mode, Server: json.RawMessage(tt.server), Client: json.RawMessage(tt.client)}
			out := map[string]interface{}{}
			addTls(&out, tls, "shadowsocks")
			if got := out["plugin"]; got != tt.plugin {
				t.Fatalf("plugin = %#v", got)
			}
			if _, exists := out["tls"]; exists {
				t.Fatalf("shadowsocks wrapper should not keep tls block: %#v", out)
			}
			opts, ok := out["plugin_opts"].(map[string]interface{})
			if !ok || opts["host"] == nil {
				t.Fatalf("unexpected plugin opts: %#v", out["plugin_opts"])
			}
		})
	}
}

func TestFillOutJsonMihomoShadowsocksWrapperReplacesStalePlugin(t *testing.T) {
	inbound := &model.Inbound{
		Type:    "shadowsocks",
		Tag:     "ss-wrapper",
		TlsId:   1,
		OutJson: json.RawMessage(`{"type":"shadowsocks","plugin":"restls","plugin_opts":{"password":"old"},"listen_port":443}`),
		Options: json.RawMessage(`{"listen_port":443,"method":"2022-blake3-aes-128-gcm","password":"ss-pass"}`),
		Tls: &model.Tls{
			Mode:   model.MihomoTlsModeShadowTLS,
			Server: json.RawMessage(`{"shadow_tls":{"enable":true,"version":3,"users":[{"name":"u","password":"shadow-pass"}],"handshake":{"dest":"edge.example.com:443"}}}`),
			Client: json.RawMessage(`{"shadow_tls_opts":{"version":3,"password":"shadow-pass"}}`),
		},
	}
	if err := FillOutJson(inbound, "panel.example.com"); err != nil {
		t.Fatalf("FillOutJson failed: %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(inbound.OutJson, &out); err != nil {
		t.Fatalf("decode out_json failed: %v", err)
	}
	if out["plugin"] != "shadow-tls" {
		t.Fatalf("plugin = %#v", out["plugin"])
	}
	if _, exists := out["tls"]; exists {
		t.Fatalf("Shadowsocks wrapper must not retain tls: %#v", out)
	}
	opts, ok := out["plugin_opts"].(map[string]interface{})
	if !ok || opts["host"] != "edge.example.com" || opts["password"] != "shadow-pass" {
		t.Fatalf("plugin_opts = %#v", out["plugin_opts"])
	}
}

func TestMihomoShadowsocksLinkIncludesWrapperPlugin(t *testing.T) {
	inbound := &model.Inbound{
		Type:    "shadowsocks",
		Tag:     "ss-link",
		Addrs:   json.RawMessage(`[]`),
		OutJson: json.RawMessage(`{}`),
		Options: json.RawMessage(`{"listen_port":443,"method":"2022-blake3-aes-128-gcm","password":"ss-pass"}`),
		TlsId:   1,
		Tls: &model.Tls{
			Mode:   model.MihomoTlsModeRestls,
			Server: json.RawMessage(`{"res_tls":{"enable":true,"dest":"edge.example.com:443","password":"restls-pass","restls_script":"300?100"}}`),
			Client: json.RawMessage(`{"restls_opts":{"password":"restls-pass","version_hint":"tls13"}}`),
		},
	}
	links := LinkGenerator(json.RawMessage(`{"shadowsocks":{}}`), inbound, "panel.example.com")
	if len(links) != 1 {
		t.Fatalf("links = %#v", links)
	}
	parsed, err := url.Parse(links[0])
	if err != nil {
		t.Fatalf("parse link failed: %v", err)
	}
	plugin := parsed.Query().Get("plugin")
	if !strings.Contains(plugin, "restls") || !strings.Contains(plugin, "host=edge.example.com") || !strings.Contains(plugin, "version-hint=tls13") {
		t.Fatalf("plugin query = %q", plugin)
	}
}
