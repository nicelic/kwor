package service

import (
	"testing"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func TestCheckUserStoresLastLoginAsRFC3339UTC(t *testing.T) {
	_ = initTimeLocationSettingTestDB(t)
	fixed := time.Date(2026, time.July, 23, 4, 5, 6, 0, time.UTC)
	oldNow := panelTimeNow
	panelTimeNow = func() time.Time { return fixed }
	t.Cleanup(func() {
		panelTimeNow = oldNow
		InvalidatePanelTimeLocationCache()
	})

	if err := database.GetDB().Create(&model.User{Username: "admin", Password: "password"}).Error; err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	if user := (&UserService{}).CheckUser("admin", "password", "203.0.113.7"); user == nil {
		t.Fatal("CheckUser did not return the matching user")
	}

	var stored model.User
	if err := database.GetDB().Where("username = ?", "admin").First(&stored).Error; err != nil {
		t.Fatalf("load stored user failed: %v", err)
	}
	want := fixed.UTC().Format(time.RFC3339) + " 203.0.113.7"
	if stored.LastLogins != want {
		t.Fatalf("last login=%q want %q", stored.LastLogins, want)
	}
}
