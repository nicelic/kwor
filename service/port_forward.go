package service

import (
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
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
	portForwardFamilyDual = "dual"
	portForwardFamilyIPv4 = "ipv4"
	portForwardFamilyIPv6 = "ipv6"

	portForwardProtocolTCP    = "tcp"
	portForwardProtocolUDP    = "udp"
	portForwardProtocolTCPUDP = "tcp_udp"

	portForwardLocalPortModeSingle = "single"
	portForwardLocalPortModeCount  = "count"
	portForwardLocalPortModeRange  = "range"

	portForwardPreroutingChain  = "pf_prerouting"
	portForwardPostroutingChain = "pf_postrouting"
	portForwardForwardChain     = "pf_forward"
	portForwardInputChain       = "pf_input"
	portForwardOutputChain      = "pf_output"
)

var (
	portForwardNftTable = loadPortForwardNftTableName()

	portForwardStateMu sync.Mutex
	portForwardState   = struct {
		lastRenderHash string
		lastReconcile  time.Time
		warnings       []string
	}{}

	portForwardCounterBlockRe = regexp.MustCompile(`(?ms)counter\s+([A-Za-z0-9_][A-Za-z0-9_-]*)\s*\{[^{}]*?packets\s+(\d+)\s+bytes\s+(\d+)\s*\}`)

	portForwardReconcileLocked = func(s *PortForwardService, minGap time.Duration) error {
		return s.reconcileLocked(minGap)
	}
)

type PortForwardService struct {
	SettingService
}

type PortForwardRulePayload struct {
	ID             uint   `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	Enabled        bool   `json:"enabled"`
	Family         string `json:"family"`
	Protocol       string `json:"protocol"`
	LocalPortMode  string `json:"localPortMode"`
	LocalPortSpec  string `json:"localPortSpec"`
	LocalPortStart int    `json:"localPortStart"`
	LocalPortCount int    `json:"localPortCount"`
	LocalPortEnd   int    `json:"localPortEnd"`
	TargetIP       string `json:"targetIP"`
	TargetPort     int    `json:"targetPort"`
	RateLimitMbps  int    `json:"rateLimitMbps"`
}

type PortForwardRuleView struct {
	model.PortForwardRule
	CurrentUp              int64  `json:"currentUp"`
	CurrentDown            int64  `json:"currentDown"`
	CurrentTotal           int64  `json:"currentTotal"`
	EffectiveRateLimitMbps int    `json:"effectiveRateLimitMbps"`
	LimitStatus            string `json:"limitStatus"`
	LimitWarning           string `json:"limitWarning"`
}

type PortForwardOverview struct {
	Available         bool                  `json:"available"`
	LastSyncAt        int64                 `json:"lastSyncAt"`
	KernelIPv4Forward bool                  `json:"kernelIPv4Forward"`
	KernelIPv6Forward bool                  `json:"kernelIPv6Forward"`
	EnabledCount      int                   `json:"enabledCount"`
	LimitedCount      int                   `json:"limitedCount"`
	TotalUp           int64                 `json:"totalUp"`
	TotalDown         int64                 `json:"totalDown"`
	TotalTraffic      int64                 `json:"totalTraffic"`
	Rules             []PortForwardRuleView `json:"rules"`
	Warnings          []string              `json:"warnings,omitempty"`
	Error             string                `json:"error,omitempty"`
}

type normalizedPortForwardRule struct {
	name           string
	description    string
	enabled        bool
	family         string
	protocol       string
	localPortMode  string
	localPortSpec  string
	localPortStart int
	localPortCount int
	localPortEnd   int
	targetIP       string
	targetPort     int
	rateLimitMbps  int
	localPortSpans []portSpan
}

type portForwardLimitStateView struct {
	EffectiveRateLimitMbps int
	Status                 string
	Warning                string
}

type portForwardProtocolFlags struct {
	tcp bool
	udp bool
}

type portForwardFamilyFlags struct {
	ipv4 bool
	ipv6 bool
}

func portForwardProtocolFlagsFor(raw string) portForwardProtocolFlags {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case portForwardProtocolTCP:
		return portForwardProtocolFlags{tcp: true}
	case portForwardProtocolUDP:
		return portForwardProtocolFlags{udp: true}
	case "tcpudp", "tcp+udp", "tcp/udp", portForwardProtocolTCPUDP:
		return portForwardProtocolFlags{tcp: true, udp: true}
	default:
		return portForwardProtocolFlags{tcp: true}
	}
}

func portForwardFamilyFlagsFor(raw string) portForwardFamilyFlags {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case portForwardFamilyIPv4:
		return portForwardFamilyFlags{ipv4: true}
	case portForwardFamilyIPv6:
		return portForwardFamilyFlags{ipv6: true}
	case "ipv4ipv6", "ipv4/ipv6", portForwardFamilyDual:
		return portForwardFamilyFlags{ipv4: true, ipv6: true}
	default:
		return portForwardFamilyFlags{ipv4: true}
	}
}

func portForwardProtocolsOverlap(left string, right string) bool {
	l := portForwardProtocolFlagsFor(left)
	r := portForwardProtocolFlagsFor(right)
	return (l.tcp && r.tcp) || (l.udp && r.udp)
}

func portForwardFamiliesOverlap(left string, right string) bool {
	l := portForwardFamilyFlagsFor(left)
	r := portForwardFamilyFlagsFor(right)
	return (l.ipv4 && r.ipv4) || (l.ipv6 && r.ipv6)
}

func portForwardProtocolDisplay(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case portForwardProtocolTCP:
		return "TCP"
	case portForwardProtocolUDP:
		return "UDP"
	case "tcpudp", "tcp+udp", "tcp/udp", portForwardProtocolTCPUDP:
		return "TCP/UDP"
	default:
		value := strings.TrimSpace(raw)
		if value == "" {
			return "UNKNOWN"
		}
		return strings.ToUpper(value)
	}
}

func portForwardExpandFamilies(raw string) []string {
	flags := portForwardFamilyFlagsFor(raw)
	families := make([]string, 0, 2)
	if flags.ipv4 {
		families = append(families, portForwardFamilyIPv4)
	}
	if flags.ipv6 {
		families = append(families, portForwardFamilyIPv6)
	}
	return families
}

func loadPortForwardLimitStateMap() map[uint]portForwardLimitStateView {
	db := database.GetDB()
	if db == nil {
		return map[uint]portForwardLimitStateView{}
	}

	rows := make([]model.PortForwardLimitState, 0)
	if err := db.Find(&rows).Error; err != nil {
		return map[uint]portForwardLimitStateView{}
	}

	out := make(map[uint]portForwardLimitStateView, len(rows))
	for _, row := range rows {
		out[row.RuleId] = portForwardLimitStateView{
			EffectiveRateLimitMbps: row.EffectiveRateLimitMbps,
			Status:                 strings.TrimSpace(row.Status),
			Warning:                strings.TrimSpace(row.Warning),
		}
	}
	return out
}

func savePortForwardLimitStates(states map[uint]portForwardLimitRuntime) {
	db := database.GetDB()
	if db == nil {
		return
	}

	if len(states) == 0 {
		_ = db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&model.PortForwardLimitState{}).Error
		return
	}

	activeIDs := make([]uint, 0, len(states))
	for ruleID, state := range states {
		activeIDs = append(activeIDs, ruleID)

		row := model.PortForwardLimitState{
			RuleId:                 ruleID,
			EffectiveRateLimitMbps: state.effectiveRateLimitMbps,
			Status:                 strings.TrimSpace(state.status),
			Warning:                strings.TrimSpace(state.warning),
		}

		var existing model.PortForwardLimitState
		if err := db.Where("rule_id = ?", ruleID).First(&existing).Error; err == nil {
			row.Id = existing.Id
			_ = db.Save(&row).Error
		} else {
			_ = db.Create(&row).Error
		}
	}

	_ = db.Where("rule_id NOT IN ?", activeIDs).Delete(&model.PortForwardLimitState{}).Error
}

func normalizePortForwardLocalMode(rawMode string, spans []portSpan) string {
	mode := strings.TrimSpace(strings.ToLower(rawMode))
	switch mode {
	case portForwardLocalPortModeSingle, portForwardLocalPortModeRange, "multi":
		return mode
	case portForwardLocalPortModeCount:
		if len(spans) == 1 && spans[0].start != spans[0].end {
			return portForwardLocalPortModeRange
		}
		if len(spans) > 1 {
			return "multi"
		}
		return portForwardLocalPortModeSingle
	default:
		if len(spans) == 1 && spans[0].start == spans[0].end {
			return portForwardLocalPortModeSingle
		}
		if len(spans) == 1 {
			return portForwardLocalPortModeRange
		}
		return "multi"
	}
}

func normalizePortForwardLocalPortSpec(raw string) ([]portSpan, string, int, int, int, error) {
	spans, normalized, err := parseStrictPortRanges(raw)
	if err != nil {
		return nil, "", 0, 0, 0, common.NewError("invalid local port spec: ", err.Error())
	}
	if len(spans) == 0 {
		return nil, "", 0, 0, 0, common.NewError("local port spec is required")
	}

	start := spans[0].start
	end := spans[len(spans)-1].end
	count := countPorts(spans)
	display := strings.ReplaceAll(normalized, ":", "-")
	return spans, display, start, count, end, nil
}

func findOtherProtocolConflicts(db *gorm.DB, excludeID uint, row normalizedPortForwardRule) []string {
	if db == nil {
		return nil
	}

	rows := make([]model.PortForwardRule, 0)
	if err := db.Where("id <> ?", excludeID).Find(&rows).Error; err != nil {
		return nil
	}

	warnings := make([]string, 0)
	for _, existing := range rows {
		if !existing.Enabled {
			continue
		}
		if portForwardProtocolsOverlap(existing.Protocol, row.protocol) {
			continue
		}
		if !portForwardFamiliesOverlap(existing.Family, row.family) {
			continue
		}
		if !portForwardRangesOverlap(existing.LocalPortStart, existing.LocalPortEnd, row.localPortStart, row.localPortEnd) {
			continue
		}
		limitText := "未限速"
		if existing.RateLimitMbps > 0 {
			limitText = strconv.Itoa(existing.RateLimitMbps) + " Mbps"
		}
		warnings = append(warnings,
			fmt.Sprintf("已存在 %s 规则 %s，重叠端口范围 %s，当前限速 %s",
				portForwardProtocolDisplay(existing.Protocol),
				strings.TrimSpace(existing.Name),
				strings.TrimSpace(existing.LocalPortSpec),
				limitText,
			),
		)
	}
	return warnings
}

func loadPortForwardNftTableName() string {
	const fallback = "kwor_forward"
	raw := strings.TrimSpace(os.Getenv("KWOR_FORWARD_NFT_TABLE"))
	if raw == "" {
		return fallback
	}
	valid := regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_-]{0,31}$`)
	if !valid.MatchString(raw) {
		return fallback
	}
	return raw
}

func portForwardSupported() bool {
	return runtime.GOOS == "linux" && nftSupported()
}

func portForwardTableExists() bool {
	if !portForwardSupported() {
		return false
	}
	_, err := runNft("list", "table", nftFamily, portForwardNftTable)
	return err == nil
}

func cleanupManagedPortForwardTable() error {
	if !portForwardSupported() || !portForwardTableExists() {
		return nil
	}
	_, err := runNft("delete", "table", nftFamily, portForwardNftTable)
	return err
}

func wrapPortForwardRollbackError(actionErr error, notes ...string) error {
	if actionErr == nil {
		return nil
	}
	details := make([]string, 0, len(notes))
	for _, note := range notes {
		trimmed := strings.TrimSpace(note)
		if trimmed == "" {
			continue
		}
		details = append(details, trimmed)
	}
	if len(details) == 0 {
		return actionErr
	}
	return fmt.Errorf("%w; rollback: %s", actionErr, strings.Join(details, "; "))
}

func mergePortForwardWarnings(primary []string, secondary []string) []string {
	if len(primary) == 0 && len(secondary) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(primary)+len(secondary))
	out := make([]string, 0, len(primary)+len(secondary))
	appendUnique := func(items []string) {
		for _, item := range items {
			trimmed := strings.TrimSpace(item)
			if trimmed == "" {
				continue
			}
			if _, exists := seen[trimmed]; exists {
				continue
			}
			seen[trimmed] = struct{}{}
			out = append(out, trimmed)
		}
	}
	appendUnique(primary)
	appendUnique(secondary)
	return out
}

func (s *PortForwardService) GetOverview() (*PortForwardOverview, error) {
	portForwardStateMu.Lock()
	defer portForwardStateMu.Unlock()

	available := portForwardSupported()
	var syncErr error
	if available {
		syncErr = portForwardReconcileLocked(s, 2*time.Second)
	}

	rows, err := loadPortForwardRulesLocked()
	if err != nil {
		return nil, err
	}

	counterBytes := make(map[string]int64)
	limitStates := loadPortForwardLimitStateMap()
	if available {
		counterBytes, err = readPortForwardCounterBytes()
		if err != nil && syncErr == nil {
			syncErr = err
		}
	}

	views := make([]PortForwardRuleView, 0, len(rows))
	enabledCount := 0
	limitedCount := 0
	var totalUp int64
	var totalDown int64
	for _, row := range rows {
		up := counterBytes[portForwardCounterName(row.Id, "up")]
		down := counterBytes[portForwardCounterName(row.Id, "down")]
		total := up + down
		views = append(views, PortForwardRuleView{
			PortForwardRule:        row,
			CurrentUp:              up,
			CurrentDown:            down,
			CurrentTotal:           total,
			EffectiveRateLimitMbps: limitStates[row.Id].EffectiveRateLimitMbps,
			LimitStatus:            limitStates[row.Id].Status,
			LimitWarning:           limitStates[row.Id].Warning,
		})
		if row.Enabled {
			enabledCount++
			totalUp += up
			totalDown += down
			if row.RateLimitMbps > 0 {
				limitedCount++
			}
		}
	}

	lastSyncAt := int64(0)
	if !portForwardState.lastReconcile.IsZero() {
		lastSyncAt = portForwardState.lastReconcile.Unix()
	}

	overview := &PortForwardOverview{
		Available:         available,
		LastSyncAt:        lastSyncAt,
		KernelIPv4Forward: readKernelForwardingEnabled("/proc/sys/net/ipv4/ip_forward"),
		KernelIPv6Forward: readKernelForwardingEnabled("/proc/sys/net/ipv6/conf/all/forwarding"),
		EnabledCount:      enabledCount,
		LimitedCount:      limitedCount,
		TotalUp:           totalUp,
		TotalDown:         totalDown,
		TotalTraffic:      totalUp + totalDown,
		Rules:             views,
		Warnings:          append([]string(nil), portForwardState.warnings...),
	}

	if !available {
		overview.Error = "nftables 转发仅支持 Linux"
	} else if syncErr != nil {
		overview.Error = strings.TrimSpace(syncErr.Error())
	}
	return overview, nil
}

func (s *PortForwardService) UpsertRule(payload PortForwardRulePayload) error {
	portForwardStateMu.Lock()
	defer portForwardStateMu.Unlock()

	db := database.GetDB()
	row := model.PortForwardRule{}
	var previous model.PortForwardRule
	if payload.ID > 0 {
		if err := db.Where("id = ?", payload.ID).First(&row).Error; err != nil {
			return err
		}
		previous = row
	}

	normalized, err := normalizePortForwardRulePayload(payload)
	if err != nil {
		return err
	}
	if err := validatePortForwardRuleOverlap(db, payload.ID, normalized); err != nil {
		return err
	}
	if err := validatePortForwardRuleAvailability(db, normalized); err != nil {
		return err
	}
	protocolWarnings := findOtherProtocolConflicts(db, payload.ID, normalized)
	if normalized.name == "" {
		autoName, err := generateUniqueThreeDigitPortForwardName(db, payload.ID)
		if err != nil {
			return err
		}
		normalized.name = autoName
	}

	row.Name = normalized.name
	row.Description = normalized.description
	row.Enabled = normalized.enabled
	row.Family = normalized.family
	row.Protocol = normalized.protocol
	row.LocalPortMode = normalized.localPortMode
	row.LocalPortSpec = normalized.localPortSpec
	row.LocalPortStart = normalized.localPortStart
	row.LocalPortCount = normalized.localPortCount
	row.LocalPortEnd = normalized.localPortEnd
	row.TargetIP = normalized.targetIP
	row.TargetPort = normalized.targetPort
	row.RateLimitMbps = normalized.rateLimitMbps

	if payload.ID > 0 {
		if err := db.Save(&row).Error; err != nil {
			return err
		}
	} else {
		if err := db.Create(&row).Error; err != nil {
			return err
		}
	}

	rollbackCreate := func(actionErr error) error {
		notes := make([]string, 0, 4)
		if row.Id > 0 {
			if err := db.Where("id = ?", row.Id).Delete(&model.PortForwardRule{}).Error; err != nil {
				notes = append(notes, "remove newly created rule failed: "+err.Error())
			} else {
				notes = append(notes, "removed newly created rule")
			}
		}
		if err := portForwardReconcileLocked(s, 0); err != nil {
			notes = append(notes, "restore forwarding render failed: "+err.Error())
		} else {
			notes = append(notes, "restored forwarding render")
		}
		return wrapPortForwardRollbackError(actionErr, notes...)
	}

	rollbackUpdate := func(actionErr error) error {
		notes := make([]string, 0, 4)
		if err := db.Save(&previous).Error; err != nil {
			notes = append(notes, "restore previous rule failed: "+err.Error())
		} else {
			notes = append(notes, "restored previous rule")
		}
		if err := portForwardReconcileLocked(s, 0); err != nil {
			notes = append(notes, "restore forwarding render failed: "+err.Error())
		} else {
			notes = append(notes, "restored forwarding render")
		}
		return wrapPortForwardRollbackError(actionErr, notes...)
	}

	if err := portForwardReconcileLocked(s, 0); err != nil {
		if payload.ID == 0 {
			return rollbackCreate(err)
		}
		return rollbackUpdate(err)
	}
	portForwardState.warnings = mergePortForwardWarnings(portForwardState.warnings, protocolWarnings)
	return nil
}

func (s *PortForwardService) DeleteRule(id uint) error {
	portForwardStateMu.Lock()
	defer portForwardStateMu.Unlock()

	db := database.GetDB()
	var row model.PortForwardRule
	if err := db.Where("id = ?", id).First(&row).Error; err != nil {
		return err
	}
	if err := db.Delete(&row).Error; err != nil {
		return err
	}

	rollbackDelete := func(actionErr error) error {
		notes := make([]string, 0, 4)
		if restoreErr := db.Save(&row).Error; restoreErr != nil {
			notes = append(notes, "restore deleted rule failed: "+restoreErr.Error())
		} else {
			notes = append(notes, "restored deleted rule")
		}
		if reconcileErr := portForwardReconcileLocked(s, 0); reconcileErr != nil {
			notes = append(notes, "restore forwarding render failed: "+reconcileErr.Error())
		} else {
			notes = append(notes, "restored forwarding render")
		}
		return wrapPortForwardRollbackError(actionErr, notes...)
	}

	if err := portForwardReconcileLocked(s, 0); err != nil {
		return rollbackDelete(err)
	}
	cleanupPortForwardNftObjects(id)
	return nil
}

func (s *PortForwardService) SyncIfNeeded(minGap time.Duration) error {
	portForwardStateMu.Lock()
	defer portForwardStateMu.Unlock()
	return portForwardReconcileLocked(s, minGap)
}

func (s *PortForwardService) CleanupOnShutdown() {
	portForwardStateMu.Lock()
	defer portForwardStateMu.Unlock()

	if runtime.GOOS == "linux" && portForwardSupported() {
		if err := cleanupManagedPortForwardTable(); err != nil && !portForwardNftObjectMissing(err) {
			logger.Warning("failed to cleanup managed port-forward nft table on shutdown: ", err)
		}
	}

	savePortForwardLimitStates(nil)
	portForwardState.lastRenderHash = ""
	portForwardState.lastReconcile = time.Time{}
	portForwardState.warnings = nil
}

func (s *PortForwardService) reconcileLocked(minGap time.Duration) error {
	if !portForwardSupported() {
		return nil
	}

	now := time.Now()
	if minGap > 0 && !portForwardState.lastReconcile.IsZero() && now.Sub(portForwardState.lastReconcile) < minGap {
		return nil
	}

	if err := s.renderLocked(false); err != nil {
		return err
	}
	portForwardState.lastReconcile = now
	return nil
}

func (s *PortForwardService) renderLocked(force bool) error {
	rows, err := loadPortForwardRulesLocked()
	if err != nil {
		return err
	}

	activeRows := make([]model.PortForwardRule, 0, len(rows))
	for _, row := range rows {
		if row.Enabled {
			activeRows = append(activeRows, row)
		}
	}

	hash := computePortForwardRenderHash(rows)
	if len(activeRows) > 0 {
		if err := ensureKernelForwardingForRows(activeRows); err != nil {
			return err
		}
	}

	if !force && hash == portForwardState.lastRenderHash && portForwardRenderIntact(activeRows) {
		return nil
	}

	if len(activeRows) == 0 {
		if portForwardTableExists() {
			if err := flushManagedPortForwardChains(); err != nil {
				return err
			}
		}
		savePortForwardLimitStates(nil)
		portForwardState.warnings = nil
		portForwardState.lastRenderHash = hash
		return nil
	}

	if err := ensureManagedPortForwardBase(); err != nil {
		return err
	}
	if err := flushManagedPortForwardChains(); err != nil {
		return err
	}
	limitStates := make(map[uint]portForwardLimitRuntime, len(activeRows))
	renderWarnings := make([]string, 0)
	for _, row := range activeRows {
		limitState, err := addManagedPortForwardRule(row)
		if err != nil {
			return err
		}
		limitStates[row.Id] = limitState
		if strings.TrimSpace(limitState.warning) != "" {
			renderWarnings = append(renderWarnings, strings.TrimSpace(limitState.warning))
		}
	}
	savePortForwardLimitStates(limitStates)
	portForwardState.warnings = renderWarnings
	portForwardState.lastRenderHash = hash
	return nil
}

func loadPortForwardRulesLocked() ([]model.PortForwardRule, error) {
	db := database.GetDB()
	rows := make([]model.PortForwardRule, 0)
	if err := db.Order("id asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].Id < rows[j].Id
	})
	return rows, nil
}

func normalizePortForwardRulePayload(payload PortForwardRulePayload) (normalizedPortForwardRule, error) {
	targetIP, family, err := normalizePortForwardTarget(payload.TargetIP, payload.Family)
	if err != nil {
		return normalizedPortForwardRule{}, err
	}
	protocol, err := normalizePortForwardProtocol(payload.Protocol)
	if err != nil {
		return normalizedPortForwardRule{}, err
	}
	mode, start, count, end, spec, spans, err := normalizePortForwardLocalPorts(payload.LocalPortMode, payload.LocalPortSpec, payload.LocalPortStart, payload.LocalPortCount, payload.LocalPortEnd)
	if err != nil {
		return normalizedPortForwardRule{}, err
	}
	if payload.TargetPort < 1 || payload.TargetPort > 65535 {
		return normalizedPortForwardRule{}, common.NewError("target port must be between 1 and 65535")
	}
	rateLimitMbps := payload.RateLimitMbps
	if rateLimitMbps < 0 {
		rateLimitMbps = 0
	}

	return normalizedPortForwardRule{
		name:           strings.TrimSpace(payload.Name),
		description:    strings.TrimSpace(payload.Description),
		enabled:        payload.Enabled,
		family:         family,
		protocol:       protocol,
		localPortMode:  mode,
		localPortSpec:  spec,
		localPortStart: start,
		localPortCount: count,
		localPortEnd:   end,
		targetIP:       targetIP,
		targetPort:     payload.TargetPort,
		rateLimitMbps:  rateLimitMbps,
		localPortSpans: spans,
	}, nil
}

func normalizePortForwardProtocol(raw string) (string, error) {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case portForwardProtocolTCP:
		return portForwardProtocolTCP, nil
	case portForwardProtocolUDP:
		return portForwardProtocolUDP, nil
	case "tcpudp", "tcp+udp", "tcp/udp", portForwardProtocolTCPUDP:
		return portForwardProtocolTCPUDP, nil
	default:
		return "", common.NewError("forward protocol must be tcp, udp, or tcp_udp")
	}
}

func normalizePortForwardLocalPorts(rawMode string, rawSpec string, start int, count int, end int) (string, int, int, int, string, []portSpan, error) {
	trimmedSpec := strings.TrimSpace(rawSpec)
	if trimmedSpec != "" {
		spans, normalizedSpec, normalizedStart, normalizedCount, normalizedEnd, err := normalizePortForwardLocalPortSpec(trimmedSpec)
		if err != nil {
			return "", 0, 0, 0, "", nil, err
		}
		mode := normalizePortForwardLocalMode(rawMode, spans)
		return mode, normalizedStart, normalizedCount, normalizedEnd, normalizedSpec, spans, nil
	}

	mode := strings.TrimSpace(strings.ToLower(rawMode))
	switch mode {
	case "", portForwardLocalPortModeSingle:
		mode = portForwardLocalPortModeSingle
		if start < 1 || start > 65535 {
			return "", 0, 0, 0, "", nil, common.NewError("local port must be between 1 and 65535")
		}
		spec := strconv.Itoa(start)
		spans := []portSpan{{start: start, end: start}}
		return mode, start, 1, start, spec, spans, nil
	case portForwardLocalPortModeCount:
		if start < 1 || start > 65535 {
			return "", 0, 0, 0, "", nil, common.NewError("local start port must be between 1 and 65535")
		}
		if count < 1 {
			return "", 0, 0, 0, "", nil, common.NewError("local port count must be at least 1")
		}
		calculatedEnd := start + count - 1
		if calculatedEnd > 65535 {
			return "", 0, 0, 0, "", nil, common.NewError("local port count exceeds the valid port range")
		}
		if count == 1 {
			spec := strconv.Itoa(start)
			spans := []portSpan{{start: start, end: start}}
			return portForwardLocalPortModeSingle, start, 1, start, spec, spans, nil
		}
		spec := fmt.Sprintf("%d-%d", start, calculatedEnd)
		spans := []portSpan{{start: start, end: calculatedEnd}}
		return portForwardLocalPortModeRange, start, count, calculatedEnd, spec, spans, nil
	case portForwardLocalPortModeRange:
		if start < 1 || start > 65535 {
			return "", 0, 0, 0, "", nil, common.NewError("local start port must be between 1 and 65535")
		}
		if end < start || end > 65535 {
			return "", 0, 0, 0, "", nil, common.NewError("local port range is invalid")
		}
		localCount := end - start + 1
		if localCount == 1 {
			spec := strconv.Itoa(start)
			spans := []portSpan{{start: start, end: start}}
			return portForwardLocalPortModeSingle, start, 1, start, spec, spans, nil
		}
		spec := fmt.Sprintf("%d-%d", start, end)
		spans := []portSpan{{start: start, end: end}}
		return mode, start, localCount, end, spec, spans, nil
	default:
		return "", 0, 0, 0, "", nil, common.NewError("unknown local port mode: ", rawMode)
	}
}

func validatePortForwardRuleOverlap(db *gorm.DB, excludeID uint, row normalizedPortForwardRule) error {
	if db == nil || !row.enabled {
		return nil
	}
	rowSpans := row.localPortSpans
	if len(rowSpans) == 0 {
		rowSpans = []portSpan{{start: row.localPortStart, end: row.localPortEnd}}
	}
	rows := make([]model.PortForwardRule, 0)
	if err := db.Where("id <> ?", excludeID).Find(&rows).Error; err != nil {
		return err
	}
	for _, existing := range rows {
		if !existing.Enabled {
			continue
		}
		if !portForwardProtocolsOverlap(existing.Protocol, row.protocol) {
			continue
		}
		if !portForwardFamiliesOverlap(existing.Family, row.family) {
			continue
		}
		existingSpans, _, _, _, _, err := normalizePortForwardLocalPortSpec(existing.LocalPortSpec)
		if err != nil || len(existingSpans) == 0 {
			existingSpans = []portSpan{{start: existing.LocalPortStart, end: existing.LocalPortEnd}}
		}
		overlap := collectPortForwardSpanOverlapPorts(rowSpans, existingSpans)
		if len(overlap) > 0 {
			limitText := "未限速"
			if existing.RateLimitMbps > 0 {
				limitText = strconv.Itoa(existing.RateLimitMbps) + " Mbps"
			}
			return common.NewError(
				"local port spec overlaps with existing ",
				portForwardProtocolDisplay(existing.Protocol),
				" forwarding rule ",
				existing.Name,
				" (ports: ",
				existing.LocalPortSpec,
				", limit: ",
				limitText,
				")",
			)
		}
	}
	return nil
}

func generateUniqueThreeDigitPortForwardName(db *gorm.DB, excludeID uint) (string, error) {
	used := make(map[string]struct{})

	if db != nil {
		rows := make([]model.PortForwardRule, 0)
		query := db.Select("id, name")
		if excludeID > 0 {
			query = query.Where("id <> ?", excludeID)
		}
		if err := query.Find(&rows).Error; err != nil {
			return "", err
		}
		for _, row := range rows {
			name := strings.TrimSpace(row.Name)
			if name == "" {
				continue
			}
			used[name] = struct{}{}
		}
	}

	candidates := make([]string, 0, 1000)
	for n := 0; n <= 999; n++ {
		candidate := fmt.Sprintf("%03d", n)
		if _, exists := used[candidate]; exists {
			continue
		}
		candidates = append(candidates, candidate)
	}
	if len(candidates) == 0 {
		return "", common.NewError("all 3-digit forwarding names are already used; please enter a custom name")
	}

	index, err := crand.Int(crand.Reader, big.NewInt(int64(len(candidates))))
	if err != nil {
		return "", err
	}
	return candidates[int(index.Int64())], nil
}

func portForwardRangesOverlap(startA int, endA int, startB int, endB int) bool {
	return startA <= endB && startB <= endA
}

func computePortForwardRenderHash(rows []model.PortForwardRule) string {
	raw, _ := json.Marshal(rows)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
