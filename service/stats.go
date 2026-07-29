package service

import (
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
)

type onlines struct {
	Inbound  []string `json:"inbound,omitempty"`
	User     []string `json:"user,omitempty"`
	Outbound []string `json:"outbound,omitempty"`
}

type onlineResourcesSnapshot struct {
	mu    sync.RWMutex
	value onlines
}

var onlineResources onlineResourcesSnapshot
var mihomoOnlineResources onlineResourcesSnapshot

func setOnlines(inboundTags []string, userTags []string, outboundTags []string) {
	onlineResources.mu.Lock()
	onlineResources.value = onlines{
		Inbound:  normalizeOnlineTags(inboundTags),
		Outbound: normalizeOnlineTags(outboundTags),
		User:     normalizeOnlineTags(userTags),
	}
	onlineResources.mu.Unlock()
}

type StatsService struct {
	NftTrafficService
}

func (s *StatsService) SaveStats(_ bool) error {
	setOnlines(nil, nil, nil)
	return nil
}

func (s *StatsService) GetStats(resource string, tag string, limit int) ([]model.Stats, error) {
	return queryStatsHistory(resource, tag, limit)
}

func (s *StatsService) GetOnlines() (onlines, error) {
	return onlineResources.clone(), nil
}

func (s *StatsService) GetMihomoOnlines() (onlines, error) {
	return mihomoOnlineResources.clone(), nil
}

func setMihomoOnlines(inboundTags []string, userTags []string) {
	mihomoOnlineResources.mu.Lock()
	mihomoOnlineResources.value = onlines{
		Inbound:  normalizeOnlineTags(inboundTags),
		Outbound: []string{},
		User:     normalizeOnlineTags(userTags),
	}
	mihomoOnlineResources.mu.Unlock()
}

func (s *onlineResourcesSnapshot) clone() onlines {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return onlines{
		Inbound:  append([]string(nil), s.value.Inbound...),
		User:     append([]string(nil), s.value.User...),
		Outbound: append([]string(nil), s.value.Outbound...),
	}
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
