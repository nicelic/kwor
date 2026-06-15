package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"gorm.io/gorm"
)

type subJSONFileOwner struct {
	FileName    string
	OwnerType   string
	DisplayName string
}

func effectiveSubJSONGuardDB(db *gorm.DB) (*gorm.DB, error) {
	effectiveDB := db
	if effectiveDB == nil {
		effectiveDB = database.GetDB()
	}
	if effectiveDB == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	return effectiveDB, nil
}

func buildClientSubJSONBaseName(inboundTag string, clientName string) string {
	inboundTag = strings.TrimSpace(inboundTag)
	clientName = strings.TrimSpace(clientName)
	if inboundTag == "" || clientName == "" {
		return ""
	}
	return fmt.Sprintf("%s_%s", sanitizeFilename(inboundTag), sanitizeFilename(clientName))
}

func collectEnabledClientSubJSONFileOwners(db *gorm.DB) ([]subJSONFileOwner, error) {
	var clients []model.Client
	if err := db.Model(model.Client{}).
		Select("id", "enable", "name", "inbounds").
		Where("enable = ?", true).
		Order("id ASC").
		Find(&clients).Error; err != nil {
		return nil, err
	}
	if len(clients) == 0 {
		return nil, nil
	}

	var inbounds []model.Inbound
	if err := db.Model(model.Inbound{}).
		Select("id", "tag").
		Order("id ASC").
		Find(&inbounds).Error; err != nil {
		return nil, err
	}

	inboundTagByID := make(map[uint]string, len(inbounds))
	for _, inbound := range inbounds {
		inboundTagByID[inbound.Id] = strings.TrimSpace(inbound.Tag)
	}

	owners := make([]subJSONFileOwner, 0)
	for _, client := range clients {
		clientName := strings.TrimSpace(client.Name)
		if clientName == "" {
			continue
		}

		var inboundIDs []uint
		if len(client.Inbounds) > 0 {
			if err := json.Unmarshal(client.Inbounds, &inboundIDs); err != nil {
				return nil, fmt.Errorf("parse client %q inbounds failed: %w", clientName, err)
			}
		}

		for _, inboundID := range inboundIDs {
			inboundTag := strings.TrimSpace(inboundTagByID[inboundID])
			baseName := buildClientSubJSONBaseName(inboundTag, clientName)
			if baseName == "" {
				continue
			}
			owners = append(owners, subJSONFileOwner{
				FileName:    baseName,
				OwnerType:   "client subscription",
				DisplayName: fmt.Sprintf("%s/%s", inboundTag, clientName),
			})
		}
	}

	return owners, nil
}

func findClientSubJSONFileConflict(db *gorm.DB, baseName string) (*subJSONFileOwner, error) {
	owners, err := collectEnabledClientSubJSONFileOwners(db)
	if err != nil {
		return nil, err
	}
	for _, owner := range owners {
		if owner.FileName == baseName {
			conflict := owner
			return &conflict, nil
		}
	}
	return nil, nil
}

func validateManagedSubJSONFileNames(db *gorm.DB) error {
	effectiveDB, err := effectiveSubJSONGuardDB(db)
	if err != nil {
		return err
	}

	seen := make(map[string]subJSONFileOwner)

	var groups []model.SubGroup
	if err := effectiveDB.Model(model.SubGroup{}).
		Select("id", "name").
		Order("id ASC").
		Find(&groups).Error; err != nil {
		return err
	}
	for _, group := range groups {
		owner := subJSONFileOwner{
			FileName:    sanitizeGroupFilename(group.Name),
			OwnerType:   "subgroup name",
			DisplayName: strings.TrimSpace(group.Name),
		}
		if existing, ok := seen[owner.FileName]; ok {
			return fmt.Errorf(
				"%s %q conflicts with %s %q after sub_json filename normalization",
				owner.OwnerType,
				owner.DisplayName,
				existing.OwnerType,
				existing.DisplayName,
			)
		}
		seen[owner.FileName] = owner
	}

	var subOutbounds []model.SubOutbound
	if err := effectiveDB.Model(model.SubOutbound{}).
		Select("id", "tag").
		Order("id ASC").
		Find(&subOutbounds).Error; err != nil {
		return err
	}
	for _, subOutbound := range subOutbounds {
		owner := subJSONFileOwner{
			FileName:    sanitizeSubFilename(subOutbound.Tag),
			OwnerType:   "suboutbound tag",
			DisplayName: strings.TrimSpace(subOutbound.Tag),
		}
		if existing, ok := seen[owner.FileName]; ok {
			return fmt.Errorf(
				"%s %q conflicts with %s %q after sub_json filename normalization",
				owner.OwnerType,
				owner.DisplayName,
				existing.OwnerType,
				existing.DisplayName,
			)
		}
		seen[owner.FileName] = owner
	}

	clientOwners, err := collectEnabledClientSubJSONFileOwners(effectiveDB)
	if err != nil {
		return err
	}
	for _, owner := range clientOwners {
		if existing, ok := seen[owner.FileName]; ok {
			return fmt.Errorf(
				"%s %q conflicts with %s %q after sub_json filename normalization",
				owner.OwnerType,
				owner.DisplayName,
				existing.OwnerType,
				existing.DisplayName,
			)
		}
		seen[owner.FileName] = owner
	}

	return nil
}

func ValidateManagedSubOutboundTagForSubJSON(tag string) error {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return nil
	}

	effectiveDB, err := effectiveSubJSONGuardDB(nil)
	if err != nil {
		return err
	}

	subOutbound := &model.SubOutbound{}
	if err := effectiveDB.Model(model.SubOutbound{}).
		Select("id", "tag").
		Where("tag = ?", tag).
		First(subOutbound).Error; err != nil {
		return err
	}

	return validateSubOutboundSubJSONFileName(effectiveDB, subOutbound)
}

func validateSubGroupSubJSONFileName(db *gorm.DB, group *model.SubGroup) error {
	if group == nil {
		return nil
	}

	effectiveDB, err := effectiveSubJSONGuardDB(db)
	if err != nil {
		return err
	}

	baseName := sanitizeGroupFilename(group.Name)

	var groups []model.SubGroup
	if err := effectiveDB.Model(model.SubGroup{}).Select("id", "name").Find(&groups).Error; err != nil {
		return err
	}
	for _, existing := range groups {
		if existing.Id == group.Id {
			continue
		}
		if sanitizeGroupFilename(existing.Name) == baseName {
			return fmt.Errorf(
				"subgroup name %q conflicts with subgroup %q after sub_json filename normalization",
				strings.TrimSpace(group.Name),
				strings.TrimSpace(existing.Name),
			)
		}
	}

	var subOutbounds []model.SubOutbound
	if err := effectiveDB.Model(model.SubOutbound{}).Select("id", "tag").Find(&subOutbounds).Error; err != nil {
		return err
	}
	for _, existing := range subOutbounds {
		if sanitizeSubFilename(existing.Tag) == baseName {
			return fmt.Errorf(
				"subgroup name %q conflicts with suboutbound tag %q after sub_json filename normalization",
				strings.TrimSpace(group.Name),
				strings.TrimSpace(existing.Tag),
			)
		}
	}

	clientConflict, err := findClientSubJSONFileConflict(effectiveDB, baseName)
	if err != nil {
		return err
	}
	if clientConflict != nil {
		return fmt.Errorf(
			"subgroup name %q conflicts with %s %q after sub_json filename normalization",
			strings.TrimSpace(group.Name),
			clientConflict.OwnerType,
			clientConflict.DisplayName,
		)
	}

	return nil
}

func validateSubOutboundSubJSONFileName(db *gorm.DB, subOutbound *model.SubOutbound) error {
	if subOutbound == nil {
		return nil
	}

	effectiveDB, err := effectiveSubJSONGuardDB(db)
	if err != nil {
		return err
	}

	baseName := sanitizeSubFilename(subOutbound.Tag)

	var subOutbounds []model.SubOutbound
	if err := effectiveDB.Model(model.SubOutbound{}).Select("id", "tag").Find(&subOutbounds).Error; err != nil {
		return err
	}
	for _, existing := range subOutbounds {
		if existing.Id == subOutbound.Id {
			continue
		}
		if sanitizeSubFilename(existing.Tag) == baseName {
			return fmt.Errorf(
				"suboutbound tag %q conflicts with suboutbound tag %q after sub_json filename normalization",
				strings.TrimSpace(subOutbound.Tag),
				strings.TrimSpace(existing.Tag),
			)
		}
	}

	var groups []model.SubGroup
	if err := effectiveDB.Model(model.SubGroup{}).Select("id", "name").Find(&groups).Error; err != nil {
		return err
	}
	for _, existing := range groups {
		if sanitizeGroupFilename(existing.Name) == baseName {
			return fmt.Errorf(
				"suboutbound tag %q conflicts with subgroup name %q after sub_json filename normalization",
				strings.TrimSpace(subOutbound.Tag),
				strings.TrimSpace(existing.Name),
			)
		}
	}

	clientConflict, err := findClientSubJSONFileConflict(effectiveDB, baseName)
	if err != nil {
		return err
	}
	if clientConflict != nil {
		return fmt.Errorf(
			"suboutbound tag %q conflicts with %s %q after sub_json filename normalization",
			strings.TrimSpace(subOutbound.Tag),
			clientConflict.OwnerType,
			clientConflict.DisplayName,
		)
	}

	return nil
}
