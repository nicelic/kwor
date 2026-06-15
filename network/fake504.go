package network

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
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

func writeFake504Response(conn net.Conn) error {
	body := fake504HTML
	date := time.Now().UTC().Format(http.TimeFormat)
	resp := fmt.Sprintf(
		"HTTP/1.1 504 Gateway Time-out\r\nServer: nginx\r\nDate: %s\r\nContent-Type: text/html\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
		date,
		len(body),
		body,
	)
	_, err := io.WriteString(conn, resp)
	return err
}
