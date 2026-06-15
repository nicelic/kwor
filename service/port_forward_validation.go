package service

import (
	"fmt"
	"net/netip"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util/common"
	"gorm.io/gorm"
)

const (
	portForwardLoopbackIPv4 = "127.0.0.1"
	portForwardLoopbackIPv6 = "::1"
)

var (
	portForwardRuntimeGOOS = func() string {
		return runtime.GOOS
	}
	portForwardReadSocketSnapshot = readSocketSnapshot
)

type portForwardReservedRange struct {
	label      string
	normalized string
	tcp        bool
	udp        bool
	spans      []portSpan
}

func (r portForwardReservedRange) matchesProtocol(protocol string) bool {
	reservedProtocol := ""
	switch {
	case r.tcp && r.udp:
		reservedProtocol = portForwardProtocolTCPUDP
	case r.tcp:
		reservedProtocol = portForwardProtocolTCP
	case r.udp:
		reservedProtocol = portForwardProtocolUDP
	default:
		return false
	}
	return portForwardProtocolsOverlap(protocol, reservedProtocol)
}

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

	reservedRanges, err := collectPortForwardReservedRanges(db)
	if err != nil {
		return err
	}
	for _, reserved := range reservedRanges {
		if !reserved.matchesProtocol(row.protocol) {
			continue
		}
		overlap := collectPortForwardSpanOverlapPorts(row.localPortSpans, reserved.spans)
		if len(overlap) == 0 {
			continue
		}
		issues = append(issues, fmt.Sprintf(
			"conflicts with %s port-hop range %s (overlap: %s)",
			reserved.label,
			reserved.normalized,
			portForwardFormatPortSample(overlap),
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
	snapshot, err := portForwardReadSocketSnapshot()
	if err != nil {
		return nil, nil, err
	}
	spans := row.localPortSpans
	if len(spans) == 0 {
		spans = []portSpan{{start: row.localPortStart, end: row.localPortEnd}}
	}

	flags := portForwardProtocolFlagsFor(row.protocol)
	occupiedTCP := make([]int, 0)
	occupiedUDP := make([]int, 0)
	if flags.tcp {
		occupiedTCP = collectOccupiedPorts(snapshot.tcp, spans)
	}
	if flags.udp {
		occupiedUDP = collectOccupiedPorts(snapshot.udp, spans)
	}
	return occupiedTCP, occupiedUDP, nil
}

func collectPortForwardSpanOverlapPorts(a []portSpan, b []portSpan) []int {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	out := make([]int, 0)
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
			for port := start; port <= end; port++ {
				out = append(out, port)
			}
		}
	}
	return out
}

func collectPortForwardReservedRanges(db *gorm.DB) ([]portForwardReservedRange, error) {
	if db == nil {
		return nil, nil
	}

	result := make([]portForwardReservedRange, 0)

	defaultInbounds := make([]model.Inbound, 0)
	if err := db.Select("id, type, tag, options").
		Where("type IN ?", []string{"hysteria", "hysteria2"}).
		Find(&defaultInbounds).Error; err != nil {
		return nil, err
	}
	for _, inbound := range defaultInbounds {
		rangeText := strings.TrimSpace(extractPortHopRange(inbound.Options))
		if rangeText == "" {
			continue
		}
		spans, normalized, err := parseStrictPortRanges(rangeText)
		if err != nil || len(spans) == 0 {
			continue
		}
		result = append(result, portForwardReservedRange{
			label:      buildPortForwardReservedLabel(false, inbound.Type, inbound.Tag),
			normalized: normalized,
			udp:        true,
			spans:      spans,
		})
	}

	mihomoInbounds := make([]model.MihomoInbound, 0)
	if err := db.Select("id, type, tag, options, out_json").
		Where("type IN ?", []string{"hysteria", "hysteria2", "mieru"}).
		Find(&mihomoInbounds).Error; err != nil {
		return nil, err
	}
	for _, inbound := range mihomoInbounds {
		rangeText, redirectTCP := resolveMihomoInboundRedirectSpec(&inbound)
		rangeText = strings.TrimSpace(rangeText)
		if rangeText == "" {
			continue
		}
		spans, normalized, err := parseStrictPortRanges(rangeText)
		if err != nil || len(spans) == 0 {
			continue
		}
		result = append(result, portForwardReservedRange{
			label:      buildPortForwardReservedLabel(true, inbound.Type, inbound.Tag),
			normalized: normalized,
			tcp:        redirectTCP,
			udp:        true,
			spans:      spans,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].label == result[j].label {
			return result[i].normalized < result[j].normalized
		}
		return result[i].label < result[j].label
	})
	return result, nil
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

func collectPortForwardOverlapPorts(start int, end int, spans []portSpan) []int {
	if start < 1 || end < start || len(spans) == 0 {
		return nil
	}
	out := make([]int, 0)
	for _, span := range spans {
		overlapStart := start
		if span.start > overlapStart {
			overlapStart = span.start
		}
		overlapEnd := end
		if span.end < overlapEnd {
			overlapEnd = span.end
		}
		if overlapStart > overlapEnd {
			continue
		}
		for port := overlapStart; port <= overlapEnd; port++ {
			out = append(out, port)
		}
	}
	return out
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
