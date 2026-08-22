package api

import (
	"encoding/json"
	"testing"

	"github.com/alireza0/s-ui/service"
)

func TestSaveSingboxDNSReturnsSnapshotForNoopMutation(t *testing.T) {
	setupSettingsPatchAPITestDB(t)
	apiService := &ApiService{}
	if err := apiService.SettingService.SaveSetting("config", `{"dns":{"rules":[]},"route":{"rules":[]}}`); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	context, err := apiService.ConfigService.GetSingboxDNSSnapshot()
	if err != nil {
		t.Fatalf("load DNS context: %v", err)
	}
	body, err := json.Marshal(service.SingboxDNSMutationRequest{
		ExpectedRevision: context.Revision,
		DNS:              context.DNS,
	})
	if err != nil {
		t.Fatalf("marshal DNS request: %v", err)
	}

	_, response := performSingboxRouteJSONPost(t, apiService.SaveSingboxDNS, string(body))
	if !response.Success {
		t.Fatalf("DNS no-op save failed: %#v", response)
	}
	object, ok := response.Obj.(map[string]any)
	if !ok {
		t.Fatalf("unexpected DNS save response: %#v", response.Obj)
	}
	if _, ok := object["snapshot"].(map[string]any); !ok {
		t.Fatalf("DNS save response lacks snapshot: %#v", object)
	}
}

func TestSaveSingboxDNSReturnsStructuredRevisionConflict(t *testing.T) {
	setupSettingsPatchAPITestDB(t)
	apiService := &ApiService{}
	if err := apiService.SettingService.SaveSetting("config", `{"dns":{"rules":[]},"route":{"rules":[]}}`); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	context, err := apiService.ConfigService.GetSingboxDNSSnapshot()
	if err != nil {
		t.Fatalf("load DNS context: %v", err)
	}
	body, err := json.Marshal(service.SingboxDNSMutationRequest{
		ExpectedRevision: context.Revision,
		DNS:              context.DNS,
	})
	if err != nil {
		t.Fatalf("marshal DNS request: %v", err)
	}
	currentConfig, err := apiService.SettingService.GetConfig()
	if err != nil {
		t.Fatalf("load current config: %v", err)
	}
	if err := apiService.SettingService.SetConfig(currentConfig); err != nil {
		t.Fatalf("advance DNS revision: %v", err)
	}

	_, conflict := performSingboxRouteJSONPost(t, apiService.SaveSingboxDNS, string(body))
	if conflict.Success {
		t.Fatal("stale DNS save unexpectedly succeeded")
	}
	object, ok := conflict.Obj.(map[string]any)
	if !ok || object["code"] != "revision_conflict" {
		t.Fatalf("missing structured DNS conflict: %#v", conflict.Obj)
	}
	if current, ok := object["currentRevision"].(float64); !ok || current != float64(context.Revision+1) {
		t.Fatalf("unexpected DNS conflict revision: %#v", conflict.Obj)
	}
}
