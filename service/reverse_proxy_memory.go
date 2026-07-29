package service

import (
	"container/heap"
	"container/list"
	"sync"
	"time"

	"github.com/alireza0/s-ui/database/model"
	"github.com/miekg/dns"
)

// reverseProxyMemoryPool accounts for memory intentionally retained by the
// reverse-proxy.  It is an admission controller, not a replacement for Go's
// garbage collector: every successful reservation has to be released by its
// owner.  The per-rule accounting prevents one DNS cache or body conversion
// from consuming the complete shared pool.
type reverseProxyMemoryPool struct {
	mu          sync.Mutex
	capacity    int64
	used        int64
	ruleUsed    map[uint]int64
	cacheUsed   int64
	rewriteUsed int64
}

type reverseProxyMemoryLease struct {
	pool     *reverseProxyMemoryPool
	ruleID   uint
	bytes    int64
	category string
	once     sync.Once
}

func newReverseProxyMemoryPool(capacity int64) *reverseProxyMemoryPool {
	return &reverseProxyMemoryPool{
		capacity: capacity,
		ruleUsed: make(map[uint]int64),
	}
}

func (p *reverseProxyMemoryPool) Configure(capacity int64) {
	if p == nil {
		return
	}
	if capacity < reverseProxyMinimumRewriteReservationBytes {
		capacity = reverseProxyMinimumRewriteReservationBytes
	}
	p.mu.Lock()
	p.capacity = capacity
	p.mu.Unlock()
}

func (p *reverseProxyMemoryPool) TryAcquire(ruleID uint, bytes int64, ruleLimit int64, category string) *reverseProxyMemoryLease {
	if p == nil || bytes <= 0 {
		return &reverseProxyMemoryLease{}
	}
	if ruleLimit > 0 && bytes > ruleLimit {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.capacity > 0 && p.used+bytes > p.capacity {
		return nil
	}
	if ruleID != 0 && ruleLimit > 0 && p.ruleUsed[ruleID]+bytes > ruleLimit {
		return nil
	}
	p.used += bytes
	if ruleID != 0 {
		p.ruleUsed[ruleID] += bytes
	}
	switch category {
	case "cache":
		p.cacheUsed += bytes
	case "rewrite":
		p.rewriteUsed += bytes
	}
	return &reverseProxyMemoryLease{pool: p, ruleID: ruleID, bytes: bytes, category: category}
}

func (l *reverseProxyMemoryLease) Release() {
	if l == nil || l.pool == nil || l.bytes <= 0 {
		return
	}
	l.once.Do(func() {
		p := l.pool
		p.mu.Lock()
		p.used -= l.bytes
		if p.used < 0 {
			p.used = 0
		}
		if l.ruleID != 0 {
			p.ruleUsed[l.ruleID] -= l.bytes
			if p.ruleUsed[l.ruleID] <= 0 {
				delete(p.ruleUsed, l.ruleID)
			}
		}
		switch l.category {
		case "cache":
			p.cacheUsed -= l.bytes
			if p.cacheUsed < 0 {
				p.cacheUsed = 0
			}
		case "rewrite":
			p.rewriteUsed -= l.bytes
			if p.rewriteUsed < 0 {
				p.rewriteUsed = 0
			}
		}
		p.mu.Unlock()
	})
}

func (p *reverseProxyMemoryPool) Snapshot() (used int64, cache int64, rewrite int64) {
	if p == nil {
		return 0, 0, 0
	}
	p.mu.Lock()
	used, cache, rewrite = p.used, p.cacheUsed, p.rewriteUsed
	p.mu.Unlock()
	return used, cache, rewrite
}

// reverseProxyResourceController owns the mutable global guards.  Listener
// groups hold their own rule-level limiters, while this object applies the
// panel-wide protection shared by every group and DNS route.
type reverseProxyResourceController struct {
	mu             sync.RWMutex
	settings       ReverseProxyResourceSettings
	httpLimiter    *reverseProxyAdjustableLimiter
	dnsLimiter     *reverseProxyAdjustableLimiter
	rewriteLimiter *reverseProxyAdjustableLimiter
	memory         *reverseProxyMemoryPool
}

var reverseProxyResources = newReverseProxyResourceController(reverseProxySettingsView(ptrReverseProxySettings(defaultReverseProxySettingsModel())))

func newReverseProxyResourceController(settings ReverseProxyResourceSettings) *reverseProxyResourceController {
	controller := &reverseProxyResourceController{
		settings:       settings,
		httpLimiter:    &reverseProxyAdjustableLimiter{},
		dnsLimiter:     &reverseProxyAdjustableLimiter{},
		rewriteLimiter: &reverseProxyAdjustableLimiter{},
		memory:         newReverseProxyMemoryPool(settings.MemoryPoolBytes),
	}
	controller.apply(settings)
	return controller
}

func (c *reverseProxyResourceController) apply(settings ReverseProxyResourceSettings) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.settings = settings
	if c.memory == nil {
		c.memory = newReverseProxyMemoryPool(settings.MemoryPoolBytes)
	}
	memory := c.memory
	httpLimiter := c.httpLimiter
	dnsLimiter := c.dnsLimiter
	rewriteLimiter := c.rewriteLimiter
	c.mu.Unlock()
	memory.Configure(settings.MemoryPoolBytes)
	httpLimiter.SetMax(settings.GlobalHTTPMaxConcurrent)
	dnsLimiter.SetMax(settings.GlobalDNSMaxConcurrent)
	rewriteLimiter.SetMax(settings.ResponseRewriteMaxConcurrent)
}

func (c *reverseProxyResourceController) current() ReverseProxyResourceSettings {
	if c == nil {
		return reverseProxySettingsView(ptrReverseProxySettings(defaultReverseProxySettingsModel()))
	}
	c.mu.RLock()
	settings := c.settings
	c.mu.RUnlock()
	return settings
}

func (c *reverseProxyResourceController) tryAcquireHTTP() bool {
	if c == nil {
		return true
	}
	return c.httpLimiter.TryAcquire()
}

func (c *reverseProxyResourceController) releaseHTTP() {
	if c != nil {
		c.httpLimiter.Release()
	}
}

func (c *reverseProxyResourceController) tryAcquireDNS() bool {
	if c == nil {
		return true
	}
	return c.dnsLimiter.TryAcquire()
}

func (c *reverseProxyResourceController) releaseDNS() {
	if c != nil {
		c.dnsLimiter.Release()
	}
}

func (c *reverseProxyResourceController) tryAcquireRewrite(ruleID uint, ruleLimit int64, bytes int64) (*reverseProxyMemoryLease, bool) {
	if c == nil {
		return nil, false
	}
	if !c.rewriteLimiter.TryAcquire() {
		return nil, false
	}
	c.mu.RLock()
	memory := c.memory
	c.mu.RUnlock()
	lease := memory.TryAcquire(ruleID, bytes, ruleLimit, "rewrite")
	if lease == nil {
		c.rewriteLimiter.Release()
		return nil, false
	}
	return lease, true
}

func (c *reverseProxyResourceController) releaseRewrite(lease *reverseProxyMemoryLease) {
	if lease != nil {
		lease.Release()
	}
	if c != nil {
		c.rewriteLimiter.Release()
	}
}

func (c *reverseProxyResourceController) runtimeUsage() ReverseProxyRuntimeResourceUsage {
	if c == nil {
		return ReverseProxyRuntimeResourceUsage{}
	}
	httpActive, _ := c.httpLimiter.Snapshot()
	dnsActive, _ := c.dnsLimiter.Snapshot()
	c.mu.RLock()
	memory := c.memory
	c.mu.RUnlock()
	used, cache, rewrite := memory.Snapshot()
	return ReverseProxyRuntimeResourceUsage{
		ActiveHTTPRequests: httpActive,
		ActiveDNSQueries:   dnsActive,
		MemoryUsedBytes:    used,
		CacheUsedBytes:     cache,
		RewriteUsedBytes:   rewrite,
	}
}

func reverseProxyEffectiveRuleMemoryLimit(rule *model.ReverseProxyRule) int64 {
	settings := reverseProxyResources.current()
	if rule != nil && rule.MemoryLimitBytes > 0 {
		return rule.MemoryLimitBytes
	}
	return settings.DefaultRuleMemoryLimitBytes
}

func reverseProxyEffectiveUpstreamMaxIdleConnections(rule *model.ReverseProxyRule) int {
	if rule != nil && rule.UpstreamMaxIdleConnections > 0 {
		return rule.UpstreamMaxIdleConnections
	}
	return reverseProxyResources.current().DefaultUpstreamMaxIdleConnections
}

// reverseProxyDNSResponseCache is a bounded per-rule LRU.  Its backing bytes
// are reserved from the shared pool, so a full cache never forces a body
// rewrite to allocate outside the configured reverse-proxy budget.
type reverseProxyDNSResponseCache struct {
	mu              sync.Mutex
	ruleID          uint
	ruleLimit       int64
	maxBytes        int64
	entries         map[string]*reverseProxyDNSCacheEntry
	lru             *list.List
	expiry          reverseProxyDNSExpiryHeap
	timer           *time.Timer
	timerExpires    time.Time
	timerGeneration uint64
	used            int64
	closed          bool
}

type reverseProxyDNSCacheEntry struct {
	key         string
	wire        []byte
	expires     time.Time
	bytes       int64
	element     *list.Element
	expiryIndex int
	lease       *reverseProxyMemoryLease
}

type reverseProxyDNSExpiryHeap []*reverseProxyDNSCacheEntry

func (h reverseProxyDNSExpiryHeap) Len() int { return len(h) }

func (h reverseProxyDNSExpiryHeap) Less(i int, j int) bool {
	return h[i].expires.Before(h[j].expires)
}

func (h reverseProxyDNSExpiryHeap) Swap(i int, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].expiryIndex = i
	h[j].expiryIndex = j
}

func (h *reverseProxyDNSExpiryHeap) Push(value interface{}) {
	entry, _ := value.(*reverseProxyDNSCacheEntry)
	if entry == nil {
		return
	}
	entry.expiryIndex = len(*h)
	*h = append(*h, entry)
}

func (h *reverseProxyDNSExpiryHeap) Pop() interface{} {
	items := *h
	if len(items) == 0 {
		return nil
	}
	last := items[len(items)-1]
	items[len(items)-1] = nil
	last.expiryIndex = -1
	*h = items[:len(items)-1]
	return last
}

func newReverseProxyDNSResponseCache(rule *model.ReverseProxyRule) *reverseProxyDNSResponseCache {
	if rule == nil || !rule.DNSCacheEnabled {
		return nil
	}
	ruleLimit := reverseProxyEffectiveRuleMemoryLimit(rule)
	maxBytes := int64(reverseProxyDNSCacheSizeBytes(rule.DNSCacheSizeBytes))
	if ruleLimit > 0 && maxBytes > ruleLimit {
		maxBytes = ruleLimit
	}
	// DNS messages are independently bounded and may legitimately use a cache
	// much smaller than the minimum body-rewrite buffer.  The persisted DNS
	// cache size is validated as positive, so do not silently turn a valid
	// small cache into a disabled cache by applying the unrelated 500 KiB
	// rewrite admission threshold here.
	if maxBytes <= 0 {
		return nil
	}
	return &reverseProxyDNSResponseCache{
		ruleID:    rule.Id,
		ruleLimit: ruleLimit,
		maxBytes:  maxBytes,
		entries:   make(map[string]*reverseProxyDNSCacheEntry),
		lru:       list.New(),
		expiry:    make(reverseProxyDNSExpiryHeap, 0),
	}
}

func reverseProxyDNSCacheKey(request *dns.Msg) string {
	if request == nil {
		return ""
	}
	copyRequest := request.Copy()
	copyRequest.Id = 0
	wire, err := copyRequest.Pack()
	if err != nil {
		return ""
	}
	return string(wire)
}

func (c *reverseProxyDNSResponseCache) Get(request *dns.Msg) *dns.Msg {
	if c == nil || request == nil {
		return nil
	}
	key := reverseProxyDNSCacheKey(request)
	if key == "" {
		return nil
	}
	now := time.Now()
	c.mu.Lock()
	entry := c.entries[key]
	if entry == nil || !now.Before(entry.expires) {
		if entry != nil {
			c.removeLocked(entry)
			c.scheduleExpiryLocked(now)
		}
		c.mu.Unlock()
		return nil
	}
	c.lru.MoveToFront(entry.element)
	wire := append([]byte(nil), entry.wire...)
	c.mu.Unlock()
	response := new(dns.Msg)
	if err := response.Unpack(wire); err != nil {
		return nil
	}
	response.Id = request.Id
	reverseProxyDNSAgeResponseTTLs(response, now, entry.expires)
	return response
}

func (c *reverseProxyDNSResponseCache) Put(request *dns.Msg, response *dns.Msg, minTTL int, maxTTL int) {
	if c == nil || request == nil || response == nil {
		return
	}
	key := reverseProxyDNSCacheKey(request)
	if key == "" {
		return
	}
	ttl := reverseProxyDNSResponseTTL(response, minTTL, maxTTL)
	if ttl <= 0 {
		return
	}
	copyResponse := response.Copy()
	copyResponse.Id = 0
	wire, err := copyResponse.Pack()
	if err != nil {
		return
	}
	entryBytes := int64(len(key) + len(wire) + 96)
	if entryBytes > c.maxBytes {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	defer c.scheduleExpiryLocked(time.Now())
	if c.closed {
		return
	}
	if previous := c.entries[key]; previous != nil {
		c.removeLocked(previous)
	}
	for c.used+entryBytes > c.maxBytes && c.lru.Len() > 0 {
		oldest, _ := c.lru.Back().Value.(*reverseProxyDNSCacheEntry)
		c.removeLocked(oldest)
	}
	if c.used+entryBytes > c.maxBytes {
		return
	}
	lease := reverseProxyResources.memory.TryAcquire(c.ruleID, entryBytes, c.ruleLimit, "cache")
	if lease == nil {
		return
	}
	entry := &reverseProxyDNSCacheEntry{
		key:         key,
		wire:        wire,
		expires:     time.Now().Add(ttl),
		bytes:       entryBytes,
		lease:       lease,
		expiryIndex: -1,
	}
	entry.element = c.lru.PushFront(entry)
	c.entries[key] = entry
	heap.Push(&c.expiry, entry)
	c.used += entryBytes
}

func (c *reverseProxyDNSResponseCache) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	if !c.closed {
		c.closed = true
		c.timerGeneration++
		if c.timer != nil {
			c.timer.Stop()
			c.timer = nil
		}
		c.timerExpires = time.Time{}
		for _, entry := range c.entries {
			if entry != nil && entry.lease != nil {
				entry.lease.Release()
			}
		}
		c.entries = make(map[string]*reverseProxyDNSCacheEntry)
		c.lru.Init()
		c.expiry = make(reverseProxyDNSExpiryHeap, 0)
		c.used = 0
	}
	c.mu.Unlock()
}

func (c *reverseProxyDNSResponseCache) scheduleExpiryLocked(now time.Time) {
	if c == nil || c.closed || len(c.expiry) == 0 {
		if c != nil && c.timer != nil {
			c.timerGeneration++
			c.timer.Stop()
			c.timer = nil
			c.timerExpires = time.Time{}
		}
		return
	}
	next := c.expiry[0].expires
	if c.timer != nil && c.timerExpires.Equal(next) {
		return
	}
	c.timerGeneration++
	generation := c.timerGeneration
	if c.timer != nil {
		c.timer.Stop()
	}
	delay := next.Sub(now)
	if delay < 0 {
		delay = 0
	}
	c.timerExpires = next
	c.timer = time.AfterFunc(delay, func() {
		c.expireDue(generation)
	})
}

func (c *reverseProxyDNSResponseCache) expireDue(generation uint64) {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.closed || generation != c.timerGeneration {
		c.mu.Unlock()
		return
	}
	c.timer = nil
	c.timerExpires = time.Time{}
	now := time.Now()
	for len(c.expiry) > 0 {
		entry := c.expiry[0]
		if entry != nil && now.Before(entry.expires) {
			break
		}
		c.removeLocked(entry)
	}
	c.scheduleExpiryLocked(now)
	c.mu.Unlock()
}

func (c *reverseProxyDNSResponseCache) removeLocked(entry *reverseProxyDNSCacheEntry) {
	if c == nil || entry == nil {
		return
	}
	delete(c.entries, entry.key)
	if entry.expiryIndex >= 0 && entry.expiryIndex < len(c.expiry) && c.expiry[entry.expiryIndex] == entry {
		heap.Remove(&c.expiry, entry.expiryIndex)
	}
	if entry.element != nil {
		c.lru.Remove(entry.element)
	}
	c.used -= entry.bytes
	if c.used < 0 {
		c.used = 0
	}
	if entry.lease != nil {
		entry.lease.Release()
	}
}

func reverseProxyDNSResponseTTL(response *dns.Msg, minTTL int, maxTTL int) time.Duration {
	if response == nil {
		return 0
	}
	lowest := uint32(0)
	found := false
	for _, section := range [][]dns.RR{response.Answer, response.Ns, response.Extra} {
		for _, record := range section {
			if record == nil || record.Header().Rrtype == dns.TypeOPT {
				continue
			}
			ttl := record.Header().Ttl
			if !found || ttl < lowest {
				lowest = ttl
				found = true
			}
		}
	}
	if !found {
		return 0
	}
	if minTTL > 0 && lowest < uint32(minTTL) {
		lowest = uint32(minTTL)
	}
	if maxTTL > 0 && lowest > uint32(maxTTL) {
		lowest = uint32(maxTTL)
	}
	if lowest == 0 {
		return 0
	}
	for _, section := range [][]dns.RR{response.Answer, response.Ns, response.Extra} {
		for _, record := range section {
			if record != nil && record.Header().Rrtype != dns.TypeOPT {
				record.Header().Ttl = lowest
			}
		}
	}
	return time.Duration(lowest) * time.Second
}

func reverseProxyDNSAgeResponseTTLs(response *dns.Msg, now time.Time, expires time.Time) {
	if response == nil || expires.IsZero() {
		return
	}
	remaining := int64(expires.Sub(now).Seconds())
	if remaining < 0 {
		remaining = 0
	}
	if remaining > int64(^uint32(0)) {
		remaining = int64(^uint32(0))
	}
	for _, section := range [][]dns.RR{response.Answer, response.Ns, response.Extra} {
		for _, record := range section {
			if record != nil && record.Header().Rrtype != dns.TypeOPT && int64(record.Header().Ttl) > remaining {
				record.Header().Ttl = uint32(remaining)
			}
		}
	}
}
