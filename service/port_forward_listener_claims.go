package service

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"

	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util/common"
	"gorm.io/gorm"
)

// portForwardListenerClaim is the shared declaration model used before a
// forwarding rule or another managed listener is saved. It represents intent,
// not a live socket; live sockets are reported separately in overview.
type portForwardListenerClaim struct {
	label       string
	source      string
	bindAddress string
	family      string
	protocol    string
	spans       []portSpan
}

func collectPortForwardListenerClaims(db *gorm.DB) ([]portForwardListenerClaim, error) {
	if db == nil {
		return nil, nil
	}
	claims := make([]portForwardListenerClaim, 0)
	appendClaim := func(claim portForwardListenerClaim) {
		if len(claim.spans) == 0 || !validPortForwardClaimProtocol(claim.protocol) {
			return
		}
		if claim.family == "" {
			claim.family = portForwardFamilyDual
		}
		claims = append(claims, claim)
	}

	settings, err := loadPortForwardListenerSettings(db)
	if err != nil {
		return nil, err
	}
	if port := portForwardSettingPort(settings, "webPort"); port > 0 {
		appendClaim(portForwardListenerClaim{
			label:       "面板",
			source:      "panel",
			bindAddress: settings["webListen"],
			family:      portForwardDeclaredBindFamily(settings["webListen"]),
			protocol:    portForwardProtocolTCP,
			spans:       []portSpan{{start: port, end: port}},
		})
	}
	if port := portForwardSettingPort(settings, "subPort"); port > 0 {
		appendClaim(portForwardListenerClaim{
			label:       "订阅",
			source:      "subscription",
			bindAddress: settings["subListen"],
			family:      portForwardDeclaredBindFamily(settings["subListen"]),
			protocol:    portForwardProtocolTCP,
			spans:       []portSpan{{start: port, end: port}},
		})
	}

	reverseRules := make([]model.ReverseProxyRule, 0)
	if err := db.Select(
		"id",
		"name",
		"enabled",
		"listen_protocol",
		"listen_protocol_alias",
		"listen_port",
		"listen_http_version_strategy",
	).Where("enabled = ?", true).Find(&reverseRules).Error; err != nil {
		return nil, err
	}
	for _, row := range reverseRules {
		if row.ListenPort < 1 || row.ListenPort > 65535 {
			continue
		}
		alias := normalizeReverseProxyProtocolAlias(row.ListenProtocolAlias, row.ListenProtocol)
		tcp, udp := reverseProxyListenerUsesUnderlyingSockets(row.ListenProtocol, row.ListenHTTPVersionStrategy, alias)
		protocol := portForwardProtocolFromFlags(tcp, udp)
		if protocol == "" {
			continue
		}
		listenIPs := reverseProxyHTTPRuntimeListenIPs(&row)
		if reverseProxyProtocolIsDNS(alias) {
			listenIPs = reverseProxyDNSRuntimeListenIPs(&row)
		}
		for _, listenIP := range listenIPs {
			appendClaim(portForwardListenerClaim{
				label:       "反向代理 " + strings.TrimSpace(row.Name),
				source:      "reverse_proxy",
				bindAddress: listenIP,
				family:      portForwardDeclaredBindFamily(listenIP),
				protocol:    protocol,
				spans:       []portSpan{{start: row.ListenPort, end: row.ListenPort}},
			})
		}
	}

	defaultInbounds := make([]model.Inbound, 0)
	if err := db.Select("type", "tag", "options", "addrs", "out_json").Find(&defaultInbounds).Error; err != nil {
		return nil, err
	}
	for _, inbound := range defaultInbounds {
		appendPortForwardInboundClaims(&claims, false, inbound.Type, inbound.Tag, inbound.Options, inbound.Addrs, inbound.OutJson)
	}
	mihomoInbounds := make([]model.MihomoInbound, 0)
	if err := db.Select("type", "tag", "options", "addrs", "out_json").Find(&mihomoInbounds).Error; err != nil {
		return nil, err
	}
	for _, inbound := range mihomoInbounds {
		appendPortForwardInboundClaims(&claims, true, inbound.Type, inbound.Tag, inbound.Options, inbound.Addrs, inbound.OutJson)
	}

	sort.SliceStable(claims, func(i, j int) bool {
		if claims[i].source != claims[j].source {
			return claims[i].source < claims[j].source
		}
		if claims[i].label != claims[j].label {
			return claims[i].label < claims[j].label
		}
		if claims[i].bindAddress != claims[j].bindAddress {
			return claims[i].bindAddress < claims[j].bindAddress
		}
		return formatPortForwardSpans(claims[i].spans) < formatPortForwardSpans(claims[j].spans)
	})
	return claims, nil
}

func loadPortForwardListenerSettings(db *gorm.DB) (map[string]string, error) {
	values := map[string]string{
		"webListen": defaultValueMap["webListen"],
		"webPort":   defaultValueMap["webPort"],
		"subListen": defaultValueMap["subListen"],
		"subPort":   defaultValueMap["subPort"],
	}
	rows := make([]model.Setting, 0, len(values))
	if err := db.Where("key IN ?", []string{"webListen", "webPort", "subListen", "subPort"}).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		values[row.Key] = strings.TrimSpace(row.Value)
	}
	return values, nil
}

func portForwardSettingPort(values map[string]string, key string) int {
	port, err := strconv.Atoi(strings.TrimSpace(values[key]))
	if err != nil || port < 1 || port > 65535 {
		return 0
	}
	return port
}

func appendPortForwardInboundClaims(claims *[]portForwardListenerClaim, mihomo bool, inboundType string, tag string, options json.RawMessage, addrs json.RawMessage, outJSON json.RawMessage) {
	if claims == nil {
		return
	}
	tcp, udp := portForwardInboundListenerProtocols(inboundType, options)
	protocol := portForwardProtocolFromFlags(tcp, udp)
	if protocol == "" {
		return
	}
	label := buildPortForwardReservedLabel(mihomo, inboundType, tag)
	bindAddress := portForwardInboundBindAddress(options, addrs, outJSON)
	family := portForwardDeclaredBindFamily(bindAddress)
	appendClaim := func(spans []portSpan, source string) {
		if len(spans) == 0 {
			return
		}
		*claims = append(*claims, portForwardListenerClaim{
			label:       label,
			source:      source,
			bindAddress: bindAddress,
			family:      family,
			protocol:    protocol,
			spans:       spans,
		})
	}
	if port := portForwardExtractListenPort(options, outJSON); port > 0 {
		appendClaim([]portSpan{{start: port, end: port}}, "inbound")
	}
	if rawRange := strings.TrimSpace(extractPortHopRange(options)); rawRange != "" {
		if spans, _, err := parseStrictPortRanges(rawRange); err == nil {
			appendClaim(spans, "port_hop")
		}
	}
	if mihomo {
		// Mieru and Mihomo Hysteria compatibility can render a separate
		// redirect range from OutJson; include it even when Options omitted it.
		if rawRange, _ := resolveMihomoInboundRedirectSpec(&model.MihomoInbound{Type: inboundType, Tag: tag, Options: options, OutJson: outJSON}); strings.TrimSpace(rawRange) != "" {
			if spans, _, err := parseStrictPortRanges(rawRange); err == nil {
				appendClaim(spans, "port_hop")
			}
		}
	}
}

func portForwardExtractListenPort(options json.RawMessage, outJSON json.RawMessage) int {
	if port := extractPort(options); port > 0 {
		return port
	}
	for _, raw := range []json.RawMessage{outJSON, options} {
		var values map[string]json.RawMessage
		if err := json.Unmarshal(raw, &values); err != nil {
			continue
		}
		for _, key := range []string{"listen_port", "port"} {
			var port int
			if value, exists := values[key]; exists && json.Unmarshal(value, &port) == nil && port >= 1 && port <= 65535 {
				return port
			}
		}
	}
	return 0
}

func portForwardInboundBindAddress(options json.RawMessage, addrs json.RawMessage, outJSON json.RawMessage) string {
	for _, raw := range []json.RawMessage{options, outJSON, addrs} {
		var values map[string]json.RawMessage
		if err := json.Unmarshal(raw, &values); err != nil {
			continue
		}
		for _, key := range []string{"listen", "listen_address", "listen_ip", "address"} {
			var value string
			if item, exists := values[key]; exists && json.Unmarshal(item, &value) == nil && strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}

func portForwardInboundListenerProtocols(inboundType string, options json.RawMessage) (bool, bool) {
	var values map[string]json.RawMessage
	_ = json.Unmarshal(options, &values)
	if raw, exists := values["network"]; exists {
		var network string
		if json.Unmarshal(raw, &network) == nil {
			return portForwardNetworkProtocols(network)
		}
		var networks []string
		if json.Unmarshal(raw, &networks) == nil {
			tcp := false
			udp := false
			for _, item := range networks {
				currentTCP, currentUDP := portForwardNetworkProtocols(item)
				tcp = tcp || currentTCP
				udp = udp || currentUDP
			}
			if tcp || udp {
				return tcp, udp
			}
		}
	}
	switch strings.ToLower(strings.TrimSpace(inboundType)) {
	case "hysteria", "hysteria2", "tuic", "shadowquic", "wireguard":
		return false, true
	case "shadowsocks", "socks", "mixed", "mieru", "dns":
		return true, true
	case "tun", "tproxy", "redirect", "direct":
		return false, false
	default:
		return true, false
	}
}

func portForwardNetworkProtocols(raw string) (bool, bool) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return false, false
	}
	tcp := strings.Contains(value, "tcp")
	udp := strings.Contains(value, "udp")
	if value == "both" || value == "tcp_udp" || value == "tcp/udp" || value == "tcp+udp" || value == "mixed" {
		return true, true
	}
	return tcp, udp
}

func portForwardProtocolFromFlags(tcp bool, udp bool) string {
	switch {
	case tcp && udp:
		return portForwardProtocolTCPUDP
	case tcp:
		return portForwardProtocolTCP
	case udp:
		return portForwardProtocolUDP
	default:
		return ""
	}
}

func validPortForwardClaimProtocol(protocol string) bool {
	flags := portForwardProtocolFlagsFor(protocol)
	return flags.tcp || flags.udp
}

func portForwardDeclaredBindFamily(raw string) string {
	value := strings.TrimSpace(strings.Trim(raw, "[]"))
	if value == "" {
		return portForwardFamilyDual
	}
	addr, err := netip.ParseAddr(value)
	if err != nil {
		return portForwardFamilyDual
	}
	if addr.Is4() || addr.Is4In6() {
		return portForwardFamilyIPv4
	}
	if addr.IsUnspecified() {
		if bindV6Only, known := readIPv6BindV6OnlyDefault(); known && !bindV6Only {
			return portForwardFamilyDual
		}
	}
	return portForwardFamilyIPv6
}

func validatePortForwardListenerClaimsAgainstActiveRules(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	claims, err := collectPortForwardListenerClaims(db)
	if err != nil {
		return err
	}
	rules := make([]model.PortForwardRule, 0)
	if err := db.Select(
		"id",
		"name",
		"family",
		"protocol",
		"local_port_mode",
		"local_port_spec",
		"local_port_start",
		"local_port_count",
		"local_port_end",
	).Where("enabled = ?", true).Find(&rules).Error; err != nil {
		return err
	}
	for _, rule := range rules {
		spans := portForwardRuleSpans(rule)
		if len(spans) == 0 {
			continue
		}
		for _, claim := range claims {
			if !portForwardProtocolsOverlap(rule.Protocol, claim.protocol) || !portForwardFamiliesOverlap(rule.Family, claim.family) {
				continue
			}
			overlap := summarizePortForwardSpanOverlap(spans, claim.spans)
			if overlap.count == 0 {
				continue
			}
			return common.NewError(
				"listener ", claim.label,
				" conflicts with forwarding rule ", strings.TrimSpace(rule.Name),
				" (", portForwardFormatOverlap(overlap), ")",
			)
		}
	}
	if err := validateManagedListenerClaimPairs(claims); err != nil {
		return err
	}
	return nil
}

func validateManagedListenerClaimPairs(claims []portForwardListenerClaim) error {
	for i := range claims {
		for j := i + 1; j < len(claims); j++ {
			left := claims[i]
			right := claims[j]
			// Rules belonging to the same managed runtime are checked by that
			// runtime's route validator, which may intentionally share a DoH path
			// listener. Cross-component listeners cannot share a socket.
			if left.source == right.source {
				continue
			}
			if !portForwardProtocolsOverlap(left.protocol, right.protocol) || !portForwardFamiliesOverlap(left.family, right.family) {
				continue
			}
			if !portForwardClaimAddressesOverlap(left, right) {
				continue
			}
			overlap := summarizePortForwardSpanOverlap(left.spans, right.spans)
			if overlap.count == 0 {
				continue
			}
			return common.NewError(
				"listener ", left.label,
				" conflicts with listener ", right.label,
				" (", portForwardFormatOverlap(overlap), ")",
			)
		}
	}
	return nil
}

func portForwardClaimAddressesOverlap(left portForwardListenerClaim, right portForwardListenerClaim) bool {
	leftValue := strings.Trim(strings.TrimSpace(left.bindAddress), "[]")
	rightValue := strings.Trim(strings.TrimSpace(right.bindAddress), "[]")
	if leftValue == "" || rightValue == "" {
		return true
	}
	leftAddr, leftErr := netip.ParseAddr(leftValue)
	rightAddr, rightErr := netip.ParseAddr(rightValue)
	if leftErr != nil || rightErr != nil {
		// A non-literal managed bind cannot be proven disjoint before runtime.
		return true
	}
	if leftAddr.IsUnspecified() || rightAddr.IsUnspecified() {
		return portForwardFamiliesOverlap(left.family, right.family)
	}
	return leftAddr.Unmap() == rightAddr.Unmap()
}

func formatPortForwardSpans(spans []portSpan) string {
	parts := make([]string, 0, len(spans))
	for _, span := range spans {
		if span.start == span.end {
			parts = append(parts, strconv.Itoa(span.start))
			continue
		}
		parts = append(parts, fmt.Sprintf("%d-%d", span.start, span.end))
	}
	return strings.Join(parts, ",")
}
