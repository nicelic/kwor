package service

import (
	"encoding/json"
	"testing"

	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util"
)

func TestMihomoHysteria2RealmOpts_Normalize(t *testing.T) {
	input := map[string]interface{}{
		"enable":           true,
		"server_url":       "https://realm.hy2.io",
		"token":            "public",
		"realm_id":         "my-cabin-1f3a8c2e9b",
		"stun_servers":     []string{"stun.nextcloud.com:3478", "stun.sip.us:3478"},
		"sni":              "realm.hy2.io",
		"skip_cert_verify": true,
		"name_cert_verify": "example.com",
		"fingerprint":      "chrome",
		"certificate":      "/etc/ssl/cert.pem",
		"private_key":      "/etc/ssl/key.pem",
		"alpn":             []string{"h2", "http/1.1"},
		"proxy":            "DIRECT",
	}

	norm, ok := util.NormalizeMihomoHysteria2RealmOpts(input)
	if !ok {
		t.Fatalf("expected NormalizeMihomoHysteria2RealmOpts to succeed")
	}

	if norm["enable"] != true {
		t.Errorf("expected enable=true, got %v", norm["enable"])
	}
	if norm["server-url"] != "https://realm.hy2.io" {
		t.Errorf("expected server-url=https://realm.hy2.io, got %v", norm["server-url"])
	}
	if norm["token"] != "public" {
		t.Errorf("expected token=public, got %v", norm["token"])
	}
	if norm["realm-id"] != "my-cabin-1f3a8c2e9b" {
		t.Errorf("expected realm-id=my-cabin-1f3a8c2e9b, got %v", norm["realm-id"])
	}
	stuns, ok := norm["stun-servers"].([]string)
	if !ok || len(stuns) != 2 {
		t.Errorf("expected 2 stun servers, got %v", norm["stun-servers"])
	}
	if norm["skip-cert-verify"] != true {
		t.Errorf("expected skip-cert-verify=true, got %v", norm["skip-cert-verify"])
	}
	if norm["proxy"] != "DIRECT" {
		t.Errorf("expected proxy=DIRECT, got %v", norm["proxy"])
	}
	if _, exists := norm["alpn"]; exists {
		t.Errorf("expected alpn to be excluded from server listener realm-opts")
	}

	// Test NormalizeMihomoHysteria2RealmOptsSnake (client out_json storage)
	snake, ok := util.NormalizeMihomoHysteria2RealmOptsSnake(input)
	if !ok {
		t.Fatalf("expected NormalizeMihomoHysteria2RealmOptsSnake to succeed")
	}
	if snake["proxy"] != nil {
		t.Errorf("expected proxy to be excluded from client snake realm_opts")
	}
	alpnsSnake, ok := snake["alpn"].([]string)
	if !ok || len(alpnsSnake) != 2 {
		t.Errorf("expected 2 alpn entries in snake, got %v", snake["alpn"])
	}

	// Test BuildMihomoRealmOptsForClash
	obMap := map[string]interface{}{
		"realm_opts": input,
	}
	clashOpts := util.BuildMihomoRealmOptsForClash(obMap)
	if clashOpts == nil {
		t.Fatalf("expected BuildMihomoRealmOptsForClash to return non-nil")
	}
	if clashOpts["server-url"] != "https://realm.hy2.io" {
		t.Errorf("expected clashOpts server-url=https://realm.hy2.io, got %v", clashOpts["server-url"])
	}
	if clashOpts["proxy"] != nil {
		t.Errorf("expected proxy to be excluded from clash client realm-opts")
	}
	alpnsClash, ok := clashOpts["alpn"].([]string)
	if !ok || len(alpnsClash) != 2 {
		t.Errorf("expected 2 alpn entries in clashOpts, got %v", clashOpts["alpn"])
	}

	// Disabled case should return nil
	disabledMap := map[string]interface{}{
		"realm_opts": map[string]interface{}{
			"enable":     false,
			"server_url": "https://realm.hy2.io",
		},
	}
	if util.BuildMihomoRealmOptsForClash(disabledMap) != nil {
		t.Errorf("expected disabled realm-opts to return nil for Clash")
	}
}

func TestMihomoHysteria2Listener_NormalizeRealmOpts(t *testing.T) {
	listener := map[string]interface{}{
		"type": "hysteria2",
		"realm_opts": map[string]interface{}{
			"enable":     true,
			"server-url": "https://realm.hy2.io",
			"realm-id":   "room-42",
		},
	}

	normalizeMihomoHysteria2Listener(listener)

	if _, exists := listener["realm_opts"]; exists {
		t.Errorf("expected realm_opts to be deleted from listener")
	}
	realmOpts, ok := listener["realm-opts"].(map[string]interface{})
	if !ok || realmOpts == nil {
		t.Fatalf("expected listener to contain normalized realm-opts")
	}
	if realmOpts["enable"] != true || realmOpts["server-url"] != "https://realm.hy2.io" || realmOpts["realm-id"] != "room-42" {
		t.Errorf("unexpected listener realm-opts content: %v", realmOpts)
	}
}

func TestMihomoHysteria2OutJSONListenerCompat_MergeRealmOpts(t *testing.T) {
	payload := map[string]interface{}{}
	outboundJSON := map[string]interface{}{
		"realm_opts": map[string]interface{}{
			"enable":     true,
			"server_url": "https://realm.hy2.io",
			"token":      "secret",
		},
	}
	raw, _ := json.Marshal(outboundJSON)

	mergeMihomoHysteria2OutJSONListenerCompat(payload, raw)

	merged, ok := payload["realm-opts"].(map[string]interface{})
	if !ok || merged == nil {
		t.Fatalf("expected merged realm-opts in payload")
	}
	if merged["enable"] != true || merged["server-url"] != "https://realm.hy2.io" || merged["token"] != "secret" {
		t.Errorf("unexpected merged content: %v", merged)
	}

	// Existing listener realm-opts should take precedence
	existingPayload := map[string]interface{}{
		"realm-opts": map[string]interface{}{
			"enable":     true,
			"server-url": "https://custom.realm.org",
		},
	}
	mergeMihomoHysteria2OutJSONListenerCompat(existingPayload, raw)
	if existingPayload["realm-opts"].(map[string]interface{})["server-url"] != "https://custom.realm.org" {
		t.Errorf("expected existing realm-opts in payload to be preserved")
	}
}

func TestMihomoClientCommonFields_Hysteria2RealmOpts(t *testing.T) {
	// Hysteria2 preserves realm_opts
	hy2Out := map[string]interface{}{
		"realm_opts": map[string]interface{}{
			"enable":     true,
			"server-url": "https://realm.hy2.io",
			"token":      "abc",
		},
	}
	rawHy2, _ := json.Marshal(hy2Out)
	inboundHy2 := &model.MihomoInbound{
		Type:    "hysteria2",
		OutJson: rawHy2,
	}
	if err := sanitizeMihomoClientCommonFields(inboundHy2); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var resHy2 map[string]interface{}
	_ = json.Unmarshal(inboundHy2.OutJson, &resHy2)
	opts, ok := resHy2["realm_opts"].(map[string]interface{})
	if !ok || opts["server_url"] != "https://realm.hy2.io" {
		t.Errorf("expected realm_opts to be preserved in hysteria2 client out_json, got: %v", resHy2)
	}

	// Non-Hysteria2 strips realm_opts
	vmessOut := map[string]interface{}{
		"realm_opts": map[string]interface{}{
			"enable": true,
		},
	}
	rawVmess, _ := json.Marshal(vmessOut)
	inboundVmess := &model.MihomoInbound{
		Type:    "vmess",
		OutJson: rawVmess,
	}
	if err := sanitizeMihomoClientCommonFields(inboundVmess); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var resVmess map[string]interface{}
	_ = json.Unmarshal(inboundVmess.OutJson, &resVmess)
	if _, exists := resVmess["realm_opts"]; exists {
		t.Errorf("expected realm_opts to be stripped for non-hysteria2 inbound")
	}
}

func TestSingboxIsolation_RealmOptsStripped(t *testing.T) {
	outbound := map[string]interface{}{
		"type":       "hysteria2",
		"server":     "1.2.3.4",
		"realm_opts": map[string]interface{}{"enable": true},
		"realm-opts": map[string]interface{}{"enable": true},
	}

	util.SanitizeSingboxSubscriptionOutbound(outbound)
	if _, exists := outbound["realm_opts"]; exists {
		t.Errorf("expected realm_opts to be stripped for singbox subscription")
	}
	if _, exists := outbound["realm-opts"]; exists {
		t.Errorf("expected realm-opts to be stripped for singbox subscription")
	}

	runtimeOut := map[string]interface{}{
		"type":       "hysteria2",
		"server":     "1.2.3.4",
		"realm_opts": map[string]interface{}{"enable": true},
		"realm-opts": map[string]interface{}{"enable": true},
	}
	sanitizeSingboxRuntimeOutbound(runtimeOut)
	if _, exists := runtimeOut["realm_opts"]; exists {
		t.Errorf("expected realm_opts to be stripped for singbox runtime")
	}
	if _, exists := runtimeOut["realm-opts"]; exists {
		t.Errorf("expected realm-opts to be stripped for singbox runtime")
	}
}

func TestOutboundEditMergeSchema_Hysteria2RealmOptsAccepted(t *testing.T) {
	schema := protocolEditableMergeSchema("hysteria2")
	if schema == nil {
		t.Fatalf("expected non-nil schema for hysteria2")
	}

	// Verify realm_opts and realm-opts nodes exist in schema
	realmNode, ok := schema["realm_opts"]
	if !ok || realmNode == nil {
		t.Fatalf("expected realm_opts in hysteria2 merge schema")
	}
	if _, ok := realmNode.Children["server_url"]; !ok {
		t.Errorf("expected server_url in realm_opts schema children")
	}
	if _, ok := realmNode.Children["enable"]; !ok {
		t.Errorf("expected enable in realm_opts schema children")
	}

	realmKebabNode, ok := schema["realm-opts"]
	if !ok || realmKebabNode == nil {
		t.Fatalf("expected realm-opts in hysteria2 merge schema")
	}
	if _, ok := realmKebabNode.Children["server-url"]; !ok {
		t.Errorf("expected server-url in realm-opts schema children")
	}
}

func TestMihomoHysteria2OutJson_SyncFromInboundRealmOpts(t *testing.T) {
	// Case 1: Inbound has realm_opts enabled with proxy & alpn
	inboundData := map[string]interface{}{
		"type":        "hysteria2",
		"tag":         "hy2-in",
		"listen_port": 443,
		"realm_opts": map[string]interface{}{
			"enable":       true,
			"server_url":   "https://realm.hy2.io",
			"token":        "tok123",
			"realm_id":     "id456",
			"stun_servers": []string{"stun.hy2.io:3478"},
			"proxy":        "http://127.0.0.1:7890",
			"alpn":         []string{"h3", "h2"},
		},
		"out_json": map[string]interface{}{
			"type": "hysteria2",
		},
	}
	rawInbound, _ := json.Marshal(inboundData)
	var in model.Inbound
	if err := json.Unmarshal(rawInbound, &in); err != nil {
		t.Fatalf("unmarshal inbound failed: %v", err)
	}

	if err := util.FillOutJson(&in, "example.com"); err != nil {
		t.Fatalf("FillOutJson failed: %v", err)
	}

	var out map[string]interface{}
	if err := json.Unmarshal(in.OutJson, &out); err != nil {
		t.Fatalf("unmarshal out_json failed: %v", err)
	}

	realmOpts, ok := out["realm_opts"].(map[string]interface{})
	if !ok || realmOpts == nil {
		t.Fatalf("expected realm_opts to be synced to client out_json")
	}
	if realmOpts["enable"] != true {
		t.Errorf("expected enable=true, got %v", realmOpts["enable"])
	}
	if realmOpts["server_url"] != "https://realm.hy2.io" {
		t.Errorf("expected server_url=https://realm.hy2.io, got %v", realmOpts["server_url"])
	}
	// proxy must be stripped from client out_json
	if _, exists := realmOpts["proxy"]; exists {
		t.Errorf("expected proxy to be stripped from client out_json")
	}
	// alpn must be preserved in client out_json
	alpn, ok := realmOpts["alpn"].([]interface{})
	if !ok || len(alpn) != 2 {
		t.Errorf("expected 2 alpn items in client out_json, got %v", realmOpts["alpn"])
	}

	// Case 2: Inbound has NO realm_opts, but old out_json had stale realm_opts
	inboundNoRealm := map[string]interface{}{
		"type":        "hysteria2",
		"tag":         "hy2-no-realm",
		"listen_port": 443,
		"out_json": map[string]interface{}{
			"type": "hysteria2",
			"realm_opts": map[string]interface{}{
				"enable":     true,
				"server_url": "https://stale.realm.org",
			},
		},
	}
	rawNoRealm, _ := json.Marshal(inboundNoRealm)
	var inNoRealm model.Inbound
	if err := json.Unmarshal(rawNoRealm, &inNoRealm); err != nil {
		t.Fatalf("unmarshal inbound failed: %v", err)
	}

	if err := util.FillOutJson(&inNoRealm, "example.com"); err != nil {
		t.Fatalf("FillOutJson failed: %v", err)
	}

	var outNoRealm map[string]interface{}
	if err := json.Unmarshal(inNoRealm.OutJson, &outNoRealm); err != nil {
		t.Fatalf("unmarshal out_json failed: %v", err)
	}
	if _, exists := outNoRealm["realm_opts"]; exists {
		t.Errorf("expected stale realm_opts in client out_json to be removed when server inbound has no realm_opts")
	}
}
