package service

import (
	"encoding/json"
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
