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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/util/common"
	"golang.org/x/sync/singleflight"
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

	portForwardRuleNameMaxRunes        = 120
	portForwardRuleDescriptionMaxRunes = 1000
	portForwardPortSpecMaxRunes        = 512
	portForwardTargetIPMaxRunes        = 253
	portForwardRateLimitMaxMbps        = 1000000
	portForwardRuleMaxCount            = 500
	portForwardIdleVerifyInterval      = 5 * time.Minute
)

var (
	portForwardNftTable = loadPortForwardNftTableName()

	portForwardStateMu sync.Mutex
	portForwardState   = struct {
		lastRenderHash      string
		lastLayout          string
		lastReconcile       time.Time
		lastActiveRuleCount int
		warnings            []string
		nftSnapshot         *portForwardNftSnapshot
		lastError           string
	}{}
	portForwardOverviewRuntimeMu sync.RWMutex
	portForwardOverviewRuntime   struct {
		lastSyncAt        int64
		kernelIPv4Forward bool
		kernelIPv6Forward bool
		totalUp           int64
		totalDown         int64
		totalTraffic      int64
		rules             []PortForwardRuntimeRuleView
		warnings          []string
		lastError         string
	}

	portForwardCounterBlockRe         = regexp.MustCompile(`(?ms)counter\s+([A-Za-z0-9_][A-Za-z0-9_-]*)\s*\{[^{}]*?packets\s+(\d+)\s+bytes\s+(\d+)\s*\}`)
	portForwardInlineCounterCommentRe = regexp.MustCompile(`^kwor_pf_rule_(\d+)_(up|down)$`)

	portForwardReconcileLocked = func(s *PortForwardService, minGap time.Duration) error {
		return s.reconcileLocked(minGap)
	}
	portForwardOverviewFlight singleflight.Group
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

	// Pointer fields keep older clients compatible: an update from a client
	// that predates traffic controls must retain existing quota settings.
	TrafficLimitGiB   *float64 `json:"trafficLimitGiB"`
	TrafficResetDay   *int     `json:"trafficResetDay"`
	TrafficExpiryDate *string  `json:"trafficExpiryDate"`
}

type PortForwardRuleView struct {
	model.PortForwardRule
	CurrentUp              int64   `json:"currentUp"`
	CurrentDown            int64   `json:"currentDown"`
	CurrentTotal           int64   `json:"currentTotal"`
	TrafficLimitGiB        float64 `json:"trafficLimitGiB"`
	TrafficNextResetAt     int64   `json:"trafficNextResetAt"`
	TrafficLastResetAt     int64   `json:"trafficLastResetAt"`
	TrafficLimitReached    bool    `json:"trafficLimitReached"`
	TrafficExpired         bool    `json:"trafficExpired"`
	TrafficBlocked         bool    `json:"trafficBlocked"`
	TrafficBlockReason     string  `json:"trafficBlockReason"`
	EffectiveRateLimitMbps int     `json:"effectiveRateLimitMbps"`
	LimitStatus            string  `json:"limitStatus"`
	LimitWarning           string  `json:"limitWarning"`
	RuntimeConflictCount   int     `json:"runtimeConflictCount"`
}

type PortForwardOverview struct {
	Supported               bool                         `json:"supported"`
	Ready                   bool                         `json:"ready"`
	Available               bool                         `json:"available"`
	NftVersion              string                       `json:"nftVersion,omitempty"`
	KernelVersion           string                       `json:"kernelVersion,omitempty"`
	CompatibilityMode       string                       `json:"compatibilityMode,omitempty"`
	RendererSupported       bool                         `json:"rendererSupported"`
	SupportsTransportHeader bool                         `json:"supportsTransportHeader"`
	SupportsTableComments   bool                         `json:"supportsTableComments"`
	CapabilityError         string                       `json:"capabilityError,omitempty"`
	VersionProbeError       string                       `json:"versionProbeError,omitempty"`
	JSONProbeError          string                       `json:"jsonProbeError,omitempty"`
	MeterProbeError         string                       `json:"meterProbeError,omitempty"`
	LayoutPending           bool                         `json:"layoutPending"`
	LastApplyError          string                       `json:"lastApplyError,omitempty"`
	LastSyncAt              int64                        `json:"lastSyncAt"`
	KernelIPv4Forward       bool                         `json:"kernelIPv4Forward"`
	KernelIPv6Forward       bool                         `json:"kernelIPv6Forward"`
	EnabledCount            int                          `json:"enabledCount"`
	LimitedCount            int                          `json:"limitedCount"`
	TotalUp                 int64                        `json:"totalUp"`
	TotalDown               int64                        `json:"totalDown"`
	TotalTraffic            int64                        `json:"totalTraffic"`
	Rules                   []PortForwardRuleView        `json:"rules"`
	RuntimeConflicts        []PortForwardRuntimeConflict `json:"runtimeConflicts"`
	Warnings                []string                     `json:"warnings,omitempty"`
	Error                   string                       `json:"error,omitempty"`
}

// PortForwardRuntimeOverview is intentionally limited to fields refreshed by
// the page poller. It is served from a process-local snapshot and does not
// read SQLite, start nft commands, or scan /proc.
type PortForwardRuntimeOverview struct {
	LastSyncAt        int64                        `json:"lastSyncAt"`
	KernelIPv4Forward bool                         `json:"kernelIPv4Forward"`
	KernelIPv6Forward bool                         `json:"kernelIPv6Forward"`
	TotalUp           int64                        `json:"totalUp"`
	TotalDown         int64                        `json:"totalDown"`
	TotalTraffic      int64                        `json:"totalTraffic"`
	RuntimeConflicts  []PortForwardRuntimeConflict `json:"runtimeConflicts"`
	Rules             []PortForwardRuntimeRuleView `json:"rules"`
	Warnings          []string                     `json:"warnings,omitempty"`
	Error             string                       `json:"error,omitempty"`
}

type PortForwardRuntimeRuleView struct {
	RuleID              uint   `json:"ruleId"`
	CurrentUp           int64  `json:"currentUp"`
	CurrentDown         int64  `json:"currentDown"`
	CurrentTotal        int64  `json:"currentTotal"`
	TrafficNextResetAt  int64  `json:"trafficNextResetAt"`
	TrafficLastResetAt  int64  `json:"trafficLastResetAt"`
	TrafficLimitReached bool   `json:"trafficLimitReached"`
	TrafficExpired      bool   `json:"trafficExpired"`
	TrafficBlocked      bool   `json:"trafficBlocked"`
	TrafficBlockReason  string `json:"trafficBlockReason"`
}

type normalizedPortForwardRule struct {
	name                  string
	description           string
	enabled               bool
	family                string
	protocol              string
	localPortMode         string
	localPortSpec         string
	localPortStart        int
	localPortCount        int
	localPortEnd          int
	targetIP              string
	targetPort            int
	rateLimitMbps         int
	trafficLimitBytes     int64
	trafficResetDay       int
	trafficExpiryDate     string
	trafficLimitProvided  bool
	trafficResetProvided  bool
	trafficExpiryProvided bool
	localPortSpans        []portSpan
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
		existingSpans, _, _, _, _, err := normalizePortForwardLocalPortSpec(existing.LocalPortSpec)
		if err != nil || len(existingSpans) == 0 {
			existingSpans = []portSpan{{start: existing.LocalPortStart, end: existing.LocalPortEnd}}
		}
		overlap := summarizePortForwardSpanOverlap(existingSpans, row.localPortSpans)
		if overlap.count == 0 {
			continue
		}
		limitText := "未限速"
		if existing.RateLimitMbps > 0 {
			limitText = strconv.Itoa(existing.RateLimitMbps) + " Mbps"
		}
		warnings = append(warnings,
			fmt.Sprintf("已存在 %s 规则 %s，重叠端口 %s，当前限速 %s",
				portForwardProtocolDisplay(existing.Protocol),
				strings.TrimSpace(existing.Name),
				portForwardFormatOverlap(overlap),
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
	return IsSystemPlatformLinux() && nftSupported()
}

// portForwardCapabilityStatus separates static host support from the ability
// to execute nft at this moment. Older clients only read Available, while new
// clients can distinguish a missing Linux/nft installation from a permission
// or kernel/runtime failure.
func portForwardCapabilityStatus() (supported bool, ready bool, available bool, reason string) {
	if !IsSystemPlatformLinux() {
		return false, false, false, "nftables 转发仅支持 Linux"
	}
	if !nftSupported() {
		return false, false, false, "未找到 nft 命令"
	}
	supported = true
	capabilities := GetNftablesCapabilities()
	if !capabilities.RendererSupported {
		reason = strings.TrimSpace(capabilities.CapabilityError)
		if reason == "" {
			reason = "nftables 版本能力尚未就绪"
		}
		return supported, false, false, reason
	}
	if _, err := runNft("list", "tables"); err != nil {
		return supported, false, false, strings.TrimSpace(err.Error())
	}
	available = true
	if nftCapabilityLayoutReconcilePending() {
		reason = nftCapabilityLayoutLastApplyError()
		if strings.TrimSpace(reason) == "" {
			reason = "nftables 兼容布局仍在等待完整恢复与校验"
		}
		return supported, false, available, reason
	}
	return supported, true, true, ""
}

func portForwardTableExists() bool {
	if !portForwardSupported() {
		return false
	}
	exists, err := inspectOwnedNftTableForMutation(nftFamily, portForwardNftTable)
	return err == nil && exists
}

func cleanupManagedPortForwardTable() error {
	if !portForwardSupported() {
		return nil
	}

	var firstErr error
	for _, tableFamily := range append([]string{nftFamily}, nftCompatibilityNatFamilies()...) {
		if err := deleteOwnedNftTableForRuntime(tableFamily, portForwardNftTable); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
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
	value, err, _ := portForwardOverviewFlight.Do("port-forward-overview", func() (interface{}, error) {
		return s.getOverview()
	})
	if err != nil {
		return nil, err
	}
	overview, ok := value.(*PortForwardOverview)
	if !ok || overview == nil {
		return nil, common.NewError("port forwarding overview is unavailable")
	}
	return overview, nil
}

func (s *PortForwardService) getOverview() (*PortForwardOverview, error) {
	supported, ready, available, capabilityErr := portForwardCapabilityStatus()

	rows, err := loadPortForwardRulesLocked()
	if err != nil {
		return nil, err
	}
	trafficRuntime, err := loadPortForwardTrafficRuntime(rows, PanelNow())
	if err != nil {
		return nil, err
	}
	storePortForwardRuntimeSnapshot(rows, trafficRuntime)
	limitStates := loadPortForwardLimitStateMap()

	views := make([]PortForwardRuleView, 0, len(rows))
	enabledCount := 0
	limitedCount := 0
	for _, row := range rows {
		traffic := trafficRuntime.Rules[row.Id]
		total := addPortForwardTrafficBytes(traffic.UsedUpBytes, traffic.UsedDownBytes)
		views = append(views, PortForwardRuleView{
			PortForwardRule:        row,
			CurrentUp:              traffic.UsedUpBytes,
			CurrentDown:            traffic.UsedDownBytes,
			CurrentTotal:           total,
			TrafficLimitGiB:        portForwardTrafficLimitGiB(row.TrafficLimitBytes),
			TrafficNextResetAt:     traffic.NextResetAt,
			TrafficLastResetAt:     traffic.LastResetAt,
			TrafficLimitReached:    traffic.LimitReached,
			TrafficExpired:         traffic.Expired,
			TrafficBlocked:         row.Enabled && traffic.BlockReason != "",
			TrafficBlockReason:     traffic.BlockReason,
			EffectiveRateLimitMbps: limitStates[row.Id].EffectiveRateLimitMbps,
			LimitStatus:            limitStates[row.Id].Status,
			LimitWarning:           limitStates[row.Id].Warning,
		})
		if row.Enabled {
			enabledCount++
			if row.RateLimitMbps > 0 {
				limitedCount++
			}
		}
	}

	runtimeSnapshot := portForwardRuntimeOverviewSnapshot()

	runtimeConflicts := loadPortForwardRuntimeConflicts(rows)
	conflictsByRule := make(map[uint]int, len(runtimeConflicts))
	for _, conflict := range runtimeConflicts {
		conflictsByRule[conflict.RuleID]++
	}
	for index := range views {
		views[index].RuntimeConflictCount = conflictsByRule[views[index].Id]
	}
	capabilities := GetNftablesCapabilities()

	overview := &PortForwardOverview{
		Supported:               supported,
		Ready:                   ready,
		Available:               available,
		NftVersion:              capabilities.NftVersion,
		KernelVersion:           capabilities.KernelVersion,
		CompatibilityMode:       capabilities.CompatibilityMode,
		RendererSupported:       capabilities.RendererSupported,
		SupportsTransportHeader: capabilities.SupportsTransportHeader,
		SupportsTableComments:   capabilities.SupportsTableComments,
		CapabilityError:         capabilities.CapabilityError,
		VersionProbeError:       capabilities.VersionProbeError,
		JSONProbeError:          capabilities.JSONProbeError,
		MeterProbeError:         capabilities.MeterProbeError,
		LayoutPending:           nftCapabilityLayoutReconcilePending(),
		LastApplyError:          nftCapabilityLayoutLastApplyError(),
		LastSyncAt:              runtimeSnapshot.lastSyncAt,
		KernelIPv4Forward:       readKernelForwardingEnabled("/proc/sys/net/ipv4/ip_forward"),
		KernelIPv6Forward:       readKernelForwardingEnabled("/proc/sys/net/ipv6/conf/all/forwarding"),
		EnabledCount:            enabledCount,
		LimitedCount:            limitedCount,
		TotalUp:                 trafficRuntime.OverviewUp,
		TotalDown:               trafficRuntime.OverviewDown,
		TotalTraffic:            addPortForwardTrafficBytes(trafficRuntime.OverviewUp, trafficRuntime.OverviewDown),
		Rules:                   views,
		RuntimeConflicts:        runtimeConflicts,
		Warnings:                runtimeSnapshot.warnings,
	}

	if runtimeSnapshot.lastError != "" {
		overview.Error = runtimeSnapshot.lastError
	} else if !ready {
		overview.Error = capabilityErr
	}
	return overview, nil
}

// GetRuntimeOverview keeps the active-page polling path bounded. Full
// overview reads are still used at entry, manual refresh, and after mutations.
func (s *PortForwardService) GetRuntimeOverview() (*PortForwardRuntimeOverview, error) {
	runtime := portForwardRuntimeOverviewSnapshot()
	return &PortForwardRuntimeOverview{
		LastSyncAt:        runtime.lastSyncAt,
		KernelIPv4Forward: runtime.kernelIPv4Forward,
		KernelIPv6Forward: runtime.kernelIPv6Forward,
		TotalUp:           runtime.totalUp,
		TotalDown:         runtime.totalDown,
		TotalTraffic:      runtime.totalTraffic,
		RuntimeConflicts:  portForwardRuntimeConflictSnapshot(),
		Rules:             runtime.rules,
		Warnings:          runtime.warnings,
		Error:             runtime.lastError,
	}, nil
}

type portForwardRuntimeSnapshot struct {
	lastSyncAt        int64
	kernelIPv4Forward bool
	kernelIPv6Forward bool
	totalUp           int64
	totalDown         int64
	totalTraffic      int64
	rules             []PortForwardRuntimeRuleView
	warnings          []string
	lastError         string
}

func storePortForwardRuntimeSnapshot(rows []model.PortForwardRule, traffic portForwardTrafficRuntime) {
	runtimeRules := make([]PortForwardRuntimeRuleView, 0, len(rows))
	for _, row := range rows {
		current := traffic.Rules[row.Id]
		runtimeRules = append(runtimeRules, PortForwardRuntimeRuleView{
			RuleID:              row.Id,
			CurrentUp:           current.UsedUpBytes,
			CurrentDown:         current.UsedDownBytes,
			CurrentTotal:        addPortForwardTrafficBytes(current.UsedUpBytes, current.UsedDownBytes),
			TrafficNextResetAt:  current.NextResetAt,
			TrafficLastResetAt:  current.LastResetAt,
			TrafficLimitReached: current.LimitReached,
			TrafficExpired:      current.Expired,
			TrafficBlocked:      row.Enabled && current.BlockReason != "",
			TrafficBlockReason:  current.BlockReason,
		})
	}
	portForwardOverviewRuntimeMu.Lock()
	portForwardOverviewRuntime.kernelIPv4Forward = readKernelForwardingEnabled("/proc/sys/net/ipv4/ip_forward")
	portForwardOverviewRuntime.kernelIPv6Forward = readKernelForwardingEnabled("/proc/sys/net/ipv6/conf/all/forwarding")
	portForwardOverviewRuntime.totalUp = traffic.OverviewUp
	portForwardOverviewRuntime.totalDown = traffic.OverviewDown
	portForwardOverviewRuntime.totalTraffic = addPortForwardTrafficBytes(traffic.OverviewUp, traffic.OverviewDown)
	portForwardOverviewRuntime.rules = runtimeRules
	portForwardOverviewRuntimeMu.Unlock()
}

func portForwardRuntimeOverviewSnapshot() portForwardRuntimeSnapshot {
	if portForwardStateMu.TryLock() {
		lastSyncAt := int64(0)
		if !portForwardState.lastReconcile.IsZero() {
			lastSyncAt = portForwardState.lastReconcile.Unix()
		}
		warnings := append([]string(nil), portForwardState.warnings...)
		lastError := strings.TrimSpace(portForwardState.lastError)
		portForwardStateMu.Unlock()

		portForwardOverviewRuntimeMu.Lock()
		portForwardOverviewRuntime.lastSyncAt = lastSyncAt
		portForwardOverviewRuntime.warnings = append([]string(nil), warnings...)
		portForwardOverviewRuntime.lastError = lastError
		result := portForwardRuntimeSnapshot{
			lastSyncAt:        portForwardOverviewRuntime.lastSyncAt,
			kernelIPv4Forward: portForwardOverviewRuntime.kernelIPv4Forward,
			kernelIPv6Forward: portForwardOverviewRuntime.kernelIPv6Forward,
			totalUp:           portForwardOverviewRuntime.totalUp,
			totalDown:         portForwardOverviewRuntime.totalDown,
			totalTraffic:      portForwardOverviewRuntime.totalTraffic,
			rules:             append([]PortForwardRuntimeRuleView(nil), portForwardOverviewRuntime.rules...),
			warnings:          append([]string(nil), portForwardOverviewRuntime.warnings...),
			lastError:         portForwardOverviewRuntime.lastError,
		}
		portForwardOverviewRuntimeMu.Unlock()
		return result
	}

	portForwardOverviewRuntimeMu.RLock()
	result := portForwardRuntimeSnapshot{
		lastSyncAt:        portForwardOverviewRuntime.lastSyncAt,
		kernelIPv4Forward: portForwardOverviewRuntime.kernelIPv4Forward,
		kernelIPv6Forward: portForwardOverviewRuntime.kernelIPv6Forward,
		totalUp:           portForwardOverviewRuntime.totalUp,
		totalDown:         portForwardOverviewRuntime.totalDown,
		totalTraffic:      portForwardOverviewRuntime.totalTraffic,
		rules:             append([]PortForwardRuntimeRuleView(nil), portForwardOverviewRuntime.rules...),
		warnings:          append([]string(nil), portForwardOverviewRuntime.warnings...),
		lastError:         portForwardOverviewRuntime.lastError,
	}
	portForwardOverviewRuntimeMu.RUnlock()
	return result
}

func (s *PortForwardService) UpsertRule(payload PortForwardRulePayload) error {
	portForwardStateMu.Lock()
	defer portForwardStateMu.Unlock()

	db := database.GetDB()
	if db == nil {
		return common.NewError("database is not ready")
	}
	// Sample counters before changing a rule. Compatibility counters are reset
	// by the following atomic redraw, so accounting must happen first.
	existingRows, err := loadPortForwardRulesLocked()
	if err != nil {
		return err
	}
	if _, err := s.capturePortForwardTrafficLocked(existingRows); err != nil {
		return err
	}
	row := model.PortForwardRule{}
	var previous model.PortForwardRule

	normalized, err := normalizePortForwardRulePayload(payload)
	if err != nil {
		return err
	}
	protocolWarnings := make([]string, 0)
	err = db.Transaction(func(tx *gorm.DB) error {
		if payload.ID > 0 {
			if err := tx.Where("id = ?", payload.ID).First(&row).Error; err != nil {
				return err
			}
			previous = row
		} else {
			var count int64
			if err := tx.Model(&model.PortForwardRule{}).Count(&count).Error; err != nil {
				return err
			}
			if count >= portForwardRuleMaxCount {
				return common.NewError("port forwarding rules are limited to ", strconv.Itoa(portForwardRuleMaxCount))
			}
		}
		if err := validatePortForwardRuleOverlap(tx, payload.ID, normalized); err != nil {
			return err
		}
		if err := validatePortForwardRuleAvailability(tx, normalized); err != nil {
			return err
		}
		protocolWarnings = findOtherProtocolConflicts(tx, payload.ID, normalized)
		if normalized.name == "" {
			autoName, nameErr := generateUniqueThreeDigitPortForwardName(tx, payload.ID)
			if nameErr != nil {
				return nameErr
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
		if normalized.trafficLimitProvided {
			row.TrafficLimitBytes = normalized.trafficLimitBytes
		}
		if normalized.trafficResetProvided {
			row.TrafficResetDay = normalized.trafficResetDay
		}
		if normalized.trafficExpiryProvided {
			row.TrafficExpiryDate = normalized.trafficExpiryDate
		}

		if payload.ID > 0 {
			return tx.Save(&row).Error
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		// New rules start from zero. Existing installations without a state row
		// are instead bootstrapped from visible nft counters by the sampler.
		return tx.Create(&model.PortForwardRuleTrafficState{RuleId: row.Id}).Error
	})
	if err != nil {
		return err
	}

	rollbackCreate := func(actionErr error) error {
		notes := make([]string, 0, 4)
		if row.Id > 0 {
			if err := db.Where("id = ?", row.Id).Delete(&model.PortForwardRule{}).Error; err != nil {
				notes = append(notes, "remove newly created rule failed: "+err.Error())
			} else {
				if stateErr := db.Where("rule_id = ?", row.Id).Delete(&model.PortForwardRuleTrafficState{}).Error; stateErr != nil {
					notes = append(notes, "remove newly created traffic state failed: "+stateErr.Error())
				}
				// Named counters are created before the atomic script so nft can
				// validate references in that transaction. If the script is
				// rejected, remove counters that belonged only to this rolled-back
				// new rule as well.
				cleanupPortForwardNftObjects(row.Id)
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
	if db == nil {
		return common.NewError("database is not ready")
	}
	rows, err := loadPortForwardRulesLocked()
	if err != nil {
		return err
	}
	if _, err := s.capturePortForwardTrafficLocked(rows); err != nil {
		return err
	}
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
	if err := db.Where("rule_id = ?", id).Delete(&model.PortForwardRuleTrafficState{}).Error; err != nil {
		logger.Warning("failed to remove deleted port-forward traffic state: ", err)
	}
	cleanupPortForwardNftObjects(id)
	return nil
}

func (s *PortForwardService) SyncIfNeeded(minGap time.Duration) error {
	portForwardStateMu.Lock()
	defer portForwardStateMu.Unlock()
	err := portForwardReconcileLocked(s, minGap)
	if err != nil {
		portForwardState.lastError = strings.TrimSpace(err.Error())
	} else {
		portForwardState.lastError = ""
	}
	return err
}

func (s *PortForwardService) CleanupOnShutdown() {
	portForwardStateMu.Lock()
	defer portForwardStateMu.Unlock()

	if IsSystemPlatformLinux() && portForwardSupported() {
		if err := cleanupManagedPortForwardTable(); err != nil && !portForwardNftObjectMissing(err) {
			logger.Warning("failed to cleanup managed port-forward nft table on shutdown: ", err)
		} else if err := restorePortForwardKernelForwarding(); err != nil {
			logger.Warning("failed to restore managed forwarding sysctl state on shutdown: ", err)
		}
	}

	savePortForwardLimitStates(nil)
	portForwardState.lastRenderHash = ""
	portForwardState.lastLayout = ""
	portForwardState.lastReconcile = time.Time{}
	portForwardState.lastActiveRuleCount = 0
	portForwardState.warnings = nil
	portForwardState.lastError = ""
	portForwardState.nftSnapshot = nil
}

func (s *PortForwardService) reconcileLocked(minGap time.Duration) error {
	if !portForwardSupported() {
		return nil
	}
	if err := ensureNftRendererSupported(); err != nil {
		return err
	}

	now := time.Now()
	if minGap > 0 && !portForwardState.lastReconcile.IsZero() && now.Sub(portForwardState.lastReconcile) < minGap {
		return nil
	}

	// Once a stable empty configuration has removed all managed nft tables,
	// polling every five seconds cannot change the host state. Rule saves still
	// reconcile immediately, so this skips only idle work with no rules.
	if minGap > 0 && !portForwardState.lastReconcile.IsZero() &&
		portForwardState.lastActiveRuleCount == 0 && now.Sub(portForwardState.lastReconcile) < portForwardIdleVerifyInterval {
		return nil
	}

	if err := s.renderLocked(false); err != nil {
		return err
	}
	portForwardState.lastReconcile = now
	return nil
}

func (s *PortForwardService) renderLocked(force bool) error {
	if !portForwardSupported() {
		return nil
	}
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

	// Read each owned table once before any redraw. The same snapshot supplies
	// persistent traffic deltas and the integrity decision below, and keeps nft
	// command execution outside the SQLite transaction used by the sampler.
	snapshot, snapshotErr := loadPortForwardNftSnapshot()
	if snapshotErr != nil {
		portForwardState.nftSnapshot = nil
		return snapshotErr
	}
	portForwardState.nftSnapshot = &snapshot
	trafficRuntime, err := syncPortForwardTrafficStateRows(rows, readPortForwardCounterBytesFromSnapshot(snapshot), PanelNow())
	if err != nil {
		return err
	}
	storePortForwardRuntimeSnapshot(rows, trafficRuntime)
	trafficBlocks := portForwardTrafficBlockReasons(rows, trafficRuntime)

	layout := nftCapabilityLayoutSignature()
	hash := computePortForwardRenderHashWithTrafficBlocks(rows, trafficBlocks)
	if len(activeRows) > 0 {
		if err := ensureKernelForwardingForRows(activeRows); err != nil {
			return err
		}
	}

	if !force && layout == portForwardState.lastLayout && hash == portForwardState.lastRenderHash {
		if snapshot.renderIntactWithTrafficBlocks(activeRows, loadPortForwardLimitStateMap(), trafficBlocks) {
			// A prior restore can fail after the tables were already removed.
			// Keep retrying that host-local cleanup even when the empty nft
			// snapshot itself is intact.
			if len(activeRows) == 0 {
				if err := restorePortForwardKernelForwarding(); err != nil {
					return err
				}
			}
			return nil
		}
	}

	if portForwardLayoutMigrationRequired(portForwardState.lastLayout, layout) {
		if err := cleanupManagedPortForwardTable(); err != nil {
			return err
		}
		portForwardState.nftSnapshot = nil
		portForwardState.lastRenderHash = ""
		portForwardState.lastLayout = ""
	}

	if len(activeRows) == 0 {
		// Deleting the owned tables also removes native named counters and stale
		// compatibility NAT chains. Leaving an empty inet table made an
		// externally reintroduced forwarding rule invisible to integrity checks.
		if err := cleanupManagedPortForwardTable(); err != nil {
			return err
		}
		portForwardState.nftSnapshot = nil
		if err := restorePortForwardKernelForwarding(); err != nil {
			return err
		}
		savePortForwardLimitStates(nil)
		portForwardState.warnings = nil
		portForwardState.lastRenderHash = hash
		portForwardState.lastLayout = layout
		portForwardState.lastActiveRuleCount = 0
		return nil
	}

	if err := ensureManagedPortForwardBase(); err != nil {
		return err
	}
	limitStates, renderWarnings, err := renderManagedPortForwardRulesAtomicWithTrafficBlocks(activeRows, trafficBlocks)
	if err != nil {
		// The script contains flush and add statements in one nft transaction.
		// Do not clean up here: a batch failure must leave the previous ruleset
		// intact so traffic never sees a transient empty forwarding chain.
		return err
	}
	portForwardState.nftSnapshot = nil
	savePortForwardLimitStates(limitStates)
	if !portForwardRowsNeedKernelForwarding(activeRows) {
		if err := restorePortForwardKernelForwarding(); err != nil {
			return err
		}
	}
	portForwardState.warnings = renderWarnings
	portForwardState.lastRenderHash = computePortForwardRenderHashWithTrafficBlocks(rows, trafficBlocks)
	portForwardState.lastLayout = layout
	portForwardState.lastActiveRuleCount = len(activeRows)
	return nil
}

// portForwardLayoutMigrationRequired deliberately treats an empty prior
// signature as a cold start, not as a layout change. In the native layout the
// table owns named counters; deleting it on every panel-only restart would
// reset traffic totals even though neither nft nor the kernel changed.
func portForwardLayoutMigrationRequired(previous string, current string) bool {
	return strings.TrimSpace(previous) != "" && previous != current
}

// rollbackManagedPortForwardRender makes compatibility forwarding all-or-none
// even when a failure occurs while creating its base chains. The caller holds
// portForwardStateMu, so state and owned nftables tables are reset together.
func rollbackManagedPortForwardRender(renderErr error) error {
	cleanupErr := cleanupManagedPortForwardTable()
	savePortForwardLimitStates(nil)
	portForwardState.warnings = nil
	portForwardState.lastRenderHash = ""
	portForwardState.lastLayout = ""
	portForwardState.lastActiveRuleCount = 0
	portForwardState.nftSnapshot = nil
	if cleanupErr != nil {
		return wrapPortForwardRollbackError(renderErr, "remove partially rendered forwarding tables failed: "+cleanupErr.Error())
	}
	return renderErr
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
	name := strings.TrimSpace(payload.Name)
	description := strings.TrimSpace(payload.Description)
	localPortSpec := strings.TrimSpace(payload.LocalPortSpec)
	targetIPRaw := strings.TrimSpace(payload.TargetIP)
	if err := validatePortForwardTextField("name", name, portForwardRuleNameMaxRunes, true); err != nil {
		return normalizedPortForwardRule{}, err
	}
	if err := validatePortForwardTextField("description", description, portForwardRuleDescriptionMaxRunes, true); err != nil {
		return normalizedPortForwardRule{}, err
	}
	if err := validatePortForwardTextField("local port spec", localPortSpec, portForwardPortSpecMaxRunes, true); err != nil {
		return normalizedPortForwardRule{}, err
	}
	if err := validatePortForwardTextField("target ip", targetIPRaw, portForwardTargetIPMaxRunes, true); err != nil {
		return normalizedPortForwardRule{}, err
	}
	if localPortSpec != "" && strings.Count(localPortSpec, ",") >= 128 {
		return normalizedPortForwardRule{}, common.NewError("local port spec contains too many segments")
	}
	targetIP, family, err := normalizePortForwardTarget(targetIPRaw, payload.Family)
	if err != nil {
		return normalizedPortForwardRule{}, err
	}
	protocol, err := normalizePortForwardProtocol(payload.Protocol)
	if err != nil {
		return normalizedPortForwardRule{}, err
	}
	mode, start, count, end, spec, spans, err := normalizePortForwardLocalPorts(payload.LocalPortMode, localPortSpec, payload.LocalPortStart, payload.LocalPortCount, payload.LocalPortEnd)
	if err != nil {
		return normalizedPortForwardRule{}, err
	}
	if payload.TargetPort < 1 || payload.TargetPort > 65535 {
		return normalizedPortForwardRule{}, common.NewError("target port must be between 1 and 65535")
	}
	rateLimitMbps := payload.RateLimitMbps
	if rateLimitMbps < 0 {
		return normalizedPortForwardRule{}, common.NewError("rate limit must not be negative")
	}
	if rateLimitMbps > portForwardRateLimitMaxMbps {
		return normalizedPortForwardRule{}, common.NewError("rate limit exceeds the maximum supported value")
	}

	trafficLimitBytes := int64(0)
	trafficResetDay := 0
	trafficExpiryDate := ""
	trafficLimitProvided := payload.TrafficLimitGiB != nil
	trafficResetProvided := payload.TrafficResetDay != nil
	trafficExpiryProvided := payload.TrafficExpiryDate != nil
	if trafficLimitProvided {
		var trafficErr error
		trafficLimitBytes, trafficErr = normalizePortForwardTrafficLimitGiB(*payload.TrafficLimitGiB)
		if trafficErr != nil {
			return normalizedPortForwardRule{}, trafficErr
		}
	}
	if trafficResetProvided {
		var trafficErr error
		trafficResetDay, trafficErr = normalizePortForwardTrafficResetDay(*payload.TrafficResetDay)
		if trafficErr != nil {
			return normalizedPortForwardRule{}, trafficErr
		}
	}
	if trafficExpiryProvided {
		var trafficErr error
		trafficExpiryDate, trafficErr = normalizePortForwardTrafficExpiryDate(*payload.TrafficExpiryDate)
		if trafficErr != nil {
			return normalizedPortForwardRule{}, trafficErr
		}
	}

	return normalizedPortForwardRule{
		name:                  name,
		description:           description,
		enabled:               payload.Enabled,
		family:                family,
		protocol:              protocol,
		localPortMode:         mode,
		localPortSpec:         spec,
		localPortStart:        start,
		localPortCount:        count,
		localPortEnd:          end,
		targetIP:              targetIP,
		targetPort:            payload.TargetPort,
		rateLimitMbps:         rateLimitMbps,
		trafficLimitBytes:     trafficLimitBytes,
		trafficResetDay:       trafficResetDay,
		trafficExpiryDate:     trafficExpiryDate,
		trafficLimitProvided:  trafficLimitProvided,
		trafficResetProvided:  trafficResetProvided,
		trafficExpiryProvided: trafficExpiryProvided,
		localPortSpans:        spans,
	}, nil
}

func validatePortForwardTextField(field string, value string, maxRunes int, allowEmpty bool) error {
	if value == "" && !allowEmpty {
		return common.NewError(field, " is required")
	}
	if !utf8.ValidString(value) {
		return common.NewError(field, " must be valid UTF-8")
	}
	if utf8.RuneCountInString(value) > maxRunes {
		return common.NewError(field, " is too long")
	}
	if strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return common.NewError(field, " contains unsupported control characters")
	}
	return nil
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

func computePortForwardRenderHash(rows []model.PortForwardRule) string {
	return computePortForwardRenderHashWithTrafficBlocks(rows, nil)
}

func computePortForwardRenderHashWithTrafficBlocks(rows []model.PortForwardRule, trafficBlocks map[uint]string) string {
	blockRows := make([]struct {
		RuleID uint   `json:"ruleId"`
		Reason string `json:"reason"`
	}, 0, len(trafficBlocks))
	for _, row := range rows {
		if !row.Enabled {
			continue
		}
		if reason := strings.TrimSpace(trafficBlocks[row.Id]); reason != "" {
			blockRows = append(blockRows, struct {
				RuleID uint   `json:"ruleId"`
				Reason string `json:"reason"`
			}{RuleID: row.Id, Reason: reason})
		}
	}
	payload := struct {
		Rows             []model.PortForwardRule `json:"rows"`
		TrafficBlocks    interface{}             `json:"trafficBlocks"`
		CapabilityLayout string                  `json:"capabilityLayout"`
		SupportsMeters   bool                    `json:"supportsMeters"`
	}{
		Rows:             rows,
		TrafficBlocks:    blockRows,
		CapabilityLayout: nftCapabilityLayoutSignature(),
		SupportsMeters:   GetNftablesCapabilities().SupportsMeters,
	}
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
