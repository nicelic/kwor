package service

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func TestEnsureMihomoSudokuSharedUUIDGeneratesKey(t *testing.T) {
	inbound := &model.MihomoInbound{
		Type:    "sudoku",
		Options: json.RawMessage(`{"listen":"::","listen_port":38571}`),
	}

	sharedUUID, err := ensureMihomoSudokuSharedUUID(inbound)
	if err != nil {
		t.Fatalf("ensureMihomoSudokuSharedUUID failed: %v", err)
	}
	if sharedUUID == "" {
		t.Fatalf("expected generated sudoku shared uuid, got empty string")
	}
	if got := mihomoSudokuSharedUUIDFromOptions(inbound.Options); got != sharedUUID {
		t.Fatalf("expected inbound options key %q, got %q", sharedUUID, got)
	}
}

func TestSynchronizeMihomoSudokuBindingsPropagatesClientKeyAcrossComponent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mihomo-sudoku-shared-uuid.db")
	if err := database.InitDB(dbPath); err != nil {
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

	inboundOne := model.MihomoInbound{
		Type:    "sudoku",
		Tag:     "sudoku-38571",
		Options: json.RawMessage(`{"listen":"::","listen_port":38571,"key":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}`),
	}
	inboundTwo := model.MihomoInbound{
		Type:    "sudoku",
		Tag:     "sudoku-38572",
		Options: json.RawMessage(`{"listen":"::","listen_port":38572,"key":"bbbbbbbb-cccc-dddd-eeee-ffffffffffff"}`),
	}
	if err := db.Create(&inboundOne).Error; err != nil {
		t.Fatalf("create first mihomo sudoku inbound failed: %v", err)
	}
	if err := db.Create(&inboundTwo).Error; err != nil {
		t.Fatalf("create second mihomo sudoku inbound failed: %v", err)
	}

	clientB := model.MihomoClient{
		Enable:   true,
		Name:     "bridge",
		Config:   json.RawMessage(`{"sudoku":{"uuid":"old-bridge-key"}}`),
		Inbounds: json.RawMessage(fmt.Sprintf("[%d,%d]", inboundOne.Id, inboundTwo.Id)),
		Links:    json.RawMessage(`[]`),
	}
	clientC := model.MihomoClient{
		Enable:   true,
		Name:     "leaf",
		Config:   json.RawMessage(`{"sudoku":{"uuid":"old-leaf-key"}}`),
		Inbounds: json.RawMessage(fmt.Sprintf("[%d]", inboundTwo.Id)),
		Links:    json.RawMessage(`[]`),
	}
	if err := db.Create(&clientB).Error; err != nil {
		t.Fatalf("create bridge mihomo client failed: %v", err)
	}
	if err := db.Create(&clientC).Error; err != nil {
		t.Fatalf("create leaf mihomo client failed: %v", err)
	}

	clientA := &model.MihomoClient{
		Id:       clientB.Id,
		Enable:   true,
		Name:     "bridge",
		Config:   json.RawMessage(`{"sudoku":{"uuid":" 11111111-2222-3333-4444-555555555555 \n"}}`),
		Inbounds: json.RawMessage(fmt.Sprintf("[%d,%d]", inboundOne.Id, inboundTwo.Id)),
		Links:    json.RawMessage(`[]`),
	}

	sharedUUID, err := synchronizeMihomoSudokuBindings(db, []*model.MihomoClient{clientA}, nil, nil)
	if err != nil {
		t.Fatalf("synchronizeMihomoSudokuBindings failed: %v", err)
	}
	if sharedUUID != "11111111-2222-3333-4444-555555555555" {
		t.Fatalf("expected normalized sudoku key from client config, got %q", sharedUUID)
	}

	var reloadedInboundOne model.MihomoInbound
	if err := db.First(&reloadedInboundOne, inboundOne.Id).Error; err != nil {
		t.Fatalf("reload first mihomo inbound failed: %v", err)
	}
	if got := mihomoSudokuSharedUUIDFromOptions(reloadedInboundOne.Options); got != sharedUUID {
		t.Fatalf("expected first inbound key %q, got %q", sharedUUID, got)
	}

	var reloadedInboundTwo model.MihomoInbound
	if err := db.First(&reloadedInboundTwo, inboundTwo.Id).Error; err != nil {
		t.Fatalf("reload second mihomo inbound failed: %v", err)
	}
	if got := mihomoSudokuSharedUUIDFromOptions(reloadedInboundTwo.Options); got != sharedUUID {
		t.Fatalf("expected second inbound key %q, got %q", sharedUUID, got)
	}

	if got := mihomoSudokuUUIDFromClientConfig(clientA.Config); got != sharedUUID {
		t.Fatalf("expected in-memory client key %q, got %q", sharedUUID, got)
	}

	var reloadedClientC model.MihomoClient
	if err := db.First(&reloadedClientC, clientC.Id).Error; err != nil {
		t.Fatalf("reload leaf mihomo client failed: %v", err)
	}
	if got := mihomoSudokuUUIDFromClientConfig(reloadedClientC.Config); got != sharedUUID {
		t.Fatalf("expected synced leaf client key %q, got %q", sharedUUID, got)
	}
}

func TestSynchronizeMihomoSudokuBindingsPrefersActiveInboundKey(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mihomo-sudoku-shared-uuid-inbound-priority.db")
	if err := database.InitDB(dbPath); err != nil {
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

	inbound := model.MihomoInbound{
		Type:    "sudoku",
		Tag:     "sudoku-38573",
		Options: json.RawMessage(`{"listen":"::","listen_port":38573,"key":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}`),
	}
	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create mihomo sudoku inbound failed: %v", err)
	}

	client := model.MihomoClient{
		Enable:   true,
		Name:     "sudoku-user",
		Config:   json.RawMessage(`{"sudoku":{"uuid":"bbbbbbbb-cccc-dddd-eeee-ffffffffffff"}}`),
		Inbounds: json.RawMessage(fmt.Sprintf("[%d]", inbound.Id)),
		Links:    json.RawMessage(`[]`),
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create mihomo sudoku client failed: %v", err)
	}

	inMemoryInbound := &model.MihomoInbound{
		Id:      inbound.Id,
		Type:    "sudoku",
		Tag:     inbound.Tag,
		Options: json.RawMessage(`{"listen":"::","listen_port":38573,"key":"11111111-2222-3333-4444-555555555555"}`),
	}

	sharedUUID, err := synchronizeMihomoSudokuBindings(db, nil, []*model.MihomoInbound{inMemoryInbound}, nil)
	if err != nil {
		t.Fatalf("synchronizeMihomoSudokuBindings failed: %v", err)
	}
	if sharedUUID != "11111111-2222-3333-4444-555555555555" {
		t.Fatalf("expected shared uuid from active inbound, got %q", sharedUUID)
	}

	var reloadedClient model.MihomoClient
	if err := db.First(&reloadedClient, client.Id).Error; err != nil {
		t.Fatalf("reload mihomo client failed: %v", err)
	}
	if got := mihomoSudokuUUIDFromClientConfig(reloadedClient.Config); got != sharedUUID {
		t.Fatalf("expected synced client key %q, got %q", sharedUUID, got)
	}

	if got := mihomoSudokuSharedUUIDFromOptions(inMemoryInbound.Options); got != sharedUUID {
		t.Fatalf("expected synced in-memory inbound key %q, got %q", sharedUUID, got)
	}
}
