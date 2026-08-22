package api

import (
	"encoding/json"
	"testing"

	"github.com/alireza0/s-ui/service"
)

func TestSaveSingboxBasicsReturnsResultForNoopMutation(t *testing.T) {
	setupSettingsPatchAPITestDB(t)
	apiService := &ApiService{}
	if err := apiService.SettingService.SaveSetting("config", `{"dns":{"rules":[]},"route":{"rules":[]},"experimental":{}}`); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	context, err := apiService.ConfigService.GetSingboxBasicsEditorContext()
	if err != nil {
		t.Fatalf("load basics context: %v", err)
	}
	body, err := json.Marshal(service.SingboxBasicsSaveRequest{
		ExpectedRevision: context.Revision,
		Basics:           context.Basics,
	})
	if err != nil {
		t.Fatalf("marshal basics request: %v", err)
	}

	_, response := performSingboxRouteJSONPost(t, apiService.SaveSingboxBasics, string(body))
	if !response.Success {
		t.Fatalf("basics no-op save failed: %#v", response)
	}
	object, ok := response.Obj.(map[string]any)
	if !ok || object["revision"] != float64(context.Revision) {
		t.Fatalf("unexpected basics save response: %#v", response.Obj)
	}
}

func TestSaveSingboxBasicsReturnsStructuredRevisionConflict(t *testing.T) {
	setupSettingsPatchAPITestDB(t)
	apiService := &ApiService{}
	if err := apiService.SettingService.SaveSetting("config", `{"dns":{"rules":[]},"route":{"rules":[]},"experimental":{}}`); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	context, err := apiService.ConfigService.GetSingboxBasicsEditorContext()
	if err != nil {
		t.Fatalf("load basics context: %v", err)
	}
	body, err := json.Marshal(service.SingboxBasicsSaveRequest{
		ExpectedRevision: context.Revision,
		Basics:           context.Basics,
	})
	if err != nil {
		t.Fatalf("marshal basics request: %v", err)
	}
	currentConfig, err := apiService.SettingService.GetConfig()
	if err != nil {
		t.Fatalf("load current config: %v", err)
	}
	if err := apiService.SettingService.SetConfig(currentConfig); err != nil {
		t.Fatalf("advance basics revision: %v", err)
	}

	_, conflict := performSingboxRouteJSONPost(t, apiService.SaveSingboxBasics, string(body))
	if conflict.Success {
		t.Fatal("stale basics save unexpectedly succeeded")
	}
	object, ok := conflict.Obj.(map[string]any)
	if !ok || object["code"] != "revision_conflict" {
		t.Fatalf("missing structured basics conflict: %#v", conflict.Obj)
	}
	if current, ok := object["currentRevision"].(float64); !ok || current != float64(context.Revision+1) {
		t.Fatalf("unexpected basics conflict revision: %#v", conflict.Obj)
	}
}
