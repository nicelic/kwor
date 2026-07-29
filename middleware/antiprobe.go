package middleware

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	fakeProbeDelay         = 3 * time.Second
	fakeProbeMaxConcurrent = 32
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

var fakeProbeSlots = make(chan struct{}, fakeProbeMaxConcurrent)

// DelayedFake504 keeps the anti-probe response while bounding the number of
// requests that can occupy a goroutine. Waiting is tied to the request
// context, so a disconnected client releases its slot immediately.
func DelayedFake504(c *gin.Context) {
	if c == nil {
		return
	}
	select {
	case fakeProbeSlots <- struct{}{}:
		defer func() { <-fakeProbeSlots }()
	default:
		writeFake504(c)
		return
	}

	timer := time.NewTimer(fakeProbeDelay)
	defer timer.Stop()
	if c.Request != nil {
		select {
		case <-timer.C:
			writeFake504(c)
		case <-c.Request.Context().Done():
			c.Abort()
		}
		return
	}
	<-timer.C
	writeFake504(c)
}

func writeFake504(c *gin.Context) {
	c.Header("Server", "nginx")
	c.Header("Connection", "close")
	c.Data(http.StatusGatewayTimeout, "text/html; charset=utf-8", []byte(fake504HTML))
	c.Abort()
}
