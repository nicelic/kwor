package service

import (
	"encoding/json"
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func testMihomoWrapperTLS(mode string) *model.MihomoTls {
	server := map[string]interface{}{}
	client := map[string]interface{}{}
	switch mode {
	case model.MihomoTlsModeShadowTLS:
		server["shadow_tls"] = map[string]interface{}{
			"enable":    true,
			"version":   3,
			"users":     []interface{}{map[string]interface{}{"name": "alice", "password": "shadow-pass"}},
			"handshake": map[string]interface{}{"dest": "edge.example.com:443"},
		}
		client["shadow_tls_opts"] = map[string]interface{}{"version": 3, "password": "shadow-pass"}
	case model.MihomoTlsModeRestls:
		server["res_tls"] = map[string]interface{}{
			"enable":        true,
			"dest":          "edge.example.com:443",
			"password":      "restls-pass",
			"restls_script": "300?100",
			"rate_limit":    204800,
		}
		client["restls_opts"] = map[string]interface{}{"password": "restls-pass", "version_hint": "tls13", "restls_script": "300?100"}
	case model.MihomoTlsModeJLS:
		server["jls_config"] = map[string]interface{}{
			"enable": true,
			"users":  []interface{}{map[string]interface{}{"username": "alice", "password": "jls-pass"}},
			"dest":   "edge.example.com:443",
			"sni":    "edge.example.com",
			"alpn":   []interface{}{"h2"},
		}
		client["jls_opts"] = map[string]interface{}{"username": "alice", "password": "jls-pass"}
	}
	serverJSON, _ := json.Marshal(server)
	clientJSON, _ := json.Marshal(client)
	return &model.MihomoTls{Mode: mode, Server: serverJSON, Client: clientJSON}
}

func TestBuildMihomoListenerProjectsTLSWrappersForSupportedProtocols(t *testing.T) {
	protocols := []string{"vmess", "vless", "trojan", "anytls", "shadowsocks"}
	modes := []string{model.MihomoTlsModeShadowTLS, model.MihomoTlsModeRestls, model.MihomoTlsModeJLS}
	wrapperKeys := map[string]string{
		model.MihomoTlsModeShadowTLS: "shadow-tls",
		model.MihomoTlsModeRestls:    "res-tls",
		model.MihomoTlsModeJLS:       "jls-config",
	}

	for _, protocol := range protocols {
		for _, mode := range modes {
			t.Run(protocol+"/"+mode, func(t *testing.T) {
				inbound := model.MihomoInbound{
					Type: "",
					Tag:  protocol + "-in",
					Tls:  testMihomoWrapperTLS(mode),
				}
				inbound.Type = protocol
				options := map[string]interface{}{"listen": "0.0.0.0", "listen_port": 443}
				if protocol == "shadowsocks" {
					options["method"] = "2022-blake3-aes-128-gcm"
					options["password"] = "ss-pass"
				}
				inbound.Options, _ = json.Marshal(options)
				raw, err := inbound.MarshalJSON()
				if err != nil {
					t.Fatalf("marshal inbound failed: %v", err)
				}
				payload, err := marshalJSONMap(raw)
				if err != nil {
					t.Fatalf("decode inbound failed: %v", err)
				}
				listener := buildMihomoListener(inbound, payload, mihomoInboundRouteRef{})
				if _, exists := listener["tls"]; exists {
					t.Fatalf("internal tls block leaked into listener: %#v", listener)
				}
				wrapper, ok := listener[wrapperKeys[mode]].(map[string]interface{})
				if !ok {
					t.Fatalf("wrapper %s missing from listener: %#v", wrapperKeys[mode], listener)
				}
				switch mode {
				case model.MihomoTlsModeShadowTLS:
					users, ok := wrapper["users"].([]interface{})
					if !ok || len(users) != 1 {
						t.Fatalf("ShadowTLS listener users = %#v", wrapper["users"])
					}
					user, ok := users[0].(map[string]interface{})
					if !ok || user["name"] != "alice" || user["password"] != "shadow-pass" {
						t.Fatalf("ShadowTLS listener credentials = %#v", users[0])
					}
				case model.MihomoTlsModeRestls:
					if wrapper["password"] != "restls-pass" {
						t.Fatalf("Restls listener password = %#v", wrapper["password"])
					}
					if wrapper["rate-limit"] != float64(204800) {
						t.Fatalf("Restls listener rate-limit = %#v", wrapper["rate-limit"])
					}
				case model.MihomoTlsModeJLS:
					users, ok := wrapper["users"].([]interface{})
					if !ok || len(users) != 1 {
						t.Fatalf("JLS listener users = %#v", wrapper["users"])
					}
					user, ok := users[0].(map[string]interface{})
					if !ok || user["username"] != "alice" || user["password"] != "jls-pass" {
						t.Fatalf("JLS listener credentials = %#v", users[0])
					}
				}
				if listener["type"] != protocol {
					t.Fatalf("listener type = %#v", listener["type"])
				}
			})
		}
	}
}

func TestBuildMihomoListenerPreservesOfficialWrapperFields(t *testing.T) {
	tests := []struct {
		name       string
		wrapperKey string
		wrapper    map[string]interface{}
		assert     func(*testing.T, map[string]interface{})
	}{
		{
			name:       "shadowtls wildcard and SNI handshakes",
			wrapperKey: "shadow-tls",
			wrapper: map[string]interface{}{
				"enable":       true,
				"version":      3,
				"users":        []interface{}{map[string]interface{}{"name": "alice", "password": "shadow-pass"}},
				"handshake":    map[string]interface{}{"dest": "edge.example.com:443", "proxy": "direct"},
				"wildcard_sni": "authed",
				"handshake_for_server_name": map[string]interface{}{
					"cdn.example.com": map[string]interface{}{"dest": "cdn.example.com:8443", "proxy": "proxy-a"},
				},
			},
			assert: func(t *testing.T, wrapper map[string]interface{}) {
				if wrapper["wildcard-sni"] != "authed" {
					t.Fatalf("wildcard-sni = %#v", wrapper["wildcard-sni"])
				}
				mappings, ok := wrapper["handshake-for-server-name"].(map[string]interface{})
				if !ok || mappings["cdn.example.com"] == nil {
					t.Fatalf("handshake-for-server-name = %#v", wrapper["handshake-for-server-name"])
				}
			},
		},
		{
			name:       "restls script and min record length",
			wrapperKey: "res-tls",
			wrapper: map[string]interface{}{
				"enable":         true,
				"dest":           "edge.example.com:443",
				"password":       "restls-pass",
				"restls_script":  "300?100",
				"min_record_len": 64,
				"rate_limit":     204800,
				"proxy":          "proxy-a",
			},
			assert: func(t *testing.T, wrapper map[string]interface{}) {
				if wrapper["restls-script"] != "300?100" || wrapper["min-record-len"] != 64 || wrapper["rate-limit"] != 204800 {
					t.Fatalf("unexpected Restls fields: %#v", wrapper)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := map[string]interface{}{
				"type":        "shadowsocks",
				"listen":      "0.0.0.0",
				"listen_port": 443,
				"method":      "2022-blake3-aes-128-gcm",
				"password":    "ss-pass",
				"tls": map[string]interface{}{
					"shadow_tls": tt.wrapper,
				},
			}
			if tt.wrapperKey == "res-tls" {
				payload["tls"] = map[string]interface{}{"res_tls": tt.wrapper}
			}
			listener := buildMihomoListener(model.MihomoInbound{Type: "shadowsocks", Tag: "ss-in"}, payload, mihomoInboundRouteRef{})
			wrapper, ok := listener[tt.wrapperKey].(map[string]interface{})
			if !ok {
				t.Fatalf("missing %s wrapper: %#v", tt.wrapperKey, listener)
			}
			tt.assert(t, wrapper)
		})
	}
}
