package middleware

import (
	"net"
	"strings"
)

var localHostWhitelist = map[string]struct{}{
	"127.0.0.1": {},
	"::1":       {},
}

func normalizeHost(hostport string) string {
	host := hostport

	if strings.HasPrefix(hostport, "[") {
		if h, _, err := net.SplitHostPort(hostport); err == nil {
			return h
		}
		return strings.TrimSuffix(strings.TrimPrefix(hostport, "["), "]")
	}

	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}

	return host
}

// IsLocalWhitelistHost reports whether the request Host matches a local whitelist entry.
func IsLocalWhitelistHost(hostport string) bool {
	host := normalizeHost(hostport)
	_, ok := localHostWhitelist[host]
	return ok
}
