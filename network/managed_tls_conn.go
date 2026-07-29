package network

import (
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type ManagedTLSConn struct {
	net.Conn
	generation  atomic.Uint64
	mu          sync.RWMutex
	fingerprint string
}

type unwrapConn interface {
	Unwrap() net.Conn
}

type netConnProvider interface {
	NetConn() net.Conn
}

func NewManagedTLSConn(conn net.Conn) *ManagedTLSConn {
	return &ManagedTLSConn{Conn: conn}
}

func (c *ManagedTLSConn) SetGeneration(value uint64) {
	c.generation.Store(value)
}

func (c *ManagedTLSConn) Generation() uint64 {
	return c.generation.Load()
}

func (c *ManagedTLSConn) SetFingerprint(value string) {
	c.mu.Lock()
	c.fingerprint = strings.TrimSpace(value)
	c.mu.Unlock()
}

func (c *ManagedTLSConn) Fingerprint() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.fingerprint
}

func ManagedTLSConnFromNetConn(conn net.Conn) *ManagedTLSConn {
	// Listener wrappers expose Unwrap, while crypto/tls exposes NetConn.
	// Keep the walk bounded so an unexpected self-referencing wrapper cannot
	// trap connection lifecycle callbacks.
	for depth := 0; conn != nil && depth < 16; depth++ {
		if managed, ok := conn.(*ManagedTLSConn); ok {
			return managed
		}
		if unwrapper, ok := conn.(unwrapConn); ok {
			conn = unwrapper.Unwrap()
			continue
		}
		if provider, ok := conn.(netConnProvider); ok {
			conn = provider.NetConn()
			continue
		}
		return nil
	}
	return nil
}

func CloseManagedTLSConnections(conns map[*ManagedTLSConn]struct{}, generationCutoff uint64, gracePeriod time.Duration) {
	if gracePeriod < 0 {
		gracePeriod = 0
	}
	go func(snapshot map[*ManagedTLSConn]struct{}) {
		if gracePeriod > 0 {
			time.Sleep(gracePeriod)
		}
		for conn := range snapshot {
			if conn == nil {
				continue
			}
			if conn.Generation() >= generationCutoff {
				continue
			}
			_ = conn.Close()
		}
	}(conns)
}

func CloseManagedTLSConnectionsByFingerprint(conns map[*ManagedTLSConn]struct{}, fingerprint string, gracePeriod time.Duration) {
	fingerprint = strings.TrimSpace(fingerprint)
	if fingerprint == "" {
		return
	}
	if gracePeriod < 0 {
		gracePeriod = 0
	}
	go func(snapshot map[*ManagedTLSConn]struct{}) {
		if gracePeriod > 0 {
			time.Sleep(gracePeriod)
		}
		for conn := range snapshot {
			if conn == nil {
				continue
			}
			if !strings.EqualFold(conn.Fingerprint(), fingerprint) {
				continue
			}
			_ = conn.Close()
		}
	}(conns)
}
