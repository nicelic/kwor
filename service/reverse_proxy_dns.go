package service

import (
	"container/list"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"hash/crc32"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	dnsproxy "github.com/AdguardTeam/dnsproxy/proxy"
	dnsupstream "github.com/AdguardTeam/dnsproxy/upstream"
	aghnetutil "github.com/AdguardTeam/golibs/netutil"
	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util/common"
	"github.com/miekg/dns"
	"github.com/quic-go/quic-go"
)

const (
	reverseProxyDNSShutdownTimeout     = 5 * time.Second
	reverseProxyDNSAdmissionClientTTL  = 5 * time.Minute
	reverseProxyDNSAdmissionMaxClients = 16384
	reverseProxyDNSAdmissionShardCount = 16
)

type reverseProxyDNSRuntimeManager struct {
	mu                    sync.Mutex
	running               map[string]*reverseProxyDNSInstance
	retry                 map[string]reverseProxyDNSRetryState
	revision              uint64
	certificateGeneration uint64
}

type reverseProxyDNSRetryState struct {
	NextRetryAt time.Time
	RetryDelay  time.Duration
	LastError   string
}

type reverseProxyDNSInstance struct {
	key               string
	ruleID            uint
	dnsServers        []*dns.Server
	doqListeners      []*quic.Listener
	doqPacketConns    []net.PacketConn
	connectionLimiter *reverseProxyAdjustableLimiter
	handler           *reverseProxyDNSRuleHandler
	rules             []model.ReverseProxyRule
	runtimeStateKey   string
	listenerStateKey  string
	cancel            context.CancelFunc
	doneCh            chan struct{}
}

type reverseProxyDNSRuleHandler struct {
	mu           sync.RWMutex
	defaultRoute *reverseProxyDNSRoute
	routes       map[string]*reverseProxyDNSRoute
	routesByRule map[uint]*reverseProxyDNSRoute
	logger       *slog.Logger
}

type reverseProxyDNSRoute struct {
	mu              sync.Mutex
	rule            *model.ReverseProxyRule
	runtimeStateKey string
	resolver        *dnsproxy.Proxy
	admission       *reverseProxyDNSAdmission
	cache           *reverseProxyDNSResponseCache
	active          int
	closing         bool
	cleanupOnce     sync.Once
	closeErr        error
}

type reverseProxyDNSRouteLease struct {
	route *reverseProxyDNSRoute
}

type reverseProxyDNSAdmission struct {
	allowedCIDRs []netip.Prefix
	qps          int
	slots        *reverseProxyAdjustableLimiter
	shards       [reverseProxyDNSAdmissionShardCount]reverseProxyDNSAdmissionClientShard
}

type reverseProxyDNSAdmissionClient struct {
	tokens    float64
	updatedAt time.Time
	element   *list.Element
}

type reverseProxyDNSAdmissionClientShard struct {
	mu      sync.Mutex
	clients map[string]*reverseProxyDNSAdmissionClient
	lru     *list.List
}

type reverseProxyDNSSequentialUpstream struct {
	upstreams []dnsupstream.Upstream
	closeOnce sync.Once
	closeErr  error
}

type reverseProxyDNSIPStrategyResolver struct {
	base      dnsupstream.Resolver
	strategy  string
	loopGuard func(netip.Addr) bool
}

func buildReverseProxyDNSAdmission(row *model.ReverseProxyRule) (*reverseProxyDNSAdmission, error) {
	if row == nil {
		return nil, errors.New("dns reverse proxy admission requires rule")
	}
	allowedRaw := decodeReverseProxyList(row.DNSAllowedCIDRs)
	allowed := make([]netip.Prefix, 0, len(allowedRaw))
	for _, item := range allowedRaw {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(item))
		if err != nil || prefix.Bits() == 0 {
			return nil, common.NewError("invalid dns allowed cidr")
		}
		allowed = append(allowed, prefix.Masked())
	}
	if len(allowed) == 0 {
		return nil, common.NewError("dns wildcard listeners require at least one non-global allowed cidr")
	}
	qps := reverseProxyDNSRateLimitQPS(row.DNSRateLimitQPS)
	if qps < 1 || qps > reverseProxyDNSMaxRateLimitQPS {
		return nil, common.NewError("dns rate limit qps is invalid")
	}
	maxConcurrent := reverseProxyDNSMaxConcurrentQueries(row.DNSMaxConcurrentQueries)
	if maxConcurrent < 0 || maxConcurrent > reverseProxyDNSMaxConcurrentQueryLimit {
		return nil, common.NewError("dns max concurrent queries is invalid")
	}
	limiter := newReverseProxyAdjustableLimiter(maxConcurrent)
	admission := &reverseProxyDNSAdmission{
		allowedCIDRs: allowed,
		qps:          qps,
		slots:        limiter,
	}
	for index := range admission.shards {
		admission.shards[index].clients = make(map[string]*reverseProxyDNSAdmissionClient)
		admission.shards[index].lru = list.New()
	}
	return admission, nil
}

func (a *reverseProxyDNSAdmission) acquire(dctx *dnsproxy.DNSContext) (func(), string) {
	if a == nil {
		return func() {}, ""
	}
	client := reverseProxyDNSClientAddress(dctx)
	if !client.IsValid() {
		return nil, "dns_client_address_unavailable"
	}
	if len(a.allowedCIDRs) > 0 {
		allowed := false
		for _, prefix := range a.allowedCIDRs {
			if prefix.Contains(client) {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, "dns_acl_denied"
		}
	}
	if !a.takeRateToken(client.String()) {
		return nil, "dns_rate_limited"
	}
	if !reverseProxyResources.tryAcquireDNS() {
		return nil, "dns_global_concurrency_limited"
	}
	if a.slots != nil && !a.slots.TryAcquire() {
		reverseProxyResources.releaseDNS()
		return nil, "dns_concurrency_limited"
	}
	return func() {
		if a.slots != nil {
			a.slots.Release()
		}
		reverseProxyResources.releaseDNS()
	}, ""
}

func (a *reverseProxyDNSAdmission) takeRateToken(client string) bool {
	if a == nil {
		return true
	}
	now := time.Now()
	shard := &a.shards[int(crc32.ChecksumIEEE([]byte(client))%reverseProxyDNSAdmissionShardCount)]
	shard.mu.Lock()
	defer shard.mu.Unlock()
	if shard.clients == nil {
		shard.clients = make(map[string]*reverseProxyDNSAdmissionClient)
	}
	if shard.lru == nil {
		shard.lru = list.New()
	}
	for shard.lru.Len() > 0 {
		oldest, _ := shard.lru.Back().Value.(string)
		state := shard.clients[oldest]
		if state != nil && now.Sub(state.updatedAt) < reverseProxyDNSAdmissionClientTTL {
			break
		}
		if state != nil && state.element != nil {
			shard.lru.Remove(state.element)
		}
		delete(shard.clients, oldest)
	}
	state := shard.clients[client]
	if state == nil {
		perShardLimit := reverseProxyDNSAdmissionMaxClients / reverseProxyDNSAdmissionShardCount
		if len(shard.clients) >= perShardLimit && shard.lru.Len() > 0 {
			oldest, _ := shard.lru.Back().Value.(string)
			if previous := shard.clients[oldest]; previous != nil && previous.element != nil {
				shard.lru.Remove(previous.element)
			}
			delete(shard.clients, oldest)
		}
		state = &reverseProxyDNSAdmissionClient{tokens: float64(a.qps), updatedAt: now}
		state.element = shard.lru.PushFront(client)
		shard.clients[client] = state
	} else if state.element != nil {
		shard.lru.MoveToFront(state.element)
	}
	elapsed := now.Sub(state.updatedAt).Seconds()
	if elapsed > 0 {
		state.tokens += elapsed * float64(a.qps)
		if state.tokens > float64(a.qps) {
			state.tokens = float64(a.qps)
		}
	}
	state.updatedAt = now
	if state.tokens < 1 {
		return false
	}
	state.tokens--
	return true
}

func reverseProxyDNSClientAddress(dctx *dnsproxy.DNSContext) netip.Addr {
	if dctx == nil {
		return netip.Addr{}
	}
	remoteAddr := ""
	if dctx.HTTPRequest != nil {
		remoteAddr = dctx.HTTPRequest.RemoteAddr
	}
	if remoteAddr == "" && dctx.Addr.IsValid() {
		remoteAddr = dctx.Addr.String()
	}
	value := strings.Trim(strings.TrimSpace(extractRemoteIP(remoteAddr)), "[]")
	addr, err := netip.ParseAddr(value)
	if err != nil {
		return netip.Addr{}
	}
	return addr.Unmap()
}

func reverseProxyDNSRefusedResponse(dctx *dnsproxy.DNSContext) {
	if dctx == nil || dctx.Req == nil {
		return
	}
	response := new(dns.Msg)
	response.SetReply(dctx.Req)
	response.Rcode = dns.RcodeRefused
	dctx.Res = response
}

func (u *reverseProxyDNSSequentialUpstream) Exchange(req *dns.Msg) (*dns.Msg, error) {
	if u == nil || len(u.upstreams) == 0 {
		return nil, errors.New("dns primary upstream is unavailable")
	}
	errs := make([]error, 0, len(u.upstreams))
	for _, upstream := range u.upstreams {
		if upstream == nil {
			continue
		}
		response, err := upstream.Exchange(req.Copy())
		if err == nil && response != nil {
			return response, nil
		}
		if err == nil {
			err = errors.New("dns upstream returned an empty response")
		}
		errs = append(errs, err)
	}
	if len(errs) == 0 {
		return nil, errors.New("dns primary upstream is unavailable")
	}
	return nil, fmt.Errorf("all primary dns upstreams failed: %w", errors.Join(errs...))
}

func (u *reverseProxyDNSSequentialUpstream) Address() string {
	return "reverse-proxy-dns-primary"
}

func (u *reverseProxyDNSSequentialUpstream) Close() error {
	if u == nil {
		return nil
	}
	u.closeOnce.Do(func() {
		errs := make([]error, 0)
		for _, upstream := range u.upstreams {
			if upstream == nil {
				continue
			}
			if err := upstream.Close(); err != nil {
				errs = append(errs, err)
			}
		}
		u.closeErr = errors.Join(errs...)
	})
	return u.closeErr
}

func (r *reverseProxyDNSRoute) acquire() (*reverseProxyDNSRouteLease, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.Lock()
	if r.closing {
		r.mu.Unlock()
		return nil, false
	}
	r.active++
	r.mu.Unlock()
	return &reverseProxyDNSRouteLease{route: r}, true
}

func (l *reverseProxyDNSRouteLease) release() {
	if l == nil || l.route == nil {
		return
	}
	route := l.route
	l.route = nil
	cleanup := false
	route.mu.Lock()
	if route.active > 0 {
		route.active--
	}
	cleanup = route.closing && route.active == 0
	route.mu.Unlock()
	if cleanup {
		_ = route.cleanup()
	}
}

func (r *reverseProxyDNSRoute) cleanup() error {
	if r == nil {
		return nil
	}
	r.cleanupOnce.Do(func() {
		if r.cache != nil {
			r.cache.Close()
		}
		if r.resolver == nil {
			return
		}
		errs := make([]error, 0, 2)
		if r.resolver.UpstreamConfig != nil {
			if err := r.resolver.UpstreamConfig.Close(); err != nil {
				errs = append(errs, err)
			}
		}
		if r.resolver.Fallbacks != nil {
			if err := r.resolver.Fallbacks.Close(); err != nil {
				errs = append(errs, err)
			}
		}
		r.closeErr = errors.Join(errs...)
	})
	return r.closeErr
}

func (r *reverseProxyDNSRoute) close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	r.closing = true
	cleanup := r.active == 0
	r.mu.Unlock()
	if cleanup {
		return r.cleanup()
	}
	return nil
}

var reverseProxyDNSRuntime = &reverseProxyDNSRuntimeManager{
	running: make(map[string]*reverseProxyDNSInstance),
	retry:   make(map[string]reverseProxyDNSRetryState),
}

func (m *reverseProxyDNSRuntimeManager) sync(service *ReverseProxyService, rows []model.ReverseProxyRule) error {
	if m == nil {
		return nil
	}
	revision := uint64(0)
	certificateGeneration := currentReverseProxyCertificateGeneration()
	if service != nil {
		currentRevision, err := service.peekReverseProxyRevision()
		if err != nil {
			return err
		}
		revision = currentRevision
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.retry == nil {
		m.retry = make(map[string]reverseProxyDNSRetryState)
	}
	now := time.Now()

	want := make(map[string][]model.ReverseProxyRule)
	for i := range rows {
		row := rows[i]
		if !row.Enabled {
			continue
		}
		listenAlias := normalizeReverseProxyProtocolAlias(row.ListenProtocolAlias, row.ListenProtocol)
		if !reverseProxyProtocolIsDNS(listenAlias) || reverseProxyIsHTTPDNSAlias(listenAlias) {
			continue
		}
		key := reverseProxyDNSInstanceKey(&row)
		want[key] = append(want[key], row)
	}
	for key := range want {
		sortReverseProxyDNSRules(want[key])
	}
	certificateState := loadReverseProxyCertificateRenderState(database.GetDB(), rows)

	nextRunning := make(map[string]*reverseProxyDNSInstance, len(want)+len(m.running))
	reportState := func(groupRows []model.ReverseProxyRule, status string, runtimeErr error) {
		message := ""
		if runtimeErr != nil {
			message = strings.TrimSpace(runtimeErr.Error())
		}
		for i := range groupRows {
			reverseProxyRuntime.reportRuleState(groupRows[i].Id, status, message)
		}
	}
	type failedGroup struct {
		key  string
		rows []model.ReverseProxyRule
	}
	failed := make([]failedGroup, 0)
	stopped := make(map[*reverseProxyDNSInstance]struct{})
	markFailure := func(key string, groupRows []model.ReverseProxyRule, runtimeErr error) {
		if runtimeErr == nil {
			runtimeErr = errors.New("dns listener rebuild failed")
		}
		m.noteRetryLocked(key, runtimeErr, now)
		failed = append(failed, failedGroup{key: key, rows: groupRows})
		reportState(groupRows, "listener_error", runtimeErr)
	}
	markSuccess := func(key string, groupRows []model.ReverseProxyRule) {
		delete(m.retry, key)
		reportState(groupRows, "running", nil)
	}
	for key, groupRows := range want {
		stateKey := reverseProxyDNSRuntimeStateKey(groupRows, certificateState)
		listenerStateKey := reverseProxyDNSListenerRuntimeStateKey(groupRows, certificateState)
		if retry, waiting := m.retry[key]; waiting && !retry.NextRetryAt.IsZero() && now.Before(retry.NextRetryAt) {
			if instance := m.running[key]; instance != nil {
				nextRunning[key] = instance
			}
			markFailure := errors.New(strings.TrimSpace(retry.LastError))
			if strings.TrimSpace(retry.LastError) == "" {
				markFailure = errors.New("dns listener rebuild is waiting for retry")
			}
			reportState(groupRows, "listener_error", markFailure)
			failed = append(failed, failedGroup{key: key, rows: groupRows})
			continue
		}
		if instance, exists := m.running[key]; exists {
			if reverseProxyDNSInstanceMatchesRules(instance, groupRows, stateKey) {
				// Listener connection limits are adjustable and intentionally stay
				// off the resolver/cache state key.  Apply them without replacing
				// healthy routes or dropping their cache contents.
				instance.applyResourceLimits()
				nextRunning[key] = instance
				markSuccess(key, groupRows)
				continue
			}
			if instance.listenerStateKey == listenerStateKey {
				refreshed, refreshErr := refreshReverseProxyDNSInstanceRoutes(instance, groupRows, stateKey)
				if refreshErr != nil {
					nextRunning[key] = instance
					markFailure(key, groupRows, refreshErr)
					continue
				}
				if refreshed {
					nextRunning[key] = instance
					markSuccess(key, groupRows)
					continue
				}
			}
			restorePoint := snapshotReverseProxyDNSInstance(key, instance)
			candidate, createErr := newReverseProxyDNSInstance(service, key, groupRows, stateKey, listenerStateKey)
			if createErr != nil {
				// An overlapping old listener owns the socket. Stop it before the
				// second bind attempt, but retain a restore point so a failure does
				// not publish a stopped instance or unnecessarily drop the last
				// working configuration.
				if !reverseProxyDNSInstanceOverlapsRows(instance, groupRows) {
					nextRunning[key] = instance
					markFailure(key, groupRows, createErr)
					continue
				}
				if stopErr := instance.stop(); stopErr != nil {
					stopped[instance] = struct{}{}
					if restored, restoreErr := restoreReverseProxyDNSInstance(service, restorePoint); restoreErr == nil && restored != nil {
						nextRunning[restorePoint.key] = restored
					} else if restoreErr != nil {
						stopErr = errors.Join(stopErr, fmt.Errorf("restore previous dns listener: %w", restoreErr))
					}
					markFailure(key, groupRows, errors.Join(createErr, stopErr))
					continue
				}
				stopped[instance] = struct{}{}
				candidate, createErr = newReverseProxyDNSInstance(service, key, groupRows, stateKey, listenerStateKey)
				if createErr == nil {
					nextRunning[key] = candidate
					markSuccess(key, groupRows)
					continue
				}
				if restored, restoreErr := restoreReverseProxyDNSInstance(service, restorePoint); restoreErr == nil && restored != nil {
					nextRunning[restorePoint.key] = restored
				} else if restoreErr != nil {
					createErr = errors.Join(createErr, fmt.Errorf("restore previous dns listener: %w", restoreErr))
				}
				markFailure(key, groupRows, createErr)
				continue
			}
			if err := instance.stop(); err != nil {
				_ = candidate.stop()
				stopped[instance] = struct{}{}
				if restored, restoreErr := restoreReverseProxyDNSInstance(service, restorePoint); restoreErr == nil && restored != nil {
					nextRunning[restorePoint.key] = restored
				} else if restoreErr != nil {
					err = errors.Join(err, fmt.Errorf("restore previous dns listener: %w", restoreErr))
				}
				markFailure(key, groupRows, err)
				continue
			}
			stopped[instance] = struct{}{}
			nextRunning[key] = candidate
			markSuccess(key, groupRows)
			continue
		}
		blockers := reverseProxyDNSBlockingInstances(m.running, want, key, groupRows, stopped)
		restorePoints := make([]reverseProxyDNSInstanceRestorePoint, 0, len(blockers))
		stopErrors := make([]error, 0)
		for _, blocker := range blockers {
			if blocker.instance == nil {
				continue
			}
			restorePoints = append(restorePoints, snapshotReverseProxyDNSInstance(blocker.key, blocker.instance))
			if stopErr := blocker.instance.stop(); stopErr != nil {
				stopErrors = append(stopErrors, fmt.Errorf("%s: %w", blocker.key, stopErr))
			}
			stopped[blocker.instance] = struct{}{}
		}
		if stopErr := errors.Join(stopErrors...); stopErr != nil {
			restoreErrors := restoreReverseProxyDNSInstances(service, restorePoints, nextRunning)
			markFailure(key, groupRows, errors.Join(stopErr, errors.Join(restoreErrors...)))
			continue
		}
		instance, createErr := newReverseProxyDNSInstance(service, key, groupRows, stateKey, listenerStateKey)
		if createErr != nil {
			restoreErrors := restoreReverseProxyDNSInstances(service, restorePoints, nextRunning)
			markFailure(key, groupRows, errors.Join(createErr, errors.Join(restoreErrors...)))
			continue
		}
		nextRunning[key] = instance
		markSuccess(key, groupRows)
	}
	for key, instance := range m.running {
		if _, exists := nextRunning[key]; exists {
			continue
		}
		if _, wasStopped := stopped[instance]; wasStopped {
			continue
		}
		keepPrevious := false
		for _, failedGroup := range failed {
			if reverseProxyDNSInstanceOverlapsRows(instance, failedGroup.rows) || reverseProxyDNSInstanceSharesRuleIDs(instance, failedGroup.rows) {
				keepPrevious = true
				break
			}
		}
		if keepPrevious {
			nextRunning[key] = instance
			continue
		}
		_ = instance.stop()
		stopped[instance] = struct{}{}
	}
	for key := range m.retry {
		if _, wanted := want[key]; !wanted {
			delete(m.retry, key)
		}
	}
	m.running = nextRunning
	m.revision = revision
	m.certificateGeneration = certificateGeneration
	return nil
}

func (m *reverseProxyDNSRuntimeManager) needsSync(revision uint64, now time.Time) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if revision == 0 || revision != m.revision {
		return true
	}
	if currentReverseProxyCertificateGeneration() != m.certificateGeneration {
		return true
	}
	for _, retry := range m.retry {
		if retry.NextRetryAt.IsZero() || !now.Before(retry.NextRetryAt) {
			return true
		}
	}
	return false
}

func (m *reverseProxyDNSRuntimeManager) noteRetryLocked(key string, runtimeErr error, now time.Time) {
	if m == nil || strings.TrimSpace(key) == "" {
		return
	}
	state := m.retry[key]
	if state.RetryDelay <= 0 {
		state.RetryDelay = time.Second
	} else {
		state.RetryDelay *= 2
		if state.RetryDelay > time.Minute {
			state.RetryDelay = time.Minute
		}
	}
	state.NextRetryAt = now.Add(state.RetryDelay)
	if runtimeErr != nil {
		state.LastError = strings.TrimSpace(runtimeErr.Error())
	}
	m.retry[key] = state
}

func reverseProxyDNSInstanceOverlapsRows(instance *reverseProxyDNSInstance, rows []model.ReverseProxyRule) bool {
	if instance == nil || len(instance.rules) == 0 || len(rows) == 0 {
		return false
	}
	for i := range instance.rules {
		left := &instance.rules[i]
		leftAlias := normalizeReverseProxyProtocolAlias(left.ListenProtocolAlias, left.ListenProtocol)
		for j := range rows {
			right := &rows[j]
			rightAlias := normalizeReverseProxyProtocolAlias(right.ListenProtocolAlias, right.ListenProtocol)
			if left.ListenPort != right.ListenPort || !reverseProxyDNSProtocolSharesSocket(leftAlias, rightAlias) {
				continue
			}
			if reverseProxyListenIPSetsOverlap(reverseProxyDNSRuntimeListenIPs(left), reverseProxyDNSRuntimeListenIPs(right)) {
				return true
			}
		}
	}
	return false
}

type reverseProxyDNSInstanceRef struct {
	key      string
	instance *reverseProxyDNSInstance
}

type reverseProxyDNSInstanceRestorePoint struct {
	key              string
	rows             []model.ReverseProxyRule
	stateKey         string
	listenerStateKey string
}

func reverseProxyDNSInstanceSharesRuleIDs(instance *reverseProxyDNSInstance, rows []model.ReverseProxyRule) bool {
	if instance == nil || len(instance.rules) == 0 || len(rows) == 0 {
		return false
	}
	desiredIDs := make(map[uint]struct{}, len(rows))
	for i := range rows {
		if rows[i].Id != 0 {
			desiredIDs[rows[i].Id] = struct{}{}
		}
	}
	for i := range instance.rules {
		if _, exists := desiredIDs[instance.rules[i].Id]; exists {
			return true
		}
	}
	return false
}

func reverseProxyDNSBlockingInstances(running map[string]*reverseProxyDNSInstance, desired map[string][]model.ReverseProxyRule, desiredKey string, rows []model.ReverseProxyRule, stopped map[*reverseProxyDNSInstance]struct{}) []reverseProxyDNSInstanceRef {
	keys := make([]string, 0)
	for key, instance := range running {
		if key == desiredKey || instance == nil {
			continue
		}
		if _, stillDesired := desired[key]; stillDesired {
			continue
		}
		if _, alreadyStopped := stopped[instance]; alreadyStopped {
			continue
		}
		if reverseProxyDNSInstanceOverlapsRows(instance, rows) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	result := make([]reverseProxyDNSInstanceRef, 0, len(keys))
	for _, key := range keys {
		result = append(result, reverseProxyDNSInstanceRef{key: key, instance: running[key]})
	}
	return result
}

func snapshotReverseProxyDNSInstance(key string, instance *reverseProxyDNSInstance) reverseProxyDNSInstanceRestorePoint {
	if instance == nil {
		return reverseProxyDNSInstanceRestorePoint{}
	}
	return reverseProxyDNSInstanceRestorePoint{
		key:              key,
		rows:             append([]model.ReverseProxyRule(nil), instance.rules...),
		stateKey:         instance.runtimeStateKey,
		listenerStateKey: instance.listenerStateKey,
	}
}

func restoreReverseProxyDNSInstance(service *ReverseProxyService, point reverseProxyDNSInstanceRestorePoint) (*reverseProxyDNSInstance, error) {
	if service == nil || strings.TrimSpace(point.key) == "" || len(point.rows) == 0 {
		return nil, nil
	}
	return newReverseProxyDNSInstance(service, point.key, point.rows, point.stateKey, point.listenerStateKey)
}

func restoreReverseProxyDNSInstances(service *ReverseProxyService, points []reverseProxyDNSInstanceRestorePoint, target map[string]*reverseProxyDNSInstance) []error {
	errs := make([]error, 0)
	for _, point := range points {
		instance, err := restoreReverseProxyDNSInstance(service, point)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", point.key, err))
			continue
		}
		if instance != nil {
			target[point.key] = instance
		}
	}
	return errs
}

func reverseProxyListenIPSetsOverlap(a []string, b []string) bool {
	if len(a) == 0 {
		a = []string{"0.0.0.0"}
	}
	if len(b) == 0 {
		b = []string{"0.0.0.0"}
	}
	for _, left := range a {
		for _, right := range b {
			if reverseProxyListenIPsOverlap(left, right) {
				return true
			}
		}
	}
	return false
}

func reverseProxyListenIPsOverlap(a string, b string) bool {
	left := net.ParseIP(strings.TrimSpace(a))
	right := net.ParseIP(strings.TrimSpace(b))
	if left == nil || right == nil {
		return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
	}
	left4 := left.To4() != nil
	right4 := right.To4() != nil
	if left4 != right4 {
		return false
	}
	if left.Equal(right) {
		return true
	}
	return left.IsUnspecified() || right.IsUnspecified()
}

func reverseProxyDNSInstanceKey(row *model.ReverseProxyRule) string {
	if row == nil {
		return ""
	}
	alias := normalizeReverseProxyProtocolAlias(row.ListenProtocolAlias, row.ListenProtocol)
	listenIPs := reverseProxyDNSRuntimeListenIPsForAlias(row, alias)
	normalizedIPs := make([]string, 0, len(listenIPs))
	for _, item := range listenIPs {
		normalizedIPs = append(normalizedIPs, strings.ToLower(strings.TrimSpace(item)))
	}
	sort.Strings(normalizedIPs)
	keyParts := []string{
		alias,
		fmt.Sprintf("%d", row.ListenPort),
		strings.Join(normalizedIPs, ","),
	}
	if !reverseProxyDNSProtocolUsesPath(alias) {
		keyParts = append(keyParts, fmt.Sprintf("%d", row.Id))
	}
	return strings.Join(keyParts, "|")
}

func reverseProxyDNSRuntimeListenIPs(row *model.ReverseProxyRule) []string {
	return []string{"0.0.0.0", "::"}
}

func reverseProxyDNSRuntimeListenIPsForAlias(row *model.ReverseProxyRule, alias string) []string {
	return reverseProxyDNSRuntimeListenIPs(row)
}

func reverseProxyDNSIPv6WildcardAvailable() bool {
	conn, err := net.ListenPacket("udp6", "[::]:0")
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func sortReverseProxyDNSRules(rows []model.ReverseProxyRule) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].ListOrder == rows[j].ListOrder {
			return rows[i].Id < rows[j].Id
		}
		return rows[i].ListOrder < rows[j].ListOrder
	})
}

func reverseProxyDNSRuntimeStateKey(rows []model.ReverseProxyRule, certificateState map[uint]model.CertificateRecord) string {
	parts := make([]string, 0, len(rows))
	for i := range rows {
		row := rows[i]
		parts = append(parts, strings.Join([]string{
			fmt.Sprintf("%d", row.Id),
			fmt.Sprintf("%d", row.ListOrder),
			row.ListenProtocol,
			row.ListenProtocolAlias,
			fmt.Sprintf("%d", row.ListenPort),
			row.ListenDNSPath,
			row.TargetProtocol,
			row.TargetProtocolAlias,
			row.TargetAddresses,
			fmt.Sprintf("%d", row.TargetPort),
			row.TargetDNSPath,
			row.FallbackDNSUpstreams,
			fmt.Sprintf("%d", reverseProxyDNSUpstreamTimeoutSeconds(row.DNSUpstreamTimeoutSeconds)),
			fmt.Sprintf("%t", row.DNSCacheEnabled),
			fmt.Sprintf("%d", reverseProxyDNSCacheSizeBytes(row.DNSCacheSizeBytes)),
			fmt.Sprintf("%d", row.DNSCacheMinTTL),
			fmt.Sprintf("%d", row.DNSCacheMaxTTL),
			row.DNSAllowedCIDRs,
			fmt.Sprintf("%d", reverseProxyDNSRateLimitQPS(row.DNSRateLimitQPS)),
			fmt.Sprintf("%d", reverseProxyDNSMaxConcurrentQueries(row.DNSMaxConcurrentQueries)),
			fmt.Sprintf("%t", row.EDNSEnabled),
			row.EDNSMode,
			row.EDNSCustomIP,
			row.EDNSClientSubnetPolicy,
			fmt.Sprintf("%t", row.DisableIPv4Answer),
			fmt.Sprintf("%t", row.DisableIPv6Answer),
			row.IPStrategy,
			fmt.Sprintf("%t", row.UpstreamTLSVerify),
			row.CertificateRecordList,
			fmt.Sprintf("%d", row.CertificateRecordID),
			reverseProxyDNSCertificateStateKey(&row, certificateState),
			reverseProxyDNSCacheResourceStateKey(&row),
		}, "\x1f"))
	}
	return strings.Join(parts, "\x1e")
}

// reverseProxyDNSListenerRuntimeStateKey contains only settings that require
// rebinding the DNS listener itself.  Per-rule resolver settings intentionally
// stay out of this key so a cache or upstream change on one shared DoH path
// does not interrupt the other paths.
func reverseProxyDNSListenerRuntimeStateKey(rows []model.ReverseProxyRule, certificateState map[uint]model.CertificateRecord) string {
	parts := make([]string, 0, len(rows))
	for i := range rows {
		row := &rows[i]
		listenAlias := normalizeReverseProxyProtocolAlias(row.ListenProtocolAlias, row.ListenProtocol)
		listenPath := ""
		if reverseProxyDNSProtocolUsesPath(listenAlias) {
			listenPath = reverseProxyDNSRulePath(row)
		}
		parts = append(parts, strings.Join([]string{
			fmt.Sprintf("%d", row.Id),
			fmt.Sprintf("%d", row.ListOrder),
			strings.ToLower(strings.TrimSpace(row.ListenProtocol)),
			listenAlias,
			fmt.Sprintf("%d", row.ListenPort),
			listenPath,
			row.CertificateRecordList,
			fmt.Sprintf("%d", row.CertificateRecordID),
			reverseProxyDNSCertificateStateKey(row, certificateState),
		}, "\x1f"))
	}
	parts = append(parts, "listener-resources="+reverseProxyDNSListenerResourceStateKey(rows))
	return strings.Join(parts, "\x1e")
}

func reverseProxyDNSListenerResourceStateKey(rows []model.ReverseProxyRule) string {
	resources := reverseProxyResources.current()
	parts := make([]string, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for i := range rows {
		alias := normalizeReverseProxyProtocolAlias(rows[i].ListenProtocolAlias, rows[i].ListenProtocol)
		value := "dynamic"
		switch alias {
		case reverseProxyDNSProtocolDoH:
			value = "h2=" + fmt.Sprintf("%d", resources.HTTP2MaxConcurrentStreams)
		case reverseProxyDNSProtocolDoHH3, reverseProxyDNSProtocolDoQ:
			value = "quic=" + fmt.Sprintf("%d", resources.QUICMaxIncomingStreams)
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		parts = append(parts, value)
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func reverseProxyDNSRulePath(row *model.ReverseProxyRule) string {
	if row == nil {
		return ""
	}
	path := normalizeReverseProxyDNSPath(row.ListenDNSPath)
	if path == "" {
		return "/dns-query"
	}
	return path
}

func reverseProxyDNSRouteIdentity(row *model.ReverseProxyRule) string {
	if row == nil {
		return ""
	}
	listenAlias := normalizeReverseProxyProtocolAlias(row.ListenProtocolAlias, row.ListenProtocol)
	if reverseProxyDNSProtocolUsesPath(listenAlias) {
		return listenAlias + "\x1f" + reverseProxyDNSRulePath(row)
	}
	return listenAlias + "\x1f" + fmt.Sprintf("%d", row.Id)
}

func reverseProxyDNSRouteRuntimeStateKey(row *model.ReverseProxyRule) string {
	if row == nil {
		return ""
	}
	return strings.Join([]string{
		strings.ToLower(strings.TrimSpace(row.TargetProtocol)),
		normalizeReverseProxyProtocolAlias(row.TargetProtocolAlias, row.TargetProtocol),
		row.TargetAddresses,
		fmt.Sprintf("%d", row.TargetPort),
		normalizeReverseProxyDNSPath(row.TargetDNSPath),
		row.FallbackDNSUpstreams,
		fmt.Sprintf("%d", reverseProxyDNSUpstreamTimeoutSeconds(row.DNSUpstreamTimeoutSeconds)),
		fmt.Sprintf("%t", row.DNSCacheEnabled),
		fmt.Sprintf("%d", reverseProxyDNSCacheSizeBytes(row.DNSCacheSizeBytes)),
		fmt.Sprintf("%d", row.DNSCacheMinTTL),
		fmt.Sprintf("%d", row.DNSCacheMaxTTL),
		row.DNSAllowedCIDRs,
		fmt.Sprintf("%d", reverseProxyDNSRateLimitQPS(row.DNSRateLimitQPS)),
		fmt.Sprintf("%d", reverseProxyDNSMaxConcurrentQueries(row.DNSMaxConcurrentQueries)),
		fmt.Sprintf("%t", row.EDNSEnabled),
		row.EDNSMode,
		row.EDNSCustomIP,
		row.EDNSClientSubnetPolicy,
		fmt.Sprintf("%t", row.DisableIPv4Answer),
		fmt.Sprintf("%t", row.DisableIPv6Answer),
		strings.ToLower(strings.TrimSpace(row.IPStrategy)),
		fmt.Sprintf("%t", row.UpstreamTLSVerify),
		reverseProxyDNSCacheResourceStateKey(row),
	}, "\x1f")
}

// reverseProxyDNSCacheResourceStateKey includes only memory policy that
// changes an already-created cache.  Global DNS/request/connection guards
// are adjustable limiters, so putting every resource field in this key would
// unnecessarily recreate resolver routes on a harmless limit change.
func reverseProxyDNSCacheResourceStateKey(row *model.ReverseProxyRule) string {
	if row == nil || !row.DNSCacheEnabled {
		return ""
	}
	resources := reverseProxyResources.current()
	ruleLimit := row.MemoryLimitBytes
	if ruleLimit <= 0 {
		ruleLimit = resources.DefaultRuleMemoryLimitBytes
	}
	return fmt.Sprintf("cache-memory=%d|%d", resources.MemoryPoolBytes, ruleLimit)
}

func (h *reverseProxyDNSRuleHandler) routeForRuleLocked(row *model.ReverseProxyRule) *reverseProxyDNSRoute {
	if h == nil || row == nil {
		return nil
	}
	if route := h.routesByRule[row.Id]; route != nil {
		return route
	}
	listenAlias := normalizeReverseProxyProtocolAlias(row.ListenProtocolAlias, row.ListenProtocol)
	if reverseProxyDNSProtocolUsesPath(listenAlias) {
		return h.routes[reverseProxyDNSRulePath(row)]
	}
	if h.defaultRoute != nil && h.defaultRoute.rule != nil && h.defaultRoute.rule.Id == row.Id {
		return h.defaultRoute
	}
	return nil
}

type reverseProxyDNSRouteReplacement struct {
	row      *model.ReverseProxyRule
	old      *reverseProxyDNSRoute
	next     *reverseProxyDNSRoute
	usesPath bool
	path     string
}

// refreshReverseProxyDNSInstanceRoutes replaces only changed resolver routes
// while keeping their listener alive.  It is used for cache, fallback,
// timeout, EDNS, response-filter, and upstream changes on an otherwise
// unchanged listener topology.
func refreshReverseProxyDNSInstanceRoutes(instance *reverseProxyDNSInstance, rows []model.ReverseProxyRule, stateKey string) (bool, error) {
	if instance == nil || instance.handler == nil || len(instance.rules) != len(rows) {
		return false, nil
	}
	oldByID := make(map[uint]*model.ReverseProxyRule, len(instance.rules))
	for i := range instance.rules {
		row := &instance.rules[i]
		if row.Id == 0 {
			return false, nil
		}
		oldByID[row.Id] = row
	}
	if len(oldByID) != len(rows) {
		return false, nil
	}

	handler := instance.handler
	currentRoutes := make(map[uint]*reverseProxyDNSRoute, len(rows))
	handler.mu.RLock()
	for i := range rows {
		row := &rows[i]
		oldRow, exists := oldByID[row.Id]
		if !exists || reverseProxyDNSRouteIdentity(oldRow) != reverseProxyDNSRouteIdentity(row) {
			handler.mu.RUnlock()
			return false, nil
		}
		route := handler.routeForRuleLocked(row)
		if route == nil || route.rule == nil || route.rule.Id != row.Id {
			handler.mu.RUnlock()
			return false, nil
		}
		currentRoutes[row.Id] = route
	}
	handler.mu.RUnlock()

	replacements := make([]reverseProxyDNSRouteReplacement, 0)
	closeNewRoutes := func() {
		for _, replacement := range replacements {
			_ = replacement.next.close()
		}
	}
	for i := range rows {
		row := &rows[i]
		oldRoute := currentRoutes[row.Id]
		// The route key is captured when its resolver/cache was created.  Do
		// not recompute the old side from the mutable global resource settings:
		// doing so would hide a changed default memory limit and retain a cache
		// that was admitted under the old policy.
		if oldRoute != nil && oldRoute.runtimeStateKey == reverseProxyDNSRouteRuntimeStateKey(row) {
			continue
		}
		next, err := buildReverseProxyDNSRoute(row)
		if err != nil {
			closeNewRoutes()
			return false, err
		}
		listenAlias := normalizeReverseProxyProtocolAlias(row.ListenProtocolAlias, row.ListenProtocol)
		replacements = append(replacements, reverseProxyDNSRouteReplacement{
			row:      row,
			old:      currentRoutes[row.Id],
			next:     next,
			usesPath: reverseProxyDNSProtocolUsesPath(listenAlias),
			path:     reverseProxyDNSRulePath(row),
		})
	}

	handler.mu.Lock()
	for _, replacement := range replacements {
		if handler.routeForRuleLocked(replacement.row) != replacement.old {
			handler.mu.Unlock()
			closeNewRoutes()
			return false, nil
		}
	}
	for _, replacement := range replacements {
		handler.routesByRule[replacement.row.Id] = replacement.next
		if replacement.usesPath {
			handler.routes[replacement.path] = replacement.next
		}
		if handler.defaultRoute == replacement.old {
			handler.defaultRoute = replacement.next
		}
	}
	handler.mu.Unlock()

	closed := make(map[*reverseProxyDNSRoute]struct{}, len(replacements))
	for _, replacement := range replacements {
		if _, exists := closed[replacement.old]; exists {
			continue
		}
		closed[replacement.old] = struct{}{}
		_ = replacement.old.close()
	}
	instance.rules = cloneReverseProxyRules(rows)
	instance.runtimeStateKey = stateKey
	instance.applyResourceLimits()
	return true, nil
}

func (i *reverseProxyDNSInstance) applyResourceLimits() {
	if i == nil || i.connectionLimiter == nil {
		return
	}
	i.connectionLimiter.SetMax(reverseProxyResources.current().ListenerConnectionLimit)
}

func (m *reverseProxyDNSRuntimeManager) stopAll() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var firstErr error
	for id, instance := range m.running {
		if err := instance.stop(); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(m.running, id)
	}
	m.retry = make(map[string]reverseProxyDNSRetryState)
	m.revision = 0
	m.certificateGeneration = 0
	return firstErr
}

func reverseProxyDNSInstanceMatchesRules(instance *reverseProxyDNSInstance, rows []model.ReverseProxyRule, stateKey string) bool {
	if instance == nil {
		return false
	}
	return instance.runtimeStateKey == stateKey
}

func newReverseProxyDNSInstance(service *ReverseProxyService, key string, rows []model.ReverseProxyRule, stateKey string, listenerStateKey string) (*reverseProxyDNSInstance, error) {
	if service == nil || len(rows) == 0 {
		return nil, errors.New("dns reverse proxy instance init failed: invalid rule")
	}
	handler, err := buildReverseProxyDNSRuleHandler(rows)
	if err != nil {
		return nil, err
	}
	instance, err := newReverseProxyOwnedDNSListenerInstance(service, key, rows, handler, stateKey, listenerStateKey)
	if err != nil {
		_ = closeReverseProxyDNSHandler(handler)
		return nil, err
	}
	for _, ruleID := range reverseProxyDNSRuleIDs(rows) {
		reverseProxyRuntime.reportRuleState(ruleID, "running", "")
	}
	return instance, nil
}

func shutdownReverseProxyDNSServer(server *dns.Server) error {
	if server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), reverseProxyDNSShutdownTimeout)
	err := server.ShutdownContext(ctx)
	cancel()
	if err == nil || errors.Is(err, net.ErrClosed) {
		return nil
	}
	if server.Listener != nil {
		_ = server.Listener.Close()
	}
	if server.PacketConn != nil {
		_ = server.PacketConn.Close()
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func (i *reverseProxyDNSInstance) stop() error {
	if i == nil {
		return nil
	}
	if i.cancel != nil {
		i.cancel()
	}
	var firstErr error
	for _, server := range i.dnsServers {
		if server == nil {
			continue
		}
		err := shutdownReverseProxyDNSServer(server)
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	for _, listener := range i.doqListeners {
		if listener != nil {
			_ = listener.Close()
		}
	}
	for _, conn := range i.doqPacketConns {
		if conn != nil {
			_ = conn.Close()
		}
	}
	if i.doneCh != nil {
		select {
		case <-i.doneCh:
		case <-time.After(reverseProxyDNSShutdownTimeout):
		}
	}
	if err := closeReverseProxyDNSHandler(i.handler); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

func buildReverseProxyDNSRuleHandler(rows []model.ReverseProxyRule) (*reverseProxyDNSRuleHandler, error) {
	if len(rows) == 0 {
		return nil, errors.New("dns reverse proxy handler init failed: invalid rule")
	}
	handler := &reverseProxyDNSRuleHandler{
		routes:       make(map[string]*reverseProxyDNSRoute),
		routesByRule: make(map[uint]*reverseProxyDNSRoute),
		logger:       slog.Default(),
	}
	for i := range rows {
		route, err := buildReverseProxyDNSRoute(&rows[i])
		if err != nil {
			closeReverseProxyDNSHandler(handler)
			return nil, err
		}
		if handler.defaultRoute == nil {
			handler.defaultRoute = route
		}
		handler.routesByRule[rows[i].Id] = route
		alias := normalizeReverseProxyProtocolAlias(rows[i].ListenProtocolAlias, rows[i].ListenProtocol)
		if reverseProxyDNSProtocolUsesPath(alias) {
			path := normalizeReverseProxyDNSPath(rows[i].ListenDNSPath)
			if path == "" {
				path = "/dns-query"
			}
			if _, exists := handler.routes[path]; !exists {
				handler.routes[path] = route
			}
		}
	}
	return handler, nil
}

func buildReverseProxyDNSRoute(row *model.ReverseProxyRule) (*reverseProxyDNSRoute, error) {
	admission, err := buildReverseProxyDNSAdmission(row)
	if err != nil {
		return nil, err
	}
	targetAlias := normalizeReverseProxyProtocolAlias(row.TargetProtocolAlias, row.TargetProtocol)
	targets := decodeReverseProxyList(row.TargetAddresses)
	if len(targets) == 0 {
		return nil, errors.New("dns reverse proxy target is empty")
	}

	listenIPs := reverseProxyDNSRuntimeListenIPs(row)
	for _, target := range targets {
		if address, parseErr := netip.ParseAddr(strings.Trim(strings.TrimSpace(target), "[]")); parseErr == nil &&
			reverseProxyResolvedTargetLoopsToListener(listenIPs, row.ListenPort, row.TargetPort, address.String()) {
			return nil, common.NewError("dns target points back to the local listener")
		}
	}
	opts := buildReverseProxyDNSUpstreamOptions(row, targetAlias)

	upstreams := make([]dnsupstream.Upstream, 0, len(targets))
	for _, target := range targets {
		targetPath := row.TargetDNSPath
		if reverseProxyDNSProtocolUsesPath(targetAlias) && strings.TrimSpace(targetPath) == "" {
			targetPath = "/dns-query"
		}
		address, err := buildReverseProxyDNSUpstreamAddress(targetAlias, target, row.TargetPort, targetPath)
		if err != nil {
			closeReverseProxyDNSUpstreams(upstreams)
			return nil, err
		}
		ups, err := dnsupstream.AddressToUpstream(address, opts.Clone())
		if err != nil {
			closeReverseProxyDNSUpstreams(upstreams)
			return nil, err
		}
		upstreams = append(upstreams, ups)
	}

	primaryConfig := &dnsproxy.UpstreamConfig{
		Upstreams: []dnsupstream.Upstream{&reverseProxyDNSSequentialUpstream{upstreams: upstreams}},
	}
	fallbacks, err := buildReverseProxyDNSFallbackUpstreamConfig(row)
	if err != nil {
		_ = primaryConfig.Close()
		return nil, err
	}

	resolverConfig := &dnsproxy.Config{
		UpstreamMode:   dnsproxy.UpstreamModeLoadBalance,
		UpstreamConfig: primaryConfig,
		Fallbacks:      fallbacks,
		// dnsproxy remains an upstream resolver only.  The panel owns the
		// shared TTL/LRU cache so cache memory is visible to resource control.
		CacheEnabled:           false,
		EnableEDNSClientSubnet: row.EDNSEnabled,
		Logger:                 slog.Default(),
	}
	resolver, err := dnsproxy.New(resolverConfig)
	if err != nil {
		_ = primaryConfig.Close()
		if fallbacks != nil {
			_ = fallbacks.Close()
		}
		return nil, err
	}

	return &reverseProxyDNSRoute{
		rule:            cloneReverseProxyRule(row),
		runtimeStateKey: reverseProxyDNSRouteRuntimeStateKey(row),
		resolver:        resolver,
		admission:       admission,
		cache:           newReverseProxyDNSResponseCache(row),
	}, nil
}

func (h *reverseProxyDNSRuleHandler) ServeDNS(ctx context.Context, _ *dnsproxy.Proxy, dctx *dnsproxy.DNSContext) error {
	if h == nil || dctx == nil || dctx.Req == nil {
		return errors.New("dns reverse proxy handler received empty request")
	}
	h.mu.RLock()
	route := h.defaultRoute
	if dctx.HTTPRequest != nil {
		if dctx.HTTPRequest.URL == nil {
			h.mu.RUnlock()
			return errors.New("dns reverse proxy request url is unavailable")
		}
		path := normalizeReverseProxyDNSPath(dctx.HTTPRequest.URL.Path)
		selected := h.routes[path]
		if selected == nil {
			h.mu.RUnlock()
			return fmt.Errorf("dns reverse proxy path is not configured: %s", path)
		}
		route = selected
	}
	if route == nil {
		h.mu.RUnlock()
		return errors.New("dns reverse proxy route is unavailable")
	}
	h.mu.RUnlock()
	return h.serveDNSRoute(ctx, dctx, route)
}

func (h *reverseProxyDNSRuleHandler) serveDNSRule(ctx context.Context, dctx *dnsproxy.DNSContext, ruleID uint) error {
	if h == nil || dctx == nil || dctx.Req == nil || ruleID == 0 {
		return errors.New("dns reverse proxy rule is unavailable")
	}
	h.mu.RLock()
	route := h.routesByRule[ruleID]
	h.mu.RUnlock()
	if route == nil {
		return errors.New("dns reverse proxy rule route is unavailable")
	}
	return h.serveDNSRoute(ctx, dctx, route)
}

func (h *reverseProxyDNSRuleHandler) serveDNSRoute(ctx context.Context, dctx *dnsproxy.DNSContext, route *reverseProxyDNSRoute) error {
	if h == nil || dctx == nil || dctx.Req == nil || route == nil {
		return errors.New("dns reverse proxy route is unavailable")
	}
	lease, acquired := route.acquire()
	if !acquired {
		return errors.New("dns reverse proxy route is retiring")
	}
	defer lease.release()
	if route.rule == nil || route.resolver == nil {
		return errors.New("dns reverse proxy resolver is unavailable")
	}
	rule := route.rule
	resolver := route.resolver
	release, rejected := route.admission.acquire(dctx)
	if rejected != "" {
		reverseProxyDNSRefusedResponse(dctx)
		reverseProxyRuntime.reportRuleState(rule.Id, "running", "")
		return nil
	}
	defer release()
	req := dctx.Req.Copy()
	reverseProxyDNSApplyEDNSPolicy(req, dctx, rule)
	if cached := route.cache.Get(req); cached != nil {
		dctx.Res = cached
		reverseProxyRuntime.reportRuleState(rule.Id, "running", "")
		return nil
	}
	resolverContext := &dnsproxy.DNSContext{
		Req:             req,
		Addr:            dctx.Addr,
		Proto:           dctx.Proto,
		IsPrivateClient: dctx.IsPrivateClient,
		HTTPRequest:     dctx.HTTPRequest,
	}
	err := resolver.Resolve(ctx, resolverContext)
	if resolverContext.Res != nil {
		if rule.DisableIPv4Answer || rule.DisableIPv6Answer {
			reverseProxyDNSFilterResponse(resolverContext.Res, rule.DisableIPv4Answer, rule.DisableIPv6Answer)
		}
		route.cache.Put(req, resolverContext.Res, rule.DNSCacheMinTTL, rule.DNSCacheMaxTTL)
		dctx.Res = resolverContext.Res
	}
	if err == nil {
		reverseProxyRuntime.reportRuleState(rule.Id, "running", "")
		return nil
	}
	reverseProxyRuntime.reportRuleState(rule.Id, "upstream_error", err.Error())
	return err
}

func buildReverseProxyDNSUpstreamOptions(row *model.ReverseProxyRule, targetAlias string) *dnsupstream.Options {
	listenIPs := reverseProxyDNSRuntimeListenIPs(row)
	loopGuard := func(address netip.Addr) bool {
		if row == nil {
			return false
		}
		return reverseProxyResolvedTargetLoopsToListener(listenIPs, row.ListenPort, row.TargetPort, address.String())
	}
	opts := &dnsupstream.Options{
		Timeout:            time.Duration(reverseProxyDNSUpstreamTimeoutSeconds(row.DNSUpstreamTimeoutSeconds)) * time.Second,
		InsecureSkipVerify: !row.UpstreamTLSVerify,
		Logger:             slog.Default(),
		Bootstrap: reverseProxyDNSIPStrategyResolver{
			base:      net.DefaultResolver,
			strategy:  row.IPStrategy,
			loopGuard: loopGuard,
		},
		PreferIPv6: strings.EqualFold(strings.TrimSpace(row.IPStrategy), reverseProxyIPStrategyPreferIPv6) ||
			strings.EqualFold(strings.TrimSpace(row.IPStrategy), reverseProxyIPStrategyIPv6Only),
	}
	if targetAlias == reverseProxyDNSProtocolDoH {
		opts.HTTPVersions = []dnsupstream.HTTPVersion{dnsupstream.HTTPVersion11, dnsupstream.HTTPVersion2}
	}
	if targetAlias == reverseProxyDNSProtocolDoHH3 {
		opts.HTTPVersions = []dnsupstream.HTTPVersion{dnsupstream.HTTPVersion3}
	}
	return opts
}

func buildReverseProxyDNSFallbackUpstreamOptions(row *model.ReverseProxyRule) *dnsupstream.Options {
	opts := buildReverseProxyDNSUpstreamOptions(row, "")
	opts.HTTPVersions = []dnsupstream.HTTPVersion{
		dnsupstream.HTTPVersion11,
		dnsupstream.HTTPVersion2,
		dnsupstream.HTTPVersion3,
	}
	return opts
}

func reverseProxyDNSUpstreamLines(raw string) []string {
	raw = normalizeReverseProxyDNSUpstreamsText(raw)
	if raw == "" {
		return nil
	}
	return strings.Split(raw, "\n")
}

func reverseProxyDNSUpstreamConfigHasUsableUpstream(config *dnsproxy.UpstreamConfig) bool {
	if config == nil {
		return false
	}
	if len(config.Upstreams) > 0 {
		return true
	}
	for _, upstreams := range config.DomainReservedUpstreams {
		if len(upstreams) > 0 {
			return true
		}
	}
	for _, upstreams := range config.SpecifiedDomainUpstreams {
		if len(upstreams) > 0 {
			return true
		}
	}
	return false
}

func buildReverseProxyDNSFallbackUpstreamConfig(row *model.ReverseProxyRule) (*dnsproxy.UpstreamConfig, error) {
	if row == nil {
		return nil, errors.New("dns fallback upstream rule is nil")
	}
	lines := reverseProxyDNSUpstreamLines(row.FallbackDNSUpstreams)
	if len(lines) == 0 {
		return nil, nil
	}
	config, err := dnsproxy.ParseUpstreamsConfig(lines, buildReverseProxyDNSFallbackUpstreamOptions(row))
	if err != nil {
		if config != nil {
			_ = config.Close()
		}
		return nil, fmt.Errorf("invalid fallback dns upstreams: %w", err)
	}
	if !reverseProxyDNSUpstreamConfigHasUsableUpstream(config) {
		if config != nil {
			_ = config.Close()
		}
		return nil, errors.New("fallback dns upstreams require at least one valid upstream")
	}
	return config, nil
}

func validateReverseProxyDNSCacheSettings(row reverseProxyNormalizedRule) error {
	if row.dnsUpstreamTimeoutSeconds < 1 || row.dnsUpstreamTimeoutSeconds > reverseProxyDNSMaxUpstreamTimeoutSeconds {
		return common.NewError("dns upstream timeout must be between 1 and 120 seconds")
	}
	if row.dnsCacheSizeBytes < 1 {
		return common.NewError("dns cache size must be greater than zero")
	}
	if row.dnsCacheMinTTL < 0 || row.dnsCacheMaxTTL < 0 {
		return common.NewError("dns cache ttl must not be negative")
	}
	if row.dnsCacheMinTTL > reverseProxyDNSMaxCacheTTLSeconds || row.dnsCacheMaxTTL > reverseProxyDNSMaxCacheTTLSeconds {
		return common.NewError("dns cache ttl is too large")
	}
	if row.dnsCacheMaxTTL > 0 && row.dnsCacheMinTTL > row.dnsCacheMaxTTL {
		return common.NewError("dns cache minimum ttl must not exceed maximum ttl")
	}
	return nil
}

func validateReverseProxyDNSFallbackUpstreams(row reverseProxyNormalizedRule) error {
	if strings.TrimSpace(row.fallbackDNSUpstreams) == "" {
		return nil
	}
	config, err := buildReverseProxyDNSFallbackUpstreamConfig(&model.ReverseProxyRule{
		FallbackDNSUpstreams:      row.fallbackDNSUpstreams,
		DNSUpstreamTimeoutSeconds: row.dnsUpstreamTimeoutSeconds,
		IPStrategy:                row.ipStrategy,
		UpstreamTLSVerify:         row.upstreamTLSVerify,
	})
	if config != nil {
		_ = config.Close()
	}
	if err != nil {
		return common.NewError(err.Error())
	}
	return nil
}

func (r reverseProxyDNSIPStrategyResolver) LookupNetIP(ctx context.Context, network string, host string) ([]netip.Addr, error) {
	base := r.base
	if base == nil {
		base = net.DefaultResolver
	}
	addrs, err := base.LookupNetIP(ctx, network, host)
	if err != nil {
		return nil, err
	}
	eligible := make([]netip.Addr, 0, len(addrs))
	blockedByLoop := false
	for _, addr := range addrs {
		if r.loopGuard != nil && r.loopGuard(addr.Unmap()) {
			blockedByLoop = true
			continue
		}
		eligible = append(eligible, addr)
	}
	if len(eligible) == 0 && blockedByLoop {
		return nil, common.NewError("resolved dns upstream points back to the local listener")
	}
	strategy := strings.ToLower(strings.TrimSpace(r.strategy))
	if strategy != reverseProxyIPStrategyIPv4Only && strategy != reverseProxyIPStrategyIPv6Only {
		return eligible, nil
	}
	filtered := make([]netip.Addr, 0, len(eligible))
	for _, addr := range eligible {
		if strategy == reverseProxyIPStrategyIPv4Only && addr.Is4() {
			filtered = append(filtered, addr)
			continue
		}
		if strategy == reverseProxyIPStrategyIPv6Only && addr.Is6() {
			filtered = append(filtered, addr)
		}
	}
	return filtered, nil
}

func buildReverseProxyDNSUpstreamAddress(alias string, target string, port int, path string) (string, error) {
	host := strings.TrimSpace(target)
	if host == "" {
		return "", errors.New("dns target host is empty")
	}
	switch alias {
	case reverseProxyDNSProtocolUDP:
		return "udp://" + net.JoinHostPort(host, fmt.Sprintf("%d", port)), nil
	case reverseProxyDNSProtocolTCP:
		return "tcp://" + net.JoinHostPort(host, fmt.Sprintf("%d", port)), nil
	case reverseProxyDNSProtocolDoT:
		return "tls://" + net.JoinHostPort(host, fmt.Sprintf("%d", port)), nil
	case reverseProxyDNSProtocolDoQ:
		return "quic://" + net.JoinHostPort(host, fmt.Sprintf("%d", port)), nil
	case reverseProxyDNSProtocolDoH:
		return (&url.URL{Scheme: "https", Host: net.JoinHostPort(host, fmt.Sprintf("%d", port)), Path: normalizeReverseProxyDNSPath(path)}).String(), nil
	case reverseProxyDNSProtocolDoHH3:
		return (&url.URL{Scheme: "h3", Host: net.JoinHostPort(host, fmt.Sprintf("%d", port)), Path: normalizeReverseProxyDNSPath(path)}).String(), nil
	default:
		return "", fmt.Errorf("unsupported dns target protocol: %s", alias)
	}
}

func buildReverseProxyDNSServerTLSConfig(service *ReverseProxyService, rows []model.ReverseProxyRule, nextProtos []string) (*tls.Config, error) {
	if service == nil || len(rows) == 0 {
		return nil, errors.New("dns reverse proxy tls config failed")
	}
	rulePtrs := make([]*model.ReverseProxyRule, 0, len(rows))
	hasCertificate := false
	for i := range rows {
		if len(reverseProxyRuleCertificateIDs(&rows[i])) > 0 {
			hasCertificate = true
		}
		rulePtrs = append(rulePtrs, &rows[i])
	}
	if !hasCertificate {
		return nil, errors.New("dns tls listener requires certificate")
	}
	certBindingsByRule, orderedItems, err := service.loadRuleCertificates(rulePtrs)
	if err != nil {
		return nil, err
	}
	items := make([]*reverseProxyRuleCertificateBinding, 0, len(orderedItems))
	for _, item := range orderedItems {
		if item != nil {
			items = append(items, item)
		}
	}
	if len(items) == 0 {
		return nil, errors.New("dns tls listener certificate is unavailable")
	}
	selector := &reverseProxyListenerGroup{orderedCertBindings: items}
	selector.configureIPCertificateIndexesLocked()
	config := &tls.Config{
		MinVersion: tls.VersionTLS12,
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			serverName := ""
			localIP := ""
			remoteIP := ""
			if hello != nil {
				serverName = reverseProxyNormalizeServerName(hello.ServerName)
				if hello.Conn != nil {
					localIP = reverseProxyNormalizeLocalIP(hello.Conn.LocalAddr())
					remoteIP = extractRemoteIP(hello.Conn.RemoteAddr().String())
				}
			}
			if serverName == "" {
				var candidates []*reverseProxyRuleCertificateBinding
				if localAddr := reverseProxyParseIPLiteral(localIP); localAddr != nil {
					localValue := localAddr.String()
					if _, configured := selector.ipCertificateUniverse[localValue]; configured {
						candidates = selector.ipCertBindings[localValue]
					} else if reverseProxyLocalAddressMayHidePublicTarget(localAddr) {
						candidates = selector.natFallbackCertificatesForLocalIP(localAddr)
					}
				}
				if selected := reverseProxyFallbackCertificateBinding(candidates); selected != nil && selected.Certificate != nil {
					return selected.Certificate, nil
				}
				reverseProxyRuntime.registerMismatch(remoteIP, "missing_sni")
				reverseProxyCloseClientHelloConn(hello)
				return nil, common.NewError("dns tls listener has no unambiguous ip certificate for an empty sni")
			}
			if sniIP := reverseProxyParseIPLiteral(serverName); sniIP != nil {
				if localAddr := reverseProxyParseIPLiteral(localIP); localAddr != nil {
					localValue := localAddr.String()
					_, configured := selector.ipCertificateUniverse[localValue]
					if (configured || !reverseProxyLocalAddressMayHidePublicTarget(localAddr)) && !sniIP.Equal(localAddr) {
						reverseProxyRuntime.registerMismatch(remoteIP, "cross_ip_sni")
						reverseProxyCloseClientHelloConn(hello)
						return nil, common.NewError("dns tls ip sni does not match the visible local target ip")
					}
				}
				if selected := reverseProxyFallbackCertificateBinding(selector.ipCertBindings[sniIP.String()]); selected != nil && selected.Certificate != nil {
					return selected.Certificate, nil
				}
				reverseProxyRuntime.registerMismatch(remoteIP, "ip_sni")
				reverseProxyCloseClientHelloConn(hello)
				return nil, common.NewError("dns tls listener has no certificate covering the ip sni")
			}
			exactCandidates := make([]*reverseProxyRuleCertificateBinding, 0)
			wildcardCandidates := make([]*reverseProxyRuleCertificateBinding, 0)
			for index := range rows {
				rule := &rows[index]
				if !reverseProxyRuleServerNameMatch(rule, serverName) {
					continue
				}
				exact, wildcard := reverseProxySplitSNICertificateCandidates(certBindingsByRule[rule.Id], serverName)
				exactCandidates = append(exactCandidates, exact...)
				wildcardCandidates = append(wildcardCandidates, wildcard...)
			}
			if selected := reverseProxyFallbackCertificateBinding(exactCandidates); selected != nil && selected.Certificate != nil {
				return selected.Certificate, nil
			}
			if selected := reverseProxyFallbackCertificateBinding(wildcardCandidates); selected != nil && selected.Certificate != nil {
				return selected.Certificate, nil
			}
			reverseProxyRuntime.registerMismatch(remoteIP, "unrecognized_sni")
			reverseProxyCloseClientHelloConn(hello)
			return nil, common.NewError("no certificate available for requested sni")
		},
	}
	if len(nextProtos) > 0 {
		config.NextProtos = append([]string(nil), nextProtos...)
	}
	if normalizeReverseProxyProtocolAlias(rows[0].ListenProtocolAlias, rows[0].ListenProtocol) == reverseProxyDNSProtocolDoQ {
		config.MinVersion = tls.VersionTLS13
	}
	return config, nil
}

func buildReverseProxyDNSUDPListenAddrs(items []string, port int) []*net.UDPAddr {
	out := make([]*net.UDPAddr, 0, len(items))
	for _, item := range items {
		addr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(strings.TrimSpace(item), fmt.Sprintf("%d", port)))
		if err == nil && addr != nil {
			out = append(out, addr)
		}
	}
	return out
}

func buildReverseProxyDNSTCPListenAddrs(items []string, port int) []*net.TCPAddr {
	out := make([]*net.TCPAddr, 0, len(items))
	for _, item := range items {
		addr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(strings.TrimSpace(item), fmt.Sprintf("%d", port)))
		if err == nil && addr != nil {
			out = append(out, addr)
		}
	}
	return out
}

func buildReverseProxyDNSDoHListenAddrs(items []string, port int) []netip.AddrPort {
	out := make([]netip.AddrPort, 0, len(items))
	for _, item := range items {
		ip := net.ParseIP(strings.TrimSpace(item))
		if ip == nil {
			continue
		}
		addr, ok := netip.AddrFromSlice(ip)
		if !ok {
			continue
		}
		out = append(out, netip.AddrPortFrom(addr, uint16(port)))
	}
	return out
}

func buildReverseProxyDNSDoHRoutes(path string) []string {
	path = normalizeReverseProxyDNSPath(path)
	if path == "" {
		path = "/dns-query"
	}
	return []string{
		http.MethodGet + " " + path,
		http.MethodPost + " " + path,
	}
}

func cloneReverseProxyRule(row *model.ReverseProxyRule) *model.ReverseProxyRule {
	if row == nil {
		return nil
	}
	clone := *row
	return &clone
}

func cloneReverseProxyRules(rows []model.ReverseProxyRule) []model.ReverseProxyRule {
	out := make([]model.ReverseProxyRule, len(rows))
	copy(out, rows)
	return out
}

func reverseProxyDNSRuleIDs(rows []model.ReverseProxyRule) []uint {
	ids := make([]uint, 0, len(rows))
	for i := range rows {
		if rows[i].Id > 0 {
			ids = append(ids, rows[i].Id)
		}
	}
	return ids
}

func closeReverseProxyDNSUpstreams(items []dnsupstream.Upstream) {
	for _, item := range items {
		if item != nil {
			_ = item.Close()
		}
	}
}

func closeReverseProxyDNSHandler(handler *reverseProxyDNSRuleHandler) error {
	if handler == nil {
		return nil
	}
	handler.mu.RLock()
	defaultRoute := handler.defaultRoute
	routes := make([]*reverseProxyDNSRoute, 0, len(handler.routes)+len(handler.routesByRule))
	for _, route := range handler.routes {
		routes = append(routes, route)
	}
	for _, route := range handler.routesByRule {
		routes = append(routes, route)
	}
	handler.mu.RUnlock()
	seen := make(map[*reverseProxyDNSRoute]struct{})
	errs := make([]error, 0)
	closeRoute := func(route *reverseProxyDNSRoute) {
		if route == nil {
			return
		}
		if _, exists := seen[route]; exists {
			return
		}
		seen[route] = struct{}{}
		if err := route.close(); err != nil {
			errs = append(errs, err)
		}
	}
	closeRoute(defaultRoute)
	for _, route := range routes {
		closeRoute(route)
	}
	return errors.Join(errs...)
}

func reverseProxyDNSApplyEDNSPolicy(req *dns.Msg, dctx *dnsproxy.DNSContext, rule *model.ReverseProxyRule) {
	if req == nil || rule == nil {
		return
	}
	if !rule.EDNSEnabled {
		reverseProxyDNSRemoveECS(req)
		return
	}

	switch normalizeReverseProxyEDNSMode(rule.EDNSMode) {
	case reverseProxyEDNSModeCustom:
		normalizedIP, ok := normalizeReverseProxyEDNSCustomIPv4(rule.EDNSCustomIP)
		if !ok {
			reverseProxyDNSRemoveECS(req)
			return
		}
		ip := net.ParseIP(normalizedIP)
		reverseProxyDNSSetECS(req, ip)
	default:
		if normalizeReverseProxyEDNSClientSubnetPolicy(rule.EDNSClientSubnetPolicy) == reverseProxyEDNSClientSubnetPolicyPreferRequestPublic {
			if subnet, ok := reverseProxyDNSExtractUsableRequestECS(req); ok {
				reverseProxyDNSSetECSSubnet(req, subnet)
				return
			}
		}
		ip, ok := reverseProxyDNSResolveAutoEDNSIP(req, dctx, rule)
		if !ok {
			reverseProxyDNSRemoveECS(req)
			return
		}
		if ip4 := ip.To4(); ip4 != nil {
			ip = net.IPv4(ip4[0], ip4[1], ip4[2], 1)
		}
		reverseProxyDNSSetECS(req, ip)
	}
}

func reverseProxyDNSResolveAutoEDNSIP(req *dns.Msg, dctx *dnsproxy.DNSContext, rule *model.ReverseProxyRule) (net.IP, bool) {
	if dctx == nil || rule == nil {
		return nil, false
	}

	if normalizeReverseProxyEDNSClientSubnetPolicy(rule.EDNSClientSubnetPolicy) == reverseProxyEDNSClientSubnetPolicyPreferRequestPublic {
		if subnet, ok := reverseProxyDNSExtractUsableRequestECS(req); ok {
			return net.IP(append([]byte(nil), subnet.Address...)), true
		}
	}

	return reverseProxyDNSResolveClientEDNSIP(dctx)
}

func reverseProxyDNSResolveClientEDNSIP(dctx *dnsproxy.DNSContext) (net.IP, bool) {
	if dctx == nil {
		return nil, false
	}

	clientAddr := dctx.Addr.Addr()
	if !clientAddr.IsValid() || aghnetutil.IsSpecialPurpose(clientAddr) {
		return nil, false
	}

	clientIP := clientAddr.AsSlice()
	if len(clientIP) == 0 {
		return nil, false
	}

	return net.IP(append([]byte(nil), clientIP...)), true
}

func reverseProxyDNSExtractUsableRequestECS(req *dns.Msg) (*dns.EDNS0_SUBNET, bool) {
	if req == nil {
		return nil, false
	}

	opt := req.IsEdns0()
	if opt == nil {
		return nil, false
	}
	for _, option := range opt.Option {
		subnet, ok := option.(*dns.EDNS0_SUBNET)
		if !ok {
			continue
		}
		normalized, ok := reverseProxyDNSNormalizeUsableECS(subnet)
		if !ok {
			continue
		}
		return normalized, true
	}

	return nil, false
}

func reverseProxyDNSExtractUsableRequestECSIP(req *dns.Msg) (net.IP, bool) {
	subnet, ok := reverseProxyDNSExtractUsableRequestECS(req)
	if !ok || subnet == nil {
		return nil, false
	}

	return net.IP(append([]byte(nil), subnet.Address...)), true
}

func reverseProxyDNSNormalizeUsableECS(subnet *dns.EDNS0_SUBNET) (*dns.EDNS0_SUBNET, bool) {
	if subnet == nil {
		return nil, false
	}

	normalized := &dns.EDNS0_SUBNET{
		Code:          dns.EDNS0SUBNET,
		Family:        subnet.Family,
		SourceNetmask: subnet.SourceNetmask,
		SourceScope:   subnet.SourceScope,
	}

	switch subnet.Family {
	case 1:
		if subnet.SourceNetmask > net.IPv4len*8 {
			return nil, false
		}
		ip := subnet.Address.To4()
		if ip == nil {
			return nil, false
		}
		addr, ok := netip.AddrFromSlice(ip)
		if !ok || !addr.IsValid() || aghnetutil.IsSpecialPurpose(addr) {
			return nil, false
		}
		normalized.Address = net.IPv4(ip[0], ip[1], ip[2], ip[3])
	case 2:
		if subnet.SourceNetmask > net.IPv6len*8 {
			return nil, false
		}
		ip := subnet.Address.To16()
		if len(ip) != net.IPv6len {
			return nil, false
		}
		addr, ok := netip.AddrFromSlice(ip)
		if !ok || !addr.IsValid() || aghnetutil.IsSpecialPurpose(addr) {
			return nil, false
		}
		normalized.Address = append(net.IP(nil), ip...)
	default:
		return nil, false
	}

	return normalized, true
}

func reverseProxyDNSSetECS(req *dns.Msg, ip net.IP) {
	if req == nil || ip == nil {
		return
	}

	reverseProxyDNSRemoveECS(req)

	subnet := &dns.EDNS0_SUBNET{
		Code:        dns.EDNS0SUBNET,
		SourceScope: 0,
	}
	if ip4 := ip.To4(); ip4 != nil {
		subnet.Family = 1
		subnet.SourceNetmask = 32
		subnet.Address = net.IPv4(ip4[0], ip4[1], ip4[2], ip4[3])
	} else {
		subnet.Family = 2
		subnet.SourceNetmask = 128
		subnet.Address = append(net.IP(nil), ip...)
	}

	if opt := req.IsEdns0(); opt != nil {
		opt.Option = append(opt.Option, subnet)
		return
	}

	opt := &dns.OPT{
		Hdr: dns.RR_Header{
			Name:   ".",
			Rrtype: dns.TypeOPT,
		},
		Option: []dns.EDNS0{subnet},
	}
	opt.SetUDPSize(4096)
	req.Extra = append(req.Extra, opt)
}

func reverseProxyDNSSetECSSubnet(req *dns.Msg, subnet *dns.EDNS0_SUBNET) {
	if req == nil || subnet == nil {
		return
	}

	normalized, ok := reverseProxyDNSNormalizeUsableECS(subnet)
	if !ok {
		reverseProxyDNSRemoveECS(req)
		return
	}

	reverseProxyDNSRemoveECS(req)

	if opt := req.IsEdns0(); opt != nil {
		opt.Option = append(opt.Option, normalized)
		return
	}

	opt := &dns.OPT{
		Hdr: dns.RR_Header{
			Name:   ".",
			Rrtype: dns.TypeOPT,
		},
		Option: []dns.EDNS0{normalized},
	}
	opt.SetUDPSize(4096)
	req.Extra = append(req.Extra, opt)
}

func reverseProxyDNSRemoveECS(req *dns.Msg) {
	if req == nil {
		return
	}

	opt := req.IsEdns0()
	if opt == nil {
		return
	}

	filtered := opt.Option[:0]
	for _, option := range opt.Option {
		if _, ok := option.(*dns.EDNS0_SUBNET); ok {
			continue
		}
		filtered = append(filtered, option)
	}
	opt.Option = filtered
}

func reverseProxyDNSFilterResponse(resp *dns.Msg, disableIPv4 bool, disableIPv6 bool) {
	if resp == nil || (!disableIPv4 && !disableIPv6) {
		return
	}

	droppedKeys := make(map[string]struct{})
	reverseProxyDNSCollectDroppedKeys(resp.Answer, disableIPv4, disableIPv6, droppedKeys)
	reverseProxyDNSCollectDroppedKeys(resp.Ns, disableIPv4, disableIPv6, droppedKeys)
	reverseProxyDNSCollectDroppedKeys(resp.Extra, disableIPv4, disableIPv6, droppedKeys)

	answer, answerChanged := reverseProxyDNSFilterRRSection(resp.Answer, disableIPv4, disableIPv6, droppedKeys)
	ns, nsChanged := reverseProxyDNSFilterRRSection(resp.Ns, disableIPv4, disableIPv6, droppedKeys)
	extra, extraChanged := reverseProxyDNSFilterRRSection(resp.Extra, disableIPv4, disableIPv6, droppedKeys)

	resp.Answer = answer
	resp.Ns = ns
	resp.Extra = extra
	if answerChanged || nsChanged || extraChanged {
		resp.AuthenticatedData = false
	}
}

func reverseProxyDNSCollectDroppedKeys(items []dns.RR, disableIPv4 bool, disableIPv6 bool, droppedKeys map[string]struct{}) {
	if len(items) == 0 || droppedKeys == nil || (!disableIPv4 && !disableIPv6) {
		return
	}

	for _, rr := range items {
		reverseProxyDNSMarkDroppedKeys(rr, disableIPv4, disableIPv6, droppedKeys)
	}
}

func reverseProxyDNSFilterRRSection(items []dns.RR, disableIPv4 bool, disableIPv6 bool, droppedKeys map[string]struct{}) ([]dns.RR, bool) {
	if len(items) == 0 || (!disableIPv4 && !disableIPv6) {
		return items, false
	}

	filtered := make([]dns.RR, 0, len(items))
	changed := false
	for _, rr := range items {
		if rr == nil {
			changed = true
			continue
		}
		drop, rrChanged := reverseProxyDNSShouldDropRR(rr, disableIPv4, disableIPv6, droppedKeys)
		if rrChanged {
			changed = true
		}
		if drop {
			continue
		}
		if sig, ok := rr.(*dns.RRSIG); ok {
			if _, exists := droppedKeys[reverseProxyDNSRRSIGKey(sig.Hdr.Name, sig.TypeCovered)]; exists {
				changed = true
				continue
			}
		}
		filtered = append(filtered, rr)
	}

	return filtered, changed
}

func reverseProxyDNSMarkDroppedKeys(rr dns.RR, disableIPv4 bool, disableIPv6 bool, droppedKeys map[string]struct{}) {
	if rr == nil || droppedKeys == nil {
		return
	}

	switch record := rr.(type) {
	case *dns.A:
		if disableIPv4 {
			reverseProxyDNSMarkRRSIGForType(record.Header().Name, dns.TypeA, droppedKeys)
		}
	case *dns.AAAA:
		if disableIPv6 {
			reverseProxyDNSMarkRRSIGForType(record.Header().Name, dns.TypeAAAA, droppedKeys)
		}
	case *dns.NSEC:
		if (disableIPv4 && reverseProxyDNSNSECContainsType(record.TypeBitMap, dns.TypeA)) ||
			(disableIPv6 && reverseProxyDNSNSECContainsType(record.TypeBitMap, dns.TypeAAAA)) {
			reverseProxyDNSMarkRRSIGForType(record.Header().Name, dns.TypeNSEC, droppedKeys)
		}
	case *dns.NSEC3:
		if (disableIPv4 && reverseProxyDNSNSECContainsType(record.TypeBitMap, dns.TypeA)) ||
			(disableIPv6 && reverseProxyDNSNSECContainsType(record.TypeBitMap, dns.TypeAAAA)) {
			reverseProxyDNSMarkRRSIGForType(record.Header().Name, dns.TypeNSEC3, droppedKeys)
		}
	case *dns.HTTPS:
		if reverseProxyDNSHasBlockedSVCBHints(record.Value, disableIPv4, disableIPv6) {
			reverseProxyDNSMarkRRSIGForType(record.Header().Name, dns.TypeHTTPS, droppedKeys)
		}
	case *dns.SVCB:
		if reverseProxyDNSHasBlockedSVCBHints(record.Value, disableIPv4, disableIPv6) {
			reverseProxyDNSMarkRRSIGForType(record.Header().Name, dns.TypeSVCB, droppedKeys)
		}
	}
}

func reverseProxyDNSShouldDropRR(rr dns.RR, disableIPv4 bool, disableIPv6 bool, droppedKeys map[string]struct{}) (bool, bool) {
	switch record := rr.(type) {
	case *dns.A:
		if disableIPv4 {
			return true, true
		}
	case *dns.AAAA:
		if disableIPv6 {
			return true, true
		}
	case *dns.RRSIG:
		if disableIPv4 && record.TypeCovered == dns.TypeA {
			return true, true
		}
		if disableIPv6 && record.TypeCovered == dns.TypeAAAA {
			return true, true
		}
	case *dns.NSEC:
		if disableIPv4 && reverseProxyDNSNSECContainsType(record.TypeBitMap, dns.TypeA) {
			return true, true
		}
		if disableIPv6 && reverseProxyDNSNSECContainsType(record.TypeBitMap, dns.TypeAAAA) {
			return true, true
		}
	case *dns.NSEC3:
		if disableIPv4 && reverseProxyDNSNSECContainsType(record.TypeBitMap, dns.TypeA) {
			return true, true
		}
		if disableIPv6 && reverseProxyDNSNSECContainsType(record.TypeBitMap, dns.TypeAAAA) {
			return true, true
		}
	case *dns.HTTPS:
		if reverseProxyDNSFilterSVCBValueHints(&record.Value, disableIPv4, disableIPv6) {
			reverseProxyDNSMarkRRSIGForType(record.Header().Name, dns.TypeHTTPS, droppedKeys)
			return false, true
		}
	case *dns.SVCB:
		if reverseProxyDNSFilterSVCBValueHints(&record.Value, disableIPv4, disableIPv6) {
			reverseProxyDNSMarkRRSIGForType(record.Header().Name, dns.TypeSVCB, droppedKeys)
			return false, true
		}
	}

	return false, false
}

func reverseProxyDNSHasBlockedSVCBHints(values []dns.SVCBKeyValue, disableIPv4 bool, disableIPv6 bool) bool {
	if len(values) == 0 || (!disableIPv4 && !disableIPv6) {
		return false
	}

	for _, value := range values {
		switch value.(type) {
		case *dns.SVCBIPv4Hint:
			if disableIPv4 {
				return true
			}
		case *dns.SVCBIPv6Hint:
			if disableIPv6 {
				return true
			}
		}
	}

	return false
}

func reverseProxyDNSFilterSVCBValueHints(values *[]dns.SVCBKeyValue, disableIPv4 bool, disableIPv6 bool) bool {
	if values == nil || len(*values) == 0 || (!disableIPv4 && !disableIPv6) {
		return false
	}

	changed := false
	filtered := make([]dns.SVCBKeyValue, 0, len(*values))
	for _, value := range *values {
		switch value.(type) {
		case *dns.SVCBIPv4Hint:
			if disableIPv4 {
				changed = true
				continue
			}
			filtered = append(filtered, value)
		case *dns.SVCBIPv6Hint:
			if disableIPv6 {
				changed = true
				continue
			}
			filtered = append(filtered, value)
		default:
			filtered = append(filtered, value)
		}
	}
	if changed {
		*values = filtered
	}
	return changed
}

func reverseProxyDNSMarkRRSIGForType(name string, coveredType uint16, droppedKeys map[string]struct{}) {
	if droppedKeys == nil {
		return
	}
	droppedKeys[reverseProxyDNSRRSIGKey(name, coveredType)] = struct{}{}
}

func reverseProxyDNSRRSIGKey(name string, coveredType uint16) string {
	return strings.ToLower(strings.TrimSpace(name)) + "|" + fmt.Sprintf("%d", coveredType)
}

func reverseProxyDNSNSECContainsType(items []uint16, target uint16) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func translateReverseProxyDNSError(row *model.ReverseProxyRule, err error) error {
	if row == nil || err == nil {
		return err
	}
	message := strings.TrimSpace(err.Error())
	if strings.Contains(strings.ToLower(message), "address already in use") {
		return common.NewError(fmt.Sprintf("dns reverse proxy listen :%d failed: address already in use", row.ListenPort))
	}
	return common.NewError(message)
}

func syncReverseProxyDNSRuntime(service *ReverseProxyService, rows []model.ReverseProxyRule) error {
	err := reverseProxyDNSRuntime.sync(service, rows)
	reverseProxyRuntime.reconcileRuleStates(rows)
	return err
}

func stopReverseProxyDNSRuntime() error {
	return reverseProxyDNSRuntime.stopAll()
}

func (m *reverseProxyDNSRuntimeManager) listenerCount() int {
	if m == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	count := 0
	for _, instance := range m.running {
		if instance == nil {
			continue
		}
		count += len(instance.dnsServers)
		count += len(instance.doqListeners)
		if len(instance.dnsServers) == 0 && len(instance.doqListeners) == 0 {
			// Backward-compatible accounting for a listener created during a
			// rolling upgrade before the process restarts with owned listeners.
			if len(instance.rules) == 0 {
				continue
			}
			rule := &instance.rules[0]
			listenIPs := reverseProxyDNSRuntimeListenIPs(rule)
			count += len(listenIPs)
		}
	}
	return count
}

func reverseProxyDNSCertificateStateKey(row *model.ReverseProxyRule, certificateState map[uint]model.CertificateRecord) string {
	if row == nil {
		return ""
	}
	certIDs := reverseProxyRuleCertificateIDs(row)
	if len(certIDs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(certIDs))
	for _, certID := range certIDs {
		record := certificateState[certID]
		updatedAt := int64(0)
		if !record.UpdatedAt.IsZero() {
			updatedAt = record.UpdatedAt.Unix()
		}
		parts = append(parts, fmt.Sprintf("%d:%s:%d", certID, strings.TrimSpace(record.Fingerprint), updatedAt))
	}
	return strings.Join(parts, "|")
}
