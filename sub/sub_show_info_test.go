package sub

import (
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/service"
)

func TestDefaultSubscriptionAppendsClientInfoWhenEnabled(t *testing.T) {
	setupSubscriptionTestDB(t, "default-sub-show-info-enabled.db")

	if err := (&service.SettingService{}).SaveSetting("subShowInfo", "true"); err != nil {
		t.Fatalf("enable subShowInfo failed: %v", err)
	}

	client := model.Client{
		Enable:   true,
		Name:     "sub-show-info-enabled",
		Config:   mustRawJSON(t, map[string]interface{}{}),
		Inbounds: mustRawJSON(t, []uint{}),
		Links: mustRawJSON(t, []Link{
			{
				Type:   "local",
				Remark: "demo",
				Uri:    "trojan://secret@example.com:443#demo",
			},
		}),
		Volume: 2 * 1024 * 1024 * 1024,
		Up:     512 * 1024 * 1024,
		Down:   512 * 1024 * 1024,
	}
	if err := database.GetDB().Create(&client).Error; err != nil {
		t.Fatalf("create client failed: %v", err)
	}

	result, _, err := (&SubService{}).GetSubs(client.Name)
	if err != nil || result == nil {
		t.Fatalf("GetSubs failed: payload=%v err=%v", result, err)
	}

	decoded := decodeSubscriptionPayload(t, *result)
	if !strings.Contains(decoded, "trojan://secret@example.com:443#demo 1.00GB📊") {
		t.Fatalf("expected enabled subShowInfo to append traffic info, got %q", decoded)
	}
}

func TestDefaultSubscriptionSkipsClientInfoWhenDisabled(t *testing.T) {
	setupSubscriptionTestDB(t, "default-sub-show-info-disabled.db")

	if err := (&service.SettingService{}).SaveSetting("subShowInfo", "false"); err != nil {
		t.Fatalf("disable subShowInfo failed: %v", err)
	}

	client := model.Client{
		Enable:   true,
		Name:     "sub-show-info-disabled",
		Config:   mustRawJSON(t, map[string]interface{}{}),
		Inbounds: mustRawJSON(t, []uint{}),
		Links: mustRawJSON(t, []Link{
			{
				Type:   "local",
				Remark: "demo",
				Uri:    "trojan://secret@example.com:443#demo",
			},
		}),
		Volume: 2 * 1024 * 1024 * 1024,
		Up:     512 * 1024 * 1024,
		Down:   512 * 1024 * 1024,
	}
	if err := database.GetDB().Create(&client).Error; err != nil {
		t.Fatalf("create client failed: %v", err)
	}

	result, _, err := (&SubService{}).GetSubs(client.Name)
	if err != nil || result == nil {
		t.Fatalf("GetSubs failed: payload=%v err=%v", result, err)
	}

	decoded := decodeSubscriptionPayload(t, *result)
	if strings.Contains(decoded, "1.00GB📊") {
		t.Fatalf("expected disabled subShowInfo to skip traffic info, got %q", decoded)
	}
	if decoded != "trojan://secret@example.com:443#demo" {
		t.Fatalf("expected local link to stay unchanged when subShowInfo is disabled, got %q", decoded)
	}
}
