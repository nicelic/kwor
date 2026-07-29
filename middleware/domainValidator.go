package middleware

import (
	"net"
	"strings"

	"github.com/gin-gonic/gin"
)

func DomainValidator(domain string) gin.HandlerFunc {
	domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))

	return func(c *gin.Context) {
		if IsLocalWhitelistHost(c.Request.Host) {
			c.Next()
			return
		}

		host := normalizeHost(c.Request.Host)
		host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))

		// Always allow direct IP access (IPv4/IPv6), including self-signed TLS scenarios.
		if net.ParseIP(host) != nil {
			c.Next()
			return
		}

		if domain != "" && host != domain {
			DelayedFake504(c)
			return
		}

		c.Next()
	}
}
