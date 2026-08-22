package api

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/alireza0/s-ui/service"
	"github.com/gin-gonic/gin"
)

func TestSaveSingboxRouteReturnsStructuredRevisionConflict(t *testing.T) {
	setupSettingsPatchAPITestDB(t)
	apiService := &ApiService{}
	context, err := apiService.ConfigService.GetSingboxRouteEditorContext()
	if err != nil {
		t.Fatalf("load route context: %v", err)
	}
	body, err := json.Marshal(service.SingboxRouteSaveRequest{
		ExpectedRevision: context.Revision,
		Route:            json.RawMessage(`{"rules":[{"action":"sniff"}],"rule_set":[]}`),
	})
	if err != nil {
		t.Fatalf("marshal route request: %v", err)
	}
	currentConfig, err := apiService.SettingService.GetConfig()
	if err != nil {
		t.Fatalf("load current config: %v", err)
	}
	if err := apiService.SettingService.SetConfig(currentConfig); err != nil {
		t.Fatalf("advance route revision: %v", err)
	}

	_, conflict := performSingboxRouteJSONPost(t, apiService.SaveSingboxRoute, string(body))
	if conflict.Success {
		t.Fatal("stale route save unexpectedly succeeded")
	}
	object, ok := conflict.Obj.(map[string]any)
	if !ok || object["code"] != "revision_conflict" {
		t.Fatalf("missing structured route conflict: %#v", conflict.Obj)
	}
	if current, ok := object["currentRevision"].(float64); !ok || current != float64(context.Revision+1) {
		t.Fatalf("unexpected route conflict revision: %#v", conflict.Obj)
	}
}

func performSingboxRouteJSONPost(t *testing.T, handler func(*gin.Context, string), body string) (*httptest.ResponseRecorder, Msg) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("POST", "/api/singbox-route-save", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request
	handler(context, "tester")
	message := Msg{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &message); err != nil {
		t.Fatalf("decode response %q: %v", recorder.Body.String(), err)
	}
	return recorder, message
}
