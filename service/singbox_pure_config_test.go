package service

import (
	"testing"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func TestSingboxPureConfigNftShortCircuitWhenCoreNotRunning(t *testing.T) {
	initPanelSQLiteSettingTestDB(t)
	configService := &ConfigService{}

	// When core is not running, rate limit and port block policies should immediately return nil
	if err := configService.applyClientRateLimitNft(); err != nil {
		t.Fatalf("applyClientRateLimitNft returned error: %v", err)
	}
	if err := configService.applyClientPortBlockNft(); err != nil {
		t.Fatalf("applyClientPortBlockNft returned error: %v", err)
	}
}

func TestApplyInboundNftActionPreservesExistingBaselinesWhenCoreNotRunning(t *testing.T) {
	initPanelSQLiteSettingTestDB(t)
	db := database.GetDB()

	// Seed existing inbound traffic states
	existingState := model.InboundTrafficState{
		InboundId: 10,
		Tag:       "existing-inbound",
		InBytes:   12345678,
		OutBytes:  87654321,
		InHandle:  101,
		OutHandle: 102,
		UpdatedAt: time.Now(),
	}
	if err := db.Create(&existingState).Error; err != nil {
		t.Fatalf("create existing state: %v", err)
	}

	nftSvc := &NftTrafficService{}

	// Upsert a different inbound when core is not running
	action := &InboundNftAction{
		Kind:      "upsert",
		InboundID: 20,
		Tag:       "new-inbound",
		Port:      8080,
	}
	if err := nftSvc.ApplyInboundNftAction(db, action); err != nil {
		t.Fatalf("ApplyInboundNftAction upsert failed: %v", err)
	}

	// Verify existing inbound state was NOT cleared by cleanupOnShutdown
	var afterState model.InboundTrafficState
	if err := db.Where("inbound_id = ?", 10).First(&afterState).Error; err != nil {
		t.Fatalf("read existing state: %v", err)
	}
	if afterState.InBytes != 12345678 || afterState.OutBytes != 87654321 {
		t.Fatalf("existing baseline wiped! InBytes=%d, OutBytes=%d", afterState.InBytes, afterState.OutBytes)
	}
	if afterState.InHandle != 101 || afterState.OutHandle != 102 {
		t.Fatalf("existing handles wiped! InHandle=%d, OutHandle=%d", afterState.InHandle, afterState.OutHandle)
	}

	// Remove the new inbound when core is not running
	removeAction := &InboundNftAction{
		Kind:      "remove",
		InboundID: 20,
	}
	if err := nftSvc.ApplyInboundNftAction(db, removeAction); err != nil {
		t.Fatalf("ApplyInboundNftAction remove failed: %v", err)
	}

	// Verify existing baseline is STILL preserved
	if err := db.Where("inbound_id = ?", 10).First(&afterState).Error; err != nil {
		t.Fatalf("read existing state after remove: %v", err)
	}
	if afterState.InBytes != 12345678 || afterState.OutBytes != 87654321 {
		t.Fatalf("existing baseline wiped after remove! InBytes=%d, OutBytes=%d", afterState.InBytes, afterState.OutBytes)
	}
}
