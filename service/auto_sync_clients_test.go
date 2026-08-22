package service

import (
	"encoding/json"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func TestMihomoAutoSyncRegistryUsesMihomoRevisionAndListState(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "auto-sync-clients.db")); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	db := database.GetDB()
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB failed: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	defaultClient := model.Client{
		Enable:   true,
		Name:     "default-auto-sync",
		Inbounds: json.RawMessage(`[]`),
		Links:    json.RawMessage(`[]`),
	}
	if err := db.Create(&defaultClient).Error; err != nil {
		t.Fatalf("create default client failed: %v", err)
	}
	mihomoClient := model.MihomoClient{
		Enable:   true,
		Name:     "mihomo-auto-sync",
		Inbounds: json.RawMessage(`[]`),
		Links:    json.RawMessage(`[]`),
	}
	if err := db.Create(&mihomoClient).Error; err != nil {
		t.Fatalf("create mihomo client failed: %v", err)
	}

	oldDefaultRevision := atomic.LoadInt64(&LastUpdate)
	oldMihomoRevision := atomic.LoadInt64(&MihomoLastUpdate)
	t.Cleanup(func() {
		atomic.StoreInt64(&LastUpdate, oldDefaultRevision)
		atomic.StoreInt64(&MihomoLastUpdate, oldMihomoRevision)
	})
	atomic.StoreInt64(&LastUpdate, 100)
	atomic.StoreInt64(&MihomoLastUpdate, 200)

	settingService := &SettingService{}
	if err := settingService.SetSubManagerAutoSyncMihomoClient(mihomoClient.Id, true); err != nil {
		t.Fatalf("enable mihomo auto sync failed: %v", err)
	}
	if got := atomic.LoadInt64(&LastUpdate); got != 100 {
		t.Fatalf("mihomo auto sync changed default revision: got=%d want=100", got)
	}
	mihomoRevision := atomic.LoadInt64(&MihomoLastUpdate)
	if mihomoRevision <= 200 {
		t.Fatalf("mihomo auto sync did not advance mihomo revision: got=%d", mihomoRevision)
	}

	mihomoClients, err := (&MihomoClientService{}).GetAll()
	if err != nil {
		t.Fatalf("load mihomo clients failed: %v", err)
	}
	if len(*mihomoClients) != 1 || !(*mihomoClients)[0].AutoSync {
		t.Fatalf("mihomo client list did not include autoSync state: %#v", mihomoClients)
	}

	if err := settingService.SetSubManagerAutoSyncClient(defaultClient.Id, true); err != nil {
		t.Fatalf("enable default auto sync failed: %v", err)
	}
	if got := atomic.LoadInt64(&MihomoLastUpdate); got != mihomoRevision {
		t.Fatalf("default auto sync changed mihomo revision: got=%d want=%d", got, mihomoRevision)
	}
	if got := atomic.LoadInt64(&LastUpdate); got <= 100 {
		t.Fatalf("default auto sync did not advance default revision: got=%d", got)
	}
}
