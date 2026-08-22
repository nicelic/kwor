package service

import (
	"fmt"

	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util/common"
	"gorm.io/gorm"
)

const (
	acmeAccountDisplayIDMin uint64 = 1
	acmeAccountDisplayIDMax uint64 = 100000000000
	dnsAccountDisplayIDMin  uint64 = 1
	dnsAccountDisplayIDMax  uint64 = 100000000000
)

func acmeAccountResourceID(displayID uint64) string {
	if displayID == 0 {
		return ""
	}
	return fmt.Sprintf("acme_%d", displayID)
}

func dnsAccountResourceID(displayID uint64) string {
	if displayID == 0 {
		return ""
	}
	return fmt.Sprintf("dns_%d", displayID)
}

func certificateResourceID(displayID uint64) string {
	if displayID == 0 {
		return ""
	}
	return fmt.Sprintf("cert_%d", displayID)
}

func ensureAcmeAccountDisplayID(db *gorm.DB, entry *model.AcmeAccount) error {
	if entry == nil || entry.DisplayID > 0 {
		return nil
	}
	rows := make([]model.AcmeAccount, 0)
	if err := db.Select("id", "display_id").Where("display_id > 0 AND system = ?", false).Order("display_id ASC").Find(&rows).Error; err != nil {
		return err
	}
	used := make(map[uint64]struct{}, len(rows))
	for i := range rows {
		used[rows[i].DisplayID] = struct{}{}
	}
	for candidate := acmeAccountDisplayIDMin; candidate <= acmeAccountDisplayIDMax; candidate++ {
		if _, exists := used[candidate]; exists {
			continue
		}
		entry.DisplayID = candidate
		return nil
	}
	return common.NewError("ACME 账号编号已耗尽")
}

func repairAcmeAccountDisplayIDs(db *gorm.DB) error {
	rows := make([]model.AcmeAccount, 0)
	if err := db.Select("id", "display_id").Where("system = ?", false).Order("id ASC").Find(&rows).Error; err != nil {
		return err
	}
	used := make(map[uint64]struct{}, len(rows))
	for i := range rows {
		if rows[i].DisplayID > 0 {
			if _, exists := used[rows[i].DisplayID]; !exists {
				used[rows[i].DisplayID] = struct{}{}
				continue
			}
		}
		rows[i].DisplayID = 0
	}
	for i := range rows {
		if rows[i].DisplayID > 0 {
			continue
		}
		if err := ensureAcmeAccountDisplayID(db, &rows[i]); err != nil {
			return err
		}
		used[rows[i].DisplayID] = struct{}{}
		if err := db.Model(&model.AcmeAccount{}).Where("id = ?", rows[i].Id).Update("display_id", rows[i].DisplayID).Error; err != nil {
			return err
		}
	}
	return nil
}

func repairDNSAccountDisplayIDs(db *gorm.DB) error {
	rows := make([]model.AcmeDNSAccount, 0)
	if err := db.Select("id", "display_id").Order("id ASC").Find(&rows).Error; err != nil {
		return err
	}
	used := make(map[uint64]struct{}, len(rows))
	for i := range rows {
		if rows[i].DisplayID > 0 {
			if _, exists := used[rows[i].DisplayID]; !exists {
				used[rows[i].DisplayID] = struct{}{}
				continue
			}
		}
		rows[i].DisplayID = 0
	}
	for i := range rows {
		if rows[i].DisplayID > 0 {
			continue
		}
		if err := ensureDNSAccountDisplayID(db, &rows[i]); err != nil {
			return err
		}
		used[rows[i].DisplayID] = struct{}{}
		if err := db.Model(&model.AcmeDNSAccount{}).Where("id = ?", rows[i].Id).Update("display_id", rows[i].DisplayID).Error; err != nil {
			return err
		}
	}
	return nil
}

func ensureDNSAccountDisplayID(db *gorm.DB, entry *model.AcmeDNSAccount) error {
	if entry == nil || entry.DisplayID > 0 {
		return nil
	}
	rows := make([]model.AcmeDNSAccount, 0)
	if err := db.Select("id", "display_id").Where("display_id > 0").Order("display_id ASC").Find(&rows).Error; err != nil {
		return err
	}
	used := make(map[uint64]struct{}, len(rows))
	for i := range rows {
		used[rows[i].DisplayID] = struct{}{}
	}
	for candidate := dnsAccountDisplayIDMin; candidate <= dnsAccountDisplayIDMax; candidate++ {
		if _, exists := used[candidate]; exists {
			continue
		}
		entry.DisplayID = candidate
		return nil
	}
	return common.NewError("DNS 账号编号已耗尽")
}
