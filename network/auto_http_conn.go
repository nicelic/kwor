package network

import (
	"io"
	"net"
	"sync"
)

type AutoHttpConn struct {
	net.Conn

	firstBuf []byte
	bufStart int

	readRequestOnce sync.Once
	protocolBlocked bool
}

func NewAutoHttpConn(conn net.Conn) net.Conn {
	return &AutoHttpConn{
		Conn: conn,
	}
}

func (c *AutoHttpConn) Unwrap() net.Conn {
	return c.Conn
}

func (c *AutoHttpConn) readRequest() bool {
	c.firstBuf = make([]byte, 2048)
	n, err := c.Conn.Read(c.firstBuf)
	c.firstBuf = c.firstBuf[:n]
	if err != nil {
		return false
	}

	if len(c.firstBuf) >= 3 && c.firstBuf[0] == 0x16 && c.firstBuf[1] == 0x03 {
		c.protocolBlocked = true
		c.firstBuf = nil
		c.Close()
		return true
	}

	return false
}

func (c *AutoHttpConn) Read(buf []byte) (int, error) {
	c.readRequestOnce.Do(func() {
		c.readRequest()
	})

	if c.protocolBlocked {
		return 0, io.EOF
	}

	if c.firstBuf != nil {
		n := copy(buf, c.firstBuf[c.bufStart:])
		c.bufStart += n
		if c.bufStart >= len(c.firstBuf) {
			c.firstBuf = nil
		}
		return n, nil
	}

	return c.Conn.Read(buf)
}
