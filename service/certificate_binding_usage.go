package service

import (
	"strings"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util/common"
	"gorm.io/gorm"
)

// RefreshCertificateBindingUsageFlagsTx recalculates the persisted deletion
// guards for the supplied inventory records. Call it in the same transaction
// as every TLS or reverse-proxy binding change.
func RefreshCertificateBindingUsageFlagsTx(tx *gorm.DB, recordIDs []uint) error {
	if tx == nil {
		return common.NewError("database transaction is not ready")
	}
	ids := normalizeCertificateBindingRecordIDs(recordIDs)
	if len(ids) == 0 {
		return nil
	}

	type usageFlags struct {
		reverseProxy bool
		singboxTLS   bool
		mihomoTLS    bool
	}
	usageByID := make(map[uint]usageFlags, len(ids))
	idSet := make(map[uint]struct{}, len(ids))
	for _, id := range ids {
		idSet[id] = struct{}{}
	}

	defaultRows := make([]model.Tls, 0)
	if err := tx.Model(&model.Tls{}).
		Select("certificate_record_id").
		Where("certificate_record_id IN ?", ids).
		Find(&defaultRows).Error; err != nil {
		return err
	}
	for i := range defaultRows {
		id := defaultRows[i].CertificateRecordID
		flags := usageByID[id]
		flags.singboxTLS = true
		usageByID[id] = flags
	}

	mihomoRows := make([]model.MihomoTls, 0)
	if err := tx.Model(&model.MihomoTls{}).
		Select("certificate_record_id").
		Where("certificate_record_id IN ?", ids).
		Find(&mihomoRows).Error; err != nil {
		return err
	}
	for i := range mihomoRows {
		id := mihomoRows[i].CertificateRecordID
		flags := usageByID[id]
		flags.mihomoTLS = true
		usageByID[id] = flags
	}

	reverseRows := make([]model.ReverseProxyRule, 0)
	if err := tx.Model(&model.ReverseProxyRule{}).
		Select("certificate_record_id", "certificate_record_list").
		Find(&reverseRows).Error; err != nil {
		return err
	}
	for i := range reverseRows {
		for _, id := range reverseProxyRuleStoredCertificateIDs(&reverseRows[i]) {
			if _, tracked := idSet[id]; !tracked {
				continue
			}
			flags := usageByID[id]
			flags.reverseProxy = true
			usageByID[id] = flags
		}
	}

	for _, id := range ids {
		flags := usageByID[id]
		if err := tx.Model(&model.CertificateRecord{}).
			Where("id = ?", id).
			Updates(map[string]interface{}{
				"bound_by_reverse_proxy": flags.reverseProxy,
				"bound_by_singbox_tls":   flags.singboxTLS,
				"bound_by_mihomo_tls":    flags.mihomoTLS,
			}).Error; err != nil {
			return err
		}
	}
	return nil
}

// RefreshAllCertificateBindingUsageFlags repairs rows created before the
// persistent binding flags existed. Normal request paths refresh only changed
// IDs; this full scan is reserved for application bootstrap and tests.
func RefreshAllCertificateBindingUsageFlags() error {
	db := database.GetDB()
	if db == nil {
		return common.NewError("database is not ready")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		rows := make([]model.CertificateRecord, 0)
		if err := tx.Model(&model.CertificateRecord{}).Select("id").Find(&rows).Error; err != nil {
			return err
		}
		ids := make([]uint, 0, len(rows))
		for i := range rows {
			ids = append(ids, rows[i].Id)
		}
		return RefreshCertificateBindingUsageFlagsTx(tx, ids)
	})
}

func normalizeCertificateBindingRecordIDs(values []uint) []uint {
	if len(values) == 0 {
		return nil
	}
	result := make([]uint, 0, len(values))
	seen := make(map[uint]struct{}, len(values))
	for _, id := range values {
		if id == 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func certificateDeleteBlockReason(row *model.CertificateRecord) string {
	if row == nil {
		return ""
	}
	reasons := make([]string, 0, 3)
	if row.BoundByReverseProxy {
		reasons = append(reasons, "已应用到反向代理，请先解除反向代理中的证书绑定")
	}
	if row.BoundBySingboxTLS {
		reasons = append(reasons, "已绑定到 sing-box TLS 设置，请先解除 TLS 设置中的证书绑定")
	}
	if row.BoundByMihomoTLS {
		reasons = append(reasons, "已绑定到 Mihomo TLS 设置，请先解除 TLS 设置中的证书绑定")
	}
	return strings.Join(reasons, "；")
}

func certificateDeleteBlocked(row *model.CertificateRecord) bool {
	return strings.TrimSpace(certificateDeleteBlockReason(row)) != ""
}

func certificateMinimumAssignmentDeleteError(targets []PanelSelfSignedTarget) error {
	if len(targets) == 0 {
		return nil
	}
	labels := make([]string, 0, len(targets))
	for _, target := range targets {
		switch target {
		case PanelSelfSignedTargetPanel:
			labels = append(labels, "界面")
		case PanelSelfSignedTargetSub:
			labels = append(labels, "订阅")
		}
	}
	if len(labels) == 0 {
		return nil
	}
	return common.NewErrorf("%s 至少需要保留一张已应用证书，无法删除", strings.Join(labels, "、"))
}
