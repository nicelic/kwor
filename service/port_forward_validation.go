package service

import (
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"

	"github.com/alireza0/s-ui/util/common"
	"gorm.io/gorm"
)

const (
	portForwardLoopbackIPv4 = "127.0.0.1"
	portForwardLoopbackIPv6 = "::1"
)

var (
	portForwardRuntimeGOOS = func() string {
		return GetSystemPlatformOS()
	}
	portForwardReadSocketConflictSockets    = readProcListenerSockets
	portForwardReadSocketConflictBindV6Only = readIPv6BindV6OnlyDefault
)

func portForwardTargetIsLocal(targetIP string) bool {
	trimmed := strings.TrimSpace(strings.Trim(targetIP, "[]"))
	if trimmed == "" || strings.EqualFold(trimmed, "localhost") {
		return true
	}
	addr, err := netip.ParseAddr(trimmed)
	return err == nil && addr.IsLoopback()
}

func portForwardCanonicalLoopback(family string) string {
	if family == portForwardFamilyIPv6 {
		return portForwardLoopbackIPv6
	}
	return portForwardLoopbackIPv4
}

func normalizePortForwardSelectedFamily(raw string, fallback string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		switch strings.ToLower(strings.TrimSpace(fallback)) {
		case portForwardFamilyIPv6:
			return portForwardFamilyIPv6, nil
		case portForwardFamilyDual, "ipv4ipv6", "ipv4/ipv6":
			return portForwardFamilyDual, nil
		default:
			return portForwardFamilyIPv4, nil
		}
	case portForwardFamilyIPv4:
		return portForwardFamilyIPv4, nil
	case portForwardFamilyIPv6:
		return portForwardFamilyIPv6, nil
	case portForwardFamilyDual, "ipv4ipv6", "ipv4/ipv6":
		return portForwardFamilyDual, nil
	default:
		return "", common.NewError("forward family must be ipv4, ipv6, or dual")
	}
}

func normalizePortForwardTarget(rawTargetIP string, rawFamily string) (string, string, error) {
	trimmed := strings.TrimSpace(strings.Trim(rawTargetIP, "[]"))

	localFamilyHint := ""
	if trimmed != "" {
		if addr, err := netip.ParseAddr(trimmed); err == nil && addr.IsLoopback() {
			if addr.Is6() {
				localFamilyHint = portForwardFamilyIPv6
			} else {
				localFamilyHint = portForwardFamilyIPv4
			}
		}
	}

	if portForwardTargetIsLocal(trimmed) {
		family, err := normalizePortForwardSelectedFamily(rawFamily, localFamilyHint)
		if err != nil {
			return "", "", err
		}
		if family == portForwardFamilyDual {
			if localFamilyHint == portForwardFamilyIPv6 {
				return portForwardLoopbackIPv6, family, nil
			}
			return portForwardLoopbackIPv4, family, nil
		}
		return portForwardCanonicalLoopback(family), family, nil
	}

	targetAddr, err := netip.ParseAddr(trimmed)
	if err != nil {
		return "", "", common.NewError("invalid target ip: ", rawTargetIP)
	}

	expectedFamily := portForwardFamilyIPv4
	if targetAddr.Is6() {
		expectedFamily = portForwardFamilyIPv6
	}
	family, err := normalizePortForwardSelectedFamily(rawFamily, expectedFamily)
	if err != nil {
		return "", "", err
	}
	if family == portForwardFamilyDual {
		return "", "", common.NewError("dual-stack forwarding currently requires local target IP")
	}
	if family != expectedFamily {
		return "", "", common.NewError("target ip family does not match selected forwarding family")
	}
	return targetAddr.String(), family, nil
}

func validatePortForwardRuleAvailability(db *gorm.DB, row normalizedPortForwardRule) error {
	if db == nil || !row.enabled {
		return nil
	}

	issues := make([]string, 0, 4)

	occupiedTCP, occupiedUDP, err := findPortForwardSocketConflicts(row)
	if err != nil {
		return err
	}
	if len(occupiedTCP) > 0 {
		issues = append(issues, fmt.Sprintf(
			"system TCP ports already in use: %s",
			portForwardFormatPortSample(occupiedTCP),
		))
	}
	if len(occupiedUDP) > 0 {
		issues = append(issues, fmt.Sprintf(
			"system UDP ports already in use: %s",
			portForwardFormatPortSample(occupiedUDP),
		))
	}

	claims, err := collectPortForwardListenerClaims(db)
	if err != nil {
		return err
	}
	for _, claim := range claims {
		if !portForwardProtocolsOverlap(row.protocol, claim.protocol) {
			continue
		}
		if !portForwardFamiliesOverlap(row.family, claim.family) {
			continue
		}
		overlap := summarizePortForwardSpanOverlap(row.localPortSpans, claim.spans)
		if overlap.count == 0 {
			continue
		}
		issues = append(issues, fmt.Sprintf(
			"conflicts with %s listener %s (overlap: %s)",
			claim.label,
			formatPortForwardSpans(claim.spans),
			portForwardFormatOverlap(overlap),
		))
	}

	if len(issues) == 0 {
		return nil
	}

	return common.NewError(
		"local ports ", row.localPortSpec,
		" cannot create ", portForwardProtocolDisplay(row.protocol),
		" forwarding rule: ", strings.Join(issues, "; "),
	)
}

func findPortForwardSocketConflicts(row normalizedPortForwardRule) ([]int, []int, error) {
	if portForwardRuntimeGOOS() != "linux" {
		return nil, nil, nil
	}
	spans := row.localPortSpans
	if len(spans) == 0 {
		spans = []portSpan{{start: row.localPortStart, end: row.localPortEnd}}
	}
	filter := firewallListenerFilter{}
	flags := portForwardProtocolFlagsFor(row.protocol)
	for _, span := range spans {
		current := portRange{start: span.start, end: span.end}
		if flags.tcp {
			filter.tcpRanges = append(filter.tcpRanges, current)
		}
		if flags.udp {
			filter.udpRanges = append(filter.udpRanges, current)
		}
	}
	filter.tcpRanges = mergePortRanges(filter.tcpRanges)
	filter.udpRanges = mergePortRanges(filter.udpRanges)
	sockets, err := portForwardReadSocketConflictSockets(filter)
	if err != nil {
		return nil, nil, err
	}
	bindV6Only, bindV6OnlyKnown := portForwardReadSocketConflictBindV6Only()
	occupiedTCPSet := make(map[int]struct{})
	occupiedUDPSet := make(map[int]struct{})
	for _, socket := range sockets {
		stack, _ := resolveProcListenerStack(socket, bindV6Only, bindV6OnlyKnown)
		if !portForwardNormalizedRuleMatchesSocket(row, socket, stack) {
			continue
		}
		switch socket.protocol {
		case firewallProtocolTCP:
			occupiedTCPSet[socket.port] = struct{}{}
		case firewallProtocolUDP:
			occupiedUDPSet[socket.port] = struct{}{}
		}
	}
	occupiedTCP := portForwardSortedPortSet(occupiedTCPSet)
	occupiedUDP := portForwardSortedPortSet(occupiedUDPSet)
	return occupiedTCP, occupiedUDP, nil
}

type portForwardSpanOverlap struct {
	count  int
	sample []int
}

// summarizePortForwardSpanOverlap keeps conflict checks bounded even for a
// whole-port-space declaration. It counts intersections arithmetically and
// stores only a short representative sample for diagnostics.
func summarizePortForwardSpanOverlap(a []portSpan, b []portSpan) portForwardSpanOverlap {
	result := portForwardSpanOverlap{sample: make([]int, 0, 12)}
	for _, left := range a {
		for _, right := range b {
			start := left.start
			if right.start > start {
				start = right.start
			}
			end := left.end
			if right.end < end {
				end = right.end
			}
			if start > end {
				continue
			}
			result.count += end - start + 1
			for port := start; port <= end && len(result.sample) < 12; port++ {
				result.sample = append(result.sample, port)
			}
		}
	}
	return result
}

func collectPortForwardSpanOverlapPorts(a []portSpan, b []portSpan) []int {
	return summarizePortForwardSpanOverlap(a, b).sample
}

func portForwardFormatOverlap(overlap portForwardSpanOverlap) string {
	if overlap.count == 0 {
		return "-"
	}
	value := portForwardFormatPortSample(overlap.sample)
	if overlap.count > len(overlap.sample) {
		return value + " (total " + strconv.Itoa(overlap.count) + ")"
	}
	return value
}

func portForwardSortedPortSet(values map[int]struct{}) []int {
	ports := make([]int, 0, len(values))
	for port := range values {
		ports = append(ports, port)
	}
	sort.Ints(ports)
	return ports
}

func portForwardNormalizedRuleMatchesSocket(row normalizedPortForwardRule, socket procListenerSocket, socketStack string) bool {
	flags := portForwardProtocolFlagsFor(row.protocol)
	if socket.protocol == firewallProtocolTCP && !flags.tcp {
		return false
	}
	if socket.protocol == firewallProtocolUDP && !flags.udp {
		return false
	}
	spans := row.localPortSpans
	if len(spans) == 0 {
		spans = []portSpan{{start: row.localPortStart, end: row.localPortEnd}}
	}
	if !portForwardPortInSpans(socket.port, spans) {
		return false
	}
	family := portForwardFamilyFlagsFor(row.family)
	switch socketStack {
	case firewallFamilyDual:
		return family.ipv4 || family.ipv6
	case firewallFamilyIPv4:
		return family.ipv4
	default:
		return family.ipv6
	}
}

func buildPortForwardReservedLabel(isMihomo bool, inboundType string, tag string) string {
	name := strings.ToUpper(strings.TrimSpace(inboundType))
	switch strings.ToLower(strings.TrimSpace(inboundType)) {
	case "hysteria":
		name = "HY1"
	case "hysteria2":
		name = "HY2"
	case "mieru":
		name = "Mieru"
	}
	if strings.TrimSpace(tag) == "" {
		if isMihomo {
			return "Mihomo " + name
		}
		return name
	}
	if isMihomo {
		return "Mihomo " + name + " [" + tag + "]"
	}
	return name + " [" + tag + "]"
}

func portForwardFormatPortSample(ports []int) string {
	if len(ports) == 0 {
		return "-"
	}
	sort.Ints(ports)
	parts := make([]string, 0, len(ports))
	for _, port := range ports {
		parts = append(parts, strconv.Itoa(port))
	}
	if len(parts) <= 12 {
		return strings.Join(parts, ",")
	}
	return strings.Join(parts[:12], ",") + " ... (total " + strconv.Itoa(len(parts)) + ")"
}
