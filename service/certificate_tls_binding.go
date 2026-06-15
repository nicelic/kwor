package service

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/util/common"
	"gorm.io/gorm"
)

// SyncTLSBindingsForCertificateRecord refreshes TLS presets that are bound to a
// certificate inventory record. It keeps the TLS rows in sync after renewals or
// re-issues, then regenerates runtime and subscription outputs for both cores.
func SyncTLSBindingsForCertificateRecord(recordID uint, hostname string) (bool, error) {
	return syncTLSBindingsForCertificateRecord(recordID, hostname, false)
}

// ForceSyncTLSBindingsForCertificateRecord refreshes and broadcasts certificate
// binding changes even when the TLS JSON points at the same managed paths.
func ForceSyncTLSBindingsForCertificateRecord(recordID uint, hostname string) (bool, error) {
	return syncTLSBindingsForCertificateRecord(recordID, hostname, true)
}

func ForceSyncTLSPathBindingsForTLSIDs(defaultTLSIDs []uint, mihomoTLSIDs []uint, hostname string) (bool, error) {
	defaultTLSIDs = compactPositiveUintList(defaultTLSIDs)
	mihomoTLSIDs = compactPositiveUintList(mihomoTLSIDs)
	if len(defaultTLSIDs) == 0 && len(mihomoTLSIDs) == 0 {
		return false, nil
	}

	if err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		if len(defaultTLSIDs) > 0 {
			if err := refreshDefaultInboundOutJSONsForTLSIDsTx(tx, defaultTLSIDs, hostname); err != nil {
				return err
			}
		}
		if len(mihomoTLSIDs) > 0 {
			if err := refreshMihomoInboundOutJSONsForTLSIDsTx(tx, mihomoTLSIDs, hostname); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return false, err
	}

	configSvc := &ConfigService{}
	if len(defaultTLSIDs) > 0 {
		manager := NewProManagerService(configSvc)
		manager.regenerateInboundConfigs()
		manager.regenerateCoreConfig()
		manager.regenerateSubJsonConfigs()
		if err := configSvc.syncAutoManagedDefaultClientsForCertificateBinding(hostname, defaultTLSIDs); err != nil {
			return true, err
		}
	}
	if len(mihomoTLSIDs) > 0 {
		if err := NewMihomoManagerService().RegenerateServerConfig(); err != nil {
			return true, err
		}
		if err := configSvc.syncAutoManagedMihomoClientsForCertificateBinding(hostname, mihomoTLSIDs); err != nil {
			return true, err
		}
	}

	LastUpdate = time.Now().Unix()
	return true, nil
}

func syncTLSBindingsForCertificateRecord(recordID uint, hostname string, force bool) (bool, error) {
	if recordID == 0 {
		return false, nil
	}

	record, err := certificateInventory.GetRecordByID(recordID)
	if err != nil {
		return false, err
	}

	var defaultChanged bool
	var mihomoChanged bool
	var defaultTLSIDs []uint
	var mihomoTLSIDs []uint

	if err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		changed, ids, err := refreshDefaultTLSBindingsTx(tx, record)
		if err != nil {
			return err
		}
		defaultChanged = changed
		defaultTLSIDs = ids

		changed, ids, err = refreshMihomoTLSBindingsTx(tx, record)
		if err != nil {
			return err
		}
		mihomoChanged = changed
		mihomoTLSIDs = ids

		if defaultChanged || (force && len(defaultTLSIDs) > 0) {
			if err := refreshDefaultInboundOutJSONsForTLSIDsTx(tx, defaultTLSIDs, hostname); err != nil {
				return err
			}
		}
		if mihomoChanged || (force && len(mihomoTLSIDs) > 0) {
			if err := refreshMihomoInboundOutJSONsForTLSIDsTx(tx, mihomoTLSIDs, hostname); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return false, err
	}

	configSvc := &ConfigService{}
	defaultBroadcast := defaultChanged || (force && len(defaultTLSIDs) > 0)
	mihomoBroadcast := mihomoChanged || (force && len(mihomoTLSIDs) > 0)

	if defaultBroadcast {
		manager := NewProManagerService(configSvc)
		manager.regenerateInboundConfigs()
		manager.regenerateCoreConfig()
		manager.regenerateSubJsonConfigs()
		if force {
			err = configSvc.syncAutoManagedDefaultClientsForCertificateBinding(hostname, defaultTLSIDs)
		} else {
			err = configSvc.syncAutoManagedDefaultClients(hostname)
		}
		if err != nil {
			return true, err
		}
	}
	if mihomoBroadcast {
		if err := NewMihomoManagerService().RegenerateServerConfig(); err != nil {
			return true, err
		}
		if force {
			err = configSvc.syncAutoManagedMihomoClientsForCertificateBinding(hostname, mihomoTLSIDs)
		} else {
			err = configSvc.syncAutoManagedMihomoClients(hostname)
		}
		if err != nil {
			return true, err
		}
	}
	if defaultBroadcast || mihomoBroadcast {
		LastUpdate = time.Now().Unix()
	}

	return defaultBroadcast || mihomoBroadcast, nil
}

func refreshDefaultTLSBindingsTx(tx *gorm.DB, record *model.CertificateRecord) (bool, []uint, error) {
	rows := make([]model.Tls, 0)
	if err := tx.Where("certificate_record_id = ?", record.Id).Find(&rows).Error; err != nil {
		return false, nil, err
	}

	changedAny := false
	ids := make([]uint, 0, len(rows))
	for i := range rows {
		ids = append(ids, rows[i].Id)
		changed, err := applyCertificateRecordToTLSRaw(&rows[i].Server, &rows[i].Client, record)
		if err != nil {
			return false, nil, err
		}
		if !changed {
			continue
		}
		rows[i].CertificateRecordID = record.Id
		if err := sanitizeStoredTLSRecord(&rows[i]); err != nil {
			return false, nil, err
		}
		if err := tx.Save(&rows[i]).Error; err != nil {
			return false, nil, err
		}
		changedAny = true
	}
	return changedAny, ids, nil
}

func refreshMihomoTLSBindingsTx(tx *gorm.DB, record *model.CertificateRecord) (bool, []uint, error) {
	rows := make([]model.MihomoTls, 0)
	if err := tx.Where("certificate_record_id = ?", record.Id).Find(&rows).Error; err != nil {
		return false, nil, err
	}

	changedAny := false
	ids := make([]uint, 0, len(rows))
	for i := range rows {
		ids = append(ids, rows[i].Id)
		changed, err := applyCertificateRecordToTLSRaw(&rows[i].Server, &rows[i].Client, record)
		if err != nil {
			return false, nil, err
		}
		if !changed {
			continue
		}
		rows[i].CertificateRecordID = record.Id
		rows[i].Sanitize()
		if err := tx.Save(&rows[i]).Error; err != nil {
			return false, nil, err
		}
		changedAny = true
	}
	return changedAny, ids, nil
}

func refreshDefaultInboundOutJSONsForTLSIDsTx(tx *gorm.DB, tlsIDs []uint, hostname string) error {
	tlsIDs = compactPositiveUintList(tlsIDs)
	if len(tlsIDs) == 0 {
		return nil
	}
	var inboundIDs []uint
	if err := tx.Model(model.Inbound{}).
		Where("tls_id IN ?", tlsIDs).
		Pluck("id", &inboundIDs).Error; err != nil {
		return err
	}
	if len(inboundIDs) == 0 {
		return nil
	}
	return (&InboundService{}).UpdateOutJsons(tx, inboundIDs, hostname)
}

func refreshMihomoInboundOutJSONsForTLSIDsTx(tx *gorm.DB, tlsIDs []uint, hostname string) error {
	tlsIDs = compactPositiveUintList(tlsIDs)
	if len(tlsIDs) == 0 {
		return nil
	}
	var inboundIDs []uint
	if err := tx.Model(model.MihomoInbound{}).
		Where("tls_id IN ?", tlsIDs).
		Pluck("id", &inboundIDs).Error; err != nil {
		return err
	}
	if len(inboundIDs) == 0 {
		return nil
	}
	return (&MihomoInboundService{}).UpdateOutJsons(tx, inboundIDs, hostname)
}

func compactPositiveUintList(values []uint) []uint {
	if len(values) == 0 {
		return nil
	}
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

func applyCertificateRecordToTLSRaw(serverRaw *json.RawMessage, clientRaw *json.RawMessage, record *model.CertificateRecord) (bool, error) {
	if serverRaw == nil || clientRaw == nil || record == nil {
		return false, nil
	}

	serverTLS, err := decodeTLSBindingRaw(*serverRaw)
	if err != nil {
		return false, err
	}
	clientTLS, err := decodeTLSBindingRaw(*clientRaw)
	if err != nil {
		return false, err
	}

	beforeServer := append([]byte(nil), (*serverRaw)...)
	beforeClient := append([]byte(nil), (*clientRaw)...)

	applyCertificateMaterialToServerTLS(serverTLS, record)
	refreshClientDerivedCertificateValues(clientTLS, record)

	nextServer, err := json.MarshalIndent(serverTLS, "", "  ")
	if err != nil {
		return false, err
	}
	nextClient, err := json.MarshalIndent(clientTLS, "", "  ")
	if err != nil {
		return false, err
	}

	*serverRaw = json.RawMessage(nextServer)
	*clientRaw = json.RawMessage(nextClient)
	return !bytes.Equal(bytes.TrimSpace(beforeServer), bytes.TrimSpace(nextServer)) ||
		!bytes.Equal(bytes.TrimSpace(beforeClient), bytes.TrimSpace(nextClient)), nil
}

func decodeTLSBindingRaw(raw json.RawMessage) (map[string]interface{}, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return map[string]interface{}{}, nil
	}
	result := map[string]interface{}{}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	if result == nil {
		result = map[string]interface{}{}
	}
	return result, nil
}

func applyCertificateMaterialToServerTLS(serverTLS map[string]interface{}, record *model.CertificateRecord) {
	if certLines := certificateRecordFullchainLines(record); len(certLines) > 0 {
		serverTLS["certificate"] = certLines
		delete(serverTLS, "certificate_path")
	}
	if keyLines := certificateRecordKeyLines(record); len(keyLines) > 0 {
		serverTLS["key"] = keyLines
		delete(serverTLS, "key_path")
	}
}

func refreshClientDerivedCertificateValues(clientTLS map[string]interface{}, record *model.CertificateRecord) {
	certPEM := certificateRecordFullchainPEM(record)
	if len(certPEM) == 0 {
		return
	}

	if hasNonEmptyManagedTLSHash(clientTLS["certificate_public_key_sha256"]) {
		if sha256Value, ok := calculateManagedSubscriptionTLSPublicKeySHA256(certPEM); ok {
			clientTLS["certificate_public_key_sha256"] = []string{sha256Value}
			delete(clientTLS, "certificate")
			delete(clientTLS, "certificate_path")
		}
	}

	if rawFingerprint, ok := clientTLS["fingerprint"].(string); ok && strings.TrimSpace(rawFingerprint) != "" {
		if fingerprint, ok := calculateManagedSubscriptionTLSFingerprint(certPEM); ok {
			clientTLS["fingerprint"] = fingerprint
		}
	}
}

func certificateRecordFullchainPEM(record *model.CertificateRecord) []byte {
	if record == nil {
		return nil
	}
	if len(bytes.TrimSpace(record.FullchainPEM)) > 0 {
		return append([]byte(nil), record.FullchainPEM...)
	}
	if len(bytes.TrimSpace(record.CertPEM)) > 0 {
		return append([]byte(nil), record.CertPEM...)
	}
	return nil
}

func certificateRecordFullchainLines(record *model.CertificateRecord) []string {
	return splitCertificateRecordPEMLines(certificateRecordFullchainPEM(record))
}

func certificateRecordKeyLines(record *model.CertificateRecord) []string {
	if record == nil {
		return nil
	}
	return splitCertificateRecordPEMLines(record.KeyPEM)
}

func splitCertificateRecordPEMLines(raw []byte) []string {
	text := strings.TrimSpace(strings.ReplaceAll(string(raw), "\r\n", "\n"))
	if text == "" {
		return nil
	}
	parts := strings.Split(text, "\n")
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		line := strings.TrimSpace(strings.TrimSuffix(part, "\r"))
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func ensureCertificateRecordNotUsedByTLS(recordID uint) error {
	usage := collectCertificateTLSUsage(recordID)
	if !usage.inUse() {
		return nil
	}
	label := buildCertificateUsageLabel(false, false, usage, certificateReverseProxyUsage{})
	if label == "" {
		label = "TLS settings"
	}
	return common.NewError("certificate is in use by ", label)
}

func ensureCertificateRecordNotUsedByReverseProxy(recordID uint) error {
	usage := collectCertificateReverseProxyUsage(recordID)
	if !usage.inUse() {
		return nil
	}
	label := buildReverseProxyUsageMarker(usage)
	if label == "" {
		label = "reverse proxy"
	}
	return common.NewError("certificate is in use by ", label)
}

func syncTLSBindingsForCertificateRecordBestEffort(recordID uint, hostname string) {
	if changed, err := SyncTLSBindingsForCertificateRecord(recordID, hostname); err != nil {
		logger.Warning("sync TLS certificate bindings failed: ", err)
	} else if changed {
		logger.Info("synced TLS certificate bindings for certificate record: ", recordID)
	}
}
