package service

import (
	"container/list"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/alireza0/s-ui/database/model"
)

type reverseProxyLifecycleTestConn struct {
	local  net.Addr
	remote net.Addr

	mu     sync.Mutex
	closed bool
}

func (c *reverseProxyLifecycleTestConn) Read(_ []byte) (int, error)         { return 0, io.EOF }
func (c *reverseProxyLifecycleTestConn) Write(value []byte) (int, error)    { return len(value), nil }
func (c *reverseProxyLifecycleTestConn) LocalAddr() net.Addr                { return c.local }
func (c *reverseProxyLifecycleTestConn) RemoteAddr() net.Addr               { return c.remote }
func (c *reverseProxyLifecycleTestConn) SetDeadline(_ time.Time) error      { return nil }
func (c *reverseProxyLifecycleTestConn) SetReadDeadline(_ time.Time) error  { return nil }
func (c *reverseProxyLifecycleTestConn) SetWriteDeadline(_ time.Time) error { return nil }

func (c *reverseProxyLifecycleTestConn) Close() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	return nil
}

func (c *reverseProxyLifecycleTestConn) isClosed() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	return closed
}

func newReverseProxyLifecycleTestConn(localIP string, localPort int, remoteIP string, remotePort int) *reverseProxyLifecycleTestConn {
	return &reverseProxyLifecycleTestConn{
		local:  &net.TCPAddr{IP: net.ParseIP(localIP), Port: localPort},
		remote: &net.TCPAddr{IP: net.ParseIP(remoteIP), Port: remotePort},
	}
}

func TestReverseProxyCertificateBalanceRuntimePrunesReleasedEntry(t *testing.T) {
	binding := &reverseProxyRuleCertificateBinding{
		RuleID:              7,
		CertificateRecordID: 11,
		Certificate:         &tls.Certificate{},
		Leaf:                reverseProxyTestLeafState("example.com"),
	}
	group := &reverseProxyListenerGroup{key: "https|443"}

	selected, selection, err := group.selectBalancedCertificate([]*reverseProxyRuleCertificateBinding{binding}, "example.com")
	if err != nil || selected != binding || selection.CertificateRecordID != binding.CertificateRecordID {
		t.Fatalf("reserve in-memory certificate balance failed: selected=%#v selection=%#v err=%v", selected, selection, err)
	}
	group.releaseCertificateBalanceSelection(selection)

	shard := group.certificateBalanceShard("example.com")
	shard.mu.Lock()
	state := shard.states["example.com"][binding.CertificateRecordID]
	if state == nil || state.ActiveConn != 0 {
		shard.mu.Unlock()
		t.Fatalf("certificate balance lease was not released: %#v", state)
	}
	state.UpdatedAtUnix = time.Now().Add(-reverseProxyRuntimeTableTTL - time.Second).Unix()
	shard.pruneLocked(time.Now())
	remaining := len(shard.states)
	entries := shard.entries
	shard.mu.Unlock()
	if remaining != 0 || entries != 0 {
		t.Fatalf("released stale certificate balance entry must be pruned, groups=%d entries=%d", remaining, entries)
	}
}

func TestReverseProxyCertificateBalanceEvictionKeepsNewSelectionReachable(t *testing.T) {
	oldState := &reverseProxyCertificateBalanceRuntimeState{UpdatedAtUnix: time.Now().Unix()}
	group := &reverseProxyListenerGroup{key: "https|443"}
	shard := group.certificateBalanceShard("new.example")
	lru := list.New()
	oldState.element = lru.PushFront(reverseProxyCertificateBalanceLRUKey{bucket: "new.example", certificateID: 1})
	shard.states = map[string]map[uint]*reverseProxyCertificateBalanceRuntimeState{
		"new.example": {1: oldState},
	}
	shard.lru = lru
	shard.entries = reverseProxyCertificateBalanceShardLimit
	binding := &reverseProxyRuleCertificateBinding{
		RuleID:              2,
		CertificateRecordID: 2,
		Certificate:         &tls.Certificate{},
		Leaf:                reverseProxyTestLeafState("new.example"),
	}

	selected, selection, err := group.selectBalancedCertificate([]*reverseProxyRuleCertificateBinding{binding}, "new.example")
	if err != nil || selected != binding || selection.CertificateRecordID != binding.CertificateRecordID {
		t.Fatalf("selection after lru eviction failed: selected=%#v selection=%#v err=%v", selected, selection, err)
	}
	shard.mu.Lock()
	oldState = shard.states["new.example"][1]
	newState := shard.states["new.example"][binding.CertificateRecordID]
	entries := shard.entries
	shard.mu.Unlock()
	oldExists := oldState != nil
	if oldExists || newState == nil || newState.ActiveConn != 1 || entries != reverseProxyCertificateBalanceShardLimit {
		t.Fatalf("certificate balance lru eviction left an unreachable selection: old=%v new=%#v entries=%d", oldExists, newState, entries)
	}
}

func TestReverseProxyPendingTLSEvictionReleasesCertificateBalanceLease(t *testing.T) {
	binding := &reverseProxyRuleCertificateBinding{
		RuleID:              19,
		CertificateRecordID: 29,
		Certificate:         &tls.Certificate{},
		Leaf:                reverseProxyTestLeafState("pending.example"),
	}
	group := &reverseProxyListenerGroup{key: "https|443"}
	_, firstSelection, err := group.selectBalancedCertificate([]*reverseProxyRuleCertificateBinding{binding}, "pending.example")
	if err != nil || firstSelection.CertificateRecordID == 0 {
		t.Fatalf("reserve first certificate selection: selection=%#v err=%v", firstSelection, err)
	}
	_, secondSelection, err := group.selectBalancedCertificate([]*reverseProxyRuleCertificateBinding{binding}, "pending.example")
	if err != nil || secondSelection.CertificateRecordID == 0 {
		t.Fatalf("reserve second certificate selection: selection=%#v err=%v", secondSelection, err)
	}

	const oldKey = "pending-selection-old"
	shard := group.pendingCertificateShard(oldKey)
	shard.selections = make(map[string]*reverseProxyPendingCertificateSelection, reverseProxyPendingCertificateShardLimit)
	shard.lru = list.New()
	now := time.Now()
	oldPending := &reverseProxyPendingCertificateSelection{Selection: firstSelection, CreatedAt: now}
	oldPending.element = shard.lru.PushFront(oldKey)
	shard.selections[oldKey] = oldPending
	for index := 1; index < reverseProxyPendingCertificateShardLimit; index++ {
		key := fmt.Sprintf("pending-fill-%d", index)
		pending := &reverseProxyPendingCertificateSelection{CreatedAt: now}
		pending.element = shard.lru.PushFront(key)
		shard.selections[key] = pending
	}

	newKey := ""
	for index := 1; ; index++ {
		candidate := fmt.Sprintf("pending-selection-new-%d", index)
		if group.pendingCertificateShard(candidate) == shard {
			newKey = candidate
			break
		}
	}
	// The table is intentionally populated through its shard to place the old
	// selection at the LRU tail. bindCertificateSelectionToConnection() must
	// release that balance lease when inserting the new pending selection.
	group.bindCertificateSelectionToConnection(newKey, secondSelection)
	balanceShard := group.certificateBalanceShard("pending.example")
	balanceShard.mu.Lock()
	state := balanceShard.states["pending.example"][binding.CertificateRecordID]
	active := int64(0)
	if state != nil {
		active = state.ActiveConn
	}
	balanceShard.mu.Unlock()
	if active != 1 {
		t.Fatalf("evicted pending TLS selection leaked its certificate lease: active=%d", active)
	}
	if err := group.shutdown(); err != nil {
		t.Fatalf("shutdown pending TLS selection group: %v", err)
	}
	balanceShard.mu.Lock()
	if state := balanceShard.states["pending.example"][binding.CertificateRecordID]; state != nil && state.ActiveConn != 0 {
		balanceShard.mu.Unlock()
		t.Fatalf("shutdown leaked the remaining pending TLS selection: active=%d", state.ActiveConn)
	}
	balanceShard.mu.Unlock()
}

func TestReverseProxyUnboundTLSSelectionReleasesCertificateBalanceLease(t *testing.T) {
	binding := &reverseProxyRuleCertificateBinding{
		RuleID:              31,
		CertificateRecordID: 41,
		Certificate:         &tls.Certificate{},
		Leaf:                reverseProxyTestLeafState("unbound.example"),
	}
	group := &reverseProxyListenerGroup{key: "https|443"}
	_, selection, err := group.selectBalancedCertificate([]*reverseProxyRuleCertificateBinding{binding}, "unbound.example")
	if err != nil || selection.CertificateRecordID == 0 {
		t.Fatalf("reserve unbound certificate selection: selection=%#v err=%v", selection, err)
	}
	group.bindCertificateSelectionToConnection("", selection)
	shard := group.certificateBalanceShard("unbound.example")
	shard.mu.Lock()
	state := shard.states["unbound.example"][binding.CertificateRecordID]
	active := int64(0)
	if state != nil {
		active = state.ActiveConn
	}
	shard.mu.Unlock()
	if active != 0 {
		t.Fatalf("unbound TLS selection leaked its certificate balance lease: active=%d", active)
	}
}

func TestReverseProxyLocalConnectionIndexesAreReleasedByAddress(t *testing.T) {
	limiter := newReverseProxyAdjustableLimiter(1)
	group := &reverseProxyListenerGroup{listenerConnectionLimiter: limiter}
	raw := newReverseProxyLifecycleTestConn("127.0.0.1", 443, "198.51.100.10", 49152)
	ctx := group.registerTCPConnectionContext(context.Background(), raw)
	connID := group.connectionIDFromContext(ctx)
	if connID == "" {
		t.Fatal("tcp connection was not assigned an id")
	}

	group.releaseLocalConnectionByAddrKey(reverseProxyConnectionAddrKey(raw))
	group.statsMu.Lock()
	remainingByConn := len(group.localConnIDs)
	remainingByID := len(group.localConnByID)
	remainingStates := len(group.localConnStates)
	group.statsMu.Unlock()
	active, _ := limiter.Snapshot()
	if remainingByConn != 0 || remainingByID != 0 || remainingStates != 0 || active != 0 {
		t.Fatalf("closed connection leaked runtime indexes or lease: byConn=%d byID=%d states=%d active=%d", remainingByConn, remainingByID, remainingStates, active)
	}
}

func TestReverseProxyShutdownClosesHijackedWrappedConnection(t *testing.T) {
	limiter := newReverseProxyAdjustableLimiter(1)
	group := &reverseProxyListenerGroup{
		listenerConnectionLimiter: limiter,
		upstreamByRule:            make(map[uint]*reverseProxyCachedUpstream),
	}
	raw := newReverseProxyLifecycleTestConn("127.0.0.1", 443, "198.51.100.20", 49153)
	ctx := group.registerTCPConnectionContext(context.Background(), raw)
	if group.connectionIDFromContext(ctx) == "" {
		t.Fatal("tcp connection was not registered")
	}
	// net/http can report StateHijacked with a TLS/auto-protocol wrapper rather
	// than the accepted connection instance. The shared address key must still
	// identify and close the original tracked connection during shutdown.
	wrapper := newReverseProxyLifecycleTestConn("127.0.0.1", 443, "198.51.100.20", 49153)
	group.markHijackedConnection(wrapper)
	if err := group.shutdown(); err != nil {
		t.Fatalf("shutdown hijacked connection group failed: %v", err)
	}
	if !raw.isClosed() {
		t.Fatal("shutdown did not close the original hijacked tunnel")
	}
	active, _ := limiter.Snapshot()
	if active != 0 {
		t.Fatalf("shutdown leaked listener connection lease: active=%d", active)
	}
}

func TestReverseProxyRefreshClosesHijackedTunnelForRemovedRule(t *testing.T) {
	resources := reverseProxyResources.current()
	removedRule := &model.ReverseProxyRule{Id: 501}
	remainingRule := &model.ReverseProxyRule{Id: 502}
	limiter := newReverseProxyAdjustableLimiter(1)
	group := &reverseProxyListenerGroup{
		rules:                     []*model.ReverseProxyRule{removedRule},
		upstreamByRule:            make(map[uint]*reverseProxyCachedUpstream),
		connectionCounts:          make(map[uint]reverseProxyConnectionCounts),
		localConnIDs:              make(map[net.Conn]string),
		localConnByID:             make(map[string]net.Conn),
		localConnStates:           make(map[string]reverseProxyLocalConnectionState),
		localConnAddrToID:         make(map[string]string),
		localConnAddrByID:         make(map[string]string),
		hijackedConnections:       make(map[string]net.Conn),
		connectionSlotIDs:         make(map[string]struct{}),
		listenerConnectionLimiter: limiter,
	}
	group.configureRuleLimitersLocked(group.rules, resources)
	ruleLimiter := group.ruleConnectionLimiters[removedRule.Id]
	raw := newReverseProxyLifecycleTestConn("127.0.0.1", 443, "198.51.100.30", 49154)
	ctx := group.registerTCPConnectionContext(context.Background(), raw)
	connID := group.connectionIDFromContext(ctx)
	if connID == "" || !group.registerLocalConnectionRule(removedRule.Id, connID) {
		t.Fatal("failed to register the tunnel for its source rule")
	}
	group.setHijackedConnectionRule(connID, removedRule.Id)
	group.markHijackedConnection(raw)

	if err := (&ReverseProxyService{}).refreshListenerGroup(group, []*model.ReverseProxyRule{remainingRule}); err != nil {
		t.Fatalf("refresh listener group after rule removal: %v", err)
	}
	if !raw.isClosed() {
		t.Fatal("removing a rule must close its existing WebSocket tunnel")
	}
	if active, _ := limiter.Snapshot(); active != 0 {
		t.Fatalf("removed tunnel leaked listener connection lease: active=%d", active)
	}
	if active, _ := ruleLimiter.Snapshot(); active != 0 {
		t.Fatalf("removed tunnel leaked rule connection lease: active=%d", active)
	}
}

func TestReverseProxyGetCertificateRejectsCrossTargetIPSNI(t *testing.T) {
	certificate := &tls.Certificate{}
	binding := &reverseProxyRuleCertificateBinding{
		RuleID:              1,
		CertificateRecordID: 1,
		Certificate:         certificate,
		Leaf:                reverseProxyTestLeafState("127.0.0.1"),
	}
	group := &reverseProxyListenerGroup{
		key:                 "https|443",
		orderedCertBindings: []*reverseProxyRuleCertificateBinding{binding},
	}
	group.configureIPCertificateIndexesLocked()
	local := newReverseProxyLifecycleTestConn("127.0.0.1", 443, "198.51.100.30", 49154)
	selected, err := group.getCertificate(&tls.ClientHelloInfo{ServerName: "127.0.0.1", Conn: local})
	if err != nil || selected != certificate {
		t.Fatalf("matching target-ip sni must select its ip-san certificate: cert=%v err=%v", selected, err)
	}
	crossTarget := newReverseProxyLifecycleTestConn("127.0.0.1", 443, "198.51.100.31", 49155)
	selected, err = group.getCertificate(&tls.ClientHelloInfo{ServerName: "127.0.0.2", Conn: crossTarget})
	if err == nil || selected != nil {
		t.Fatalf("cross-target ip sni must be rejected: cert=%v err=%v", selected, err)
	}
}
