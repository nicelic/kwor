package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util/common"

	"gorm.io/gorm"
)

const singboxRuntimeBootstrapDNSTag = "bootstrap-dns"

func singboxRuntimeBootstrapDNSServer() map[string]interface{} {
	return map[string]interface{}{
		"server":      "8.8.8.8",
		"server_port": 53,
		"tag":         singboxRuntimeBootstrapDNSTag,
		"type":        "udp",
	}
}

// DnsServerService owns the database-backed DNS server cards. The effective
// dns.final card and the runtime-only bootstrap DNS are rendered into the
// sing-box runtime configuration.
type DnsServerService struct{}

func (s *DnsServerService) GetAll() (*[]map[string]interface{}, error) {
	servers := make([]model.DnsServer, 0)
	if err := database.GetDB().Model(&model.DnsServer{}).Order("id ASC").Find(&servers).Error; err != nil {
		return nil, err
	}

	result := make([]map[string]interface{}, 0, len(servers))
	for _, server := range servers {
		data, err := server.MarshalFull()
		if err != nil {
			return nil, err
		}
		result = append(result, data)
	}
	return &result, nil
}

func (s *DnsServerService) getSelectedServer(db *gorm.DB, finalTag string) (*model.DnsServer, bool, error) {
	if db == nil {
		return nil, false, common.NewError("database is not initialized")
	}

	server := &model.DnsServer{}
	query := db.Model(&model.DnsServer{}).Order("id ASC")
	if finalTag = strings.TrimSpace(finalTag); finalTag != "" {
		query = query.Where("tag = ?", finalTag)
	}
	err := query.First(server).Error
	if database.IsNotFound(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return server, true, nil
}

func (s *DnsServerService) GetSelectedConfig(db *gorm.DB, finalTag string) (json.RawMessage, bool, error) {
	server, found, err := s.getSelectedServer(db, finalTag)
	if err != nil || !found {
		return nil, found, err
	}

	raw, err := server.MarshalJSON()
	if err != nil {
		return nil, false, err
	}
	return raw, true, nil
}

// NormalizeConfigForStorage removes the old embedded dns.servers list and
// makes every route action use the one effective DNS server. This keeps saved
// rules from referencing a card that is deliberately absent from config.json.
func (s *DnsServerService) NormalizeConfigForStorage(tx *gorm.DB, config json.RawMessage) (json.RawMessage, error) {
	root := map[string]interface{}{}
	if err := json.Unmarshal(config, &root); err != nil {
		return nil, err
	}

	dnsMap, ok := root["dns"].(map[string]interface{})
	if !ok || dnsMap == nil {
		return config, nil
	}
	if legacyServers, exists := dnsMap["servers"].([]interface{}); exists {
		if err := s.importLegacyServers(tx, legacyServers); err != nil {
			return nil, err
		}
	}
	delete(dnsMap, "servers")

	finalTag, _ := dnsMap["final"].(string)
	server, found, err := s.getSelectedServer(tx, finalTag)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(finalTag) != "" && !found {
		return nil, common.NewErrorf("final DNS server %q does not exist", finalTag)
	}

	rules, hasRules := dnsMap["rules"].([]interface{})
	if hasRules && len(rules) > 0 {
		if !found {
			return nil, common.NewError("DNS rules require a saved DNS server selected as final")
		}
		for _, rawRule := range rules {
			normalizeDNSRuleServer(rawRule, server.Tag)
		}
	}

	root["dns"] = dnsMap
	return json.Marshal(root)
}

func normalizeDNSRuleServer(rawRule interface{}, selectedTag string) {
	rule, ok := rawRule.(map[string]interface{})
	if !ok || rule == nil {
		return
	}
	action, _ := rule["action"].(string)
	if action == "route" {
		rule["server"] = selectedTag
	}
	if children, ok := rule["rules"].([]interface{}); ok {
		for _, child := range children {
			normalizeDNSRuleServer(child, selectedTag)
		}
	}
}

// ApplySelectedServerToCoreConfig injects the fixed runtime bootstrap DNS and
// the selected database record into the generated sing-box config. The
// bootstrap server is runtime-only and is never stored as a DNS card.
func (s *DnsServerService) ApplySelectedServerToDNSConfig(db *gorm.DB, dnsConfig *json.RawMessage) error {
	if dnsConfig == nil {
		return nil
	}

	dnsMap := map[string]interface{}{}
	if len(*dnsConfig) > 0 {
		if err := json.Unmarshal(*dnsConfig, &dnsMap); err != nil {
			return err
		}
	}

	finalTag, _ := dnsMap["final"].(string)
	rawServer, found, err := s.GetSelectedConfig(db, finalTag)
	if err != nil {
		return err
	}
	if !found {
		legacyServer, legacyFound := legacySelectedDNSConfig(dnsMap, finalTag)
		if legacyFound {
			rawServer, err = json.Marshal(legacyServer)
			if err != nil {
				return err
			}
			found = true
		} else if strings.TrimSpace(finalTag) != "" {
			return fmt.Errorf("final DNS server %q does not exist", finalTag)
		} else {
			delete(dnsMap, "servers")
		}
	}
	runtimeServers := []interface{}{singboxRuntimeBootstrapDNSServer()}
	if found {
		server := map[string]interface{}{}
		if err := json.Unmarshal(rawServer, &server); err != nil {
			return err
		}
		if tag, _ := server["tag"].(string); strings.TrimSpace(tag) == singboxRuntimeBootstrapDNSTag {
			return fmt.Errorf("DNS server tag %q is reserved for the runtime bootstrap DNS", singboxRuntimeBootstrapDNSTag)
		}
		// Resolve every user DNS through the fixed bootstrap DNS, even when
		// the configured DNS address is already an IP literal.
		server["domain_resolver"] = singboxRuntimeBootstrapDNSTag
		dnsMap["servers"] = []interface{}{server}
		selectedTag, _ := server["tag"].(string)
		runtimeServers = append(runtimeServers, server)
		if rules, ok := dnsMap["rules"].([]interface{}); ok {
			for _, rawRule := range rules {
				normalizeDNSRuleServer(rawRule, selectedTag)
			}
		}
	}
	dnsMap["servers"] = runtimeServers

	rendered, err := json.Marshal(dnsMap)
	if err != nil {
		return err
	}
	*dnsConfig = rendered
	return nil
}

func (s *DnsServerService) ApplySelectedServerToCoreConfig(db *gorm.DB, config *ProManagerSingBoxConfig) error {
	if config == nil {
		return nil
	}
	if err := s.ApplySelectedServerToDNSConfig(db, &config.Dns); err != nil {
		return err
	}
	return nil
}

func (s *DnsServerService) Save(tx *gorm.DB, action string, data json.RawMessage) error {
	_, err := s.SaveWithChange(tx, action, data, 0)
	return err
}

// SaveWithChange is shared by the focused DNS API and the legacy generic save
// path.  The focused API uses the returned flag to avoid needless runtime
// config generation and revision bumps for an unchanged card.
func (s *DnsServerService) SaveWithChange(tx *gorm.DB, action string, data json.RawMessage, serverID uint) (bool, error) {
	if tx == nil {
		return false, common.NewError("database is not initialized")
	}
	switch action {
	case "new", "edit":
		server := model.DnsServer{}
		if err := server.UnmarshalJSON(data); err != nil {
			return false, err
		}
		if err := validateSingboxDNSServerPayload(&server); err != nil {
			return false, err
		}
		if err := validateSingboxDNSServerReferences(tx, &server); err != nil {
			return false, err
		}

		if action == "new" {
			server.Id = 0
			var count int64
			if err := tx.Model(&model.DnsServer{}).Count(&count).Error; err != nil {
				return false, err
			}
			if count >= SingboxDNSMaxServers {
				return false, common.NewErrorf("DNS server count exceeds %d", SingboxDNSMaxServers)
			}
			if err := ensureSingboxDNSServerOptionsBudget(tx, uint64(len(server.Options)), 0); err != nil {
				return false, err
			}
			count = 0
			if err := tx.Model(&model.DnsServer{}).Where("tag = ?", server.Tag).Count(&count).Error; err != nil {
				return false, err
			}
			if count > 0 {
				return false, common.NewErrorf("DNS server tag %q already exists", server.Tag)
			}
			return true, tx.Create(&server).Error
		}

		if server.Id == 0 {
			return false, common.NewError("DNS server id is required for edit")
		}
		existing := model.DnsServer{}
		if err := tx.Model(&model.DnsServer{}).Where("id = ?", server.Id).First(&existing).Error; err != nil {
			return false, err
		}
		selected, found, err := s.getSelectedServer(tx, configuredDNSFinalTag(tx))
		if err != nil {
			return false, err
		}
		if found && selected.Id == existing.Id && existing.Tag != server.Tag {
			return false, common.NewError("cannot rename the DNS server currently selected as final")
		}
		var count int64
		if err := tx.Model(&model.DnsServer{}).Where("tag = ? AND id <> ?", server.Tag, server.Id).Count(&count).Error; err != nil {
			return false, err
		}
		if count > 0 {
			return false, common.NewErrorf("DNS server tag %q already exists", server.Tag)
		}
		if err := ensureSingboxDNSServerOptionsBudget(tx, uint64(len(server.Options)), existing.Id); err != nil {
			return false, err
		}
		if existing.Type == server.Type && existing.Tag == server.Tag && sameDNSOptions(existing.Options, server.Options) {
			return false, nil
		}
		return true, tx.Save(&server).Error

	case "del":
		var id uint
		if serverID > 0 {
			id = serverID
		} else if err := json.Unmarshal(data, &id); err != nil {
			return false, err
		}
		if id == 0 {
			return false, common.NewError("DNS server id is required for delete")
		}
		existing := model.DnsServer{}
		if err := tx.Model(&model.DnsServer{}).Where("id = ?", id).First(&existing).Error; err != nil {
			return false, err
		}
		selected, found, err := s.getSelectedServer(tx, configuredDNSFinalTag(tx))
		if err != nil {
			return false, err
		}
		if found && selected.Id == existing.Id {
			return false, common.NewError("cannot delete the DNS server currently selected as final")
		}
		if err := validateSingboxDNSServerRemovalReferences(tx, existing.Tag); err != nil {
			return false, err
		}
		return true, tx.Delete(&existing).Error
	default:
		return false, common.NewErrorf("unknown action: %s", action)
	}
}

// validateSingboxDNSServerReferences checks the database-backed targets that
// cannot be validated from a standalone DNS card payload.  Without this
// second pass a card could be saved successfully and fail only when the
// generated sing-box config was next written.
func validateSingboxDNSServerReferences(tx *gorm.DB, server *model.DnsServer) error {
	if tx == nil {
		return common.NewError("DNS server reference validation requires a database transaction")
	}
	if server == nil {
		return common.NewError("DNS server is required")
	}

	options := map[string]interface{}{}
	if len(server.Options) > 0 && string(server.Options) != "null" {
		if err := json.Unmarshal(server.Options, &options); err != nil {
			return err
		}
	}
	if options == nil {
		options = map[string]interface{}{}
	}

	if rawDetour, exists := options["detour"]; exists && rawDetour != nil {
		detour, ok := rawDetour.(string)
		if !ok {
			return common.NewError("DNS server detour must be a string")
		}
		detour = strings.TrimSpace(detour)
		if detour != "" {
			targets, err := loadSingboxRouteOutboundTags(tx)
			if err != nil {
				return err
			}
			known := make(map[string]struct{}, len(targets))
			for _, target := range targets {
				known[target] = struct{}{}
			}
			if _, ok := known[detour]; !ok {
				return common.NewErrorf("DNS server references unknown detour %q", detour)
			}
		}
	}

	if server.Type == "tailscale" {
		endpoint, ok := options["endpoint"].(string)
		endpoint = strings.TrimSpace(endpoint)
		if !ok || endpoint == "" {
			return common.NewError("tailscale DNS server requires an endpoint")
		}
		var count int64
		if err := tx.Model(&model.Endpoint{}).Where("type = ? AND tag = ?", "tailscale", endpoint).Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return common.NewErrorf("tailscale DNS server references unknown endpoint %q", endpoint)
		}
	}

	if server.Type == "resolved" {
		service, ok := options["service"].(string)
		service = strings.TrimSpace(service)
		if !ok || service == "" {
			return common.NewError("resolved DNS server requires a service")
		}
		var count int64
		if err := tx.Model(&model.Service{}).Where("type = ? AND tag = ?", "resolved", service).Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return common.NewErrorf("resolved DNS server references unknown service %q", service)
		}
	}

	return nil
}

func ensureSingboxDNSServerOptionsBudget(tx *gorm.DB, candidateBytes uint64, replaceID uint) error {
	servers := make([]model.DnsServer, 0)
	if err := tx.Select("id", "options").Find(&servers).Error; err != nil {
		return err
	}
	total := uint64(0)
	for _, server := range servers {
		if replaceID > 0 && server.Id == replaceID {
			continue
		}
		total += uint64(len(server.Options))
	}
	if total+candidateBytes > SingboxDNSMaxServerOptionsTotal {
		return common.NewErrorf("total DNS server options exceed %d bytes", SingboxDNSMaxServerOptionsTotal)
	}
	return nil
}

func sameDNSOptions(left json.RawMessage, right json.RawMessage) bool {
	leftMap := map[string]interface{}{}
	rightMap := map[string]interface{}{}
	if json.Unmarshal(left, &leftMap) != nil || json.Unmarshal(right, &rightMap) != nil {
		return string(left) == string(right)
	}
	leftCanonical, leftErr := json.Marshal(leftMap)
	rightCanonical, rightErr := json.Marshal(rightMap)
	return leftErr == nil && rightErr == nil && string(leftCanonical) == string(rightCanonical)
}

func configuredDNSFinalTag(tx *gorm.DB) string {
	if tx == nil {
		return ""
	}
	setting := model.Setting{}
	if err := tx.Model(&model.Setting{}).Where("key = ?", "config").First(&setting).Error; err != nil {
		return ""
	}
	root := map[string]interface{}{}
	if err := json.Unmarshal([]byte(setting.Value), &root); err != nil {
		return ""
	}
	dnsMap, _ := root["dns"].(map[string]interface{})
	finalTag, _ := dnsMap["final"].(string)
	return strings.TrimSpace(finalTag)
}

func (s *DnsServerService) importLegacyServers(tx *gorm.DB, legacyServers []interface{}) error {
	parsedServers := make([]model.DnsServer, 0, len(legacyServers))
	for _, rawServer := range legacyServers {
		raw, err := json.Marshal(rawServer)
		if err != nil {
			return err
		}
		server := model.DnsServer{}
		if err := server.UnmarshalJSON(raw); err != nil {
			return err
		}
		server.Tag = strings.TrimSpace(server.Tag)
		server.Type = strings.TrimSpace(server.Type)
		if err := validateSingboxDNSServerPayload(&server); err != nil {
			return common.NewErrorf("legacy DNS server is invalid: %v", err)
		}
		parsedServers = append(parsedServers, server)
	}

	// Insert the complete candidate set before validating cross-card detours.
	// Older configs can have one legacy DNS card bootstrap through another card
	// that appears later in the same servers array.
	for _, server := range parsedServers {

		var existing model.DnsServer
		err := tx.Model(&model.DnsServer{}).Where("tag = ?", server.Tag).First(&existing).Error
		if database.IsNotFound(err) {
			var count int64
			if err := tx.Model(&model.DnsServer{}).Count(&count).Error; err != nil {
				return err
			}
			if count >= SingboxDNSMaxServers {
				return common.NewErrorf("DNS server count exceeds %d", SingboxDNSMaxServers)
			}
			if err := ensureSingboxDNSServerOptionsBudget(tx, uint64(len(server.Options)), 0); err != nil {
				return err
			}
			server.Id = 0
			if err := tx.Create(&server).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}

	for _, server := range parsedServers {
		if err := validateSingboxDNSServerReferences(tx, &server); err != nil {
			return common.NewErrorf("legacy DNS server references are invalid: %v", err)
		}
	}
	return nil
}

func legacySelectedDNSConfig(dnsMap map[string]interface{}, finalTag string) (map[string]interface{}, bool) {
	servers, ok := dnsMap["servers"].([]interface{})
	if !ok {
		return nil, false
	}
	finalTag = strings.TrimSpace(finalTag)
	for _, rawServer := range servers {
		server, ok := rawServer.(map[string]interface{})
		if !ok || server == nil {
			continue
		}
		tag, _ := server["tag"].(string)
		if tag == "" || (finalTag != "" && tag != finalTag) {
			continue
		}
		return server, true
	}
	return nil, false
}

// MigrateLegacySingboxDNSServers moves old config.dns.servers entries into
// dns_servers. The migration is idempotent and leaves existing table rows as
// the source of truth if a partially migrated database is encountered.
func MigrateLegacySingboxDNSServers() (bool, error) {
	db := database.GetDB()
	if db == nil {
		return false, common.NewError("database is not initialized")
	}

	migrated := false
	err := db.Transaction(func(tx *gorm.DB) error {
		setting := model.Setting{}
		if err := tx.Model(&model.Setting{}).Where("key = ?", "config").First(&setting).Error; err != nil {
			if database.IsNotFound(err) {
				return nil
			}
			return err
		}

		root := map[string]interface{}{}
		if err := json.Unmarshal([]byte(setting.Value), &root); err != nil {
			return err
		}
		dnsMap, ok := root["dns"].(map[string]interface{})
		if !ok || dnsMap == nil {
			return nil
		}
		legacyServers, exists := dnsMap["servers"].([]interface{})
		if !exists {
			return nil
		}

		firstTag := ""
		if firstServer, found := legacySelectedDNSConfig(map[string]interface{}{"servers": legacyServers}, ""); found {
			firstTag, _ = firstServer["tag"].(string)
		}
		if err := (&DnsServerService{}).importLegacyServers(tx, legacyServers); err != nil {
			return err
		}

		if finalTag, _ := dnsMap["final"].(string); strings.TrimSpace(finalTag) == "" && firstTag != "" {
			dnsMap["final"] = firstTag
		}
		delete(dnsMap, "servers")
		root["dns"] = dnsMap

		normalized, err := json.MarshalIndent(root, "", "  ")
		if err != nil {
			return err
		}
		if err := tx.Model(&model.Setting{}).Where("id = ?", setting.Id).Update("value", string(normalized)).Error; err != nil {
			return err
		}
		currentRevision, err := ensureSingboxConfigRevisionState(tx)
		if err != nil {
			return err
		}
		if _, err := bumpSingboxConfigRevision(tx, currentRevision); err != nil {
			return err
		}
		migrated = true
		return nil
	})
	if err == nil && migrated {
		markLastUpdate(time.Now().Unix())
	}
	return migrated, err
}
