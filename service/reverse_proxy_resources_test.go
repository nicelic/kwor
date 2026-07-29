package service

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/miekg/dns"
)

func TestReverseProxyResourceRevisionRejectsStaleSave(t *testing.T) {
	openReverseProxyTestDB(t)
	svc := &ReverseProxyService{}
	t.Cleanup(func() { _ = svc.StopRuntime() })

	settings, err := svc.loadReverseProxySettings()
	if err != nil {
		t.Fatalf("load reverse proxy settings failed: %v", err)
	}
	initialRevision := settings.Revision
	payload := ReverseProxySettingsPayload{
		ExpectedRevision:             &initialRevision,
		ReverseProxyResourceSettings: reverseProxySettingsView(settings),
	}
	if err := svc.SaveResourceSettings(payload); err != nil {
		t.Fatalf("save resource settings failed: %v", err)
	}
	if err := svc.SaveResourceSettings(payload); !errors.Is(err, errReverseProxyRevisionConflict) {
		t.Fatalf("stale resource save must return revision conflict, got %v", err)
	}
	updated, err := svc.loadReverseProxySettings()
	if err != nil {
		t.Fatalf("reload reverse proxy settings failed: %v", err)
	}
	if updated.Revision != initialRevision+1 {
		t.Fatalf("unexpected revision after one successful save: got=%d want=%d", updated.Revision, initialRevision+1)
	}
}

func TestReverseProxyRuleMutationsRejectStaleRevision(t *testing.T) {
	openReverseProxyTestDB(t)
	svc := &ReverseProxyService{}
	t.Cleanup(func() { _ = svc.StopRuntime() })

	settings, err := svc.loadReverseProxySettings()
	if err != nil {
		t.Fatalf("load reverse proxy settings failed: %v", err)
	}
	initialRevision := settings.Revision
	payload := ReverseProxyRulePayload{
		ExpectedRevision: &initialRevision,
		Name:             "revision-check",
		Enabled:          false,
		ListenProtocol:   reverseProxyProtocolHTTP,

		ListenPort:      18080,
		PathPrefix:      "/",
		TargetProtocol:  reverseProxyProtocolHTTP,
		TargetAddresses: "127.0.0.1",
		TargetPort:      18081,
		IPStrategy:      reverseProxyIPStrategyPreferIPv4,
	}
	if err := svc.UpsertRule(payload); err != nil {
		t.Fatalf("create revision test rule: %v", err)
	}

	var saved model.ReverseProxyRule
	if err := database.GetDB().Where("name = ?", payload.Name).First(&saved).Error; err != nil {
		t.Fatalf("load created revision test rule: %v", err)
	}
	staleRevision := initialRevision
	payload.ID = saved.Id
	payload.ExpectedRevision = &staleRevision
	payload.Name = "revision-check-updated"
	if err := svc.UpsertRule(payload); !errors.Is(err, errReverseProxyRevisionConflict) {
		t.Fatalf("stale rule save must return revision conflict, got %v", err)
	}
	if err := svc.ReorderRules(ReverseProxyRuleReorderPayload{
		ExpectedRevision: &staleRevision,
		IDs:              []uint{saved.Id},
	}); !errors.Is(err, errReverseProxyRevisionConflict) {
		t.Fatalf("stale rule reorder must return revision conflict, got %v", err)
	}
	if err := svc.DeleteRuleWithRevision(ReverseProxyRuleDeletePayload{
		ExpectedRevision: &staleRevision,
		ID:               saved.Id,
	}); !errors.Is(err, errReverseProxyRevisionConflict) {
		t.Fatalf("stale rule delete must return revision conflict, got %v", err)
	}

	var stillPresent model.ReverseProxyRule
	if err := database.GetDB().Where("id = ?", saved.Id).First(&stillPresent).Error; err != nil {
		t.Fatalf("stale mutations must leave the rule unchanged: %v", err)
	}
	updated, err := svc.loadReverseProxySettings()
	if err != nil {
		t.Fatalf("reload reverse proxy settings failed: %v", err)
	}
	if updated.Revision != initialRevision+1 {
		t.Fatalf("stale mutations changed revision: got=%d want=%d", updated.Revision, initialRevision+1)
	}
}

func TestReverseProxyLightweightStatusAndMoveUseRevisionCAS(t *testing.T) {
	openReverseProxyTestDB(t)
	svc := &ReverseProxyService{}
	t.Cleanup(func() { _ = svc.StopRuntime() })
	settings, err := svc.loadReverseProxySettings()
	if err != nil {
		t.Fatalf("load reverse proxy settings failed: %v", err)
	}
	rows := []model.ReverseProxyRule{
		{
			DisplayID:       1,
			ListOrder:       1,
			Name:            "status-rule",
			Enabled:         true,
			ListenProtocol:  reverseProxyProtocolHTTP,
			ListenPort:      18080,
			TargetProtocol:  reverseProxyProtocolHTTP,
			TargetAddresses: encodeReverseProxyList([]string{"127.0.0.1"}),
			TargetPort:      18081,
			IPStrategy:      reverseProxyIPStrategyPreferIPv4,
		},
		{
			DisplayID:       2,
			ListOrder:       2,
			Name:            "move-rule",
			Enabled:         false,
			ListenProtocol:  reverseProxyProtocolHTTP,
			ListenPort:      18082,
			TargetProtocol:  reverseProxyProtocolHTTP,
			TargetAddresses: encodeReverseProxyList([]string{"127.0.0.1"}),
			TargetPort:      18083,
			IPStrategy:      reverseProxyIPStrategyPreferIPv4,
		},
	}
	if err := database.GetDB().Create(&rows).Error; err != nil {
		t.Fatalf("create lightweight mutation rules failed: %v", err)
	}

	revision := settings.Revision
	statusResult, err := svc.SetRuleEnabled(ReverseProxyRuleStatusPayload{
		ExpectedRevision: &revision,
		ID:               rows[0].Id,
		Enabled:          false,
	})
	if err != nil {
		t.Fatalf("disable rule failed: %v", err)
	}
	if statusResult.Revision != revision+1 || statusResult.Enabled {
		t.Fatalf("unexpected status result: %#v", statusResult)
	}
	stale := revision
	if _, err := svc.SetRuleEnabled(ReverseProxyRuleStatusPayload{
		ExpectedRevision: &stale,
		ID:               rows[0].Id,
		Enabled:          true,
	}); !errors.Is(err, errReverseProxyRevisionConflict) {
		t.Fatalf("stale status change must conflict, got %v", err)
	}

	moveRevision := statusResult.Revision
	moveResult, err := svc.MoveRule(ReverseProxyRuleMovePayload{
		ExpectedRevision: &moveRevision,
		ID:               rows[1].Id,
		Direction:        -1,
	})
	if err != nil {
		t.Fatalf("move rule failed: %v", err)
	}
	if moveResult.Revision != moveRevision+1 || moveResult.AdjacentID != rows[0].Id {
		t.Fatalf("unexpected move result: %#v", moveResult)
	}
	var stored []model.ReverseProxyRule
	if err := database.GetDB().Order("list_order ASC, id ASC").Find(&stored).Error; err != nil {
		t.Fatalf("reload moved rules failed: %v", err)
	}
	if len(stored) != 2 || stored[0].Id != rows[1].Id || stored[1].Id != rows[0].Id {
		t.Fatalf("unexpected persisted order: %#v", stored)
	}
	if stored[1].Enabled || stored[1].RuntimeStatus != "disabled" || stored[1].LastError != "" {
		t.Fatalf("disabled runtime state was not cleared: %#v", stored[1])
	}
}

func TestReverseProxyAdjustableLimiterReleasesLease(t *testing.T) {
	limiter := newReverseProxyAdjustableLimiter(1)
	if !limiter.TryAcquire() || limiter.TryAcquire() {
		t.Fatal("limiter did not enforce the configured maximum")
	}
	limiter.Release()
	if active, maximum := limiter.Snapshot(); active != 0 || maximum != 1 {
		t.Fatalf("limiter release leaked active lease: active=%d maximum=%d", active, maximum)
	}
	if !limiter.TryAcquire() {
		t.Fatal("limiter did not allow a new lease after release")
	}
}

func TestReverseProxyRuleLimitersKeepActiveLeasesAcrossRefresh(t *testing.T) {
	group := &reverseProxyListenerGroup{}
	initial := &model.ReverseProxyRule{
		Id:                       71,
		MaxConcurrentConnections: 2,
		MaxConcurrentRequests:    2,
		UpstreamMaxConnections:   2,
	}
	resources := reverseProxyResources.current()
	group.configureRuleLimitersLocked([]*model.ReverseProxyRule{initial}, resources)

	connectionLimiter := group.ruleConnectionLimiters[initial.Id]
	requestLimiter := group.requestLimiters[initial.Id]
	upstreamLimiter := group.upstreamLimiters[initial.Id]
	if connectionLimiter == nil || requestLimiter == nil || upstreamLimiter == nil {
		t.Fatal("expected all rule limiters to be created")
	}
	if !connectionLimiter.TryAcquire() || !requestLimiter.TryAcquire() || !upstreamLimiter.TryAcquire() {
		t.Fatal("failed to create active limiter leases")
	}

	updated := *initial
	updated.MaxConcurrentConnections = 1
	updated.MaxConcurrentRequests = 1
	updated.UpstreamMaxConnections = 1
	group.configureRuleLimitersLocked([]*model.ReverseProxyRule{&updated}, resources)

	if group.ruleConnectionLimiters[updated.Id] != connectionLimiter ||
		group.requestLimiters[updated.Id] != requestLimiter ||
		group.upstreamLimiters[updated.Id] != upstreamLimiter {
		t.Fatal("refresh must preserve limiter instances that still own active leases")
	}
	for name, limiter := range map[string]*reverseProxyAdjustableLimiter{
		"connection": connectionLimiter,
		"request":    requestLimiter,
		"upstream":   upstreamLimiter,
	} {
		if active, maximum := limiter.Snapshot(); active != 1 || maximum != 1 {
			t.Fatalf("%s limiter lost its active lease after refresh: active=%d maximum=%d", name, active, maximum)
		}
		if limiter.TryAcquire() {
			t.Fatalf("%s limiter allowed a new lease above the refreshed limit", name)
		}
		limiter.Release()
		if !limiter.TryAcquire() {
			t.Fatalf("%s limiter did not recover after its active lease was released", name)
		}
		limiter.Release()
	}
}

func TestReverseProxyLocalConnectionCanServeMultipleRules(t *testing.T) {
	firstLimiter := newReverseProxyAdjustableLimiter(1)
	secondLimiter := newReverseProxyAdjustableLimiter(1)
	group := &reverseProxyListenerGroup{
		ruleConnectionLimiters: map[uint]*reverseProxyAdjustableLimiter{
			101: firstLimiter,
			202: secondLimiter,
		},
	}
	const connectionID = "shared-h2-connection"
	if !group.registerLocalConnectionRule(101, connectionID) {
		t.Fatal("first rule unexpectedly rejected the shared connection")
	}
	if !group.registerLocalConnectionRule(202, connectionID) {
		t.Fatal("second valid rule on the same multiplexed connection was rejected")
	}
	if !group.registerLocalConnectionRule(101, connectionID) {
		t.Fatal("repeated request for the first rule was rejected")
	}
	counts := group.snapshotConnectionCounts()
	if counts[101].LocalOpen != 1 || counts[202].LocalOpen != 1 {
		t.Fatalf("each rule must count the shared connection once, got %#v", counts)
	}
	if active, _ := firstLimiter.Snapshot(); active != 1 {
		t.Fatalf("first rule limiter has wrong active count: %d", active)
	}
	if active, _ := secondLimiter.Snapshot(); active != 1 {
		t.Fatalf("second rule limiter has wrong active count: %d", active)
	}

	group.releaseLocalConnectionByID(connectionID)
	if active, _ := firstLimiter.Snapshot(); active != 0 {
		t.Fatalf("first rule limiter leaked after connection close: %d", active)
	}
	if active, _ := secondLimiter.Snapshot(); active != 0 {
		t.Fatalf("second rule limiter leaked after connection close: %d", active)
	}
}

func TestReverseProxyHTTPIdlePoolZeroAvoidsImplicitPerHostDefault(t *testing.T) {
	bundle, err := (&ReverseProxyService{}).buildRoundTripper(nil, 0, reverseProxyProtocolHTTP, "127.0.0.1", 80, "", false, reverseProxyUpstreamModeHTTP, 0, 0)
	if err != nil {
		t.Fatalf("build http transport: %v", err)
	}
	defer bundle.Cleanup()
	wrapped, ok := bundle.RoundTripper.(reverseProxyResponseHeaderTimeoutTransport)
	if !ok {
		t.Fatalf("unexpected http transport wrapper type: %T", bundle.RoundTripper)
	}
	transport, ok := wrapped.base.(*http.Transport)
	if !ok {
		t.Fatalf("unexpected wrapped transport type: %T", wrapped.base)
	}
	if transport.MaxIdleConns != 0 {
		t.Fatalf("zero panel idle limit must leave the total pool unlimited, got %d", transport.MaxIdleConns)
	}
	if transport.MaxIdleConnsPerHost <= http.DefaultMaxIdleConnsPerHost {
		t.Fatalf("zero panel idle limit fell back to net/http's hidden per-host default: got %d default %d", transport.MaxIdleConnsPerHost, http.DefaultMaxIdleConnsPerHost)
	}

	if total, perHost := reverseProxyHTTPTransportIdleConnectionLimits(17); total != 17 || perHost != 17 {
		t.Fatalf("explicit panel idle limit was not preserved: total=%d perHost=%d", total, perHost)
	}
}

func TestReverseProxyListenerGroupRestartsOnlyForStaticStreamLimits(t *testing.T) {
	previous := reverseProxyResources.current()
	t.Cleanup(func() { reverseProxyResources.apply(previous) })
	rules := []*model.ReverseProxyRule{{
		ListenProtocol: reverseProxyProtocolHTTPS,
		ListenPort:     443,
	}}
	group := &reverseProxyListenerGroup{
		protocol:   reverseProxyProtocolHTTPS,
		socketKind: reverseProxySocketKindTCP,
		listenIPs:  []string{"127.0.0.1"},
		listenPort: 443,
		resources:  previous,
	}

	dynamic := previous
	dynamic.ListenerConnectionLimit++
	reverseProxyResources.apply(dynamic)
	if reverseProxyListenerGroupNeedsRestart(group, rules) {
		t.Fatal("adjustable listener connection limit must refresh without a TCP listener restart")
	}

	h2Changed := dynamic
	h2Changed.HTTP2MaxConcurrentStreams++
	reverseProxyResources.apply(h2Changed)
	if !reverseProxyListenerGroupNeedsRestart(group, rules) || !reverseProxyListenerGroupRestartNeedsClosingExisting(group, rules) {
		t.Fatal("TCP H2 stream limit must restart the existing listener after closing its socket")
	}

	group.socketKind = reverseProxySocketKindUDP
	group.resources = previous
	reverseProxyResources.apply(h2Changed)
	if reverseProxyListenerGroupNeedsRestart(group, rules) {
		t.Fatal("UDP listener must not restart when only the H2 stream limit changes")
	}
	quicChanged := h2Changed
	quicChanged.QUICMaxIncomingStreams++
	reverseProxyResources.apply(quicChanged)
	if !reverseProxyListenerGroupNeedsRestart(group, rules) || !reverseProxyListenerGroupRestartNeedsClosingExisting(group, rules) {
		t.Fatal("UDP H3 stream limit must restart the existing listener after closing its socket")
	}
}

func TestReverseProxyDynamicResourceUpdateRetainsHealthyUpstreamCache(t *testing.T) {
	resources := reverseProxyResources.current()
	rules := []*model.ReverseProxyRule{
		{Id: 801, UpstreamMaxIdleConnections: 0},
		{Id: 802, UpstreamMaxIdleConnections: 19},
	}
	defaultDisposed := 0
	explicitDisposed := 0
	defaultUpstream := &reverseProxyCachedUpstream{Cleanup: func() { defaultDisposed++ }}
	explicitUpstream := &reverseProxyCachedUpstream{Cleanup: func() { explicitDisposed++ }}
	group := &reverseProxyListenerGroup{
		rules: rules,
		upstreamByRule: map[uint]*reverseProxyCachedUpstream{
			801: defaultUpstream,
			802: explicitUpstream,
		},
	}
	group.configureRuleLimitersLocked(rules, resources)
	listenerLimiter := group.listenerConnectionLimiter

	dynamic := resources
	dynamic.ListenerConnectionLimit++
	dynamic.GlobalHTTPMaxConcurrent++
	group.applyResourceSettings(dynamic)
	if group.listenerConnectionLimiter != listenerLimiter {
		t.Fatal("dynamic guards must retain the live listener limiter")
	}
	if _, maximum := listenerLimiter.Snapshot(); maximum != dynamic.ListenerConnectionLimit {
		t.Fatalf("dynamic listener limit was not applied in place: got=%d want=%d", maximum, dynamic.ListenerConnectionLimit)
	}
	if group.upstreamByRule[801] != defaultUpstream || group.upstreamByRule[802] != explicitUpstream || defaultDisposed != 0 || explicitDisposed != 0 {
		t.Fatal("unrelated dynamic resource settings must retain healthy upstream transports")
	}

	idleChanged := dynamic
	idleChanged.DefaultUpstreamMaxIdleConnections++
	group.applyResourceSettings(idleChanged)
	if _, exists := group.upstreamByRule[801]; exists || defaultDisposed != 1 {
		t.Fatalf("inherited idle-pool transport was not retired exactly once: present=%v disposed=%d", exists, defaultDisposed)
	}
	if group.upstreamByRule[802] != explicitUpstream || explicitDisposed != 0 {
		t.Fatal("explicit rule idle-pool transport must survive a changed global default")
	}
}

func TestReverseProxyDNSCacheMemoryPolicyReplacesOnlyAffectedRoute(t *testing.T) {
	previous := reverseProxyResources.current()
	t.Cleanup(func() { reverseProxyResources.apply(previous) })
	reverseProxyResources.apply(previous)
	rows := []model.ReverseProxyRule{{
		Id:                        803,
		ListOrder:                 1,
		ListenProtocol:            reverseProxyProtocolDNS,
		ListenProtocolAlias:       reverseProxyDNSProtocolDoH,
		ListenDNSPath:             "/dns-query",
		TargetProtocol:            reverseProxyProtocolDNS,
		TargetProtocolAlias:       reverseProxyDNSProtocolUDP,
		TargetAddresses:           encodeReverseProxyList([]string{"127.0.0.1"}),
		TargetPort:                53,
		DNSUpstreamTimeoutSeconds: 2,
		DNSCacheEnabled:           true,
		DNSCacheSizeBytes:         1024 * 1024,
		IPStrategy:                reverseProxyIPStrategyPreferIPv4,
		UpstreamTLSVerify:         true,
	}}
	handler, err := buildReverseProxyDNSRuleHandler(rows)
	if err != nil {
		t.Fatalf("build dns route: %v", err)
	}
	t.Cleanup(func() { _ = closeReverseProxyDNSHandler(handler) })
	stateKey := reverseProxyDNSRuntimeStateKey(rows, nil)
	listenerStateKey := reverseProxyDNSListenerRuntimeStateKey(rows, nil)
	instance := &reverseProxyDNSInstance{
		handler:           handler,
		rules:             cloneReverseProxyRules(rows),
		runtimeStateKey:   stateKey,
		listenerStateKey:  listenerStateKey,
		connectionLimiter: newReverseProxyAdjustableLimiter(previous.ListenerConnectionLimit),
	}
	handler.mu.RLock()
	routeBefore := handler.routes["/dns-query"]
	handler.mu.RUnlock()
	if routeBefore == nil || routeBefore.cache == nil {
		t.Fatal("expected an initial DNS cache route")
	}
	cacheBefore := routeBefore.cache

	dynamic := previous
	dynamic.GlobalDNSMaxConcurrent++
	dynamic.ListenerConnectionLimit++
	reverseProxyResources.apply(dynamic)
	if got := reverseProxyDNSRuntimeStateKey(rows, nil); got != stateKey {
		t.Fatalf("dynamic DNS guards must not replace a resolver/cache route: before=%q after=%q", stateKey, got)
	}
	instance.applyResourceLimits()
	if _, maximum := instance.connectionLimiter.Snapshot(); maximum != dynamic.ListenerConnectionLimit {
		t.Fatalf("dynamic DNS listener limit was not updated: got=%d want=%d", maximum, dynamic.ListenerConnectionLimit)
	}

	memoryChanged := dynamic
	memoryChanged.DefaultRuleMemoryLimitBytes -= 1024
	reverseProxyResources.apply(memoryChanged)
	nextStateKey := reverseProxyDNSRuntimeStateKey(rows, nil)
	if nextStateKey == stateKey {
		t.Fatal("effective DNS cache memory policy must change the route state key")
	}
	if got := reverseProxyDNSListenerRuntimeStateKey(rows, nil); got != listenerStateKey {
		t.Fatalf("cache memory policy must not rebind the DoH listener: before=%q after=%q", listenerStateKey, got)
	}
	refreshed, err := refreshReverseProxyDNSInstanceRoutes(instance, rows, nextStateKey)
	if err != nil || !refreshed {
		t.Fatalf("refresh DNS cache memory policy: refreshed=%v err=%v", refreshed, err)
	}
	handler.mu.RLock()
	routeAfter := handler.routes["/dns-query"]
	handler.mu.RUnlock()
	if routeAfter == nil || routeAfter == routeBefore {
		t.Fatalf("changed cache memory policy did not replace its route: old=%p new=%p", routeBefore, routeAfter)
	}
	cacheBefore.mu.Lock()
	closed := cacheBefore.closed
	cacheBefore.mu.Unlock()
	if !closed {
		t.Fatal("replaced DNS cache route did not release its old memory lease owner")
	}
}

func TestReverseProxySyncIfNeededRefreshesStaleConfigurationSnapshot(t *testing.T) {
	openReverseProxyTestDB(t)
	service := &ReverseProxyService{}
	settings, err := service.loadReverseProxySettings()
	if err != nil {
		t.Fatalf("load reverse proxy settings: %v", err)
	}
	if err := service.refreshReverseProxyConfigurationSnapshot(); err != nil {
		t.Fatalf("create initial configuration snapshot: %v", err)
	}
	nextRevision := settings.Revision + 1
	if err := database.GetDB().Model(&model.ReverseProxySettings{}).
		Where("id = ?", settings.Id).
		Update("revision", nextRevision).Error; err != nil {
		t.Fatalf("simulate external reverse proxy revision update: %v", err)
	}
	// StopRuntime records a timestamp; clear it so this test follows the normal
	// cron branch that first sees a changed revision through the tiny probe.
	reverseProxyRuntime.mu.Lock()
	reverseProxyRuntime.state.lastSyncAt = time.Time{}
	reverseProxyRuntime.mu.Unlock()
	if err := service.SyncIfNeeded(time.Minute); err != nil {
		t.Fatalf("sync changed external revision: %v", err)
	}
	snapshot, err := service.reverseProxyConfigurationSnapshot()
	if err != nil {
		t.Fatalf("read refreshed configuration snapshot: %v", err)
	}
	if snapshot.Revision != nextRevision {
		t.Fatalf("cron did not refresh stale configuration snapshot: got=%d want=%d", snapshot.Revision, nextRevision)
	}
}

func TestReverseProxyResourceSettingsValidateRewritePeakReservation(t *testing.T) {
	settings := reverseProxySettingsView(ptrReverseProxySettings(defaultReverseProxySettingsModel()))
	settings.DefaultRuleMemoryLimitBytes = reverseProxyRewriteReservationBytes(settings.ResponseRewriteInputBytes, settings.ResponseRewriteOutputBytes) - 1
	if err := validateReverseProxyResourceSettings(settings); err == nil {
		t.Fatal("resource policy must reject a rewrite peak larger than the default rule memory limit")
	}
	settings.DefaultRuleMemoryLimitBytes++
	if err := validateReverseProxyResourceSettings(settings); err != nil {
		t.Fatalf("resource policy must accept the exact rewrite peak reservation: %v", err)
	}
}

func TestReverseProxyRuntimeFailureBackoffSkipsUntilDeadline(t *testing.T) {
	openReverseProxyTestDB(t)
	service := &ReverseProxyService{}
	settings, err := service.loadReverseProxySettings()
	if err != nil {
		t.Fatalf("load reverse proxy settings failed: %v", err)
	}
	deadline := time.Now().Add(time.Minute)
	manager := &reverseProxyRuntimeManager{
		groups: make(map[string]*reverseProxyListenerGroup),
		state: reverseProxyRuntimeState{
			lastSyncAt:            time.Now().Add(-time.Second),
			revision:              settings.Revision,
			certificateGeneration: currentReverseProxyCertificateGeneration(),
			nextRetryAt:           deadline,
			retryDelay:            time.Second,
		},
	}
	if err := manager.reconcileLocked(service, time.Nanosecond, false); err != nil {
		t.Fatalf("retry backoff check failed: %v", err)
	}
	if !manager.state.nextRetryAt.Equal(deadline) || manager.state.retryDelay != time.Second {
		t.Fatalf("runtime retried before backoff deadline: next=%s delay=%s", manager.state.nextRetryAt, manager.state.retryDelay)
	}
}

func TestReverseProxyRuleRejectsInvalidListenIP(t *testing.T) {
	_, err := (&ReverseProxyService{}).normalizeRulePayload(ReverseProxyRulePayload{
		Enabled:        true,
		ListenProtocol: reverseProxyProtocolHTTP,

		ListenPort:      18080,
		TargetProtocol:  reverseProxyProtocolHTTP,
		TargetAddresses: "127.0.0.1",
		TargetPort:      18081,
		IPStrategy:      reverseProxyIPStrategyPreferIPv4,
	})
	if err == nil {
		t.Fatal("nonempty listener IP list must reject a non-IP item")
	}
}

func TestReverseProxyDNSCacheEvictionAndCloseReleaseSharedMemory(t *testing.T) {
	previous := reverseProxyResources.current()
	t.Cleanup(func() { reverseProxyResources.apply(previous) })
	tight := previous
	tight.MemoryPoolBytes = 2 * 1024 * 1024
	tight.DefaultRuleMemoryLimitBytes = 1024 * 1024
	tight.ResponseRewriteInputBytes = 500 * 1024
	tight.ResponseRewriteOutputBytes = 500 * 1024
	reverseProxyResources.apply(tight)

	beforeUsed, beforeCache, _ := reverseProxyResources.memory.Snapshot()
	cache := newReverseProxyDNSResponseCache(&model.ReverseProxyRule{
		Id:                8765,
		DNSCacheEnabled:   true,
		DNSCacheSizeBytes: 512 * 1024,
		MemoryLimitBytes:  512 * 1024,
	})
	if cache == nil {
		t.Fatal("expected bounded dns cache")
	}
	smallCache := newReverseProxyDNSResponseCache(&model.ReverseProxyRule{
		Id:                8766,
		DNSCacheEnabled:   true,
		DNSCacheSizeBytes: 1024,
		MemoryLimitBytes:  512 * 1024,
	})
	if smallCache == nil {
		t.Fatal("positive small DNS cache size must not be rejected by the body-rewrite buffer threshold")
	}
	smallCache.Close()
	// Keep the test small while exercising the same LRU path used by the
	// configured per-rule byte ceiling.
	cache.maxBytes = 700
	for index := 0; index < 4; index++ {
		request := new(dns.Msg)
		request.SetQuestion(fmt.Sprintf("cache-%d.example.", index), dns.TypeTXT)
		response := new(dns.Msg)
		response.SetReply(request)
		response.Answer = []dns.RR{&dns.TXT{
			Hdr: dns.RR_Header{Name: request.Question[0].Name, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 60},
			Txt: []string{strings.Repeat("x", 200)},
		}}
		cache.Put(request, response, 0, 0)
	}
	cache.mu.Lock()
	used := cache.used
	entries := len(cache.entries)
	cache.mu.Unlock()
	if entries == 0 || entries >= 4 || used > cache.maxBytes {
		t.Fatalf("dns cache did not evict by LRU bound: entries=%d used=%d max=%d", entries, used, cache.maxBytes)
	}
	_, cacheUsed, _ := reverseProxyResources.memory.Snapshot()
	if cacheUsed <= beforeCache {
		t.Fatalf("dns cache did not reserve shared memory: before=%d after=%d", beforeCache, cacheUsed)
	}

	cache.Close()
	afterUsed, afterCache, _ := reverseProxyResources.memory.Snapshot()
	if afterUsed != beforeUsed || afterCache != beforeCache {
		t.Fatalf("dns cache close leaked shared-memory lease: used=%d/%d cache=%d/%d", afterUsed, beforeUsed, afterCache, beforeCache)
	}
}

func TestReverseProxyDNSCacheExpiresWithoutAdditionalTraffic(t *testing.T) {
	previous := reverseProxyResources.current()
	t.Cleanup(func() { reverseProxyResources.apply(previous) })
	settings := previous
	settings.MemoryPoolBytes = 2 * 1024 * 1024
	settings.DefaultRuleMemoryLimitBytes = 1024 * 1024
	reverseProxyResources.apply(settings)

	beforeUsed, beforeCache, _ := reverseProxyResources.memory.Snapshot()
	cache := newReverseProxyDNSResponseCache(&model.ReverseProxyRule{
		Id:                9876,
		DNSCacheEnabled:   true,
		DNSCacheSizeBytes: 64 * 1024,
		MemoryLimitBytes:  64 * 1024,
	})
	if cache == nil {
		t.Fatal("expected dns cache")
	}
	t.Cleanup(cache.Close)
	request := new(dns.Msg)
	request.SetQuestion("expires.example.", dns.TypeA)
	response := new(dns.Msg)
	response.SetReply(request)
	response.Answer = []dns.RR{&dns.A{
		Hdr: dns.RR_Header{Name: request.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 1},
		A:   []byte{192, 0, 2, 1},
	}}
	cache.Put(request, response, 0, 0)
	if used, cacheUsed, _ := reverseProxyResources.memory.Snapshot(); used <= beforeUsed || cacheUsed <= beforeCache {
		t.Fatalf("cache entry did not reserve memory: used=%d cache=%d", used, cacheUsed)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		cache.mu.Lock()
		entries := len(cache.entries)
		cache.mu.Unlock()
		used, cacheUsed, _ := reverseProxyResources.memory.Snapshot()
		if entries == 0 && used == beforeUsed && cacheUsed == beforeCache {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	cache.mu.Lock()
	entries := len(cache.entries)
	cache.mu.Unlock()
	afterUsed, afterCache, _ := reverseProxyResources.memory.Snapshot()
	t.Fatalf("expired cache entry retained memory without traffic: entries=%d used=%d/%d cache=%d/%d", entries, afterUsed, beforeUsed, afterCache, beforeCache)
}
