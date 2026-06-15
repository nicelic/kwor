package network

import (
	"bytes"
	"net"
	"sync"
)

type AutoHttpsConn struct {
	net.Conn

	firstBuf []byte
	bufStart int

	readRequestOnce sync.Once
}

func NewAutoHttpsConn(conn net.Conn) net.Conn {
	return &AutoHttpsConn{
		Conn: conn,
	}
}

func (c *AutoHttpsConn) Unwrap() net.Conn {
	return c.Conn
}

func (c *AutoHttpsConn) readRequest() bool {
	c.firstBuf = make([]byte, 2048)
	n, err := c.Conn.Read(c.firstBuf)
	c.firstBuf = c.firstBuf[:n]
	if err != nil {
		return false
	}

	if !isLikelyHTTP(c.firstBuf) {
		return false
	}
	c.Close()
	c.firstBuf = nil
	return true
}

func (c *AutoHttpsConn) Read(buf []byte) (int, error) {
	c.readRequestOnce.Do(func() {
		c.readRequest()
	})

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

func isLikelyHTTP(buf []byte) bool {
	methodPrefixes := [][]byte{
		[]byte("GET "),
		[]byte("POST "),
		[]byte("PUT "),
		[]byte("DELETE "),
		[]byte("PATCH "),
		[]byte("HEAD "),
		[]byte("OPTIONS "),
		[]byte("TRACE "),
		[]byte("CONNECT "),
		[]byte("PRI "),
	}
	for _, prefix := range methodPrefixes {
		if bytes.HasPrefix(buf, prefix) {
			return true
		}
	}
	return false
}
