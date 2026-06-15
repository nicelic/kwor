package util

import (
	"encoding/json"
	"net"
	"strconv"
	"strings"

	"github.com/alireza0/s-ui/database/model"
)

func NormalizeSubscriptionServerHost(raw string) string {
	host := strings.TrimSpace(raw)
	if host == "" {
		return ""
	}

	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}

	host = strings.TrimSpace(host)
	if len(host) >= 2 && strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = strings.TrimSpace(host[1 : len(host)-1])
	}

	return host
}

func ResolveSubscriptionServerHost(override string, inbound *model.Inbound, fallback string) string {
	if host := NormalizeSubscriptionServerHost(override); host != "" {
		return host
	}
	if host := resolveSubscriptionServerHostFromInbound(inbound); host != "" {
		return host
	}
	return NormalizeSubscriptionServerHost(fallback)
}

func FormatSubscriptionLinkHost(raw string) string {
	host := NormalizeSubscriptionServerHost(raw)
	if host == "" {
		return ""
	}
	if isSubscriptionIPv6Literal(host) {
		return "[" + host + "]"
	}
	return host
}

func FormatSubscriptionLinkHostPort(raw string, port int) string {
	host := NormalizeSubscriptionServerHost(raw)
	if host == "" {
		return ""
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func resolveSubscriptionServerHostFromInbound(inbound *model.Inbound) string {
	if inbound == nil {
		return ""
	}

	var addrs []map[string]interface{}
	if err := json.Unmarshal(inbound.Addrs, &addrs); err == nil {
		for _, addr := range addrs {
			server, _ := addr["server"].(string)
			if host := NormalizeSubscriptionServerHost(server); host != "" {
				return host
			}
		}
	}

	var outbound map[string]interface{}
	if err := json.Unmarshal(inbound.OutJson, &outbound); err == nil {
		server, _ := outbound["server"].(string)
		if host := NormalizeSubscriptionServerHost(server); host != "" {
			return host
		}
	}

	return ""
}

func isSubscriptionIPv6Literal(raw string) bool {
	host := NormalizeSubscriptionServerHost(raw)
	if host == "" || !strings.Contains(host, ":") {
		return false
	}

	ip := net.ParseIP(host)
	return ip != nil && ip.To4() == nil
}
