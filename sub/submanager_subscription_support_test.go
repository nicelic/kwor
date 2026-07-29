package sub

import (
	"encoding/json"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"gopkg.in/yaml.v3"
)

func TestSubManagerScopesMixedExclusionToInboundSyncAndRespectsMieruOutputSupport(t *testing.T) {
	setupSubscriptionTestDB(t, "submanager-output-support.db")

	mixedTag := "mixed-node"
	createSubOutboundFromMap(
		t,
		map[string]interface{}{
			"type":        "mixed",
			"tag":         mixedTag,
			"server":      "panel.example.com",
			"server_port": 1080,
			"username":    "mixed-user",
			"password":    "mixed-secret",
			"metadata":    map[string]interface{}{"panel": true},
		},
		subManagerSourceClient,
		1001,
		0,
		nil,
	)

	renderer := &SubManagerSubService{}
	if _, err := renderer.GetSubManagerJson(mixedTag); err == nil {
		t.Fatal("mixed listener must not render as a SubManager JSON subscription")
	}
	if _, err := renderer.GetSubManagerClash(mixedTag); err == nil {
		t.Fatal("mixed listener must not render as a SubManager Clash subscription")
	}

	manualMixedTag := "manual-mixed-node"
	createSubOutboundFromMap(
		t,
		map[string]interface{}{
			"type":        "mixed",
			"tag":         manualMixedTag,
			"server":      "proxy.example.com",
			"server_port": 1080,
			"username":    "mixed-user",
			"password":    "mixed-secret",
		},
		"",
		0,
		0,
		nil,
	)

	manualMixedJSON, err := renderer.GetSubManagerJson(manualMixedTag)
	if err != nil {
		t.Fatalf("GetSubManagerJson for manually configured mixed endpoint failed: %v", err)
	}
	manualMixedJSONDoc := map[string]interface{}{}
	if err := json.Unmarshal([]byte(*manualMixedJSON), &manualMixedJSONDoc); err != nil {
		t.Fatalf("decode manual mixed JSON subscription failed: %v", err)
	}
	assertSubManagerJSONContainsTypes(t, manualMixedJSONDoc, map[string]string{
		"manual-mixed-node-socks": "socks",
		"manual-mixed-node-http":  "http",
	})

	manualMixedClash, err := renderer.GetSubManagerClash(manualMixedTag)
	if err != nil {
		t.Fatalf("GetSubManagerClash for manually configured mixed endpoint failed: %v", err)
	}
	manualMixedClashDoc := map[string]interface{}{}
	if err := yaml.Unmarshal([]byte(*manualMixedClash), &manualMixedClashDoc); err != nil {
		t.Fatalf("decode manual mixed Clash subscription failed: %v", err)
	}
	for _, name := range []string{"manual-mixed-node-socks", "manual-mixed-node-http"} {
		proxy := findNamedProxy(t, manualMixedClashDoc["proxies"], name)
		if proxy["type"] == "mixed" {
			t.Fatalf("manual mixed Clash proxy was not expanded: %#v", proxy)
		}
	}

	if err := database.GetDB().Create(&model.SubGroup{
		Name:      "manual-mixed-group",
		Outbounds: `["manual-mixed-node"]`,
	}).Error; err != nil {
		t.Fatalf("create manual mixed sub-group failed: %v", err)
	}
	manualMixedGroupJSON, err := renderer.GetSubGroupJson("manual-mixed-group")
	if err != nil {
		t.Fatalf("GetSubGroupJson for manually configured mixed endpoint failed: %v", err)
	}
	manualMixedGroupDoc := map[string]interface{}{}
	if err := json.Unmarshal([]byte(*manualMixedGroupJSON), &manualMixedGroupDoc); err != nil {
		t.Fatalf("decode manual mixed group JSON subscription failed: %v", err)
	}
	assertSubManagerJSONContainsTypes(t, manualMixedGroupDoc, map[string]string{
		"manual-mixed-node-socks": "socks",
		"manual-mixed-node-http":  "http",
	})

	mieruTag := "mieru-node"
	createSubOutboundFromMap(
		t,
		map[string]interface{}{
			"type":        "mieru",
			"tag":         mieruTag,
			"server":      "panel.example.com",
			"server_port": 16939,
		},
		subManagerSourceClient,
		1002,
		0,
		map[string]interface{}{
			"name":            mieruTag,
			"type":            "mieru",
			"server":          "panel.example.com",
			"port":            16939,
			"username":        "mieru-user",
			"password":        "mieru-secret",
			"metadata":        map[string]interface{}{"legacy": true},
			"user_management": map[string]interface{}{"selectable": true},
		},
	)

	mieruJSON, err := renderer.GetSubManagerJson(mieruTag)
	if err != nil {
		t.Fatalf("GetSubManagerJson for Mieru failed: %v", err)
	}
	jsonDoc := map[string]interface{}{}
	if err := json.Unmarshal([]byte(*mieruJSON), &jsonDoc); err != nil {
		t.Fatalf("decode Mieru JSON subscription failed: %v", err)
	}
	assertSubManagerJSONOmitsType(t, jsonDoc, mieruTag, "mieru")

	mieruClash, err := renderer.GetSubManagerClash(mieruTag)
	if err != nil {
		t.Fatalf("GetSubManagerClash for Mieru failed: %v", err)
	}
	clashDoc := map[string]interface{}{}
	if err := yaml.Unmarshal([]byte(*mieruClash), &clashDoc); err != nil {
		t.Fatalf("decode Mieru Clash subscription failed: %v", err)
	}
	mieruProxy := findNamedProxy(t, clashDoc["proxies"], mieruTag)
	if got, _ := mieruProxy["username"].(string); got != "mieru-user" {
		t.Fatalf("Mieru Clash proxy lost username: %#v", mieruProxy)
	}
	if got, _ := mieruProxy["password"].(string); got != "mieru-secret" {
		t.Fatalf("Mieru Clash proxy lost password: %#v", mieruProxy)
	}
	for _, key := range []string{"metadata", "user_management"} {
		if _, exists := mieruProxy[key]; exists {
			t.Fatalf("Mieru Clash proxy leaked %q: %#v", key, mieruProxy)
		}
	}

	for index, outboundType := range []string{"snell", "sudoku", "shadowquic", "trusttunnel"} {
		tag := "json-unsupported-" + outboundType
		createSubOutboundFromMap(
			t,
			map[string]interface{}{
				"type":        outboundType,
				"tag":         tag,
				"server":      "panel.example.com",
				"server_port": 443,
			},
			subManagerSourceMihomoClient,
			uint(1100+index),
			0,
			nil,
		)
		unsupportedJSON, err := renderer.GetSubManagerJson(tag)
		if err != nil {
			t.Fatalf("GetSubManagerJson for %s failed: %v", outboundType, err)
		}
		unsupportedDoc := map[string]interface{}{}
		if err := json.Unmarshal([]byte(*unsupportedJSON), &unsupportedDoc); err != nil {
			t.Fatalf("decode %s JSON subscription failed: %v", outboundType, err)
		}
		assertSubManagerJSONOmitsType(t, unsupportedDoc, tag, outboundType)
	}
}

func assertSubManagerJSONOmitsType(t *testing.T, document map[string]interface{}, tag string, outboundType string) {
	t.Helper()
	for _, item := range document["outbounds"].([]interface{}) {
		outbound, _ := item.(map[string]interface{})
		if outbound["tag"] == tag || outbound["type"] == outboundType {
			t.Fatalf("%s must not be emitted in SubManager sing-box JSON: %#v", outboundType, outbound)
		}
	}
}

func assertSubManagerJSONContainsTypes(t *testing.T, document map[string]interface{}, expected map[string]string) {
	t.Helper()
	seen := map[string]string{}
	for _, item := range document["outbounds"].([]interface{}) {
		outbound, _ := item.(map[string]interface{})
		tag, _ := outbound["tag"].(string)
		outboundType, _ := outbound["type"].(string)
		if _, wanted := expected[tag]; wanted {
			seen[tag] = outboundType
		}
	}
	for tag, outboundType := range expected {
		if seen[tag] != outboundType {
			t.Fatalf("expected %s with type %s in SubManager JSON, got %#v", tag, outboundType, seen)
		}
	}
}
