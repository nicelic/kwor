package service

import (
	"strings"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"

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
	if err := EnsureHistoryStorageReady(); err != nil {
		return err
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
			if normalizeStatsResource(stat.Resource) != "client" {
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

	err = upsertStatsTrafficBatch(tx, *stats)
	if err != nil {
		return err
	}

	return nil
}

func (s *StatsService) GetStats(resource string, tag string, limit int) ([]model.Stats, error) {
	return queryStatsHistory(resource, tag, limit)
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
	result := db.Where("date_time < ?", oldTime).Delete(model.Stats{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		if err := compactMainSQLiteDB(db, false); err != nil {
			logger.Warning("compact sqlite after deleting old stats failed: ", err)
		}
	}
	return nil
}
