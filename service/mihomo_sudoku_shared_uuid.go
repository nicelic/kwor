package service

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util"
	"github.com/gofrs/uuid/v5"
	"gorm.io/gorm"
)

func mihomoSudokuSharedUUIDFromOptions(options json.RawMessage) string {
	if len(options) == 0 {
		return ""
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(options, &payload); err != nil {
		return ""
	}

	return strings.TrimSpace(util.NormalizeSudokuKeyValue(payload["key"]))
}

func mihomoSudokuUUIDFromClientConfig(config json.RawMessage) string {
	if len(config) == 0 {
		return ""
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(config, &payload); err != nil {
		return ""
	}

	sudokuConfig, _ := payload["sudoku"].(map[string]interface{})
	if sudokuConfig == nil {
		return ""
	}

	return strings.TrimSpace(util.NormalizeSudokuKeyValue(sudokuConfig["uuid"]))
}

func generateMihomoSudokuUUID() (string, error) {
	newUUID, err := uuid.NewV4()
	if err != nil {
		return "", err
	}
	return newUUID.String(), nil
}

func setMihomoSudokuInboundKey(inbound *model.MihomoInbound, sharedUUID string) (bool, error) {
	if inbound == nil || inbound.Type != "sudoku" {
		return false, nil
	}

	payload := map[string]interface{}{}
	if len(inbound.Options) > 0 {
		if err := json.Unmarshal(inbound.Options, &payload); err != nil {
			return false, err
		}
	}

	if strings.TrimSpace(sharedUUID) == "" {
		var err error
		sharedUUID, err = generateMihomoSudokuUUID()
		if err != nil {
			return false, err
		}
	}

	currentUUID := strings.TrimSpace(util.NormalizeSudokuKeyValue(payload["key"]))
	if currentUUID == sharedUUID {
		return false, nil
	}

	payload["key"] = sharedUUID
	options, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return false, err
	}
	inbound.Options = options

	return true, nil
}

func ensureMihomoSudokuSharedUUID(inbound *model.MihomoInbound) (string, error) {
	if inbound == nil || inbound.Type != "sudoku" {
		return "", nil
	}

	sharedUUID := mihomoSudokuSharedUUIDFromOptions(inbound.Options)
	if sharedUUID == "" {
		var err error
		sharedUUID, err = generateMihomoSudokuUUID()
		if err != nil {
			return "", err
		}
	}

	if _, err := setMihomoSudokuInboundKey(inbound, sharedUUID); err != nil {
		return "", err
	}

	return sharedUUID, nil
}

func setMihomoClientSudokuUUID(client *model.MihomoClient, sharedUUID string) (bool, error) {
	if client == nil || strings.TrimSpace(sharedUUID) == "" {
		return false, nil
	}

	config := map[string]interface{}{}
	if len(client.Config) > 0 {
		if err := json.Unmarshal(client.Config, &config); err != nil {
			return false, err
		}
	}

	sudokuConfig, _ := config["sudoku"].(map[string]interface{})
	if sudokuConfig == nil {
		sudokuConfig = map[string]interface{}{}
	}

	if currentUUID := strings.TrimSpace(util.NormalizeSudokuKeyValue(sudokuConfig["uuid"])); currentUUID == sharedUUID {
		return false, nil
	}

	sudokuConfig["uuid"] = sharedUUID
	config["sudoku"] = sudokuConfig

	updatedConfig, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return false, err
	}
	client.Config = updatedConfig

	return true, nil
}

func synchronizeMihomoSudokuBindings(
	tx *gorm.DB,
	inMemoryClients []*model.MihomoClient,
	inMemoryInbounds []*model.MihomoInbound,
	extraSeedClientIDs []uint,
) (string, error) {
	if tx == nil {
		return "", nil
	}

	seedClientIDs := append([]uint{}, extraSeedClientIDs...)
	seedInboundIDs := make([]uint, 0)
	inMemoryClientIDs := make(map[uint]struct{})
	inMemoryInboundIDs := make(map[uint]struct{})
	activeClients := make([]*model.MihomoClient, 0, len(inMemoryClients))
	activeInbounds := make([]*model.MihomoInbound, 0, len(inMemoryInbounds))

	for _, client := range inMemoryClients {
		if client == nil {
			continue
		}
		activeClients = append(activeClients, client)
		if client.Id > 0 {
			inMemoryClientIDs[client.Id] = struct{}{}
			seedClientIDs = append(seedClientIDs, client.Id)
		}

		inboundIDs, err := parseClientInboundIDs(client.Inbounds)
		if err != nil {
			return "", err
		}
		seedInboundIDs = append(seedInboundIDs, inboundIDs...)
	}

	for _, inbound := range inMemoryInbounds {
		if inbound == nil || inbound.Type != "sudoku" {
			continue
		}
		activeInbounds = append(activeInbounds, inbound)
		if inbound.Id > 0 {
			inMemoryInboundIDs[inbound.Id] = struct{}{}
			seedInboundIDs = append(seedInboundIDs, inbound.Id)
		}
	}

	componentClients, componentInbounds, err := collectMihomoSudokuBindingComponent(tx, seedClientIDs, seedInboundIDs)
	if err != nil {
		return "", err
	}
	if len(activeInbounds) == 0 && len(componentInbounds) == 0 {
		return "", nil
	}

	sharedUUID := ""
	for _, client := range activeClients {
		sharedUUID = mihomoSudokuUUIDFromClientConfig(client.Config)
		if sharedUUID != "" {
			break
		}
	}
	if sharedUUID == "" {
		for _, inbound := range activeInbounds {
			sharedUUID = mihomoSudokuSharedUUIDFromOptions(inbound.Options)
			if sharedUUID != "" {
				break
			}
		}
	}
	if sharedUUID == "" {
		for index := range componentClients {
			if !componentClients[index].Enable {
				continue
			}
			sharedUUID = mihomoSudokuUUIDFromClientConfig(componentClients[index].Config)
			if sharedUUID != "" {
				break
			}
		}
	}
	if sharedUUID == "" {
		for index := range componentClients {
			sharedUUID = mihomoSudokuUUIDFromClientConfig(componentClients[index].Config)
			if sharedUUID != "" {
				break
			}
		}
	}
	if sharedUUID == "" {
		for index := range componentInbounds {
			sharedUUID = mihomoSudokuSharedUUIDFromOptions(componentInbounds[index].Options)
			if sharedUUID != "" {
				break
			}
		}
	}
	if sharedUUID == "" {
		sharedUUID, err = generateMihomoSudokuUUID()
		if err != nil {
			return "", err
		}
	}

	for _, client := range activeClients {
		if _, err := setMihomoClientSudokuUUID(client, sharedUUID); err != nil {
			return "", err
		}
	}
	for index := range componentClients {
		client := &componentClients[index]
		if _, exists := inMemoryClientIDs[client.Id]; exists {
			continue
		}
		changed, err := setMihomoClientSudokuUUID(client, sharedUUID)
		if err != nil {
			return "", err
		}
		if !changed {
			continue
		}
		if err := tx.Model(model.MihomoClient{}).Where("id = ?", client.Id).Update("config", client.Config).Error; err != nil {
			return "", err
		}
	}

	for _, inbound := range activeInbounds {
		if _, err := setMihomoSudokuInboundKey(inbound, sharedUUID); err != nil {
			return "", err
		}
	}
	for index := range componentInbounds {
		inbound := &componentInbounds[index]
		if _, exists := inMemoryInboundIDs[inbound.Id]; exists {
			continue
		}
		changed, err := setMihomoSudokuInboundKey(inbound, sharedUUID)
		if err != nil {
			return "", err
		}
		if !changed {
			continue
		}
		if err := tx.Model(model.MihomoInbound{}).Where("id = ?", inbound.Id).Update("options", inbound.Options).Error; err != nil {
			return "", err
		}
	}

	return sharedUUID, nil
}

func collectMihomoSudokuBindingComponent(
	tx *gorm.DB,
	seedClientIDs []uint,
	seedInboundIDs []uint,
) ([]model.MihomoClient, []model.MihomoInbound, error) {
	if tx == nil {
		return nil, nil, nil
	}

	var sudokuInbounds []model.MihomoInbound
	if err := tx.Model(model.MihomoInbound{}).Where("type = ?", "sudoku").Order("id asc").Find(&sudokuInbounds).Error; err != nil {
		return nil, nil, err
	}
	if len(sudokuInbounds) == 0 {
		return []model.MihomoClient{}, []model.MihomoInbound{}, nil
	}

	inboundByID := make(map[uint]model.MihomoInbound, len(sudokuInbounds))
	for _, inbound := range sudokuInbounds {
		inboundByID[inbound.Id] = inbound
	}

	var clients []model.MihomoClient
	if err := tx.Model(model.MihomoClient{}).Order("id asc").Find(&clients).Error; err != nil {
		return nil, nil, err
	}

	clientByID := make(map[uint]model.MihomoClient, len(clients))
	clientToInboundIDs := make(map[uint][]uint, len(clients))
	inboundToClientIDs := make(map[uint][]uint, len(sudokuInbounds))
	for _, client := range clients {
		clientByID[client.Id] = client

		inboundIDs, err := parseClientInboundIDs(client.Inbounds)
		if err != nil {
			return nil, nil, err
		}
		for _, inboundID := range inboundIDs {
			if _, exists := inboundByID[inboundID]; !exists {
				continue
			}
			clientToInboundIDs[client.Id] = append(clientToInboundIDs[client.Id], inboundID)
			inboundToClientIDs[inboundID] = append(inboundToClientIDs[inboundID], client.Id)
		}
	}

	queueClients := dedupeUintIDs(seedClientIDs)
	queueInbounds := dedupeUintIDs(seedInboundIDs)
	visitedClients := make(map[uint]struct{})
	visitedInbounds := make(map[uint]struct{})
	componentClients := make([]model.MihomoClient, 0)
	componentInbounds := make([]model.MihomoInbound, 0)

	for len(queueClients) > 0 || len(queueInbounds) > 0 {
		for len(queueClients) > 0 {
			clientID := queueClients[0]
			queueClients = queueClients[1:]
			if clientID == 0 {
				continue
			}
			if _, exists := visitedClients[clientID]; exists {
				continue
			}
			client, exists := clientByID[clientID]
			if !exists {
				continue
			}
			visitedClients[clientID] = struct{}{}
			componentClients = append(componentClients, client)
			queueInbounds = append(queueInbounds, clientToInboundIDs[clientID]...)
		}

		for len(queueInbounds) > 0 {
			inboundID := queueInbounds[0]
			queueInbounds = queueInbounds[1:]
			if inboundID == 0 {
				continue
			}
			if _, exists := visitedInbounds[inboundID]; exists {
				continue
			}
			inbound, exists := inboundByID[inboundID]
			if !exists {
				continue
			}
			visitedInbounds[inboundID] = struct{}{}
			componentInbounds = append(componentInbounds, inbound)
			queueClients = append(queueClients, inboundToClientIDs[inboundID]...)
		}
	}

	return componentClients, componentInbounds, nil
}

func dedupeUintIDs(values []uint) []uint {
	result := make([]uint, 0, len(values))
	seen := make(map[uint]struct{}, len(values))
	for _, value := range values {
		if value == 0 {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func parseIDList(raw string) []uint {
	values := strings.Split(raw, ",")
	result := make([]uint, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		parsed, err := strconv.ParseUint(value, 10, 64)
		if err != nil || parsed == 0 {
			continue
		}
		result = append(result, uint(parsed))
	}
	return result
}
