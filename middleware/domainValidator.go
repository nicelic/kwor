package middleware

import (
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const fake504HTML = `<!DOCTYPE html>
<html>
<head><title>504 Gateway Time-out</title></head>
<body>
<center><h1>504 Gateway Time-out</h1></center>
<hr><center>nginx</center>
</body>
</html>
`

const fake504Delay = 30 * time.Second

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
			time.Sleep(fake504Delay)
			c.Header("Server", "nginx")
			c.Header("Connection", "close")
			c.Data(http.StatusGatewayTimeout, "text/html", []byte(fake504HTML))
			c.Abort()
			return
		}

		c.Next()
	}
}
