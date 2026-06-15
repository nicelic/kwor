package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/util/common"
	"gorm.io/gorm"
)

const (
	firewallEnabledKey    = "firewallEnabled"
	firewallLastSyncAtKey = "firewallLastSyncAt"

	firewallDirectionIngress = "ingress"

	firewallFamilyDual = "dual"
	firewallFamilyIPv4 = "ipv4"
	firewallFamilyIPv6 = "ipv6"

	firewallProtocolTCP    = "tcp"
	firewallProtocolUDP    = "udp"
	firewallProtocolTCPUDP = "tcp_udp"
	firewallProtocolAny    = "any"
	firewallProtocolICMP   = "icmp"
	firewallProtocolICMPv4 = "icmp_v4"
	firewallProtocolICMPv6 = "icmp_v6"

	firewallOriginSystem    = "system"
	firewallOriginManual    = "manual"
	firewallOriginTemporary = "temporary"
	firewallOriginExternal  = "external"

	firewallSystemSSH   = "ssh"
	firewallSystemPanel = "panel"
	firewallSystemSub   = "sub"

	firewallInputChain = "panel_input"
)

var (
	firewallNftTable = loadFirewallNftTableName()

	firewallRuntimeCounterRe = regexp.MustCompile(`\bcounter\s+packets\s+\d+\s+bytes\s+\d+\b`)
	firewallRuntimeHandleRe  = regexp.MustCompile(`#\s*handle\s+\d+\b`)
	firewallRuntimeSpaceRe   = regexp.MustCompile(`\s+`)

	firewallStateMu sync.Mutex
	firewallState   = struct {
		lastRenderHash  string
		lastRuntimeHash string
		lastReconcile   time.Time
	}{}
)

type FirewallService struct {
	SettingService
}

type FirewallRulePayload struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Family      string `json:"family"`
	Protocol    string `json:"protocol"`
	PortSpec    string `json:"portSpec"`
	SourceSpec  string `json:"sourceSpec"`
}

type FirewallRuleView struct {
	model.FirewallRule
	CanEdit       bool                      `json:"canEdit"`
	CanDelete     bool                      `json:"canDelete"`
	ListenerState FirewallRuleListenerState `json:"listenerState"`
}

type FirewallDefaultPorts struct {
	SSH           []int `json:"ssh"`
	Panel         []int `json:"panel"`
	Sub           []int `json:"sub"`
	All           []int `json:"all"`
	Active        []int `json:"active"`
	SSHReserved   bool  `json:"sshReserved"`
	PanelReserved bool  `json:"panelReserved"`
	SubReserved   bool  `json:"subReserved"`
}

type FirewallOverview struct {
	Enabled                  bool                    `json:"enabled"`
	Available                bool                    `json:"available"`
	Mode                     string                  `json:"mode"`
	Nftables                 FirewallNftablesStatus  `json:"nftables"`
	LastSyncAt               int64                   `json:"lastSyncAt"`
	DefaultPorts             FirewallDefaultPorts    `json:"defaultPorts"`
	SSHConfig                FirewallSSHConfigStatus `json:"sshConfig"`
	ManualCount              int                     `json:"manualCount"`
	TemporaryCount           int                     `json:"temporaryCount"`
	ExternalCount            int                     `json:"externalCount"`
	SystemCount              int                     `json:"systemCount"`
	TotalCount               int                     `json:"totalCount"`
	Rules                    []FirewallRuleView      `json:"rules"`
	GeoRuleCount             int                     `json:"geoRuleCount"`
	GeoUpdateIntervalMinutes int                     `json:"geoUpdateIntervalMinutes"`
	GeoLastRefreshAt         int64                   `json:"geoLastRefreshAt"`
	GeoRules                 []FirewallGeoRuleView   `json:"geoRules"`
	Error                    string                  `json:"error,omitempty"`
}

type firewallObservedRule struct {
	Family      string
	TableFamily string
	Table       string
	Chain       string
	Handle      int
	Protocol    string
	PortSpec    string
	SourceSpec  string
	Comment     string
	Description string
	ObservedAt  int64
}

type firewallRenderTarget struct {
	family  string
	sources []string
}

func firewallProtocolIsICMP(protocol string) bool {
	switch strings.TrimSpace(protocol) {
	case firewallProtocolICMP, firewallProtocolICMPv4, firewallProtocolICMPv6:
		return true
	default:
		return false
	}
}

func firewallProtocolNeedsPort(protocol string) bool {
	return !firewallProtocolIsICMP(protocol) && strings.TrimSpace(protocol) != firewallProtocolAny
}

func firewallProtocolNeedsSource(protocol string) bool {
	return !firewallProtocolIsICMP(protocol)
}

func normalizeFirewallRuleFamily(raw string, protocol string) string {
	switch strings.TrimSpace(protocol) {
	case firewallProtocolICMP:
		return firewallFamilyDual
	case firewallProtocolICMPv4:
		return firewallFamilyIPv4
	case firewallProtocolICMPv6:
		return firewallFamilyIPv6
	default:
		return normalizeFirewallFamily(raw)
	}
}

func loadFirewallNftTableName() string {
	const fallback = "kwor_firewall"
	raw := strings.TrimSpace(os.Getenv("KWOR_FIREWALL_NFT_TABLE"))
	if raw == "" {
		return fallback
	}
	valid := regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_-]{0,31}$`)
	if !valid.MatchString(raw) {
		return fallback
	}
	return raw
}

func firewallSupported() bool {
	if !nftSupported() {
		return false
	}
	_, err := runNft("list", "tables")
	return err == nil
}

var firewallSupportedFn = firewallSupported

func firewallTableExists() bool {
	if !firewallSupported() {
		return false
	}
	_, err := runNft("list", "table", nftFamily, firewallNftTable)
	return err == nil
}

func firewallRuleComment(ruleID uint, family string) string {
	return fmt.Sprintf("kwor_firewall_rule_%d_%s", ruleID, family)
}

func firewallStaticComment(name string) string {
	return "kwor_firewall_static_" + strings.TrimSpace(name)
}

func (s *FirewallService) GetOverview() (*FirewallOverview, error) {
	firewallStateMu.Lock()
	defer firewallStateMu.Unlock()

	enabled, err := s.getFirewallEnabledLocked()
	if err != nil {
		return nil, err
	}
	available := firewallSupportedFn()
	nftStatus := buildFirewallNftablesStatus(available)
	if enabled && available {
		if syncErr := s.reconcileLocked(2 * time.Second); syncErr != nil {
			return nil, syncErr
		}
	}

	defaults := resolveFirewallDefaultPorts()
	if !enabled || !available {
		if syncErr := upsertFirewallSystemRulesLocked(database.GetDB(), defaults); syncErr != nil {
			return nil, syncErr
		}
	}
	lastSyncAt, _ := s.getFirewallLastSyncAtLocked()
	rows, err := loadFirewallRulesLocked()
	if err != nil {
		return nil, err
	}
	defaults = applyFirewallDefaultReservationStatus(defaults, rows)
	listenerStates := buildFirewallRuleListenerStates(rows)
	views := make([]FirewallRuleView, 0, len(rows))
	manualCount := 0
	temporaryCount := 0
	externalCount := 0
	systemCount := 0
	for _, row := range rows {
		if row.Origin == firewallOriginSystem && !row.Enabled {
			continue
		}
		switch row.Origin {
		case firewallOriginSystem:
			systemCount++
		case firewallOriginTemporary:
			temporaryCount++
		case firewallOriginExternal:
			externalCount++
		default:
			manualCount++
		}
		views = append(views, FirewallRuleView{
			FirewallRule:  row,
			CanEdit:       firewallRuleCanEdit(row),
			CanDelete:     firewallRuleCanDelete(row),
			ListenerState: listenerStates[row.Id],
		})
	}
	geoRows, err := loadFirewallGeoRulesLocked()
	if err != nil {
		return nil, err
	}
	geoViews := make([]FirewallGeoRuleView, 0, len(geoRows))
	for _, row := range geoRows {
		geoViews = append(geoViews, buildFirewallGeoRuleView(row))
	}
	geoUpdateIntervalMinutes, _ := s.getFirewallGeoUpdateIntervalMinutesLocked()
	geoLastRefreshAt, _ := s.getFirewallGeoLastRefreshAtLocked()
	sshConfig := resolveFirewallSSHConfigStatus()

	overview := &FirewallOverview{
		Enabled:                  enabled,
		Available:                available,
		Mode:                     "nftables",
		Nftables:                 nftStatus,
		LastSyncAt:               lastSyncAt,
		DefaultPorts:             defaults,
		SSHConfig:                sshConfig,
		ManualCount:              manualCount,
		TemporaryCount:           temporaryCount,
		ExternalCount:            externalCount,
		SystemCount:              systemCount,
		TotalCount:               len(views),
		Rules:                    views,
		GeoRuleCount:             len(geoViews),
		GeoUpdateIntervalMinutes: geoUpdateIntervalMinutes,
		GeoLastRefreshAt:         geoLastRefreshAt,
		GeoRules:                 geoViews,
	}
	overview.Error = buildFirewallNftablesOverviewError(nftStatus)
	return overview, nil
}

func (s *FirewallService) SetEnabled(enabled bool) error {
	firewallStateMu.Lock()
	defer firewallStateMu.Unlock()

	if enabled {
		if !firewallSupportedFn() {
			return common.NewError("nftables firewall is unavailable on this host")
		}
		return s.enableLocked()
	}
	return s.disableLocked()
}

func (s *FirewallService) UpsertRule(payload FirewallRulePayload) error {
	firewallStateMu.Lock()
	defer firewallStateMu.Unlock()

	enabled, err := s.getFirewallEnabledLocked()
	if err != nil {
		return err
	}
	if !enabled {
		return common.NewError("firewall is disabled")
	}

	db := database.GetDB()
	var row model.FirewallRule
	if payload.ID > 0 {
		if err := db.Where("id = ?", payload.ID).First(&row).Error; err != nil {
			return err
		}
		if !firewallRuleCanEdit(row) {
			return common.NewError("system reserved firewall rules cannot be edited")
		}
		if row.Origin == firewallOriginExternal {
			if err := deleteObservedFirewallRule(row); err != nil && !firewallNftObjectMissing(err) {
				return common.NewError("failed to delete external firewall rule: ", err)
			}
		}
	} else {
		row = model.FirewallRule{
			Enabled:   true,
			Origin:    firewallOriginManual,
			Direction: firewallDirectionIngress,
		}
	}

	name := strings.TrimSpace(payload.Name)
	if name == "" {
		name = "自定义规则"
	}
	protocol := normalizeFirewallProtocol(payload.Protocol)
	if protocol == firewallProtocolAny {
		return common.NewError("ANY protocol is no longer supported; choose TCP/UDP/TCP+UDP/ICMP")
	}
	family := normalizeFirewallRuleFamily(payload.Family, protocol)
	portSpec, err := normalizeFirewallPortSpec(payload.PortSpec, protocol)
	if err != nil {
		return err
	}
	sourceSpec := ""
	if firewallProtocolNeedsSource(protocol) {
		sourceSpec, err = normalizeFirewallSourceSpec(payload.SourceSpec, family)
		if err != nil {
			return err
		}
	}

	row.Name = name
	row.Description = strings.TrimSpace(payload.Description)
	row.Enabled = true
	row.Origin = firewallOriginManual
	row.SystemKey = ""
	row.Direction = firewallDirectionIngress
	row.Family = family
	row.Protocol = protocol
	row.PortSpec = portSpec
	row.SourceSpec = sourceSpec
	row.ObservedFamily = ""
	row.ObservedTable = ""
	row.ObservedChain = ""
	row.ObservedHandle = 0
	row.ObservedComment = ""
	row.LastSeenAt = time.Now().Unix()

	if payload.ID > 0 {
		if err := db.Save(&row).Error; err != nil {
			return err
		}
	} else {
		if err := db.Create(&row).Error; err != nil {
			return err
		}
	}

	if err := s.reconcileLocked(0); err != nil {
		return err
	}
	return nil
}

func (s *FirewallService) DeleteRule(id uint) error {
	firewallStateMu.Lock()
	defer firewallStateMu.Unlock()

	db := database.GetDB()
	var row model.FirewallRule
	if err := db.Where("id = ?", id).First(&row).Error; err != nil {
		return err
	}
	if row.Origin == firewallOriginTemporary {
		return common.NewError("temporary firewall rules cannot be deleted manually")
	}
	if row.Origin == firewallOriginSystem {
		if !firewallRuleCanDelete(row) {
			return common.NewError("system reserved firewall rules cannot be deleted")
		}
		if row.Enabled {
			row.Enabled = false
			row.LastSeenAt = time.Now().Unix()
			if err := db.Save(&row).Error; err != nil {
				return err
			}
		}
		enabled, err := s.getFirewallEnabledLocked()
		if err != nil {
			return err
		}
		if enabled {
			if err := s.reconcileLocked(0); err != nil {
				return err
			}
		}
		return nil
	}

	enabled, err := s.getFirewallEnabledLocked()
	if err != nil {
		return err
	}
	if !enabled {
		return common.NewError("firewall is disabled")
	}
	if row.Origin == firewallOriginExternal {
		if err := deleteObservedFirewallRule(row); err != nil && !firewallNftObjectMissing(err) {
			return common.NewError("failed to delete external firewall rule: ", err)
		}
	}
	if err := db.Delete(&row).Error; err != nil {
		return err
	}
	if err := s.reconcileLocked(0); err != nil {
		return err
	}
	return nil
}

func (s *FirewallService) SetSystemRuleReserved(systemKey string, enabled bool) error {
	firewallStateMu.Lock()
	defer firewallStateMu.Unlock()

	normalizedKey := strings.TrimSpace(strings.ToLower(systemKey))
	switch normalizedKey {
	case firewallSystemSSH, firewallSystemSub:
	case firewallSystemPanel:
		if !enabled {
			return common.NewError("panel reserved firewall rule cannot be deleted")
		}
	default:
		return common.NewError("unknown system firewall rule: ", systemKey)
	}

	db := database.GetDB()
	defaults := resolveFirewallDefaultPorts()
	if err := upsertFirewallSystemRulesLocked(db, defaults); err != nil {
		return err
	}

	var row model.FirewallRule
	if err := db.Where("origin = ? AND system_key = ?", firewallOriginSystem, normalizedKey).First(&row).Error; err != nil {
		return err
	}

	if row.Enabled != enabled {
		row.Enabled = enabled
		row.LastSeenAt = time.Now().Unix()
		if err := db.Save(&row).Error; err != nil {
			return err
		}
	}

	firewallEnabled, err := s.getFirewallEnabledLocked()
	if err != nil {
		return err
	}
	if firewallEnabled {
		if err := s.reconcileLocked(0); err != nil {
			return err
		}
	}
	return nil
}

func (s *FirewallService) SyncIfNeeded(minGap time.Duration) error {
	firewallStateMu.Lock()
	defer firewallStateMu.Unlock()
	return s.reconcileLocked(minGap)
}

func (s *FirewallService) CleanupTemporaryRulesOnStartup() error {
	firewallStateMu.Lock()
	defer firewallStateMu.Unlock()
	return cleanupExpiredTemporaryFirewallRulesLocked(time.Now().Unix(), true)
}

func (s *FirewallService) CleanupOnShutdown() {
	firewallStateMu.Lock()
	defer firewallStateMu.Unlock()

	if runtime.GOOS == "linux" && firewallSupportedFn() {
		if err := cleanupManagedFirewallTable(); err != nil && !firewallNftObjectMissing(err) {
			logger.Warning("failed to cleanup managed firewall nft table on shutdown: ", err)
		}
	}

	firewallState.lastRenderHash = ""
	firewallState.lastRuntimeHash = ""
	firewallState.lastReconcile = time.Time{}
	firewallGeoState.loaded = make(map[uint]firewallGeoResolvedPrefixes)
}

func (s *FirewallService) enableLocked() error {
	db := database.GetDB()

	defaults := resolveFirewallDefaultPorts()
	if err := upsertFirewallSystemRulesLocked(db, defaults); err != nil {
		return err
	}
	observed, err := scanExternalFirewallRules()
	if err != nil {
		return err
	}
	if err := syncExternalFirewallRulesLocked(db, observed); err != nil {
		return err
	}
	if err := s.renderLocked(true); err != nil {
		return err
	}
	now := time.Now()
	if err := s.setFirewallEnabledLocked(true); err != nil {
		return err
	}
	if err := s.setFirewallLastSyncAtLocked(now.Unix()); err != nil {
		return err
	}
	firewallState.lastReconcile = now
	return nil
}

func (s *FirewallService) disableLocked() error {
	if err := cleanupManagedFirewallTable(); err != nil {
		return err
	}
	if err := s.setFirewallEnabledLocked(false); err != nil {
		return err
	}
	if err := s.setFirewallLastSyncAtLocked(0); err != nil {
		return err
	}
	firewallState.lastRenderHash = ""
	firewallState.lastRuntimeHash = ""
	firewallState.lastReconcile = time.Time{}
	firewallGeoState.loaded = make(map[uint]firewallGeoResolvedPrefixes)
	return nil
}

func (s *FirewallService) reconcileLocked(minGap time.Duration) error {
	enabled, err := s.getFirewallEnabledLocked()
	if err != nil {
		return err
	}

	db := database.GetDB()
	if err := cleanupExpiredTemporaryFirewallRulesLocked(time.Now().Unix(), false); err != nil {
		return err
	}
	defaults := resolveFirewallDefaultPorts()
	if err := upsertFirewallSystemRulesLocked(db, defaults); err != nil {
		return err
	}

	if !enabled {
		if err := s.syncFirewallGeoCacheLocked(false); err != nil {
			return err
		}
		if firewallTableExists() {
			if err := cleanupManagedFirewallTable(); err != nil {
				return err
			}
		}
		firewallState.lastRenderHash = ""
		firewallState.lastRuntimeHash = ""
		firewallGeoState.loaded = make(map[uint]firewallGeoResolvedPrefixes)
		return nil
	}
	if !firewallSupportedFn() {
		return common.NewError("nftables firewall is unavailable on this host")
	}

	now := time.Now()
	if minGap > 0 && !firewallState.lastReconcile.IsZero() && now.Sub(firewallState.lastReconcile) < minGap {
		return nil
	}

	observed, err := scanExternalFirewallRules()
	if err != nil {
		return err
	}
	if err := syncExternalFirewallRulesLocked(db, observed); err != nil {
		return err
	}
	if err := s.renderLocked(false); err != nil {
		return err
	}
	if err := s.setFirewallLastSyncAtLocked(now.Unix()); err != nil {
		return err
	}
	firewallState.lastReconcile = now
	return nil
}

func (s *FirewallService) renderLocked(force bool) error {
	rows, err := loadFirewallRulesLocked()
	if err != nil {
		return err
	}
	geoRows, err := s.prepareFirewallGeoRulesLocked(false)
	if err != nil {
		return err
	}
	managedRows := filterFirewallRulesForRender(rows)
	hash := computeFirewallRenderHash(managedRows, geoRows)
	tableExists := firewallTableExists()
	runtimeHashBeforeApply := ""
	if tableExists {
		currentRuntimeHash, hashErr := computeManagedFirewallRuntimeHash()
		if hashErr != nil {
			logger.Warning("failed to read managed firewall runtime hash: ", hashErr)
		} else {
			runtimeHashBeforeApply = currentRuntimeHash
		}
	}
	if !force && hash == firewallState.lastRenderHash && tableExists && runtimeHashBeforeApply != "" && runtimeHashBeforeApply == firewallState.lastRuntimeHash {
		return nil
	}

	previousRenderHash := firewallState.lastRenderHash
	previousRuntimeHash := firewallState.lastRuntimeHash

	// Apply the whole managed table in a single nft batch so external-rule
	// observation and rule refreshes do not open a transient allow window.
	script, err := buildManagedFirewallScript(managedRows, geoRows, tableExists)
	if err != nil {
		return err
	}
	if _, err := runNftScript(script); err != nil {
		return err
	}
	firewallState.lastRenderHash = hash

	runtimeHashAfterApply, hashErr := computeManagedFirewallRuntimeHash()
	if hashErr != nil {
		logger.Warning("failed to refresh managed firewall runtime hash: ", hashErr)
		firewallState.lastRuntimeHash = ""
	} else {
		firewallState.lastRuntimeHash = runtimeHashAfterApply
	}

	shouldFlushConntrack := force || hash != previousRenderHash ||
		(runtimeHashBeforeApply != "" && previousRuntimeHash != "" && runtimeHashBeforeApply != previousRuntimeHash)
	if shouldFlushConntrack {
		if err := flushConntrackTable(); err != nil {
			logger.Warning("failed to flush conntrack after firewall reconcile: ", err)
		}
	}
	return nil
}

func loadFirewallRulesLocked() ([]model.FirewallRule, error) {
	db := database.GetDB()
	rows := make([]model.FirewallRule, 0)
	if err := db.Order("id asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	sort.SliceStable(rows, func(i, j int) bool {
		left := firewallRuleSortKey(rows[i])
		right := firewallRuleSortKey(rows[j])
		if left != right {
			return left < right
		}
		return rows[i].Id < rows[j].Id
	})
	return rows, nil
}

func firewallRuleSortKey(row model.FirewallRule) int {
	switch row.Origin {
	case firewallOriginSystem:
		switch row.SystemKey {
		case firewallSystemSSH:
			return 0
		case firewallSystemPanel:
			return 1
		case firewallSystemSub:
			return 2
		default:
			return 3
		}
	case firewallOriginManual:
		return 10
	case firewallOriginTemporary:
		return 15
	case firewallOriginExternal:
		return 20
	default:
		return 30
	}
}

func firewallRuleParticipatesInManagedChain(row model.FirewallRule) bool {
	switch row.Origin {
	case firewallOriginSystem, firewallOriginManual, firewallOriginTemporary:
		return true
	default:
		return false
	}
}

func firewallRuleCanEdit(row model.FirewallRule) bool {
	return row.Origin != firewallOriginSystem && row.Origin != firewallOriginTemporary
}

func firewallRuleCanDelete(row model.FirewallRule) bool {
	if row.Origin == firewallOriginTemporary {
		return false
	}
	if row.Origin != firewallOriginSystem {
		return true
	}
	switch row.SystemKey {
	case firewallSystemSSH, firewallSystemSub:
		return true
	default:
		return false
	}
}

func filterFirewallRulesForRender(rows []model.FirewallRule) []model.FirewallRule {
	filtered := make([]model.FirewallRule, 0, len(rows))
	for _, row := range rows {
		if !firewallRuleParticipatesInManagedChain(row) {
			continue
		}
		if strings.TrimSpace(row.Protocol) == firewallProtocolAny {
			logger.Warning("skip unsupported ANY firewall rule id=", row.Id)
			continue
		}
		filtered = append(filtered, row)
	}
	return filtered
}

func buildManagedFirewallScript(rows []model.FirewallRule, geoRows []model.FirewallGeoRule, tableExists bool) (string, error) {
	script := &strings.Builder{}
	if tableExists {
		script.WriteString(fmt.Sprintf("delete table %s %s\n", nftFamily, firewallNftTable))
	}
	script.WriteString(fmt.Sprintf("add table %s %s\n", nftFamily, firewallNftTable))
	script.WriteString(fmt.Sprintf(
		"add chain %s %s %s { type filter hook input priority -50; policy drop; }\n",
		nftFamily,
		firewallNftTable,
		firewallInputChain,
	))

	for _, args := range managedFirewallStaticRuleSpecs() {
		appendFirewallScriptArgs(script, args)
	}
	if err := appendManagedFirewallGeoRulesScript(script, geoRows); err != nil {
		return "", err
	}
	for _, row := range rows {
		if !row.Enabled {
			continue
		}
		targets, err := buildFirewallRenderTargets(row)
		if err != nil {
			return "", err
		}
		for _, target := range targets {
			args, err := buildManagedFirewallRuleArgs(row, target)
			if err != nil {
				return "", err
			}
			appendFirewallScriptArgs(script, args)
		}
	}
	appendFirewallScriptArgs(script, buildManagedFirewallFinalDropRuleArgs())
	return script.String(), nil
}

func appendFirewallScriptArgs(script *strings.Builder, args []string) {
	script.WriteString(strings.Join(args, " "))
	script.WriteByte('\n')
}

func ensureManagedFirewallBase() error {
	if !firewallSupported() {
		return nil
	}
	if _, err := runNft("add", "table", nftFamily, firewallNftTable); err != nil {
		return err
	}
	_, err := runNft(
		"add", "chain", nftFamily, firewallNftTable, firewallInputChain,
		"{", "type", "filter", "hook", "input", "priority", "-50", ";", "policy", "drop", ";", "}",
	)
	return err
}

func cleanupManagedFirewallTable() error {
	if !firewallSupported() || !firewallTableExists() {
		return nil
	}
	_, err := runNft("delete", "table", nftFamily, firewallNftTable)
	return err
}

func addManagedFirewallStaticRules() error {
	for _, args := range managedFirewallStaticRuleSpecs() {
		if _, err := runNft(args...); err != nil {
			return err
		}
	}
	return nil
}

func managedFirewallStaticRuleSpecs() [][]string {
	return [][]string{
		{
			"add", "rule", nftFamily, firewallNftTable, firewallInputChain,
			"ct", "state", "related",
			"counter", "accept",
			"comment", firewallStaticComment("related"),
		},
		{
			"add", "rule", nftFamily, firewallNftTable, firewallInputChain,
			"ct", "state", "established",
			"ct", "direction", "reply",
			"counter", "accept",
			"comment", firewallStaticComment("established_reply"),
		},
		{
			"add", "rule", nftFamily, firewallNftTable, firewallInputChain,
			"iifname", "lo",
			"counter", "accept",
			"comment", firewallStaticComment("loopback"),
		},
		{
			"add", "rule", nftFamily, firewallNftTable, firewallInputChain,
			"meta", "l4proto", "icmp",
			"icmp", "type", "{", "destination-unreachable", ",", "time-exceeded", ",", "parameter-problem", ",", "echo-reply", "}",
			"counter", "accept",
			"comment", firewallStaticComment("icmp_control"),
		},
		{
			"add", "rule", nftFamily, firewallNftTable, firewallInputChain,
			"meta", "l4proto", "ipv6-icmp",
			"icmpv6", "type", "{", "destination-unreachable", ",", "packet-too-big", ",", "time-exceeded", ",", "parameter-problem", ",", "echo-reply", ",", "nd-neighbor-solicit", ",", "nd-neighbor-advert", ",", "nd-router-solicit", ",", "nd-router-advert", "}",
			"counter", "accept",
			"comment", firewallStaticComment("icmp6_control"),
		},
	}
}

func addManagedFirewallFinalDropRule() error {
	_, err := runNft(buildManagedFirewallFinalDropRuleArgs()...)
	return err
}

func buildManagedFirewallFinalDropRuleArgs() []string {
	return []string{
		"add", "rule", nftFamily, firewallNftTable, firewallInputChain,
		"counter", "drop",
		"comment", firewallStaticComment("default_drop"),
	}
}

func addManagedFirewallRule(row model.FirewallRule) error {
	targets, err := buildFirewallRenderTargets(row)
	if err != nil {
		return err
	}
	for _, target := range targets {
		args, err := buildManagedFirewallRuleArgs(row, target)
		if err != nil {
			return err
		}
		if _, err := runNft(args...); err != nil {
			return err
		}
	}
	return nil
}

func buildManagedFirewallRuleArgs(row model.FirewallRule, target firewallRenderTarget) ([]string, error) {
	args := []string{
		"add", "rule", nftFamily, firewallNftTable, firewallInputChain,
		"meta", "nfproto", mapFirewallTargetFamily(target.family),
	}
	switch row.Protocol {
	case firewallProtocolTCP:
		args = append(args, "meta", "l4proto", "tcp", "th", "dport")
		args = append(args, buildNftPortSetArgs(row.PortSpec)...)
	case firewallProtocolUDP:
		args = append(args, "meta", "l4proto", "udp", "th", "dport")
		args = append(args, buildNftPortSetArgs(row.PortSpec)...)
	case firewallProtocolTCPUDP:
		args = append(args, "meta", "l4proto", "{", "tcp", ",", "udp", "}", "th", "dport")
		args = append(args, buildNftPortSetArgs(row.PortSpec)...)
	case firewallProtocolAny:
		return nil, common.NewError("ANY protocol is not supported for managed firewall rules")
	case firewallProtocolICMP, firewallProtocolICMPv4, firewallProtocolICMPv6:
		switch target.family {
		case firewallFamilyIPv4:
			args = append(args, "meta", "l4proto", "icmp", "icmp", "type", "echo-request")
		case firewallFamilyIPv6:
			args = append(args, "meta", "l4proto", "ipv6-icmp", "icmpv6", "type", "echo-request")
		default:
			return nil, common.NewError("unsupported firewall target family: ", target.family)
		}
	default:
		return nil, common.NewError("unsupported firewall protocol: ", row.Protocol)
	}
	if len(target.sources) > 0 {
		switch target.family {
		case firewallFamilyIPv4:
			args = append(args, "ip", "saddr")
		case firewallFamilyIPv6:
			args = append(args, "ip6", "saddr")
		default:
			return nil, common.NewError("unsupported firewall target family: ", target.family)
		}
		args = append(args, buildNftCIDRSetArgs(target.sources)...)
	}
	args = append(args, "counter", "accept", "comment", firewallRuleComment(row.Id, target.family))
	return args, nil
}

func buildFirewallRenderTargets(row model.FirewallRule) ([]firewallRenderTarget, error) {
	v4Sources, v6Sources, err := splitFirewallSourcesByFamily(row.SourceSpec)
	if err != nil {
		return nil, err
	}
	targets := make([]firewallRenderTarget, 0, 2)
	switch row.Family {
	case firewallFamilyIPv4:
		targets = append(targets, firewallRenderTarget{family: firewallFamilyIPv4, sources: v4Sources})
	case firewallFamilyIPv6:
		targets = append(targets, firewallRenderTarget{family: firewallFamilyIPv6, sources: v6Sources})
	case firewallFamilyDual:
		if len(v4Sources) == 0 && len(v6Sources) == 0 {
			targets = append(targets,
				firewallRenderTarget{family: firewallFamilyIPv4},
				firewallRenderTarget{family: firewallFamilyIPv6},
			)
			return targets, nil
		}
		if len(v4Sources) > 0 {
			targets = append(targets, firewallRenderTarget{family: firewallFamilyIPv4, sources: v4Sources})
		}
		if len(v6Sources) > 0 {
			targets = append(targets, firewallRenderTarget{family: firewallFamilyIPv6, sources: v6Sources})
		}
	default:
		return nil, common.NewError("unsupported firewall family: ", row.Family)
	}
	if len(targets) == 0 {
		return nil, common.NewError("firewall rule has no render targets")
	}
	return targets, nil
}

func mapFirewallTargetFamily(family string) string {
	switch family {
	case firewallFamilyIPv4:
		return "ipv4"
	case firewallFamilyIPv6:
		return "ipv6"
	default:
		return family
	}
}

func computeFirewallRenderHash(rows []model.FirewallRule, geoRows []model.FirewallGeoRule) string {
	raw, _ := json.Marshal(struct {
		Rules    []model.FirewallRule    `json:"rules"`
		GeoRules []model.FirewallGeoRule `json:"geoRules"`
	}{
		Rules:    rows,
		GeoRules: geoRows,
	})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func computeManagedFirewallRuntimeHash() (string, error) {
	out, err := runNft("--handle", "--numeric", "list", "table", nftFamily, firewallNftTable)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(out), "\n")
	normalized := make([]string, 0, len(lines))
	for _, line := range lines {
		current := strings.TrimSpace(line)
		if current == "" {
			continue
		}
		current = firewallRuntimeCounterRe.ReplaceAllString(current, "counter")
		current = firewallRuntimeHandleRe.ReplaceAllString(current, "")
		current = strings.TrimSpace(current)
		if current == "" {
			continue
		}
		current = firewallRuntimeSpaceRe.ReplaceAllString(current, " ")
		normalized = append(normalized, current)
	}
	sum := sha256.Sum256([]byte(strings.Join(normalized, "\n")))
	return hex.EncodeToString(sum[:]), nil
}

func resolveFirewallDefaultPorts() FirewallDefaultPorts {
	settingSvc := &SettingService{}
	panelPort, panelErr := settingSvc.GetPort()
	if panelErr != nil || panelPort <= 0 {
		panelPort = 8888
	}
	subPort, subErr := settingSvc.GetSubPort()
	if subErr != nil || subPort <= 0 {
		subPort = 22780
	}

	panelPorts := normalizePortList([]int{panelPort, loadActiveRuntimePanelPort()})
	subPorts := normalizePortList([]int{subPort, loadActiveRuntimeSubPort()})
	sshPorts := detectSSHPorts()
	all := make([]int, 0, len(sshPorts)+len(panelPorts)+len(subPorts))
	all = append(all, sshPorts...)
	all = append(all, panelPorts...)
	all = append(all, subPorts...)
	return FirewallDefaultPorts{
		SSH:           sshPorts,
		Panel:         panelPorts,
		Sub:           subPorts,
		All:           normalizePortList(all),
		Active:        normalizePortList(all),
		SSHReserved:   true,
		PanelReserved: true,
		SubReserved:   true,
	}
}

func applyFirewallDefaultReservationStatus(defaults FirewallDefaultPorts, rows []model.FirewallRule) FirewallDefaultPorts {
	sshReserved := true
	panelReserved := true
	subReserved := true
	for _, row := range rows {
		if row.Origin != firewallOriginSystem {
			continue
		}
		switch row.SystemKey {
		case firewallSystemSSH:
			sshReserved = row.Enabled
		case firewallSystemPanel:
			panelReserved = row.Enabled
		case firewallSystemSub:
			subReserved = row.Enabled
		}
	}

	active := make([]int, 0, len(defaults.All))
	if sshReserved {
		active = append(active, defaults.SSH...)
	}
	if panelReserved {
		active = append(active, defaults.Panel...)
	}
	if subReserved {
		active = append(active, defaults.Sub...)
	}

	defaults.Active = normalizePortList(active)
	defaults.SSHReserved = sshReserved
	defaults.PanelReserved = panelReserved
	defaults.SubReserved = subReserved
	return defaults
}

func cleanupExpiredTemporaryFirewallRulesLocked(nowUnix int64, includeLegacy bool) error {
	db := database.GetDB()
	if db == nil {
		return nil
	}

	query := db.Where(
		"(origin = ? AND temporary_type = ? AND temporary_expire_at > 0 AND temporary_expire_at <= ?) OR "+
			"(origin = ? AND temporary_type = ?)",
		firewallOriginTemporary,
		"acme",
		nowUnix,
		firewallOriginTemporary,
		"",
	)
	if includeLegacy {
		query = query.Or(
			"origin = ? AND name LIKE ? AND description = ?",
			firewallOriginManual,
			"ACME temporary allow %",
			"Temporary ACME validation rule, auto removed after issue or renew",
		)
	}

	rows := make([]model.FirewallRule, 0)
	if err := query.Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	ids := make([]uint, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.Id)
	}
	return db.Where("id IN ?", ids).Delete(&model.FirewallRule{}).Error
}

func upsertFirewallSystemRulesLocked(db *gorm.DB, defaults FirewallDefaultPorts) error {
	type systemRuleSpec struct {
		key         string
		name        string
		description string
		ports       []int
	}

	specs := []systemRuleSpec{
		{
			key:         firewallSystemSSH,
			name:        "SSH 保留",
			description: "系统自动保留的 SSH 端口放行规则",
			ports:       defaults.SSH,
		},
		{
			key:         firewallSystemPanel,
			name:        "界面保留",
			description: "系统自动保留的面板端口放行规则",
			ports:       defaults.Panel,
		},
		{
			key:         firewallSystemSub,
			name:        "订阅保留",
			description: "系统自动保留的订阅端口放行规则",
			ports:       defaults.Sub,
		},
	}

	now := time.Now().Unix()
	for _, spec := range specs {
		portSpec := portRangesToNft(intsToPortRanges(spec.ports))
		if portSpec == "" {
			switch spec.key {
			case firewallSystemSSH:
				portSpec = "22"
			case firewallSystemPanel:
				portSpec = "8888"
			case firewallSystemSub:
				portSpec = "22780"
			}
		}

		var row model.FirewallRule
		err := db.Where("origin = ? AND system_key = ?", firewallOriginSystem, spec.key).First(&row).Error
		if err == nil {
			changed := false
			if row.Name != spec.name {
				row.Name = spec.name
				changed = true
			}
			if row.Description != spec.description {
				row.Description = spec.description
				changed = true
			}
			if spec.key == firewallSystemPanel && !row.Enabled {
				row.Enabled = true
				changed = true
			}
			if row.Direction != firewallDirectionIngress {
				row.Direction = firewallDirectionIngress
				changed = true
			}
			if row.Family != firewallFamilyDual {
				row.Family = firewallFamilyDual
				changed = true
			}
			if row.Protocol != firewallProtocolTCP {
				row.Protocol = firewallProtocolTCP
				changed = true
			}
			if row.PortSpec != portSpec {
				row.PortSpec = portSpec
				changed = true
			}
			if row.SourceSpec != "" {
				row.SourceSpec = ""
				changed = true
			}
			if changed {
				row.LastSeenAt = now
				if saveErr := db.Save(&row).Error; saveErr != nil {
					return saveErr
				}
			}
			continue
		}
		if !database.IsNotFound(err) {
			return err
		}

		row = model.FirewallRule{
			Name:        spec.name,
			Description: spec.description,
			Enabled:     true,
			Origin:      firewallOriginSystem,
			SystemKey:   spec.key,
			Direction:   firewallDirectionIngress,
			Family:      firewallFamilyDual,
			Protocol:    firewallProtocolTCP,
			PortSpec:    portSpec,
			SourceSpec:  "",
			LastSeenAt:  now,
		}
		if createErr := db.Create(&row).Error; createErr != nil {
			return createErr
		}
	}
	return nil
}

func syncExternalFirewallRulesLocked(db *gorm.DB, observed []firewallObservedRule) error {
	existingRows := make([]model.FirewallRule, 0)
	if err := db.Where("origin = ?", firewallOriginExternal).Find(&existingRows).Error; err != nil {
		return err
	}

	existingByKey := make(map[string]*model.FirewallRule, len(existingRows))
	for index := range existingRows {
		row := &existingRows[index]
		existingByKey[observedRuleKey(row.ObservedFamily, row.ObservedTable, row.ObservedChain, row.ObservedHandle)] = row
	}

	seen := make(map[string]struct{}, len(observed))
	for _, entry := range observed {
		key := observedRuleKey(entry.TableFamily, entry.Table, entry.Chain, entry.Handle)
		seen[key] = struct{}{}
		if existing, ok := existingByKey[key]; ok {
			existing.Name = entry.Description
			existing.Description = entry.Description
			existing.Enabled = true
			existing.Direction = firewallDirectionIngress
			existing.Family = entry.Family
			existing.Protocol = entry.Protocol
			existing.PortSpec = entry.PortSpec
			existing.SourceSpec = entry.SourceSpec
			existing.ObservedComment = entry.Comment
			existing.LastSeenAt = entry.ObservedAt
			if err := db.Save(existing).Error; err != nil {
				return err
			}
			continue
		}

		row := model.FirewallRule{
			Name:            entry.Description,
			Description:     entry.Description,
			Enabled:         true,
			Origin:          firewallOriginExternal,
			Direction:       firewallDirectionIngress,
			Family:          entry.Family,
			Protocol:        entry.Protocol,
			PortSpec:        entry.PortSpec,
			SourceSpec:      entry.SourceSpec,
			ObservedFamily:  entry.TableFamily,
			ObservedTable:   entry.Table,
			ObservedChain:   entry.Chain,
			ObservedHandle:  entry.Handle,
			ObservedComment: entry.Comment,
			LastSeenAt:      entry.ObservedAt,
		}
		if err := db.Create(&row).Error; err != nil {
			return err
		}
	}

	for _, row := range existingRows {
		key := observedRuleKey(row.ObservedFamily, row.ObservedTable, row.ObservedChain, row.ObservedHandle)
		if _, ok := seen[key]; ok {
			continue
		}
		if err := db.Delete(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func observedRuleKey(family string, table string, chain string, handle int) string {
	return strings.Join([]string{
		strings.TrimSpace(family),
		strings.TrimSpace(table),
		strings.TrimSpace(chain),
		strconv.Itoa(handle),
	}, "|")
}

func deleteObservedFirewallRule(row model.FirewallRule) error {
	if !firewallSupported() {
		return nil
	}
	family := strings.TrimSpace(row.ObservedFamily)
	table := strings.TrimSpace(row.ObservedTable)
	chain := strings.TrimSpace(row.ObservedChain)
	if family == "" || table == "" || chain == "" || row.ObservedHandle <= 0 {
		return nil
	}
	_, err := runNft("delete", "rule", family, table, chain, "handle", strconv.Itoa(row.ObservedHandle))
	return err
}

func firewallNftObjectMissing(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "no such file or directory") ||
		strings.Contains(message, "no such file") ||
		strings.Contains(message, "not found")
}

func normalizeFirewallFamily(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case firewallFamilyIPv4:
		return firewallFamilyIPv4
	case firewallFamilyIPv6:
		return firewallFamilyIPv6
	default:
		return firewallFamilyDual
	}
}

func normalizeFirewallProtocol(raw string) string {
	switch strings.TrimSpace(strings.ToLower(strings.ReplaceAll(raw, "-", "_"))) {
	case firewallProtocolTCP:
		return firewallProtocolTCP
	case firewallProtocolUDP:
		return firewallProtocolUDP
	case "tcpudp", "tcp+udp", firewallProtocolTCPUDP:
		return firewallProtocolTCPUDP
	case firewallProtocolAny:
		return firewallProtocolAny
	case firewallProtocolICMP:
		return firewallProtocolICMP
	case firewallProtocolICMPv4, "icmp4", "icmpv4":
		return firewallProtocolICMPv4
	case firewallProtocolICMPv6, "icmp6", "icmpv6":
		return firewallProtocolICMPv6
	default:
		return firewallProtocolTCPUDP
	}
}

func normalizeFirewallPortSpec(raw string, protocol string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if !firewallProtocolNeedsPort(protocol) {
		return "", nil
	}
	ranges := parsePortRangeInput(trimmed)
	if len(ranges) == 0 {
		return "", common.NewError("firewall ports are required")
	}
	return portRangesToNft(ranges), nil
}

func normalizeFirewallSourceSpec(raw string, family string) (string, error) {
	entries, err := parseFirewallSourceEntries(raw)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "", nil
	}

	v4 := false
	v6 := false
	for _, entry := range entries {
		prefix, parseErr := netip.ParsePrefix(entry)
		if parseErr != nil {
			return "", parseErr
		}
		if prefix.Addr().Is4() {
			v4 = true
		} else if prefix.Addr().Is6() {
			v6 = true
		}
	}

	switch family {
	case firewallFamilyIPv4:
		if v6 {
			return "", common.NewError("ipv4 rule cannot contain ipv6 source")
		}
	case firewallFamilyIPv6:
		if v4 {
			return "", common.NewError("ipv6 rule cannot contain ipv4 source")
		}
	}
	return strings.Join(entries, ", "), nil
}

func parseFirewallSourceEntries(raw string) ([]string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	trimmed = strings.TrimPrefix(trimmed, "{")
	trimmed = strings.TrimSuffix(trimmed, "}")
	parts := strings.Split(trimmed, ",")
	normalized := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		value := strings.Trim(strings.TrimSpace(part), "\"'")
		if value == "" {
			continue
		}
		if !strings.Contains(value, "/") {
			addr, err := netip.ParseAddr(value)
			if err != nil {
				return nil, common.NewError("invalid source ip/cidr: ", value)
			}
			if addr.Is4() {
				value = netip.PrefixFrom(addr, 32).String()
			} else {
				value = netip.PrefixFrom(addr, 128).String()
			}
		} else {
			prefix, err := netip.ParsePrefix(value)
			if err != nil {
				return nil, common.NewError("invalid source ip/cidr: ", value)
			}
			value = prefix.Masked().String()
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	sort.Strings(normalized)
	return normalized, nil
}

func splitFirewallSourcesByFamily(raw string) ([]string, []string, error) {
	entries, err := parseFirewallSourceEntries(raw)
	if err != nil {
		return nil, nil, err
	}
	v4 := make([]string, 0)
	v6 := make([]string, 0)
	for _, entry := range entries {
		prefix, parseErr := netip.ParsePrefix(entry)
		if parseErr != nil {
			return nil, nil, parseErr
		}
		if prefix.Addr().Is4() {
			v4 = append(v4, prefix.Masked().String())
			continue
		}
		v6 = append(v6, prefix.Masked().String())
	}
	return v4, v6, nil
}

func buildNftCIDRSetArgs(entries []string) []string {
	if len(entries) == 0 {
		return nil
	}
	if len(entries) == 1 {
		return []string{entries[0]}
	}
	args := []string{"{"}
	for index, entry := range entries {
		if index > 0 {
			args = append(args, ",")
		}
		args = append(args, entry)
	}
	args = append(args, "}")
	return args
}

func intsToPortRanges(ports []int) []portRange {
	if len(ports) == 0 {
		return nil
	}
	ranges := make([]portRange, 0, len(ports))
	for _, port := range normalizePortList(ports) {
		ranges = append(ranges, portRange{start: port, end: port})
	}
	return mergePortRanges(ranges)
}

func (s *FirewallService) getFirewallEnabledLocked() (bool, error) {
	return s.getBool(firewallEnabledKey)
}

func (s *FirewallService) setFirewallEnabledLocked(enabled bool) error {
	return s.setString(firewallEnabledKey, strconv.FormatBool(enabled))
}

func (s *FirewallService) getFirewallLastSyncAtLocked() (int64, error) {
	raw, err := s.getString(firewallLastSyncAtKey)
	if err != nil {
		return 0, err
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, nil
	}
	value, parseErr := strconv.ParseInt(trimmed, 10, 64)
	if parseErr != nil {
		return 0, nil
	}
	return value, nil
}

func (s *FirewallService) setFirewallLastSyncAtLocked(timestamp int64) error {
	return s.setString(firewallLastSyncAtKey, strconv.FormatInt(timestamp, 10))
}

var gormSessionAllowAll = gorm.Session{AllowGlobalUpdate: true}
