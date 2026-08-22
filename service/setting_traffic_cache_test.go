package service

import "testing"

func TestTrafficAgeCacheInvalidatesAfterDirectSave(t *testing.T) {
	settingService := initTimeLocationSettingTestDB(t)
	if _, err := settingService.GetAllSetting(); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
	if err := settingService.SaveSetting("trafficAge", "30"); err != nil {
		t.Fatalf("save initial trafficAge: %v", err)
	}
	value, err := settingService.GetTrafficAge()
	if err != nil || value != 30 {
		t.Fatalf("load initial trafficAge: value=%d err=%v", value, err)
	}
	if err := settingService.SaveSetting("trafficAge", "0"); err != nil {
		t.Fatalf("save updated trafficAge: %v", err)
	}
	value, err = settingService.GetTrafficAge()
	if err != nil || value != 0 {
		t.Fatalf("trafficAge cache was not invalidated: value=%d err=%v", value, err)
	}
}
