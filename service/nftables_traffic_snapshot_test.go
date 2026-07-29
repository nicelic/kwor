package service

import (
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestSyncClientBindingsUsesInboundCounterSnapshots(t *testing.T) {
	db := initClientLimitTestDB(t, "nft-traffic-snapshot.db")
	client := mustCreateDefaultClient(t, db, model.Client{Name: "snapshot-client"})

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction failed: %v", tx.Error)
	}
	if err := (&NftTrafficService{}).syncClientBindings(tx, client.Id, []uint{101}, map[uint]inboundCounterSnapshot{
		101: {inBytes: 1234, outBytes: 5678},
	}); err != nil {
		tx.Rollback()
		t.Fatalf("sync default client bindings failed: %v", err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit transaction failed: %v", err)
	}

	var binding model.ClientInboundTrafficState
	if err := db.Where("client_id = ? AND inbound_id = ?", client.Id, 101).First(&binding).Error; err != nil {
		t.Fatalf("load default client binding failed: %v", err)
	}
	if binding.LastInBytes != 1234 || binding.LastOutBytes != 5678 {
		t.Fatalf("unexpected default binding baseline: in=%d out=%d", binding.LastInBytes, binding.LastOutBytes)
	}
}

func TestMihomoSyncClientBindingsUsesInboundCounterSnapshots(t *testing.T) {
	db := initClientLimitTestDB(t, "mihomo-nft-traffic-snapshot.db")
	client := mustCreateMihomoClient(t, db, model.MihomoClient{Name: "mihomo-snapshot-client"})

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction failed: %v", tx.Error)
	}
	if err := (&MihomoNftTrafficService{}).syncClientBindings(tx, client.Id, []uint{202}, map[uint]inboundCounterSnapshot{
		202: {inBytes: 2468, outBytes: 1357},
	}); err != nil {
		tx.Rollback()
		t.Fatalf("sync mihomo client bindings failed: %v", err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit transaction failed: %v", err)
	}

	var binding model.MihomoClientInboundTrafficState
	if err := db.Where("client_id = ? AND inbound_id = ?", client.Id, 202).First(&binding).Error; err != nil {
		t.Fatalf("load mihomo client binding failed: %v", err)
	}
	if binding.LastInBytes != 2468 || binding.LastOutBytes != 1357 {
		t.Fatalf("unexpected mihomo binding baseline: in=%d out=%d", binding.LastInBytes, binding.LastOutBytes)
	}
}

func TestSyncClientBindingsUsesPersistedCountersWhenCollectionSnapshotMissesInbound(t *testing.T) {
	db := initClientLimitTestDB(t, "nft-traffic-persisted-fallback.db")
	client := mustCreateDefaultClient(t, db, model.Client{Name: "persisted-fallback-client"})
	if err := db.Create(&model.InboundTrafficState{
		InboundId: 303,
		InBytes:   4321,
		OutBytes:  8765,
	}).Error; err != nil {
		t.Fatalf("create inbound traffic state failed: %v", err)
	}

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction failed: %v", tx.Error)
	}
	if err := (&NftTrafficService{}).syncClientBindings(tx, client.Id, []uint{303}, map[uint]inboundCounterSnapshot{}); err != nil {
		tx.Rollback()
		t.Fatalf("sync default client bindings failed: %v", err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit transaction failed: %v", err)
	}

	var binding model.ClientInboundTrafficState
	if err := db.Where("client_id = ? AND inbound_id = ?", client.Id, 303).First(&binding).Error; err != nil {
		t.Fatalf("load default client binding failed: %v", err)
	}
	if binding.LastInBytes != 4321 || binding.LastOutBytes != 8765 {
		t.Fatalf("expected persisted default binding baseline, got in=%d out=%d", binding.LastInBytes, binding.LastOutBytes)
	}
}

func TestMihomoSyncClientBindingsUsesPersistedCountersWhenCollectionSnapshotMissesInbound(t *testing.T) {
	db := initClientLimitTestDB(t, "mihomo-nft-traffic-persisted-fallback.db")
	client := mustCreateMihomoClient(t, db, model.MihomoClient{Name: "mihomo-persisted-fallback-client"})
	if err := db.Create(&model.MihomoInboundRedirectState{
		InboundId: 404,
		InBytes:   24680,
		OutBytes:  13579,
	}).Error; err != nil {
		t.Fatalf("create mihomo inbound traffic state failed: %v", err)
	}

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction failed: %v", tx.Error)
	}
	if err := (&MihomoNftTrafficService{}).syncClientBindings(tx, client.Id, []uint{404}, map[uint]inboundCounterSnapshot{}); err != nil {
		tx.Rollback()
		t.Fatalf("sync mihomo client bindings failed: %v", err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit transaction failed: %v", err)
	}

	var binding model.MihomoClientInboundTrafficState
	if err := db.Where("client_id = ? AND inbound_id = ?", client.Id, 404).First(&binding).Error; err != nil {
		t.Fatalf("load mihomo client binding failed: %v", err)
	}
	if binding.LastInBytes != 24680 || binding.LastOutBytes != 13579 {
		t.Fatalf("expected persisted mihomo binding baseline, got in=%d out=%d", binding.LastInBytes, binding.LastOutBytes)
	}
}
