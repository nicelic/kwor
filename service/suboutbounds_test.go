package service

import (
	"encoding/json"
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestExpandSubOutboundsForSubscription_PassThroughTypes(t *testing.T) {
	passThroughTypes := []string{
		"direct",
		"socks",
		"http",
		"shadowsocks",
		"vmess",
		"trojan",
		"hysteria",
		"vless",
		"tuic",
		"hysteria2",
		"anytls",
		"tor",
		"ssh",
		"selector",
		"urltest",
	}

	for _, typ := range passThroughTypes {
		raw := []map[string]interface{}{
			{
				"type": typ,
				"tag":  "node-" + typ,
			},
		}
		outbounds, tags := expandSubOutboundsForSubscription(raw)
		if len(outbounds) != 1 {
			t.Fatalf("type=%s expected 1 outbound, got %d", typ, len(outbounds))
		}
		if len(tags) != 1 || tags[0] != "node-"+typ {
			t.Fatalf("type=%s expected out tag node-%s, got %#v", typ, typ, tags)
		}
		if gotType, _ := outbounds[0]["type"].(string); gotType != typ {
			t.Fatalf("type=%s expected outbound type %s, got %s", typ, typ, gotType)
		}
	}
}

func TestExpandSubOutboundsForSubscription_ExpandsManualMixed(t *testing.T) {
	raw := []map[string]interface{}{
		{
			"type":        "mixed",
			"tag":         "mixed-node",
			"server":      "proxy.example.com",
			"server_port": 1080,
			"username":    "alice",
			"password":    "secret",
		},
	}

	outbounds, tags := expandSubOutboundsForSubscription(raw)
	if len(outbounds) != 2 || len(tags) != 2 {
		t.Fatalf("mixed endpoint must expand into two subscription nodes, got outbounds=%#v tags=%#v", outbounds, tags)
	}
	if tags[0] != "mixed-node-socks" || tags[1] != "mixed-node-http" {
		t.Fatalf("unexpected expanded mixed tags: %#v", tags)
	}
	if outbounds[0]["type"] != "socks" || outbounds[0]["tag"] != "mixed-node-socks" {
		t.Fatalf("unexpected SOCKS mixed expansion: %#v", outbounds[0])
	}
	if outbounds[1]["type"] != "http" || outbounds[1]["tag"] != "mixed-node-http" {
		t.Fatalf("unexpected HTTP mixed expansion: %#v", outbounds[1])
	}
	for _, outbound := range outbounds {
		if outbound["server"] != "proxy.example.com" || outbound["server_port"] != 1080 || outbound["username"] != "alice" || outbound["password"] != "secret" {
			t.Fatalf("mixed expansion lost endpoint credentials: %#v", outbound)
		}
	}
}

func TestSubOutboundServiceScopesMixedExclusionToInboundSync(t *testing.T) {
	db := initSubOutboundOrderTestDB(t, "suboutbounds-mixed-hidden.db")

	for _, record := range []*model.SubOutbound{
		{
			Type:       "mixed",
			Tag:        "legacy-mixed-type",
			SourceType: subOutboundSourceClient,
			RawOutbound: mustSubOutboundOrderJSON(t, map[string]interface{}{
				"type": "mixed",
				"tag":  "legacy-mixed-type",
			}),
		},
		{
			Type:       "socks",
			Tag:        "legacy-mixed-raw",
			SourceType: subOutboundSourceMihomoClient,
			RawOutbound: mustSubOutboundOrderJSON(t, map[string]interface{}{
				"type": "mixed",
				"tag":  "legacy-mixed-raw",
			}),
		},
		{
			Type: "mixed",
			Tag:  "manual-mixed",
			RawOutbound: mustSubOutboundOrderJSON(t, map[string]interface{}{
				"type":        "mixed",
				"tag":         "manual-mixed",
				"server":      "proxy.example.com",
				"server_port": 1080,
				"username":    "alice",
				"password":    "secret",
			}),
		},
		{
			Type: "socks",
			Tag:  "manual-mixed-raw",
			RawOutbound: mustSubOutboundOrderJSON(t, map[string]interface{}{
				"type":        "mixed",
				"tag":         "manual-mixed-raw",
				"server":      "proxy.example.com",
				"server_port": 1081,
			}),
		},
	} {
		if err := db.Create(record).Error; err != nil {
			t.Fatalf("create legacy mixed suboutbound failed: %v", err)
		}
	}
	group := &model.SubGroup{
		Name:      "legacy-mixed-group",
		Outbounds: `["legacy-mixed-type","legacy-mixed-raw","manual-mixed","manual-mixed-raw"]`,
	}
	if err := db.Create(group).Error; err != nil {
		t.Fatalf("create legacy mixed group failed: %v", err)
	}
	groups, err := (&SubGroupService{}).GetAll()
	if err != nil {
		t.Fatalf("GetAll groups failed: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("unexpected groups: %#v", groups)
	}
	if tags := parseSubGroupOutboundTags(groups[0].Outbounds); len(tags) != 2 || tags[0] != "manual-mixed" || tags[1] != "manual-mixed-raw" {
		t.Fatalf("subscription manager group must retain only manual mixed, got %#v", tags)
	}

	items, err := (&SubOutboundService{}).GetAll()
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}
	if len(*items) != 2 {
		t.Fatalf("subscription manager must hide only inbound-synced mixed entries, got %#v", items)
	}
	itemTypes := map[string]string{}
	for _, item := range *items {
		tag, _ := item["tag"].(string)
		itemTypes[tag], _ = item["type"].(string)
	}
	if itemTypes["manual-mixed"] != "mixed" || itemTypes["manual-mixed-raw"] != "mixed" {
		t.Fatalf("manual mixed entries must remain visible from their raw payloads, got %#v", itemTypes)
	}

	configs, err := (&SubOutboundService{}).GetAllConfig(db)
	if err != nil {
		t.Fatalf("GetAllConfig failed: %v", err)
	}
	if len(configs) != 4 {
		t.Fatalf("manual mixed must expand in subscription config output, got %#v", configs)
	}
	configTags := make([]string, 0, len(configs))
	for _, config := range configs {
		payload := map[string]interface{}{}
		if err := json.Unmarshal(config, &payload); err != nil {
			t.Fatalf("decode expanded config failed: %v", err)
		}
		configTags = append(configTags, payload["tag"].(string))
		if payload["type"] == "mixed" {
			t.Fatalf("manual mixed config was not expanded: %#v", payload)
		}
	}
	configTagSet := map[string]bool{}
	for _, tag := range configTags {
		configTagSet[tag] = true
	}
	for _, tag := range []string{"manual-mixed-socks", "manual-mixed-http", "manual-mixed-raw-socks", "manual-mixed-raw-http"} {
		if !configTagSet[tag] {
			t.Fatalf("missing expanded config tag %q in %#v", tag, configTags)
		}
	}
	if len(configTagSet) != 4 {
		t.Fatalf("unexpected expanded config tags: %#v", configTags)
	}

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction failed: %v", tx.Error)
	}
	err = (&SubOutboundService{}).Save(tx, "new", mustSubOutboundOrderJSON(t, map[string]interface{}{
		"type": "mixed",
		"tag":  "new-mixed",
	}))
	tx.Rollback()
	if err != nil {
		t.Fatalf("subscription manager must allow a manually configured mixed endpoint: %v", err)
	}
}

func TestExpandSubOutboundsForSubscription_ShadowTLSSplit(t *testing.T) {
	raw := []map[string]interface{}{
		{
			"type":         "shadowtls",
			"tag":          "stls",
			"server":       "1.2.3.4",
			"wildcard_sni": "all",
			"strict_mode":  true,
			"handshake": map[string]interface{}{
				"server":      "addons.mozilla.org",
				"server_port": 443,
			},
			"ss_config": map[string]interface{}{
				"method":       "2022-blake3-aes-128-gcm",
				"network":      "tcp",
				"password":     "pass",
				"udp_over_tcp": true,
				"multiplex": map[string]interface{}{
					"enabled": true,
				},
			},
		},
	}

	outbounds, tags := expandSubOutboundsForSubscription(raw)
	if len(outbounds) != 2 {
		t.Fatalf("shadowtls expected 2 outbounds, got %d", len(outbounds))
	}
	if len(tags) != 1 || tags[0] != "stls" {
		t.Fatalf("shadowtls expected tags [stls], got %#v", tags)
	}

	if outbounds[0]["type"] != "shadowsocks" || outbounds[0]["tag"] != "stls" || outbounds[0]["detour"] != "stls-out" {
		t.Fatalf("unexpected shadowsocks outbound: %#v", outbounds[0])
	}
	if outbounds[0]["network"] != "tcp" {
		t.Fatalf("expected shadowsocks network=tcp, got %#v", outbounds[0]["network"])
	}
	if outbounds[1]["type"] != "shadowtls" || outbounds[1]["tag"] != "stls-out" {
		t.Fatalf("unexpected shadowtls outbound: %#v", outbounds[1])
	}
	if _, ok := outbounds[1]["wildcard_sni"]; ok {
		t.Fatalf("shadowtls outbound should not contain wildcard_sni: %#v", outbounds[1])
	}
	if _, ok := outbounds[1]["strict_mode"]; ok {
		t.Fatalf("shadowtls outbound should not contain strict_mode: %#v", outbounds[1])
	}
	if _, ok := outbounds[1]["handshake"]; ok {
		t.Fatalf("shadowtls outbound should not contain handshake: %#v", outbounds[1])
	}
	if _, ok := outbounds[1]["ss_config"]; ok {
		t.Fatalf("shadowtls outbound should not contain ss_config: %#v", outbounds[1])
	}
}

func TestExpandSubOutboundsForSubscription_ShadowTLSNoSsConfigSanitizesInboundOnlyFields(t *testing.T) {
	raw := []map[string]interface{}{
		{
			"type":         "shadowtls",
			"tag":          "stls-no-ss",
			"wildcard_sni": "all",
			"strict_mode":  true,
			"handshake": map[string]interface{}{
				"server":      "addons.mozilla.org",
				"server_port": 443,
			},
		},
	}

	outbounds, tags := expandSubOutboundsForSubscription(raw)
	if len(outbounds) != 1 {
		t.Fatalf("shadowtls without ss_config expected 1 outbound, got %d", len(outbounds))
	}
	if len(tags) != 1 || tags[0] != "stls-no-ss" {
		t.Fatalf("shadowtls without ss_config expected tags [stls-no-ss], got %#v", tags)
	}
	if outbounds[0]["type"] != "shadowtls" || outbounds[0]["tag"] != "stls-no-ss" {
		t.Fatalf("unexpected outbound: %#v", outbounds[0])
	}
	if _, ok := outbounds[0]["wildcard_sni"]; ok {
		t.Fatalf("shadowtls outbound should not contain wildcard_sni: %#v", outbounds[0])
	}
	if _, ok := outbounds[0]["strict_mode"]; ok {
		t.Fatalf("shadowtls outbound should not contain strict_mode: %#v", outbounds[0])
	}
	if _, ok := outbounds[0]["handshake"]; ok {
		t.Fatalf("shadowtls outbound should not contain handshake: %#v", outbounds[0])
	}
}
