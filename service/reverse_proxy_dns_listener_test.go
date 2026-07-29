package service

import (
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/alireza0/s-ui/database/model"
	"github.com/miekg/dns"
)

func TestReverseProxyOwnedDNSUDPListenerForwardsQuery(t *testing.T) {
	upstreamPort, _ := startReverseProxyTestDNSServer(t, 30, "203.0.113.99")
	row := model.ReverseProxyRule{
		Id:                  991,
		Enabled:             true,
		ListenProtocol:      reverseProxyProtocolDNS,
		ListenProtocolAlias: reverseProxyDNSProtocolUDP,

		ListenPort:                0,
		TargetProtocol:            reverseProxyProtocolDNS,
		TargetProtocolAlias:       reverseProxyDNSProtocolUDP,
		TargetAddresses:           encodeReverseProxyList([]string{"127.0.0.1"}),
		TargetPort:                upstreamPort,
		DNSUpstreamTimeoutSeconds: 2,
		DNSCacheSizeBytes:         reverseProxyDNSDefaultCacheSizeBytes,
		DNSAllowedCIDRs:           `["127.0.0.0/8","::1/128"]`,
		IPStrategy:                reverseProxyIPStrategyPreferIPv4,
	}
	instance, err := newReverseProxyDNSInstance(&ReverseProxyService{}, "owned-udp-test", []model.ReverseProxyRule{row}, "test-state", "test-listener")
	if err != nil {
		t.Fatalf("start owned udp dns listener failed: %v", err)
	}
	t.Cleanup(func() { _ = instance.stop() })
	if len(instance.dnsServers) == 0 || instance.dnsServers[0].PacketConn == nil {
		t.Fatalf("dns listener must be project-owned, servers=%d", len(instance.dnsServers))
	}

	message := new(dns.Msg)
	message.SetQuestion("owned-listener.example.", dns.TypeA)
	client := &dns.Client{Net: "udp", Timeout: 3 * time.Second}
	address := instance.dnsServers[0].PacketConn.LocalAddr().String()
	response, _, err := client.Exchange(message, address)
	if err != nil {
		t.Fatalf("query through owned udp dns listener failed: %v", err)
	}
	if response == nil || response.Rcode != dns.RcodeSuccess || len(response.Answer) != 1 {
		t.Fatalf("unexpected owned udp dns response: %#v", response)
	}
}

func TestReverseProxyDNSRetryBackoffIsBounded(t *testing.T) {
	manager := &reverseProxyDNSRuntimeManager{retry: make(map[string]reverseProxyDNSRetryState)}
	now := time.Now()
	for index := 0; index < 10; index++ {
		manager.noteRetryLocked("dns-test|"+strconv.Itoa(index%2), net.ErrClosed, now)
	}
	for key, state := range manager.retry {
		if state.RetryDelay <= 0 || state.RetryDelay > time.Minute {
			t.Fatalf("retry delay for %s is outside bounds: %s", key, state.RetryDelay)
		}
		if !state.NextRetryAt.After(now) {
			t.Fatalf("retry deadline for %s was not scheduled", key)
		}
	}
}

func TestReverseProxyDNSStoppedInstanceRollsBackWithFreshInstanceAfterRebuildFailure(t *testing.T) {
	openReverseProxyTestDB(t)
	service := &ReverseProxyService{}
	if _, err := service.loadReverseProxySettings(); err != nil {
		t.Fatalf("load reverse proxy settings failed: %v", err)
	}
	listenPort := reserveReverseProxyTestUDPPort(t)
	row := model.ReverseProxyRule{
		Id:                  995,
		Enabled:             true,
		ListenProtocol:      reverseProxyProtocolDNS,
		ListenProtocolAlias: reverseProxyDNSProtocolUDP,

		ListenPort:                listenPort,
		TargetProtocol:            reverseProxyProtocolDNS,
		TargetProtocolAlias:       reverseProxyDNSProtocolUDP,
		TargetAddresses:           encodeReverseProxyList([]string{"127.0.0.1"}),
		TargetPort:                53,
		DNSUpstreamTimeoutSeconds: 1,
		DNSCacheSizeBytes:         reverseProxyDNSDefaultCacheSizeBytes,
		DNSAllowedCIDRs:           `["127.0.0.0/8","::1/128"]`,
		IPStrategy:                reverseProxyIPStrategyPreferIPv4,
	}
	key := reverseProxyDNSInstanceKey(&row)
	stateKey := reverseProxyDNSRuntimeStateKey([]model.ReverseProxyRule{row}, nil)
	listenerStateKey := reverseProxyDNSListenerRuntimeStateKey([]model.ReverseProxyRule{row}, nil)
	instance, err := newReverseProxyDNSInstance(service, key, []model.ReverseProxyRule{row}, stateKey, listenerStateKey)
	if err != nil {
		t.Fatalf("start original dns listener failed: %v", err)
	}
	t.Cleanup(func() { _ = instance.stop() })

	desired := row
	desired.CertificateRecordList = encodeReverseProxyUintList([]uint{999})
	desired.TargetProtocolAlias = "invalid-dns-protocol"
	manager := &reverseProxyDNSRuntimeManager{
		running: map[string]*reverseProxyDNSInstance{key: instance},
		retry:   make(map[string]reverseProxyDNSRetryState),
	}
	if err := manager.sync(service, []model.ReverseProxyRule{desired}); err != nil {
		t.Fatalf("sync dns runtime failed: %v", err)
	}
	republished, exists := manager.running[key]
	if !exists || republished == nil {
		t.Fatal("the previous DNS configuration should be restored after desired rebuild failure")
	}
	if republished == instance {
		t.Fatal("a stopped DNS instance must not be republished after desired rebuild failure")
	}
	t.Cleanup(func() { _ = republished.stop() })
	if retry := manager.retry[key]; retry.NextRetryAt.IsZero() {
		t.Fatal("failed DNS rebuild must wait for a bounded retry")
	}
	select {
	case <-instance.doneCh:
	case <-time.After(time.Second):
		t.Fatal("original DNS instance was not stopped before failure was recorded")
	}
}

func TestReverseProxyDNSListenerResourceKeyTracksOnlyStaticProtocolLimits(t *testing.T) {
	previous := reverseProxyResources.current()
	t.Cleanup(func() { reverseProxyResources.apply(previous) })
	dohRows := []model.ReverseProxyRule{{
		Id:                  993,
		ListenProtocol:      reverseProxyProtocolDNS,
		ListenProtocolAlias: reverseProxyDNSProtocolDoH,
	}}
	beforeDoH := reverseProxyDNSListenerResourceStateKey(dohRows)
	dynamic := previous
	dynamic.ListenerConnectionLimit++
	dynamic.GlobalDNSMaxConcurrent++
	reverseProxyResources.apply(dynamic)
	if got := reverseProxyDNSListenerResourceStateKey(dohRows); got != beforeDoH {
		t.Fatalf("dynamic DNS limits must not force a DoH listener restart: before=%q after=%q", beforeDoH, got)
	}
	h2Changed := dynamic
	h2Changed.HTTP2MaxConcurrentStreams++
	reverseProxyResources.apply(h2Changed)
	if got := reverseProxyDNSListenerResourceStateKey(dohRows); got == beforeDoH {
		t.Fatal("DoH H2 stream limit must change the listener resource key")
	}

	doqRows := []model.ReverseProxyRule{{
		Id:                  994,
		ListenProtocol:      reverseProxyProtocolDNS,
		ListenProtocolAlias: reverseProxyDNSProtocolDoQ,
	}}
	beforeDoQ := reverseProxyDNSListenerResourceStateKey(doqRows)
	quicChanged := h2Changed
	quicChanged.QUICMaxIncomingStreams++
	reverseProxyResources.apply(quicChanged)
	if got := reverseProxyDNSListenerResourceStateKey(doqRows); got == beforeDoQ {
		t.Fatal("DoQ QUIC stream limit must change the listener resource key")
	}

	instance := &reverseProxyDNSInstance{connectionLimiter: newReverseProxyAdjustableLimiter(previous.ListenerConnectionLimit)}
	instance.applyResourceLimits()
	if _, maximum := instance.connectionLimiter.Snapshot(); maximum != quicChanged.ListenerConnectionLimit {
		t.Fatalf("dynamic DNS listener limit was not applied: got=%d want=%d", maximum, quicChanged.ListenerConnectionLimit)
	}
}
