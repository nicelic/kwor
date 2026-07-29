package sub

import (
	"encoding/json"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"gopkg.in/yaml.v3"
)

func TestDefaultSubscriptionsExcludePanelAndUnknownClientFields(t *testing.T) {
	setupSubscriptionTestDB(t, "default-subscription-client-fields.db")

	db := database.GetDB()
	inbound := model.Inbound{
		Type:  "trojan",
		Tag:   "trojan-clean-443",
		Addrs: mustRawJSON(t, []interface{}{}),
		OutJson: mustRawJSON(t, map[string]interface{}{
			"type":             "trojan",
			"tag":              "trojan-clean-443",
			"server":           "panel.example.com",
			"server_port":      443,
			"id":               99,
			"route_tag":        "trojan-clean-443",
			"user_management":  map[string]interface{}{"selectable": true},
			"metadata":         map[string]interface{}{"source": "legacy-panel"},
			"users":            []interface{}{map[string]interface{}{"name": "legacy"}},
			"mihomo_common":    map[string]interface{}{"udp": true},
			"mihomo_hy2":       map[string]interface{}{"fast_open": true},
			"mihomo_fast_open": true,
			"fast_open":        true,
			"tls": map[string]interface{}{
				"enabled":                    true,
				"mihomo_use_fingerprint":     true,
				"fingerprint":                "AA:BB",
				"include_server_certificate": true,
				"include_server_fingerprint": true,
			},
		}),
		Options: mustRawJSON(t, map[string]interface{}{"listen_port": 443}),
	}
	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create inbound failed: %v", err)
	}

	client := model.Client{
		Enable: true,
		Name:   "field-cleanup-user",
		Config: mustRawJSON(t, map[string]interface{}{
			"trojan": map[string]interface{}{
				"username":        "alice",
				"password":        "client-secret",
				"route_tag":       "client-route",
				"user_management": map[string]interface{}{"selectable": true},
				"metadata":        map[string]interface{}{"source": "client"},
				"server_port":     1,
				"unexpected":      "must-not-leak",
			},
		}),
		Inbounds: mustRawJSON(t, []uint{inbound.Id}),
		Links:    mustRawJSON(t, []map[string]string{}),
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create client failed: %v", err)
	}

	rawOutbounds, _, err := (&JsonService{}).getOutbounds(client.Name, client.Config, []*model.Inbound{&inbound})
	if err != nil {
		t.Fatalf("build default raw outbounds failed: %v", err)
	}
	if rawOutbounds == nil || len(*rawOutbounds) != 1 {
		t.Fatalf("unexpected default raw outbounds: %#v", rawOutbounds)
	}
	assertDefaultSubscriptionCleanOutbound(t, (*rawOutbounds)[0])

	jsonSub, _, err := (&JsonService{}).GetJson(client.Name, "json")
	if err != nil {
		t.Fatalf("GetJson failed: %v", err)
	}
	jsonDoc := map[string]interface{}{}
	if err := json.Unmarshal([]byte(*jsonSub), &jsonDoc); err != nil {
		t.Fatalf("decode JSON subscription failed: %v", err)
	}
	jsonOutbound := findTaggedOutbound(t, jsonDoc["outbounds"], inbound.Tag)
	assertDefaultSubscriptionCleanOutbound(t, jsonOutbound)
	assertSingboxSubscriptionStripsMihomoFields(t, jsonOutbound)

	clashSub, _, err := (&ClashService{}).GetClash(client.Name)
	if err != nil {
		t.Fatalf("GetClash failed: %v", err)
	}
	clashDoc := map[string]interface{}{}
	if err := yaml.Unmarshal([]byte(*clashSub), &clashDoc); err != nil {
		t.Fatalf("decode Clash subscription failed: %v", err)
	}
	clashProxy := findNamedProxy(t, clashDoc["proxies"], inbound.Tag)
	assertDefaultSubscriptionCleanOutbound(t, clashProxy)
	for _, key := range []string{"mihomo_common", "mihomo_hy2", "mihomo_fast_open", "fast_open"} {
		if _, exists := clashProxy[key]; exists {
			t.Fatalf("Clash proxy leaked Mihomo-only field %q: %#v", key, clashProxy)
		}
	}
}

func assertDefaultSubscriptionCleanOutbound(t *testing.T, outbound map[string]interface{}) {
	t.Helper()
	for _, key := range []string{"id", "route_tag", "user_management", "metadata", "users", "unexpected", "username"} {
		if _, exists := outbound[key]; exists {
			t.Fatalf("subscription outbound leaked %q: %#v", key, outbound)
		}
	}
	if value, exists := outbound["server_port"]; exists {
		if !defaultSubscriptionPortMatches(value, 443) {
			t.Fatalf("client config must not override server_port, got %#v", value)
		}
	} else if value, exists := outbound["port"]; !exists || !defaultSubscriptionPortMatches(value, 443) {
		t.Fatalf("client config must not override Clash port, got %#v", value)
	}
	if got, _ := outbound["password"].(string); got != "client-secret" {
		t.Fatalf("expected allowed password to remain, got %#v", outbound["password"])
	}
}

func defaultSubscriptionPortMatches(value interface{}, expected int) bool {
	switch port := value.(type) {
	case int:
		return port == expected
	case int64:
		return int(port) == expected
	case float64:
		return int(port) == expected
	default:
		return false
	}
}

func assertSingboxSubscriptionStripsMihomoFields(t *testing.T, outbound map[string]interface{}) {
	t.Helper()
	for _, key := range []string{"mihomo_common", "mihomo_hy2", "mihomo_fast_open", "fast_open"} {
		if _, exists := outbound[key]; exists {
			t.Fatalf("sing-box JSON outbound leaked Mihomo-only field %q: %#v", key, outbound)
		}
	}
	tlsMap, _ := outbound["tls"].(map[string]interface{})
	for _, key := range []string{"mihomo_use_fingerprint", "fingerprint", "include_server_certificate", "include_server_fingerprint"} {
		if _, exists := tlsMap[key]; exists {
			t.Fatalf("sing-box JSON TLS leaked Mihomo-only field %q: %#v", key, tlsMap)
		}
	}
}

func TestDefaultMieruStaysOutOfJSONButRendersForClash(t *testing.T) {
	setupSubscriptionTestDB(t, "default-mieru-subscription-support.db")

	db := database.GetDB()
	inbound := model.Inbound{
		Type:  "mieru",
		Tag:   "mieru-clash-node",
		Addrs: mustRawJSON(t, []interface{}{}),
		OutJson: mustRawJSON(t, map[string]interface{}{
			"type":        "mieru",
			"tag":         "mieru-clash-node",
			"server":      "panel.example.com",
			"server_port": 16939,
		}),
		Options: mustRawJSON(t, map[string]interface{}{}),
	}
	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create mieru inbound failed: %v", err)
	}

	client := model.Client{
		Enable: true,
		Name:   "mieru-clash-client",
		Config: mustRawJSON(t, map[string]interface{}{
			"mieru": map[string]interface{}{
				"name":       "legacy-alice",
				"password":   "client-secret",
				"metadata":   map[string]interface{}{"panel": true},
				"unexpected": "drop",
			},
		}),
		Inbounds: mustRawJSON(t, []uint{inbound.Id}),
		Links:    mustRawJSON(t, []map[string]string{}),
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create mieru client failed: %v", err)
	}

	jsonSub, _, err := (&JsonService{}).GetJson(client.Name, "json")
	if err != nil {
		t.Fatalf("GetJson failed: %v", err)
	}
	jsonDoc := map[string]interface{}{}
	if err := json.Unmarshal([]byte(*jsonSub), &jsonDoc); err != nil {
		t.Fatalf("decode JSON subscription failed: %v", err)
	}
	for _, item := range jsonDoc["outbounds"].([]interface{}) {
		outbound, _ := item.(map[string]interface{})
		if outbound["tag"] == inbound.Tag || outbound["type"] == "mieru" {
			t.Fatalf("Mieru must not be emitted in sing-box JSON: %#v", outbound)
		}
	}

	clashSub, _, err := (&ClashService{}).GetClash(client.Name)
	if err != nil {
		t.Fatalf("GetClash failed: %v", err)
	}
	clashDoc := map[string]interface{}{}
	if err := yaml.Unmarshal([]byte(*clashSub), &clashDoc); err != nil {
		t.Fatalf("decode Clash subscription failed: %v", err)
	}
	proxy := findNamedProxy(t, clashDoc["proxies"], inbound.Tag)
	if got, _ := proxy["username"].(string); got != "legacy-alice" {
		t.Fatalf("expected Mieru username in Clash, got %#v", proxy["username"])
	}
	if got, _ := proxy["password"].(string); got != "client-secret" {
		t.Fatalf("expected Mieru password in Clash, got %#v", proxy["password"])
	}
	for _, key := range []string{"metadata", "unexpected"} {
		if _, exists := proxy[key]; exists {
			t.Fatalf("Clash Mieru proxy leaked %q: %#v", key, proxy)
		}
	}
}
