package service

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/database/model"
)

const portForwardRuntimeOwnerCacheTTL = 30 * time.Second

// PortForwardRuntimeConflict is an observation only. It deliberately does not
// change a saved forwarding rule: an external listener can appear after a
// rule has been accepted, and that situation must remain diagnosable instead
// of silently disabling user intent.
type PortForwardRuntimeConflict struct {
	RuleID        uint                              `json:"ruleId"`
	RuleName      string                            `json:"ruleName"`
	LocalPortSpec string                            `json:"localPortSpec"`
	Family        string                            `json:"family"`
	Protocol      string                            `json:"protocol"`
	Port          int                               `json:"port"`
	SocketFamily  string                            `json:"socketFamily"`
	SocketStack   string                            `json:"socketStack"`
	StackSource   string                            `json:"stackSource"`
	BindAddress   string                            `json:"bindAddress"`
	Owners        []PortForwardRuntimeConflictOwner `json:"owners"`
	CheckedAt     int64                             `json:"checkedAt"`
}

// PortForwardRuntimeConflictOwner intentionally exposes just the process
// identity needed by the forwarding screen. Full command lines can be large
// and do not belong in this compact overview response.
type PortForwardRuntimeConflictOwner struct {
	PID  int    `json:"pid"`
	Name string `json:"name"`
}

type portForwardCachedOwners struct {
	owners    []FirewallListenerOwnerView
	expiresAt time.Time
}

var (
	portForwardReadListenerSocketsFn = readProcListenerSockets
	portForwardReadBindV6OnlyFn      = readIPv6BindV6OnlyDefault
	portForwardResolveOwnersFn       = resolveProcListenerOwners

	portForwardRuntimeConflictState = struct {
		mu            sync.Mutex
		ownersByInode map[string]portForwardCachedOwners
	}{
		ownersByInode: make(map[string]portForwardCachedOwners),
	}
)

func collectPortForwardRuntimeConflicts(rows []model.PortForwardRule) []PortForwardRuntimeConflict {
	if portForwardRuntimeGOOS() != "linux" {
		return []PortForwardRuntimeConflict{}
	}
	activeRows := make([]model.PortForwardRule, 0, len(rows))
	filter := firewallListenerFilter{}
	for _, row := range rows {
		if !row.Enabled {
			continue
		}
		spans := portForwardRuleSpans(row)
		if len(spans) == 0 {
			continue
		}
		activeRows = append(activeRows, row)
		flags := portForwardProtocolFlagsFor(row.Protocol)
		for _, span := range spans {
			current := portRange{start: span.start, end: span.end}
			if flags.tcp {
				filter.tcpRanges = append(filter.tcpRanges, current)
			}
			if flags.udp {
				filter.udpRanges = append(filter.udpRanges, current)
			}
		}
	}
	if len(activeRows) == 0 || filter.empty() {
		return []PortForwardRuntimeConflict{}
	}
	filter.tcpRanges = mergePortRanges(filter.tcpRanges)
	filter.udpRanges = mergePortRanges(filter.udpRanges)
	sockets, err := portForwardReadListenerSocketsFn(filter)
	if err != nil {
		return []PortForwardRuntimeConflict{}
	}

	bindV6Only, bindV6OnlyKnown := portForwardReadBindV6OnlyFn()
	matchedSocketRows := make([]struct {
		socket      procListenerSocket
		stack       string
		stackSource string
		rows        []model.PortForwardRule
	}, 0, len(sockets))
	targetInodes := make(map[string]struct{})
	for _, socket := range sockets {
		stack, source := resolveProcListenerStack(socket, bindV6Only, bindV6OnlyKnown)
		matchedRows := make([]model.PortForwardRule, 0, 1)
		for _, row := range activeRows {
			if !portForwardRuleMatchesSocket(row, socket, stack) {
				continue
			}
			matchedRows = append(matchedRows, row)
		}
		if len(matchedRows) == 0 {
			continue
		}
		matchedSocketRows = append(matchedSocketRows, struct {
			socket      procListenerSocket
			stack       string
			stackSource string
			rows        []model.PortForwardRule
		}{socket: socket, stack: stack, stackSource: source, rows: matchedRows})
		if socket.inode != "" {
			targetInodes[socket.inode] = struct{}{}
		}
	}
	if len(matchedSocketRows) == 0 {
		return []PortForwardRuntimeConflict{}
	}

	ownersByInode := loadPortForwardRuntimeOwners(targetInodes)
	checkedAt := time.Now().Unix()
	result := make([]PortForwardRuntimeConflict, 0, len(matchedSocketRows))
	for _, item := range matchedSocketRows {
		bindAddress := strings.TrimSpace(item.socket.bindAddress)
		if bindAddress == "" {
			bindAddress = "*"
		}
		owners := compactPortForwardRuntimeOwners(ownersByInode[item.socket.inode])
		for _, row := range item.rows {
			result = append(result, PortForwardRuntimeConflict{
				RuleID:        row.Id,
				RuleName:      strings.TrimSpace(row.Name),
				LocalPortSpec: strings.TrimSpace(row.LocalPortSpec),
				Family:        strings.TrimSpace(row.Family),
				Protocol:      item.socket.protocol,
				Port:          item.socket.port,
				SocketFamily:  item.socket.family,
				SocketStack:   item.stack,
				StackSource:   item.stackSource,
				BindAddress:   bindAddress,
				Owners:        owners,
				CheckedAt:     checkedAt,
			})
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].RuleID != result[j].RuleID {
			return result[i].RuleID < result[j].RuleID
		}
		if result[i].Protocol != result[j].Protocol {
			return result[i].Protocol < result[j].Protocol
		}
		if result[i].Port != result[j].Port {
			return result[i].Port < result[j].Port
		}
		return result[i].BindAddress < result[j].BindAddress
	})
	return result
}

func compactPortForwardRuntimeOwners(owners []FirewallListenerOwnerView) []PortForwardRuntimeConflictOwner {
	if len(owners) == 0 {
		return []PortForwardRuntimeConflictOwner{}
	}
	result := make([]PortForwardRuntimeConflictOwner, 0, len(owners))
	seen := make(map[string]struct{}, len(owners))
	for _, owner := range owners {
		name := strings.TrimSpace(owner.Name)
		key := strconv.Itoa(owner.PID) + "\x00" + name
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, PortForwardRuntimeConflictOwner{PID: owner.PID, Name: name})
	}
	return result
}

func loadPortForwardRuntimeOwners(targetInodes map[string]struct{}) map[string][]FirewallListenerOwnerView {
	result := make(map[string][]FirewallListenerOwnerView, len(targetInodes))
	if len(targetInodes) == 0 {
		return result
	}
	now := time.Now()
	missing := make(map[string]struct{})

	portForwardRuntimeConflictState.mu.Lock()
	for inode := range targetInodes {
		cached, ok := portForwardRuntimeConflictState.ownersByInode[inode]
		if ok && now.Before(cached.expiresAt) {
			result[inode] = append([]FirewallListenerOwnerView(nil), cached.owners...)
			continue
		}
		missing[inode] = struct{}{}
	}
	portForwardRuntimeConflictState.mu.Unlock()

	if len(missing) > 0 {
		resolved := portForwardResolveOwnersFn(missing)
		portForwardRuntimeConflictState.mu.Lock()
		for inode := range missing {
			owners := append([]FirewallListenerOwnerView(nil), resolved[inode]...)
			portForwardRuntimeConflictState.ownersByInode[inode] = portForwardCachedOwners{
				owners:    owners,
				expiresAt: now.Add(portForwardRuntimeOwnerCacheTTL),
			}
			result[inode] = append([]FirewallListenerOwnerView(nil), owners...)
		}
		for inode, cached := range portForwardRuntimeConflictState.ownersByInode {
			if now.After(cached.expiresAt) {
				delete(portForwardRuntimeConflictState.ownersByInode, inode)
			}
		}
		portForwardRuntimeConflictState.mu.Unlock()
	}
	return result
}

func portForwardRuleSpans(row model.PortForwardRule) []portSpan {
	spans, _, _, _, _, err := normalizePortForwardLocalPortSpec(row.LocalPortSpec)
	if err == nil && len(spans) > 0 {
		return spans
	}
	if row.LocalPortStart > 0 && row.LocalPortEnd >= row.LocalPortStart {
		return []portSpan{{start: row.LocalPortStart, end: row.LocalPortEnd}}
	}
	return nil
}

func portForwardRuleMatchesSocket(row model.PortForwardRule, socket procListenerSocket, socketStack string) bool {
	protocol := portForwardProtocolFlagsFor(row.Protocol)
	if socket.protocol == firewallProtocolTCP && !protocol.tcp {
		return false
	}
	if socket.protocol == firewallProtocolUDP && !protocol.udp {
		return false
	}
	if !portForwardPortInSpans(socket.port, portForwardRuleSpans(row)) {
		return false
	}
	family := portForwardFamilyFlagsFor(row.Family)
	switch socketStack {
	case firewallFamilyDual:
		return family.ipv4 || family.ipv6
	case firewallFamilyIPv4:
		return family.ipv4
	default:
		return family.ipv6
	}
}

func portForwardPortInSpans(port int, spans []portSpan) bool {
	for _, span := range spans {
		if port >= span.start && port <= span.end {
			return true
		}
	}
	return false
}

func portForwardRowsNeedKernelForwarding(rows []model.PortForwardRule) bool {
	for _, row := range rows {
		if row.Enabled && !portForwardTargetIsLocal(row.TargetIP) {
			return true
		}
	}
	return false
}
