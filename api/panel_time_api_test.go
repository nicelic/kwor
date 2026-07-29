package api

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/service"

	"github.com/gin-gonic/gin"
)

func TestGetPanelTimeContextReturnsDatabaseTimeZone(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "panel-time-api.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	if sqlDB, err := database.GetDB().DB(); err == nil && sqlDB != nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}

	settingService := &service.SettingService{}
	if err := settingService.SaveSetting("timeLocation", "Asia/Tokyo"); err != nil {
		t.Fatalf("save panel timezone failed: %v", err)
	}

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest("GET", "/api/panel-time-context", nil)
	(&ApiService{}).GetPanelTimeContext(context)

	var response Msg
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if !response.Success {
		t.Fatalf("endpoint failed: %s", response.Msg)
	}
	body, ok := response.Obj.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected context body: %#v", response.Obj)
	}
	if body["timeLocation"] != "Asia/Tokyo" {
		t.Fatalf("timeLocation=%v want Asia/Tokyo", body["timeLocation"])
	}
	if unix, ok := body["unix"].(float64); !ok || unix <= 0 {
		t.Fatalf("invalid server unix value: %#v", body["unix"])
	}
}
