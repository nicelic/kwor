package network

import (
	"errors"
	"net"
	"sync"
	"testing"
	"time"
)

type dummyManagedConn struct {
	mu     sync.Mutex
	closed bool
}

func (c *dummyManagedConn) Read(_ []byte) (int, error)         { return 0, errors.New("not implemented") }
func (c *dummyManagedConn) Write(p []byte) (int, error)        { return len(p), nil }
func (c *dummyManagedConn) Close() error                       { c.mu.Lock(); c.closed = true; c.mu.Unlock(); return nil }
func (c *dummyManagedConn) LocalAddr() net.Addr                { return dummyAddr("local") }
func (c *dummyManagedConn) RemoteAddr() net.Addr               { return dummyAddr("remote") }
func (c *dummyManagedConn) SetDeadline(time.Time) error        { return nil }
func (c *dummyManagedConn) SetReadDeadline(time.Time) error    { return nil }
func (c *dummyManagedConn) SetWriteDeadline(time.Time) error   { return nil }
func (c *dummyManagedConn) isClosed() bool                     { c.mu.Lock(); defer c.mu.Unlock(); return c.closed }

type dummyAddr string

func (a dummyAddr) Network() string { return string(a) }
func (a dummyAddr) String() string  { return string(a) }

func TestCloseManagedTLSConnectionsByFingerprintClosesOnlyMatchingConnections(t *testing.T) {
	matched := NewManagedTLSConn(&dummyManagedConn{})
	other := NewManagedTLSConn(&dummyManagedConn{})
	matched.SetFingerprint("match")
	other.SetFingerprint("other")

	conns := map[*ManagedTLSConn]struct{}{
		matched: {},
		other:   {},
	}
	CloseManagedTLSConnectionsByFingerprint(conns, "match", 0)
	waitForManagedTLSConnClosure(t, func() bool {
		return matched.Conn.(*dummyManagedConn).isClosed() && !other.Conn.(*dummyManagedConn).isClosed()
	})
}

func TestCloseManagedTLSConnectionsHonorsGenerationCutoff(t *testing.T) {
	oldConn := NewManagedTLSConn(&dummyManagedConn{})
	newConn := NewManagedTLSConn(&dummyManagedConn{})
	oldConn.SetGeneration(1)
	newConn.SetGeneration(2)

	conns := map[*ManagedTLSConn]struct{}{
		oldConn: {},
		newConn: {},
	}
	CloseManagedTLSConnections(conns, 2, 0)
	waitForManagedTLSConnClosure(t, func() bool {
		return oldConn.Conn.(*dummyManagedConn).isClosed() && !newConn.Conn.(*dummyManagedConn).isClosed()
	})
}

func waitForManagedTLSConnClosure(t *testing.T, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if ready() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for managed tls conn closure")
}
