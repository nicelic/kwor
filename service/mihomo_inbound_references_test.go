package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func mihomoRouteConfigForInboundReference(tag string) json.RawMessage {
	return json.RawMessage(`{
		"route": {
			"final": "DIRECT",
			"rules": [{
				"action": "route",
				"outbound": "DIRECT",
				"inbound": ["` + tag + `"]
			}],
			"rule_set": []
		}
	}`)
}

func createMihomoRouteReferenceInbound(t *testing.T, tag string, options string) model.MihomoInbound {
	t.Helper()
	inbound := model.MihomoInbound{
		Type:    "http",
		Tag:     tag,
		Options: json.RawMessage(options),
	}
	if err := database.GetDB().Create(&inbound).Error; err != nil {
		t.Fatalf("create mihomo route-reference inbound failed: %v", err)
	}
	return inbound
}

func TestMihomoConfigSaveRejectsUnavailableInboundReference(t *testing.T) {
	db := initMihomoManagerFinalFallbackTestDB(t, "mihomo-route-missing-inbound.db")
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction failed: %v", tx.Error)
	}
	defer tx.Rollback()

	err := (&MihomoConfigService{}).SaveConfig(tx, mihomoRouteConfigForInboundReference("missing-listener"))
	if err == nil || !strings.Contains(err.Error(), "references unavailable inbound tag(s): missing-listener") {
		t.Fatalf("expected unavailable inbound rejection, got %v", err)
	}
}

func TestMihomoConfigSaveRejectsMalformedInboundReference(t *testing.T) {
	db := initMihomoManagerFinalFallbackTestDB(t, "mihomo-route-malformed-inbound.db")
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction failed: %v", tx.Error)
	}
	defer tx.Rollback()

	config := json.RawMessage(`{
		"route": {
			"final": "DIRECT",
			"rules": [{
				"action": "route",
				"outbound": "DIRECT",
				"inbound": [123]
			}],
			"rule_set": []
		}
	}`)
	err := (&MihomoConfigService{}).SaveConfig(tx, config)
	if err == nil || !strings.Contains(err.Error(), "route rule #1 inbound must be a string or an array of strings") {
		t.Fatalf("expected malformed inbound rejection, got %v", err)
	}
}

func TestMihomoRouteEditorContextExcludesNativeDetourListeners(t *testing.T) {
	initMihomoManagerFinalFallbackTestDB(t, "mihomo-route-detour-context.db")
	createMihomoRouteReferenceInbound(t, "rule-listener", `{"listen":"::","listen_port":18080}`)
	createMihomoRouteReferenceInbound(t, "detoured-listener", `{"listen":"::","listen_port":18081,"detour":"DIRECT"}`)

	context, err := GetMihomoRouteEditorContext(database.GetDB())
	if err != nil {
		t.Fatalf("GetMihomoRouteEditorContext failed: %v", err)
	}

	seen := make(map[string]struct{}, len(context.InboundTags))
	for _, tag := range context.InboundTags {
		seen[tag] = struct{}{}
	}
	if _, ok := seen["rule-listener"]; !ok {
		t.Fatalf("expected ordinary listener in route context: %#v", context.InboundTags)
	}
	if _, ok := seen["detoured-listener"]; ok {
		t.Fatalf("native detour listener must not be selectable for route rules: %#v", context.InboundTags)
	}

	tx := database.GetDB().Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction failed: %v", tx.Error)
	}
	defer tx.Rollback()
	err = (&MihomoConfigService{}).SaveConfig(tx, mihomoRouteConfigForInboundReference("detoured-listener"))
	if err == nil || !strings.Contains(err.Error(), "references unavailable inbound tag(s): detoured-listener") {
		t.Fatalf("expected detoured listener rejection, got %v", err)
	}
}

func TestMihomoGeneratorRejectsPersistedUnavailableInboundReference(t *testing.T) {
	db := initMihomoManagerFinalFallbackTestDB(t, "mihomo-route-legacy-missing-inbound.db")
	setMihomoConfigForFallbackTest(t, db, string(mihomoRouteConfigForInboundReference("missing-listener")))

	_, err := NewMihomoManagerService().GenerateServerDocument()
	if err == nil || !strings.Contains(err.Error(), "references unavailable inbound tag(s): missing-listener") {
		t.Fatalf("expected persisted unavailable inbound rejection, got %v", err)
	}
}

func TestMihomoInboundSaveRejectsRouteReferenceAfterDeleteOrDetour(t *testing.T) {
	t.Run("delete", func(t *testing.T) {
		db := initMihomoManagerFinalFallbackTestDB(t, "mihomo-route-delete-inbound.db")
		inbound := createMihomoRouteReferenceInbound(t, "delete-listener", `{"listen":"::","listen_port":18082}`)
		setMihomoConfigForFallbackTest(t, db, string(mihomoRouteConfigForInboundReference(inbound.Tag)))

		tx := db.Begin()
		if tx.Error != nil {
			t.Fatalf("begin transaction failed: %v", tx.Error)
		}
		defer tx.Rollback()

		_, err := (&MihomoInboundService{}).Save(tx, "del", mustJSONRaw(t, inbound.Tag), "", "panel.example.com")
		if err == nil || !strings.Contains(err.Error(), "references unavailable inbound tag(s): delete-listener") {
			t.Fatalf("expected delete to reject route reference, got %v", err)
		}
	})

	t.Run("native detour", func(t *testing.T) {
		db := initMihomoManagerFinalFallbackTestDB(t, "mihomo-route-detour-inbound.db")
		inbound := createMihomoRouteReferenceInbound(t, "detour-listener", `{"listen":"::","listen_port":18083}`)
		setMihomoConfigForFallbackTest(t, db, string(mihomoRouteConfigForInboundReference(inbound.Tag)))

		payload := map[string]interface{}{
			"id":          inbound.Id,
			"type":        inbound.Type,
			"tag":         inbound.Tag,
			"listen":      "::",
			"listen_port": 18083,
			"detour":      "DIRECT",
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal inbound edit payload failed: %v", err)
		}

		tx := db.Begin()
		if tx.Error != nil {
			t.Fatalf("begin transaction failed: %v", tx.Error)
		}
		defer tx.Rollback()

		_, err = (&MihomoInboundService{}).Save(tx, "edit", raw, "", "panel.example.com")
		if err == nil || !strings.Contains(err.Error(), "references unavailable inbound tag(s): detour-listener") {
			t.Fatalf("expected detour edit to reject route reference, got %v", err)
		}
	})
}
