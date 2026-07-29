package service

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestNormalizeSingboxUsersForList_MapsLegacyUsernameToName(t *testing.T) {
	usersJSON, err := normalizeSingboxUsersForList("vless", []string{`{"username":"alice","uuid":"u-1","flow":"xtls-rprx-vision"}`}, false)
	if err != nil {
		t.Fatalf("normalizeSingboxUsersForList returned error: %v", err)
	}
	if len(usersJSON) != 1 {
		t.Fatalf("expected 1 user, got %d", len(usersJSON))
	}

	var user map[string]interface{}
	if err := json.Unmarshal(usersJSON[0], &user); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if got, _ := user["name"].(string); got != "alice" {
		t.Fatalf("expected name alice, got %#v", user["name"])
	}
	if _, ok := user["username"]; ok {
		t.Fatalf("expected username to be removed, got %#v", user["username"])
	}
	if _, ok := user["flow"]; ok {
		t.Fatalf("expected flow to be removed without TLS, got %#v", user["flow"])
	}
}

func TestNormalizeSingboxUsersForList_MapsLegacyNameToUsername(t *testing.T) {
	usersJSON, err := normalizeSingboxUsersForList("naive", []string{`{"name":"alice","password":"secret"}`}, true)
	if err != nil {
		t.Fatalf("normalizeSingboxUsersForList returned error: %v", err)
	}
	if len(usersJSON) != 1 {
		t.Fatalf("expected 1 user, got %d", len(usersJSON))
	}

	var user map[string]interface{}
	if err := json.Unmarshal(usersJSON[0], &user); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if got, _ := user["username"].(string); got != "alice" {
		t.Fatalf("expected username alice, got %#v", user["username"])
	}
	if _, ok := user["name"]; ok {
		t.Fatalf("expected name to be removed, got %#v", user["name"])
	}
}

func TestNormalizeSingboxUsersForList_WhitelistsEveryRuntimeUserSchema(t *testing.T) {
	tests := []struct {
		name     string
		inbound  string
		hasTLS   bool
		rawUser  string
		wantUser map[string]interface{}
	}{
		{
			name:     "mixed",
			inbound:  "mixed",
			rawUser:  `{"name":"alice","password":"secret","metadata":{"panel":true},"unexpected":"drop"}`,
			wantUser: map[string]interface{}{"username": "alice", "password": "secret"},
		},
		{
			name:     "vmess",
			inbound:  "vmess",
			rawUser:  `{"username":"alice","uuid":"vmess-id","alter_id":7,"route_tag":"drop","metadata":{}}`,
			wantUser: map[string]interface{}{"name": "alice", "uuid": "vmess-id", "alterId": float64(7)},
		},
		{
			name:     "vless with tls flow",
			inbound:  "vless",
			hasTLS:   true,
			rawUser:  `{"username":"alice","uuid":"vless-id","flow":"xtls-rprx-vision","users":[],"unexpected":"drop"}`,
			wantUser: map[string]interface{}{"name": "alice", "uuid": "vless-id", "flow": "xtls-rprx-vision"},
		},
		{
			name:     "trojan",
			inbound:  "trojan",
			rawUser:  `{"username":"alice","password":"secret","user_management":{"selectable":true}}`,
			wantUser: map[string]interface{}{"name": "alice", "password": "secret"},
		},
		{
			name:     "naive",
			inbound:  "naive",
			rawUser:  `{"name":"alice","password":"secret","metadata":{"panel":true}}`,
			wantUser: map[string]interface{}{"username": "alice", "password": "secret"},
		},
		{
			name:     "hysteria",
			inbound:  "hysteria",
			rawUser:  `{"username":"alice","auth_str":"secret","server_port":443,"unexpected":"drop"}`,
			wantUser: map[string]interface{}{"name": "alice", "auth_str": "secret"},
		},
		{
			name:     "shadowtls",
			inbound:  "shadowtls",
			rawUser:  `{"username":"alice","password":"secret","metadata":{"panel":true}}`,
			wantUser: map[string]interface{}{"name": "alice", "password": "secret"},
		},
		{
			name:     "tuic",
			inbound:  "tuic",
			rawUser:  `{"username":"alice","uuid":"tuic-id","password":"secret","route_tag":"drop"}`,
			wantUser: map[string]interface{}{"name": "alice", "uuid": "tuic-id", "password": "secret"},
		},
		{
			name:     "hysteria2",
			inbound:  "hysteria2",
			rawUser:  `{"username":"alice","password":"secret","metadata":{"panel":true}}`,
			wantUser: map[string]interface{}{"name": "alice", "password": "secret"},
		},
		{
			name:     "anytls",
			inbound:  "anytls",
			rawUser:  `{"username":"alice","password":"secret","users":[],"unexpected":"drop"}`,
			wantUser: map[string]interface{}{"name": "alice", "password": "secret"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			usersJSON, err := normalizeSingboxUsersForList(test.inbound, []string{test.rawUser}, test.hasTLS)
			if err != nil {
				t.Fatalf("normalizeSingboxUsersForList returned error: %v", err)
			}
			if len(usersJSON) != 1 {
				t.Fatalf("expected one normalized user, got %d", len(usersJSON))
			}

			gotUser := map[string]interface{}{}
			if err := json.Unmarshal(usersJSON[0], &gotUser); err != nil {
				t.Fatalf("json.Unmarshal failed: %v", err)
			}
			if !reflect.DeepEqual(gotUser, test.wantUser) {
				t.Fatalf("normalized user = %#v, want %#v", gotUser, test.wantUser)
			}
		})
	}
}
