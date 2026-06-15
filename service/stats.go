package service

import (
	"strings"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"

	"gorm.io/gorm"
)

type onlines struct {
	Inbound  []string `json:"inbound,omitempty"`
	User     []string `json:"user,omitempty"`
	Outbound []string `json:"outbound,omitempty"`
}

var onlineResources = &onlines{}
var mihomoOnlineResources = &onlines{}

func setOnlines(inboundTags []string, userTags []string, outboundTags []string) {
	onlineResources.Inbound = normalizeOnlineTags(inboundTags)
	onlineResources.Outbound = normalizeOnlineTags(outboundTags)
	onlineResources.User = normalizeOnlineTags(userTags)
}

type StatsService struct {
	NftTrafficService
}

func (s *StatsService) SaveStats(enableTraffic bool) error {
	if !corePtr.IsRunning() {
		setOnlines(nil, nil, nil)
		return nil
	}
	stats := corePtr.GetInstance().StatsTracker().GetStats()

	if len(*stats) == 0 {
		if !enableTraffic {
			setOnlines(nil, nil, nil)
		}
		return nil
	}

	var err error
	db := database.GetDB()
	tx := db.Begin()
	defer func() {
		if err == nil {
			tx.Commit()
		} else {
			tx.Rollback()
		}
	}()

	if !enableTraffic {
		for _, stat := range *stats {
			if stat.Resource != "user" {
				continue
			}
			if stat.Direction {
				err = tx.Model(model.Client{}).Where("name = ?", stat.Tag).
					UpdateColumn("up", gorm.Expr("up + ?", stat.Traffic)).Error
			} else {
				err = tx.Model(model.Client{}).Where("name = ?", stat.Tag).
					UpdateColumn("down", gorm.Expr("down + ?", stat.Traffic)).Error
			}
			if err != nil {
				return err
			}
		}
		return nil
	}

	err = tx.Create(&stats).Error
	if err != nil {
		return err
	}

	return nil
}

func (s *StatsService) GetStats(resource string, tag string, limit int) ([]model.Stats, error) {
	var err error
	var result []model.Stats

	currentTime := time.Now().Unix()
	timeDiff := currentTime - (int64(limit) * 3600)

	db := database.GetDB()
	resources := []string{resource}
	if resource == "endpoint" {
		resources = []string{"inbound", "outbound"}
	}
	err = db.Model(model.Stats{}).Where("resource in ? AND tag = ? AND date_time > ?", resources, tag, timeDiff).Scan(&result).Error
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *StatsService) GetOnlines() (onlines, error) {
	return *onlineResources, nil
}

func (s *StatsService) GetMihomoOnlines() (onlines, error) {
	return *mihomoOnlineResources, nil
}

func setMihomoOnlines(inboundTags []string, userTags []string) {
	mihomoOnlineResources.Inbound = normalizeOnlineTags(inboundTags)
	mihomoOnlineResources.Outbound = []string{}
	mihomoOnlineResources.User = normalizeOnlineTags(userTags)
}

func normalizeOnlineTags(tags []string) []string {
	if len(tags) == 0 {
		return []string{}
	}

	set := make(map[string]struct{}, len(tags))
	normalized := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, exists := set[tag]; exists {
			continue
		}
		set[tag] = struct{}{}
		normalized = append(normalized, tag)
	}
	return normalized
}

func (s *StatsService) DelOldStats(days int) error {
	oldTime := time.Now().AddDate(0, 0, -(days)).Unix()
	db := database.GetDB()
	return db.Where("date_time < ?", oldTime).Delete(model.Stats{}).Error
}
