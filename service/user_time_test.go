package service

import (
	"fmt"
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

func TestDeleteTokenRequiresOwningUser(t *testing.T) {
	_ = initTimeLocationSettingTestDB(t)
	db := database.GetDB()
	users := []model.User{
		{Username: "alice", Password: "a"},
		{Username: "bob", Password: "b"},
	}
	for i := range users {
		if err := db.Create(&users[i]).Error; err != nil {
			t.Fatalf("create user %s failed: %v", users[i].Username, err)
		}
	}
	token := model.Tokens{Token: "token", Desc: "test", UserId: users[1].Id}
	if err := db.Create(&token).Error; err != nil {
		t.Fatalf("create token failed: %v", err)
	}

	service := &UserService{}
	if err := service.DeleteToken("alice", fmt.Sprint(token.Id)); err == nil {
		t.Fatal("non-owner was allowed to delete token")
	}
	if err := service.DeleteToken("bob", fmt.Sprint(token.Id)); err != nil {
		t.Fatalf("owner could not delete token: %v", err)
	}
}

func TestChangePassRejectsDuplicateUsername(t *testing.T) {
	_ = initTimeLocationSettingTestDB(t)
	db := database.GetDB()
	users := []model.User{
		{Username: "alice", Password: "a"},
		{Username: "bob", Password: "b"},
	}
	for i := range users {
		if err := db.Create(&users[i]).Error; err != nil {
			t.Fatalf("create user %s failed: %v", users[i].Username, err)
		}
	}

	if _, err := (&UserService{}).ChangePass(fmt.Sprint(users[1].Id), "b", "alice", "next"); err == nil {
		t.Fatal("duplicate username was accepted")
	}
	var stored model.User
	if err := db.First(&stored, users[1].Id).Error; err != nil {
		t.Fatalf("reload original user failed: %v", err)
	}
	if stored.Username != "bob" || stored.Password != "b" {
		t.Fatalf("duplicate rename partially changed user: %#v", stored)
	}
}
