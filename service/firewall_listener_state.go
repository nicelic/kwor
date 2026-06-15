package service

import (
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/database/model"
)

const firewallListenerSnapshotMinGap = 2 * time.Second

type FirewallRuleListenerState struct {
	Supported     bool                       `json:"supported"`
	CheckedAt     int64                      `json:"checkedAt"`
	Occupied      bool                       `json:"occupied"`
	ListenerCount int                        `json:"listenerCount"`
	Listeners     []FirewallPortListenerView `json:"listeners"`
	Error         string                     `json:"error,omitempty"`
}

type FirewallPortListenerView struct {
	Port         int                         `json:"port"`
	Protocol     string                      `json:"protocol"`
	SocketFamily string                      `json:"socketFamily"`
	Stack        string                      `json:"stack"`
	StackSource  string                      `json:"stackSource"`
	BindAddress  string                      `json:"bindAddress"`
	Owners       []FirewallListenerOwnerView `json:"owners"`
}

type FirewallListenerOwnerView struct {
	PID        int    `json:"pid"`
	Name       string `json:"name"`
	Command    string `json:"command"`
	Executable string `json:"executable"`
}

type firewallListenerSnapshot struct {
	supported bool
	checkedAt int64
	listeners []FirewallPortListenerView
	err       string
}

type firewallListenerFilter struct {
	tcpRanges []portRange
	udpRanges []portRange
}

type procListenerSocket struct {
	protocol    string
	family      string
	port        int
	bindAddress string
	wildcard    bool
	inode       string
}

var firewallListenerState = struct {
	mu         sync.Mutex
	lastKey    string
	lastScanAt time.Time
	snapshot   firewallListenerSnapshot
}{}

func buildFirewallRuleListenerStates(rows []model.FirewallRule) map[uint]FirewallRuleListenerState {
	result := make(map[uint]FirewallRuleListenerState, len(rows))
	filter := buildFirewallListenerFilter(rows)
	snapshot := loadFirewallListenerSnapshot(filter)

	for _, row := range rows {
		state := FirewallRuleListenerState{
			Supported: snapshot.supported,
			CheckedAt: snapshot.checkedAt,
			Listeners: []FirewallPortListenerView{},
		}
		if snapshot.err != "" {
			state.Error = snapshot.err
		}
		if !firewallRuleSupportsListenerTracking(row) {
			result[row.Id] = state
			continue
		}

		matched := matchFirewallListenersForRule(row, snapshot.listeners)
		state.ListenerCount = len(matched)
		state.Occupied = len(matched) > 0
		state.Listeners = matched
		result[row.Id] = state
	}

	return result
}

func firewallRuleSupportsListenerTracking(row model.FirewallRule) bool {
	if !row.Enabled {
		return false
	}
	if !firewallProtocolNeedsPort(row.Protocol) {
		return false
	}
	return strings.TrimSpace(row.PortSpec) != ""
}

func buildFirewallListenerFilter(rows []model.FirewallRule) firewallListenerFilter {
	filter := firewallListenerFilter{
		tcpRanges: make([]portRange, 0),
		udpRanges: make([]portRange, 0),
	}
	for _, row := range rows {
		if !firewallRuleSupportsListenerTracking(row) {
			continue
		}
		ranges := parsePortRangeInput(row.PortSpec)
		if len(ranges) == 0 {
			continue
		}
		switch strings.TrimSpace(row.Protocol) {
		case firewallProtocolTCP:
			filter.tcpRanges = append(filter.tcpRanges, ranges...)
		case firewallProtocolUDP:
			filter.udpRanges = append(filter.udpRanges, ranges...)
		case firewallProtocolTCPUDP:
			filter.tcpRanges = append(filter.tcpRanges, ranges...)
			filter.udpRanges = append(filter.udpRanges, ranges...)
		}
	}
	filter.tcpRanges = mergePortRanges(filter.tcpRanges)
	filter.udpRanges = mergePortRanges(filter.udpRanges)
	return filter
}

func (f firewallListenerFilter) empty() bool {
	return len(f.tcpRanges) == 0 && len(f.udpRanges) == 0
}

func (f firewallListenerFilter) key() string {
	return "tcp:" + portRangesToNft(f.tcpRanges) + "|udp:" + portRangesToNft(f.udpRanges)
}

func (f firewallListenerFilter) matches(protocol string, port int) bool {
	switch strings.TrimSpace(protocol) {
	case firewallProtocolTCP:
		return portInRanges(port, f.tcpRanges)
	case firewallProtocolUDP:
		return portInRanges(port, f.udpRanges)
	default:
		return false
	}
}

func loadFirewallListenerSnapshot(filter firewallListenerFilter) firewallListenerSnapshot {
	snapshot := firewallListenerSnapshot{
		supported: runtime.GOOS == "linux",
		checkedAt: time.Now().Unix(),
		listeners: []FirewallPortListenerView{},
	}
	if !snapshot.supported || filter.empty() {
		return snapshot
	}

	key := filter.key()
	firewallListenerState.mu.Lock()
	defer firewallListenerState.mu.Unlock()

	if key == firewallListenerState.lastKey &&
		!firewallListenerState.lastScanAt.IsZero() &&
		time.Since(firewallListenerState.lastScanAt) < firewallListenerSnapshotMinGap {
		return firewallListenerState.snapshot
	}

	next, err := scanFirewallListenerSnapshot(filter)
	if err != nil {
		next = firewallListenerSnapshot{
			supported: true,
			checkedAt: time.Now().Unix(),
			listeners: []FirewallPortListenerView{},
			err:       err.Error(),
		}
	}

	firewallListenerState.lastKey = key
	firewallListenerState.lastScanAt = time.Now()
	firewallListenerState.snapshot = next
	return next
}

func scanFirewallListenerSnapshot(filter firewallListenerFilter) (firewallListenerSnapshot, error) {
	sockets, err := readProcListenerSockets(filter)
	if err != nil {
		return firewallListenerSnapshot{}, err
	}

	targetInodes := make(map[string]struct{}, len(sockets))
	for _, socket := range sockets {
		if socket.inode == "" {
			continue
		}
		targetInodes[socket.inode] = struct{}{}
	}

	ownersByInode := resolveProcListenerOwners(targetInodes)
	bindV6OnlyDefault, bindV6OnlyKnown := readIPv6BindV6OnlyDefault()

	listeners := make([]FirewallPortListenerView, 0, len(sockets))
	for _, socket := range sockets {
		stack, stackSource := resolveProcListenerStack(socket, bindV6OnlyDefault, bindV6OnlyKnown)
		listener := FirewallPortListenerView{
			Port:         socket.port,
			Protocol:     socket.protocol,
			SocketFamily: socket.family,
			Stack:        stack,
			StackSource:  stackSource,
			BindAddress:  socket.bindAddress,
			Owners:       ownersByInode[socket.inode],
		}
		if listener.BindAddress == "" {
			listener.BindAddress = "*"
		}
		listeners = append(listeners, listener)
	}

	sort.SliceStable(listeners, func(i, j int) bool {
		if listeners[i].Port != listeners[j].Port {
			return listeners[i].Port < listeners[j].Port
		}
		if listeners[i].Protocol != listeners[j].Protocol {
			return listeners[i].Protocol < listeners[j].Protocol
		}
		if listeners[i].Stack != listeners[j].Stack {
			return listeners[i].Stack < listeners[j].Stack
		}
		return listeners[i].BindAddress < listeners[j].BindAddress
	})

	return firewallListenerSnapshot{
		supported: true,
		checkedAt: time.Now().Unix(),
		listeners: listeners,
	}, nil
}

func readProcListenerSockets(filter firewallListenerFilter) ([]procListenerSocket, error) {
	specs := []struct {
		path     string
		protocol string
		family   string
		tcp      bool
	}{
		{path: procTCP, protocol: firewallProtocolTCP, family: firewallFamilyIPv4, tcp: true},
		{path: procTCP6, protocol: firewallProtocolTCP, family: firewallFamilyIPv6, tcp: true},
		{path: procUDP, protocol: firewallProtocolUDP, family: firewallFamilyIPv4, tcp: false},
		{path: procUDP6, protocol: firewallProtocolUDP, family: firewallFamilyIPv6, tcp: false},
	}

	sockets := make([]procListenerSocket, 0, 16)
	for _, spec := range specs {
		data, err := readFileFresh(spec.path)
		if err != nil {
			return nil, err
		}
		lines := strings.Split(string(data), "\n")
		for i := 1; i < len(lines); i++ {
			line := strings.TrimSpace(lines[i])
			if line == "" {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 10 {
				continue
			}

			localField := fields[1]
			remoteField := fields[2]
			stateField := strings.TrimSpace(fields[3])
			if spec.tcp {
				if stateField != "0A" {
					continue
				}
			} else if !procSocketRemoteIsWildcard(remoteField) {
				continue
			}

			port, ok := parseLocalPortHex(localField)
			if !ok || !filter.matches(spec.protocol, port) {
				continue
			}

			bindAddress, wildcard := decodeProcSocketLocalAddress(localField, spec.family)
			sockets = append(sockets, procListenerSocket{
				protocol:    spec.protocol,
				family:      spec.family,
				port:        port,
				bindAddress: bindAddress,
				wildcard:    wildcard,
				inode:       strings.TrimSpace(fields[9]),
			})
		}
	}
	return sockets, nil
}

func procSocketRemoteIsWildcard(raw string) bool {
	index := strings.LastIndex(raw, ":")
	if index <= 0 || index+1 >= len(raw) {
		return false
	}
	addressHex := strings.TrimSpace(raw[:index])
	portHex := strings.TrimSpace(raw[index+1:])
	if portHex != "0000" {
		return false
	}
	for _, ch := range addressHex {
		if ch != '0' {
			return false
		}
	}
	return true
}

func decodeProcSocketLocalAddress(raw string, family string) (string, bool) {
	index := strings.LastIndex(raw, ":")
	if index <= 0 {
		return "", false
	}
	addressHex := strings.TrimSpace(raw[:index])
	switch family {
	case firewallFamilyIPv4:
		addr, ok := decodeProcIPv4Address(addressHex)
		if !ok {
			return "", isZeroHexString(addressHex)
		}
		return addr.String(), addr.IsUnspecified()
	case firewallFamilyIPv6:
		addr, ok := decodeProcIPv6Address(addressHex)
		if !ok {
			return "", isZeroHexString(addressHex)
		}
		if addr.Is4In6() {
			unmapped := addr.Unmap()
			return unmapped.String(), unmapped.IsUnspecified()
		}
		return addr.String(), addr.IsUnspecified()
	default:
		return "", false
	}
}

func decodeProcIPv4Address(hexAddress string) (netip.Addr, bool) {
	if len(hexAddress) != 8 {
		return netip.Addr{}, false
	}
	var octets [4]byte
	for i := 0; i < 4; i++ {
		start := i * 2
		value, err := strconv.ParseUint(hexAddress[start:start+2], 16, 8)
		if err != nil {
			return netip.Addr{}, false
		}
		octets[3-i] = byte(value)
	}
	return netip.AddrFrom4(octets), true
}

func decodeProcIPv6Address(hexAddress string) (netip.Addr, bool) {
	if len(hexAddress) != 32 {
		return netip.Addr{}, false
	}
	var bytes16 [16]byte
	for block := 0; block < 4; block++ {
		blockStart := block * 8
		for i := 0; i < 4; i++ {
			start := blockStart + (i * 2)
			value, err := strconv.ParseUint(hexAddress[start:start+2], 16, 8)
			if err != nil {
				return netip.Addr{}, false
			}
			bytes16[(block*4)+(3-i)] = byte(value)
		}
	}
	return netip.AddrFrom16(bytes16), true
}

func isZeroHexString(value string) bool {
	for _, ch := range strings.TrimSpace(value) {
		if ch != '0' {
			return false
		}
	}
	return value != ""
}

func readIPv6BindV6OnlyDefault() (bool, bool) {
	data, err := readFileFresh("/proc/sys/net/ipv6/bindv6only")
	if err != nil {
		return false, false
	}
	value := strings.TrimSpace(string(data))
	switch value {
	case "0":
		return false, true
	case "1":
		return true, true
	default:
		return false, false
	}
}

func resolveProcListenerStack(socket procListenerSocket, bindV6OnlyDefault bool, bindV6OnlyKnown bool) (string, string) {
	switch socket.family {
	case firewallFamilyIPv4:
		return firewallFamilyIPv4, "exact"
	case firewallFamilyIPv6:
		if !socket.wildcard {
			return firewallFamilyIPv6, "exact"
		}
		if bindV6OnlyKnown {
			if bindV6OnlyDefault {
				return firewallFamilyIPv6, "inferred"
			}
			return firewallFamilyDual, "inferred"
		}
		return firewallFamilyIPv6, "unknown"
	default:
		return firewallFamilyDual, "unknown"
	}
}

func resolveProcListenerOwners(targetInodes map[string]struct{}) map[string][]FirewallListenerOwnerView {
	result := make(map[string][]FirewallListenerOwnerView, len(targetInodes))
	if len(targetInodes) == 0 {
		return result
	}

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return result
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}

		fdEntries, fdErr := os.ReadDir(filepath.Join("/proc", entry.Name(), "fd"))
		if fdErr != nil {
			continue
		}

		matched := make(map[string]struct{})
		for _, fdEntry := range fdEntries {
			linkTarget, linkErr := os.Readlink(filepath.Join("/proc", entry.Name(), "fd", fdEntry.Name()))
			if linkErr != nil {
				continue
			}
			inode, ok := extractSocketInode(linkTarget)
			if !ok {
				continue
			}
			if _, exists := targetInodes[inode]; !exists {
				continue
			}
			matched[inode] = struct{}{}
		}
		if len(matched) == 0 {
			continue
		}

		owner := readProcOwnerInfo(pid)
		for inode := range matched {
			result[inode] = appendFirewallListenerOwner(result[inode], owner)
		}
	}

	for inode, owners := range result {
		sort.SliceStable(owners, func(i, j int) bool {
			if owners[i].PID != owners[j].PID {
				return owners[i].PID < owners[j].PID
			}
			return owners[i].Name < owners[j].Name
		})
		result[inode] = owners
	}

	return result
}

func extractSocketInode(linkTarget string) (string, bool) {
	trimmed := strings.TrimSpace(linkTarget)
	if !strings.HasPrefix(trimmed, "socket:[") || !strings.HasSuffix(trimmed, "]") {
		return "", false
	}
	inode := strings.TrimSuffix(strings.TrimPrefix(trimmed, "socket:["), "]")
	inode = strings.TrimSpace(inode)
	if inode == "" {
		return "", false
	}
	return inode, true
}

func readProcOwnerInfo(pid int) FirewallListenerOwnerView {
	owner := FirewallListenerOwnerView{
		PID: pid,
	}
	pidText := strconv.Itoa(pid)

	if data, err := readFileFresh(filepath.Join("/proc", pidText, "comm")); err == nil {
		owner.Name = strings.TrimSpace(string(data))
	}
	if data, err := readFileFresh(filepath.Join("/proc", pidText, "cmdline")); err == nil {
		command := strings.ReplaceAll(string(data), "\x00", " ")
		owner.Command = strings.TrimSpace(command)
	}
	if target, err := os.Readlink(filepath.Join("/proc", pidText, "exe")); err == nil {
		owner.Executable = strings.TrimSpace(target)
	}

	if owner.Name == "" && owner.Executable != "" {
		owner.Name = filepath.Base(owner.Executable)
	}
	if owner.Name == "" && owner.Command != "" {
		parts := strings.Fields(owner.Command)
		if len(parts) > 0 {
			owner.Name = filepath.Base(parts[0])
		}
	}
	if owner.Name == "" {
		owner.Name = "pid-" + pidText
	}
	return owner
}

func appendFirewallListenerOwner(owners []FirewallListenerOwnerView, next FirewallListenerOwnerView) []FirewallListenerOwnerView {
	for _, owner := range owners {
		if owner.PID == next.PID {
			return owners
		}
	}
	return append(owners, next)
}

func matchFirewallListenersForRule(row model.FirewallRule, listeners []FirewallPortListenerView) []FirewallPortListenerView {
	ranges := parsePortRangeInput(row.PortSpec)
	if len(ranges) == 0 {
		return nil
	}

	matched := make([]FirewallPortListenerView, 0)
	for _, listener := range listeners {
		if !firewallRuleMatchesListenerProtocol(row.Protocol, listener.Protocol) {
			continue
		}
		if !portInRanges(listener.Port, ranges) {
			continue
		}
		matched = append(matched, listener)
	}
	return matched
}

func firewallRuleMatchesListenerProtocol(ruleProtocol string, listenerProtocol string) bool {
	switch strings.TrimSpace(ruleProtocol) {
	case firewallProtocolTCP:
		return listenerProtocol == firewallProtocolTCP
	case firewallProtocolUDP:
		return listenerProtocol == firewallProtocolUDP
	case firewallProtocolTCPUDP:
		return listenerProtocol == firewallProtocolTCP || listenerProtocol == firewallProtocolUDP
	default:
		return false
	}
}

func portInRanges(port int, ranges []portRange) bool {
	for _, current := range ranges {
		if port >= current.start && port <= current.end {
			return true
		}
	}
	return false
}
