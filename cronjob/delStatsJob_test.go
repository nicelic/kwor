package cronjob

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/service"
)

func TestDelStatsJobReadsCurrentTrafficAgeAfterRuntimeChange(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "del-stats-job.db")); err != nil {
		t.Fatalf("init db: %v", err)
	}
	sqlDB, _ := database.GetDB().DB()
	t.Cleanup(func() {
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	})

	settings := &service.SettingService{}
	if _, err := settings.GetAllSetting(); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
	if err := settings.SaveSetting("trafficAge", "0"); err != nil {
		t.Fatalf("disable traffic history: %v", err)
	}
	old := model.Stats{
		DateTime: time.Now().Add(-48 * time.Hour).Unix(),
		Resource: "inbound",
		Tag:      "old",
		Traffic:  1,
	}
	if err := database.GetDB().Create(&old).Error; err != nil {
		t.Fatalf("create old stat: %v", err)
	}

	job := NewDelStatsJob()
	job.Run()
	var count int64
	if err := database.GetDB().Model(&model.Stats{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("disabled cleanup changed stats: count=%d err=%v", count, err)
	}

	if err := settings.SaveSetting("trafficAge", "1"); err != nil {
		t.Fatalf("enable traffic history: %v", err)
	}
	job.Run()
	if err := database.GetDB().Model(&model.Stats{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("runtime retention update was not applied: count=%d err=%v", count, err)
	}
}
