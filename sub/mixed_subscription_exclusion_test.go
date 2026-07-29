package sub

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"gopkg.in/yaml.v3"
)

func TestMixedListenersAreExcludedFromDefaultAndMihomoSubscriptions(t *testing.T) {
	setupSubscriptionTestDB(t, "mixed-subscription-exclusion.db")
	db := database.GetDB()

	defaultInbound := &model.Inbound{
		Type:    "mixed",
		Tag:     "default-mixed",
		Addrs:   mustRawJSON(t, []interface{}{}),
		Options: mustRawJSON(t, map[string]interface{}{"listen_port": 1080}),
		OutJson: mustRawJSON(t, map[string]interface{}{
			"type":        "mixed",
			"tag":         "default-mixed",
			"server":      "panel.example.com",
			"server_port": 1080,
		}),
	}
	if err := db.Create(defaultInbound).Error; err != nil {
		t.Fatalf("create default mixed inbound failed: %v", err)
	}
	defaultClient := &model.Client{
		Enable:   true,
		Name:     "default-mixed-client",
		Config:   mustRawJSON(t, map[string]interface{}{"mixed": map[string]interface{}{"username": "alice", "password": "secret"}}),
		Inbounds: mustRawJSON(t, []uint{defaultInbound.Id}),
		Links: mustRawJSON(t, []Link{
			{Type: "local", Remark: defaultInbound.Tag, Uri: "socks://legacy-mixed-link"},
		}),
	}
	if err := db.Create(defaultClient).Error; err != nil {
		t.Fatalf("create default mixed client failed: %v", err)
	}

	defaultJSON, defaultJSONHeaders, err := (&JsonService{}).GetJson(defaultClient.Name, "json")
	assertSubscriptionFiltersMixed(t, defaultInbound.Tag, false, defaultJSON, defaultJSONHeaders, err)
	defaultClash, defaultClashHeaders, err := (&ClashService{}).GetClash(defaultClient.Name)
	assertSubscriptionFiltersMixed(t, defaultInbound.Tag, true, defaultClash, defaultClashHeaders, err)
	assertMixedLocalLinksAreExcluded(t, &SubService{}, defaultClient, false)

	mihomoInbound := &model.MihomoInbound{
		Type:    "mixed",
		Tag:     "mihomo-mixed",
		Addrs:   mustRawJSON(t, []interface{}{}),
		Options: mustRawJSON(t, map[string]interface{}{"listen_port": 1081}),
		OutJson: mustRawJSON(t, map[string]interface{}{
			"type":        "mixed",
			"tag":         "mihomo-mixed",
			"server":      "panel.example.com",
			"server_port": 1081,
		}),
	}
	if err := db.Create(mihomoInbound).Error; err != nil {
		t.Fatalf("create Mihomo mixed inbound failed: %v", err)
	}
	mihomoClient := &model.MihomoClient{
		Enable:   true,
		Name:     "mihomo-mixed-client",
		Config:   mustRawJSON(t, map[string]interface{}{"mixed": map[string]interface{}{"username": "alice", "password": "secret"}}),
		Inbounds: mustRawJSON(t, []uint{mihomoInbound.Id}),
		Links: mustRawJSON(t, []Link{
			{Type: "local", Remark: mihomoInbound.Tag, Uri: "http://legacy-mixed-link"},
		}),
	}
	if err := db.Create(mihomoClient).Error; err != nil {
		t.Fatalf("create Mihomo mixed client failed: %v", err)
	}

	mihomoJSON, mihomoJSONHeaders, err := (&JsonService{}).GetMihomoJson(mihomoClient.Name, "json")
	assertSubscriptionFiltersMixed(t, mihomoInbound.Tag, false, mihomoJSON, mihomoJSONHeaders, err)
	mihomoClash, mihomoClashHeaders, err := (&ClashService{}).GetMihomoClash(mihomoClient.Name)
	assertSubscriptionFiltersMixed(t, mihomoInbound.Tag, true, mihomoClash, mihomoClashHeaders, err)
	assertMixedLocalLinksAreExcluded(t, &SubService{}, mihomoClientToBase(mihomoClient), true)
}

func assertSubscriptionFiltersMixed(t *testing.T, mixedTag string, clash bool, subscription *string, headers []string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("render subscription failed: %v", err)
	}
	if subscription == nil || len(headers) == 0 {
		t.Fatalf("unexpected empty subscription result: payload=%v headers=%#v", subscription, headers)
	}

	if clash {
		doc := map[string]interface{}{}
		if err := yaml.Unmarshal([]byte(*subscription), &doc); err != nil {
			t.Fatalf("decode Clash subscription failed: %v", err)
		}
		proxies, _ := doc["proxies"].([]interface{})
		for _, raw := range proxies {
			proxy, _ := raw.(map[string]interface{})
			assertNotMixedSubscriptionNode(t, proxy, "name", mixedTag)
		}
		return
	}

	doc := map[string]interface{}{}
	if err := json.Unmarshal([]byte(*subscription), &doc); err != nil {
		t.Fatalf("decode sing-box subscription failed: %v", err)
	}
	outbounds, _ := doc["outbounds"].([]interface{})
	for _, raw := range outbounds {
		outbound, _ := raw.(map[string]interface{})
		assertNotMixedSubscriptionNode(t, outbound, "tag", mixedTag)
	}
}

func assertNotMixedSubscriptionNode(t *testing.T, node map[string]interface{}, nameField string, mixedTag string) {
	t.Helper()
	name, _ := node[nameField].(string)
	if node["type"] == "mixed" || name == mixedTag || name == mixedTag+"-socks" || name == mixedTag+"-http" {
		t.Fatalf("mixed listener leaked into subscription output: %#v", node)
	}
}

func assertMixedLocalLinksAreExcluded(t *testing.T, service *SubService, client *model.Client, mihomo bool) {
	t.Helper()
	filtered, err := service.filterServerOnlyLocalLinks(client, mihomo)
	if err != nil {
		t.Fatalf("filter stale mixed links failed: %v", err)
	}
	links := []Link{}
	if err := json.Unmarshal(filtered, &links); err != nil {
		t.Fatalf("decode filtered links failed: %v", err)
	}
	if len(links) != 0 {
		t.Fatalf("legacy local Mixed links leaked into plain subscription: %#v", links)
	}

	current, err := service.buildCurrentLocalLinks(client, mihomo, "")
	if err != nil {
		t.Fatalf("build current local links failed: %v", err)
	}
	if len(current) != 0 {
		t.Fatalf("Mixed listener generated current local links: %#v", current)
	}

	var subscription *string
	if mihomo {
		subscription, _, err = service.GetMihomoSubs(client.Name)
	} else {
		subscription, _, err = service.GetSubs(client.Name)
	}
	if err != nil || subscription == nil {
		t.Fatalf("render plain subscription failed: payload=%v err=%v", subscription, err)
	}
	decoded, err := base64.StdEncoding.DecodeString(*subscription)
	if err != nil {
		t.Fatalf("decode plain subscription failed: %v", err)
	}
	if strings.Contains(string(decoded), "legacy-mixed-link") {
		t.Fatalf("plain subscription leaked legacy Mixed link: %q", decoded)
	}
}
