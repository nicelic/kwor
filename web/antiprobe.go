package web

import (
	"net/http"
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

// fake504Handler delays 30 seconds then returns a fake 504 Gateway Timeout page.
// This prevents probing/fingerprinting of the server.
func fake504Handler(c *gin.Context) {
	time.Sleep(fake504Delay)
	c.Header("Server", "nginx")
	c.Header("Connection", "close")
	c.Data(http.StatusGatewayTimeout, "text/html", []byte(fake504HTML))
	c.Abort()
}
