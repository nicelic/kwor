package sub

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util"
	"gopkg.in/yaml.v3"
)

func TestDefaultClientSubscriptionsPreserveInboundSelectionOrder(t *testing.T) {
	setupSubscriptionTestDB(t, "default-client-order.db")

	db := database.GetDB()
	first := createOrderedDefaultInbound(t, "default-first", "default-first.example.com", 41001)
	second := createOrderedDefaultInbound(t, "default-second", "default-second.example.com", 41002)
	if err := db.Create(first).Error; err != nil {
		t.Fatalf("create first inbound failed: %v", err)
	}
	if err := db.Create(second).Error; err != nil {
		t.Fatalf("create second inbound failed: %v", err)
	}

	client := model.Client{
		Enable: true,
		Name:   "default-order-user",
		Config: mustRawJSON(t, map[string]interface{}{
			"trojan": map[string]interface{}{
				"password": "default-order-pass",
			},
		}),
		Inbounds: mustRawJSON(t, []uint{second.Id, first.Id}),
		Links:    mustRawJSON(t, []map[string]string{}),
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create client failed: %v", err)
	}

	jsonSub, _, err := (&JsonService{}).GetJson(client.Name, "json")
	if err != nil {
		t.Fatalf("GetJson failed: %v", err)
	}
	var jsonDoc map[string]interface{}
	if err := json.Unmarshal([]byte(*jsonSub), &jsonDoc); err != nil {
		t.Fatalf("unmarshal default JSON subscription failed: %v", err)
	}
	gotJSON := collectOrderedJSONOutboundTags(asMapSlice(t, jsonDoc["outbounds"]), first.Tag, second.Tag)
	want := []string{second.Tag, first.Tag}
	if strings.Join(gotJSON, ",") != strings.Join(want, ",") {
		t.Fatalf("expected default JSON outbound order %v, got %v", want, gotJSON)
	}

	clashSub, _, err := (&ClashService{}).GetClash(client.Name)
	if err != nil {
		t.Fatalf("GetClash failed: %v", err)
	}
	var clashDoc map[string]interface{}
	if err := yaml.Unmarshal([]byte(*clashSub), &clashDoc); err != nil {
		t.Fatalf("unmarshal default Clash subscription failed: %v", err)
	}
	gotClash := collectOrderedClashProxyNames(asInterfaceSlice(t, clashDoc["proxies"]), first.Tag, second.Tag)
	if strings.Join(gotClash, ",") != strings.Join(want, ",") {
		t.Fatalf("expected default Clash proxy order %v, got %v", want, gotClash)
	}
}

func TestMihomoClientSubscriptionsPreserveInboundSelectionOrder(t *testing.T) {
	setupSubscriptionTestDB(t, "mihomo-client-order.db")

	db := database.GetDB()
	first := createOrderedMihomoInbound(t, "mihomo-first", "mihomo-first.example.com", 42001)
	second := createOrderedMihomoInbound(t, "mihomo-second", "mihomo-second.example.com", 42002)
	if err := db.Create(first).Error; err != nil {
		t.Fatalf("create first mihomo inbound failed: %v", err)
	}
	if err := db.Create(second).Error; err != nil {
		t.Fatalf("create second mihomo inbound failed: %v", err)
	}

	client := model.MihomoClient{
		Enable: true,
		Name:   "mihomo-order-user",
		Config: mustRawJSON(t, map[string]interface{}{
			"trojan": map[string]interface{}{
				"password": "mihomo-order-pass",
			},
		}),
		Inbounds: mustRawJSON(t, []uint{second.Id, first.Id}),
		Links:    mustRawJSON(t, []map[string]string{}),
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create mihomo client failed: %v", err)
	}

	jsonSub, _, err := (&JsonService{}).GetMihomoJson(client.Name, "json")
	if err != nil {
		t.Fatalf("GetMihomoJson failed: %v", err)
	}
	var jsonDoc map[string]interface{}
	if err := json.Unmarshal([]byte(*jsonSub), &jsonDoc); err != nil {
		t.Fatalf("unmarshal mihomo JSON subscription failed: %v", err)
	}
	gotJSON := collectOrderedJSONOutboundTags(asMapSlice(t, jsonDoc["outbounds"]), first.Tag, second.Tag)
	want := []string{second.Tag, first.Tag}
	if strings.Join(gotJSON, ",") != strings.Join(want, ",") {
		t.Fatalf("expected mihomo JSON outbound order %v, got %v", want, gotJSON)
	}

	clashSub, _, err := (&ClashService{}).GetMihomoClash(client.Name)
	if err != nil {
		t.Fatalf("GetMihomoClash failed: %v", err)
	}
	var clashDoc map[string]interface{}
	if err := yaml.Unmarshal([]byte(*clashSub), &clashDoc); err != nil {
		t.Fatalf("unmarshal mihomo Clash subscription failed: %v", err)
	}
	gotClash := collectOrderedClashProxyNames(asInterfaceSlice(t, clashDoc["proxies"]), first.Tag, second.Tag)
	if strings.Join(gotClash, ",") != strings.Join(want, ",") {
		t.Fatalf("expected mihomo Clash proxy order %v, got %v", want, gotClash)
	}
}

func createOrderedDefaultInbound(t *testing.T, tag string, host string, port int) *model.Inbound {
	t.Helper()

	inbound := &model.Inbound{
		Type:    "trojan",
		Tag:     tag,
		Addrs:   mustRawJSON(t, []interface{}{}),
		OutJson: mustRawJSON(t, map[string]interface{}{}),
		Options: mustRawJSON(t, map[string]interface{}{
			"listen_port": port,
		}),
	}
	if err := util.FillOutJson(inbound, host); err != nil {
		t.Fatalf("FillOutJson failed for %s: %v", tag, err)
	}
	return inbound
}

func createOrderedMihomoInbound(t *testing.T, tag string, host string, port int) *model.MihomoInbound {
	t.Helper()

	inbound := &model.MihomoInbound{
		Type:    "trojan",
		Tag:     tag,
		Addrs:   mustRawJSON(t, []interface{}{}),
		OutJson: mustRawJSON(t, map[string]interface{}{}),
		Options: mustRawJSON(t, map[string]interface{}{
			"listen_port": port,
		}),
	}

	baseInbound := inbound.ToBase()
	if err := util.FillOutJson(&baseInbound, host); err != nil {
		t.Fatalf("FillOutJson failed for %s: %v", tag, err)
	}
	inbound.OutJson = baseInbound.OutJson
	return inbound
}

func asMapSlice(t *testing.T, raw interface{}) []map[string]interface{} {
	t.Helper()

	items, ok := raw.([]interface{})
	if !ok {
		t.Fatalf("expected []interface{}, got %#v", raw)
	}
	result := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		value, ok := item.(map[string]interface{})
		if !ok {
			t.Fatalf("expected map item, got %#v", item)
		}
		result = append(result, value)
	}
	return result
}

func asInterfaceSlice(t *testing.T, raw interface{}) []interface{} {
	t.Helper()

	items, ok := raw.([]interface{})
	if !ok {
		t.Fatalf("expected []interface{}, got %#v", raw)
	}
	return items
}
