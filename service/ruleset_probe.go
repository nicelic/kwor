package service

import (
	"bytes"
	"compress/zlib"
	"container/list"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	compressionalgorithm "github.com/alireza0/s-ui/compression/Compression-algorithm"
	"github.com/klauspost/compress/zstd"
	"gopkg.in/yaml.v3"
)

const (
	RuleSetProbeMaxBatch           = 32
	ruleSetProbeMaxInFlightBatches = 8
	ruleSetProbeTimeout            = 6 * time.Second
	ruleSetProbeMaxRedirects       = 3
	ruleSetProbeMaxWireBytes       = 8 * 1024 * 1024
	ruleSetProbeMaxDecodedBytes    = 32 * 1024 * 1024
	ruleSetProbeAcceptEncoding     = "gzip, deflate, zstd"
	ruleSetProbeCacheEntries       = 256
	ruleSetProbeCacheTTL           = 5 * time.Minute
	ruleSetProbeGlobalConcurrency  = 4
)

type SubscriptionRuleSetProbeRequest struct {
	Items []SubscriptionRuleSetProbeItem `json:"items"`
}

type SubscriptionRuleSetProbeItem struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	SourceID      string `json:"sourceId,omitempty"`
	Scope         string `json:"scope"`
	Name          string `json:"name,omitempty"`
	URL           string `json:"url,omitempty"`
	AllowFallback *bool  `json:"allowFallback,omitempty"`
}

type SubscriptionRuleSetProbeResult struct {
	ID       string `json:"id"`
	Valid    bool   `json:"valid"`
	URL      string `json:"url,omitempty"`
	SourceID string `json:"sourceId,omitempty"`
	Format   string `json:"format,omitempty"`
	Scope    string `json:"scope"`
	Error    string `json:"error,omitempty"`
	Cached   bool   `json:"cached,omitempty"`
}

type SubscriptionRuleSetProbeResolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

type SubscriptionRuleSetProbeEngineOptions struct {
	Resolver           SubscriptionRuleSetProbeResolver
	AllowAddress       func(netip.Addr) bool
	RootCAs            *x509.CertPool
	Timeout            time.Duration
	Concurrency        int
	CacheEntries       int
	CacheTTL           time.Duration
	MaxInFlightBatches int
}

type SubscriptionRuleSetProbeEngine struct {
	engine *ruleSetProbeEngine
}

type ruleSetProbeEngine struct {
	resolver       SubscriptionRuleSetProbeResolver
	semaphore      chan struct{}
	batchSemaphore chan struct{}
	cache          *ruleSetProbeMetadataCache
	allowAddress   func(netip.Addr) bool
	rootCAs        *x509.CertPool
	timeout        time.Duration
}

type ruleSetProbeCacheEntry struct {
	key       string
	result    SubscriptionRuleSetProbeResult
	expiresAt time.Time
}

type ruleSetProbeMetadataCache struct {
	mu      sync.Mutex
	limit   int
	ttl     time.Duration
	entries map[string]*list.Element
	lru     *list.List
}

var defaultRuleSetProbeEngine = newRuleSetProbeEngine()

func newRuleSetProbeEngine() *ruleSetProbeEngine {
	return NewSubscriptionRuleSetProbeEngine(SubscriptionRuleSetProbeEngineOptions{}).engine
}

func NewSubscriptionRuleSetProbeEngine(options SubscriptionRuleSetProbeEngineOptions) *SubscriptionRuleSetProbeEngine {
	resolver := options.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	allowAddress := options.AllowAddress
	if allowAddress == nil {
		allowAddress = isPublicRuleSetProbeAddress
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = ruleSetProbeTimeout
	}
	concurrency := options.Concurrency
	if concurrency <= 0 {
		concurrency = ruleSetProbeGlobalConcurrency
	}
	cacheEntries := options.CacheEntries
	if cacheEntries <= 0 {
		cacheEntries = ruleSetProbeCacheEntries
	}
	cacheTTL := options.CacheTTL
	if cacheTTL <= 0 {
		cacheTTL = ruleSetProbeCacheTTL
	}
	maxInFlightBatches := options.MaxInFlightBatches
	if maxInFlightBatches <= 0 {
		maxInFlightBatches = ruleSetProbeMaxInFlightBatches
	}
	return &SubscriptionRuleSetProbeEngine{engine: &ruleSetProbeEngine{
		resolver:       resolver,
		semaphore:      make(chan struct{}, concurrency),
		batchSemaphore: make(chan struct{}, maxInFlightBatches),
		cache:          newRuleSetProbeMetadataCache(cacheEntries, cacheTTL),
		allowAddress:   allowAddress,
		rootCAs:        options.RootCAs,
		timeout:        timeout,
	}}
}

func (engine *SubscriptionRuleSetProbeEngine) Probe(ctx context.Context, request SubscriptionRuleSetProbeRequest) ([]SubscriptionRuleSetProbeResult, error) {
	if engine == nil || engine.engine == nil {
		return nil, errors.New("规则集探测服务未初始化")
	}
	return engine.engine.probe(ctx, request)
}

func newRuleSetProbeMetadataCache(limit int, ttl time.Duration) *ruleSetProbeMetadataCache {
	return &ruleSetProbeMetadataCache{
		limit:   limit,
		ttl:     ttl,
		entries: make(map[string]*list.Element, limit),
		lru:     list.New(),
	}
}

func ProbeSubscriptionRuleSets(ctx context.Context, request SubscriptionRuleSetProbeRequest) ([]SubscriptionRuleSetProbeResult, error) {
	return defaultRuleSetProbeEngine.probe(ctx, request)
}

func (engine *ruleSetProbeEngine) probe(ctx context.Context, request SubscriptionRuleSetProbeRequest) ([]SubscriptionRuleSetProbeResult, error) {
	if len(request.Items) == 0 {
		return []SubscriptionRuleSetProbeResult{}, nil
	}
	if len(request.Items) > RuleSetProbeMaxBatch {
		return nil, fmt.Errorf("规则集探测单批最多允许 %d 项", RuleSetProbeMaxBatch)
	}
	if engine == nil {
		return nil, errors.New("规则集探测服务未初始化")
	}
	select {
	case engine.batchSemaphore <- struct{}{}:
		defer func() { <-engine.batchSemaphore }()
	default:
		return nil, errors.New("规则集探测请求过多，请稍后重试")
	}

	timeout := engine.timeout
	if timeout <= 0 {
		timeout = ruleSetProbeTimeout
	}
	batchContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	results := make([]SubscriptionRuleSetProbeResult, len(request.Items))
	var wait sync.WaitGroup
	for index := range request.Items {
		index := index
		wait.Add(1)
		go func() {
			defer wait.Done()
			results[index] = engine.probeItem(batchContext, request.Items[index])
		}()
	}
	wait.Wait()
	return results, nil
}

func (engine *ruleSetProbeEngine) probeItem(ctx context.Context, item SubscriptionRuleSetProbeItem) SubscriptionRuleSetProbeResult {
	item = normalizeRuleSetProbeItem(item)
	result := SubscriptionRuleSetProbeResult{ID: item.ID, Scope: item.Scope}
	if err := validateRuleSetProbeItem(item); err != nil {
		result.Error = err.Error()
		return result
	}
	cacheKey := ruleSetProbeCacheKey(item)
	if cached, ok := engine.cache.get(cacheKey, time.Now()); ok {
		cached.ID = item.ID
		cached.Cached = true
		return cached
	}

	select {
	case engine.semaphore <- struct{}{}:
		defer func() { <-engine.semaphore }()
	case <-ctx.Done():
		result.Error = "规则集探测超时"
		return result
	}

	candidates, err := buildRuleSetProbeCandidates(item)
	if err != nil {
		result.Error = err.Error()
		engine.cache.put(cacheKey, result, time.Now())
		return result
	}
	var lastErr error
	for _, candidate := range candidates {
		body, contentType, finalURL, fetchErr := engine.fetch(ctx, candidate.URL)
		if fetchErr != nil {
			lastErr = fetchErr
			continue
		}
		format, validateErr := validateRuleSetProbeContent(body, contentType, finalURL, candidate.Format, item.Scope)
		if validateErr != nil {
			lastErr = validateErr
			continue
		}
		result.Valid = true
		result.URL = finalURL
		result.SourceID = candidate.SourceID
		result.Format = format
		engine.cache.put(cacheKey, result, time.Now())
		return result
	}
	if lastErr == nil {
		lastErr = errors.New("没有可探测的规则集来源")
	}
	result.Error = lastErr.Error()
	engine.cache.put(cacheKey, result, time.Now())
	return result
}

func normalizeRuleSetProbeItem(item SubscriptionRuleSetProbeItem) SubscriptionRuleSetProbeItem {
	item.ID = strings.TrimSpace(item.ID)
	item.Kind = strings.ToLower(strings.TrimSpace(item.Kind))
	item.SourceID = strings.TrimSpace(item.SourceID)
	item.Scope = normalizeRuleSetProbeScope(item.Scope)
	item.Name = strings.TrimSpace(item.Name)
	item.URL = strings.TrimSpace(item.URL)
	return item
}

func normalizeRuleSetProbeScope(scope string) string {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "domain", "geosite":
		return "domain"
	case "ip", "geoip", "ipcidr":
		return "ip"
	default:
		return strings.ToLower(strings.TrimSpace(scope))
	}
}

func validateRuleSetProbeItem(item SubscriptionRuleSetProbeItem) error {
	if item.Kind != "json" && item.Kind != "clash" {
		return errors.New("规则集类型只能是 json 或 clash")
	}
	if item.Scope != "domain" && item.Scope != "ip" {
		return errors.New("规则集 scope 只能是 domain 或 ip")
	}
	if item.URL == "" && item.Name == "" {
		return errors.New("规则集名称或完整 URL 不能为空")
	}
	if len(item.Name) > 512 || len(item.URL) > 4096 {
		return errors.New("规则集名称或 URL 过长")
	}
	if item.URL == "" {
		source, exists := SubscriptionRuleSetSource(item.Kind, item.SourceID)
		if !exists {
			return errors.New("规则集来源不存在")
		}
		if !source.SupportsScope(item.Scope) {
			return errors.New("所选规则集来源不支持当前 scope")
		}
	}
	return nil
}

type ruleSetProbeCandidate struct {
	URL      string
	SourceID string
	Format   string
}

func buildRuleSetProbeCandidates(item SubscriptionRuleSetProbeItem) ([]ruleSetProbeCandidate, error) {
	if item.URL != "" {
		format, err := ruleSetProbeFormatForURL(item.Kind, item.URL)
		if err != nil {
			return nil, err
		}
		return []ruleSetProbeCandidate{{URL: item.URL, SourceID: item.SourceID, Format: format}}, nil
	}
	registry := subscriptionRuleSetSourceRegistry[item.Kind]
	ordered := make([]RuleSetSourceEntry, 0, len(registry))
	for _, source := range registry {
		if source.ID == item.SourceID {
			ordered = append(ordered, source)
			break
		}
	}
	for _, source := range registry {
		if item.AllowFallback != nil && !*item.AllowFallback {
			break
		}
		if source.ID != item.SourceID && source.SupportsScope(item.Scope) {
			ordered = append(ordered, source)
		}
	}
	candidates := make([]ruleSetProbeCandidate, 0, len(ordered))
	for _, source := range ordered {
		template := source.TemplateForScope(item.Scope)
		if template == "" {
			continue
		}
		for _, name := range normalizeRuleSetProbeNames(item.Kind, source.ID, item.Scope, item.Name) {
			candidateURL := strings.ReplaceAll(template, "{name}", url.PathEscape(name))
			candidates = append(candidates, ruleSetProbeCandidate{
				URL:      candidateURL,
				SourceID: source.ID,
				Format:   source.Format,
			})
		}
	}
	if len(candidates) == 0 {
		return nil, errors.New("没有支持当前 scope 的规则集来源")
	}
	return candidates, nil
}

func ruleSetProbeFormatForURL(kind string, rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("规则集 URL 无效: %w", err)
	}
	extension := strings.ToLower(path.Ext(parsed.EscapedPath()))
	if unescaped, unescapeErr := url.PathUnescape(extension); unescapeErr == nil {
		extension = strings.ToLower(unescaped)
	}
	formats := map[string]map[string]string{
		"json": {
			".srs":  "srs",
			".json": "json",
		},
		"clash": {
			".mrs":  "mrs",
			".yaml": "yaml",
			".yml":  "yaml",
			".txt":  "text",
			".list": "text",
		},
	}
	if format := formats[kind][extension]; format != "" {
		return format, nil
	}
	return "", fmt.Errorf("%s 规则集 URL 扩展名不受支持: %s", kind, extension)
}

func normalizeRuleSetProbeName(sourceID string, name string) string {
	normalized := strings.TrimSpace(name)
	if sourceID == "metacubex_github" || sourceID == "metacubex_cdn" || sourceID == "karingx_github" || sourceID == "karingx_cdn" {
		switch strings.ToLower(normalized) {
		case "ads":
			return "category-ads-all"
		case "ir":
			return "category-ir"
		}
	}
	return normalized
}

func normalizeRuleSetProbeNames(kind string, sourceID string, scope string, name string) []string {
	base := normalizeRuleSetProbeName(sourceID, name)
	if kind != "json" || sourceID != "quixoticheart_github" || scope != "ip" || strings.HasSuffix(base, "cidr") {
		return []string{base}
	}
	if len(base) == 2 {
		return []string{base + "cidr", base}
	}
	return []string{base, base + "cidr"}
}

func ruleSetProbeCacheKey(item SubscriptionRuleSetProbeItem) string {
	allowFallback := true
	if item.AllowFallback != nil {
		allowFallback = *item.AllowFallback
	}
	return strings.Join([]string{item.Kind, item.SourceID, item.Scope, item.Name, item.URL, strconv.FormatBool(allowFallback)}, "\x00")
}

func (cache *ruleSetProbeMetadataCache) get(key string, now time.Time) (SubscriptionRuleSetProbeResult, bool) {
	if cache == nil {
		return SubscriptionRuleSetProbeResult{}, false
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	element, exists := cache.entries[key]
	if !exists {
		return SubscriptionRuleSetProbeResult{}, false
	}
	entry := element.Value.(*ruleSetProbeCacheEntry)
	if !now.Before(entry.expiresAt) {
		cache.removeElement(element)
		return SubscriptionRuleSetProbeResult{}, false
	}
	cache.lru.MoveToFront(element)
	return entry.result, true
}

func (cache *ruleSetProbeMetadataCache) put(key string, result SubscriptionRuleSetProbeResult, now time.Time) {
	if cache == nil || cache.limit <= 0 {
		return
	}
	result.ID = ""
	result.Cached = false
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if element, exists := cache.entries[key]; exists {
		entry := element.Value.(*ruleSetProbeCacheEntry)
		entry.result = result
		entry.expiresAt = now.Add(cache.ttl)
		cache.lru.MoveToFront(element)
		return
	}
	element := cache.lru.PushFront(&ruleSetProbeCacheEntry{key: key, result: result, expiresAt: now.Add(cache.ttl)})
	cache.entries[key] = element
	for cache.lru.Len() > cache.limit {
		cache.removeElement(cache.lru.Back())
	}
}

func (cache *ruleSetProbeMetadataCache) removeElement(element *list.Element) {
	if element == nil {
		return
	}
	entry := element.Value.(*ruleSetProbeCacheEntry)
	delete(cache.entries, entry.key)
	cache.lru.Remove(element)
}

func (engine *ruleSetProbeEngine) fetch(ctx context.Context, rawURL string) ([]byte, string, string, error) {
	current, err := url.Parse(rawURL)
	if err != nil {
		return nil, "", "", fmt.Errorf("规则集 URL 无效: %w", err)
	}
	for redirects := 0; ; redirects++ {
		body, contentType, location, fetchErr := engine.fetchOnce(ctx, current)
		if fetchErr != nil {
			return nil, "", "", fetchErr
		}
		if location == "" {
			return body, contentType, current.String(), nil
		}
		if redirects >= ruleSetProbeMaxRedirects {
			return nil, "", "", errors.New("规则集重定向超过 3 次")
		}
		next, resolveErr := current.Parse(location)
		if resolveErr != nil {
			return nil, "", "", fmt.Errorf("规则集重定向地址无效: %w", resolveErr)
		}
		current = next
	}
}

func (engine *ruleSetProbeEngine) fetchOnce(ctx context.Context, target *url.URL) ([]byte, string, string, error) {
	address, port, err := engine.validateAndResolveTarget(ctx, target)
	if err != nil {
		return nil, "", "", err
	}
	fixedAddress := net.JoinHostPort(address.String(), port)
	expectedHost := strings.TrimSuffix(strings.ToLower(target.Hostname()), ".")
	dialer := &net.Dialer{Timeout: 3 * time.Second, KeepAlive: -1}
	transport := &http.Transport{
		Proxy:                 nil,
		DisableCompression:    true,
		DisableKeepAlives:     true,
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   3 * time.Second,
		ResponseHeaderTimeout: 4 * time.Second,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: engine.rootCAs},
		DialContext: func(dialContext context.Context, network string, requested string) (net.Conn, error) {
			host, _, splitErr := net.SplitHostPort(requested)
			if splitErr != nil || strings.TrimSuffix(strings.ToLower(host), ".") != expectedHost {
				return nil, errors.New("规则集拨号目标与已验证主机不一致")
			}
			return dialer.DialContext(dialContext, network, fixedAddress)
		},
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, "", "", err
	}
	request.Header.Set("Accept", "application/octet-stream, application/json, application/yaml, text/yaml, text/plain;q=0.9")
	request.Header.Set("Accept-Encoding", ruleSetProbeAcceptEncoding)
	request.Header.Set("User-Agent", "kwor-ruleset-probe/1")
	response, err := client.Do(request)
	if err != nil {
		return nil, "", "", fmt.Errorf("规则集请求失败: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode >= 300 && response.StatusCode <= 399 {
		location := strings.TrimSpace(response.Header.Get("Location"))
		if location == "" {
			return nil, "", "", fmt.Errorf("规则集重定向缺少 Location，HTTP %d", response.StatusCode)
		}
		return nil, "", location, nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, "", "", fmt.Errorf("规则集服务器返回 HTTP %d", response.StatusCode)
	}
	if response.ContentLength > ruleSetProbeMaxWireBytes {
		return nil, "", "", errors.New("规则集压缩响应超过 8 MiB")
	}
	wire, err := readRuleSetProbeLimited(response.Body, ruleSetProbeMaxWireBytes, "规则集压缩响应超过 8 MiB")
	if err != nil {
		return nil, "", "", err
	}
	decoded, err := decodeRuleSetProbeBody(wire, strings.Join(response.Header.Values("Content-Encoding"), ","))
	if err != nil {
		return nil, "", "", err
	}
	return decoded, response.Header.Get("Content-Type"), "", nil
}

func (engine *ruleSetProbeEngine) validateAndResolveTarget(ctx context.Context, target *url.URL) (netip.Addr, string, error) {
	if target == nil || (target.Scheme != "http" && target.Scheme != "https") || target.Hostname() == "" {
		return netip.Addr{}, "", errors.New("规则集仅允许公网 HTTP/HTTPS URL")
	}
	if target.User != nil {
		return netip.Addr{}, "", errors.New("规则集 URL 不允许包含用户信息")
	}
	port := target.Port()
	if port == "" {
		if target.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	parsedPort, err := strconv.Atoi(port)
	if err != nil || parsedPort < 1 || parsedPort > 65535 {
		return netip.Addr{}, "", errors.New("规则集 URL 端口无效")
	}

	hostname := strings.TrimSuffix(target.Hostname(), ".")
	addresses := make([]netip.Addr, 0, 4)
	if literal, parseErr := netip.ParseAddr(hostname); parseErr == nil {
		addresses = append(addresses, literal.Unmap())
	} else {
		resolved, resolveErr := engine.resolver.LookupNetIP(ctx, "ip", hostname)
		if resolveErr != nil {
			return netip.Addr{}, "", fmt.Errorf("规则集域名解析失败: %w", resolveErr)
		}
		for _, address := range resolved {
			addresses = append(addresses, address.Unmap())
		}
	}
	if len(addresses) == 0 {
		return netip.Addr{}, "", errors.New("规则集域名没有可用 IP")
	}
	allowAddress := engine.allowAddress
	if allowAddress == nil {
		allowAddress = isPublicRuleSetProbeAddress
	}
	for _, address := range addresses {
		if !allowAddress(address) {
			return netip.Addr{}, "", fmt.Errorf("规则集目标不是允许的公网地址: %s", address)
		}
	}
	return addresses[0], port, nil
}

var blockedRuleSetProbePrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/128"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/32"),
	netip.MustParsePrefix("2001:2::/48"),
	netip.MustParsePrefix("2001:10::/28"),
	netip.MustParsePrefix("2001:20::/28"),
	netip.MustParsePrefix("2001:30::/28"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("fec0::/10"),
	netip.MustParsePrefix("ff00::/8"),
}

func isPublicRuleSetProbeAddress(address netip.Addr) bool {
	address = address.Unmap()
	if !address.IsValid() || !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() {
		return false
	}
	for _, prefix := range blockedRuleSetProbePrefixes {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}

func readRuleSetProbeLimited(reader io.Reader, maximum int64, message string) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maximum {
		return nil, errors.New(message)
	}
	return raw, nil
}

func decodeRuleSetProbeBody(wire []byte, contentEncoding string) ([]byte, error) {
	if strings.TrimSpace(contentEncoding) != "" && !compressionalgorithm.ContentEncodingAcceptableValues(contentEncoding, []string{ruleSetProbeAcceptEncoding}) {
		return nil, fmt.Errorf("规则集响应使用未请求的压缩格式: %s", strings.TrimSpace(contentEncoding))
	}
	reader, err := compressionalgorithm.NewDecoder(
		io.NopCloser(bytes.NewReader(wire)),
		contentEncoding,
		ruleSetProbeMaxDecodedBytes,
	)
	if err != nil {
		if errors.Is(err, compressionalgorithm.ErrUnsupportedEncoding) {
			return nil, fmt.Errorf("规则集响应使用不支持的压缩格式: %s", strings.TrimSpace(contentEncoding))
		}
		return nil, fmt.Errorf("规则集响应解压失败: %w", err)
	}
	defer reader.Close()
	return readRuleSetProbeLimited(reader, ruleSetProbeMaxDecodedBytes, "规则集解压内容超过 32 MiB")
}

func validateRuleSetProbeContent(body []byte, contentType string, finalURL string, expectedFormat string, expectedScope string) (string, error) {
	if len(body) == 0 {
		return "", errors.New("规则集内容为空")
	}
	expectedFormat = strings.ToLower(strings.TrimSpace(expectedFormat))
	if expectedFormat == "srs" || bytes.HasPrefix(body, []byte("SRS")) {
		if expectedFormat != "" && expectedFormat != "srs" {
			return "", errors.New("规则集内容格式与来源不一致")
		}
		if err := validateSRSRuleSet(body); err != nil {
			return "", err
		}
		return "srs", nil
	}
	if expectedFormat == "mrs" || looksLikeZstd(body) {
		behavior, err := validateMRSRuleSet(body)
		if err == nil {
			if behavior != expectedScope {
				return "", fmt.Errorf("MRS behavior 为 %s，与期望 scope %s 不一致", behavior, expectedScope)
			}
			return "mrs", nil
		}
		if expectedFormat == "mrs" {
			return "", err
		}
	}
	if expectedFormat == "srs" || expectedFormat == "mrs" {
		return "", fmt.Errorf("规则集内容不是有效的 %s 文件", strings.ToUpper(expectedFormat))
	}
	if !utf8.Valid(body) {
		return "", errors.New("文本规则集不是有效 UTF-8")
	}
	text := strings.TrimSpace(string(body))
	if text == "" || strings.Contains(strings.ToLower(contentType), "text/html") || looksLikeHTML(text) {
		return "", errors.New("规则集响应不是规则内容")
	}
	domainCount, ipCount, format, err := classifyTextRuleSet(body, contentType, finalURL)
	if err != nil {
		return "", err
	}
	if (expectedFormat == "json" || expectedFormat == "yaml" || expectedFormat == "text") && format != expectedFormat {
		return "", fmt.Errorf("规则集内容格式为 %s，与期望格式 %s 不一致", format, expectedFormat)
	}
	if expectedScope == "domain" && (domainCount == 0 || ipCount > 0) {
		return "", fmt.Errorf("规则集内容 scope 不一致: domain=%d ip=%d", domainCount, ipCount)
	}
	if expectedScope == "ip" && (ipCount == 0 || domainCount > 0) {
		return "", fmt.Errorf("规则集内容 scope 不一致: domain=%d ip=%d", domainCount, ipCount)
	}
	return format, nil
}

func validateSRSRuleSet(body []byte) error {
	if len(body) < 6 || !bytes.Equal(body[:3], []byte("SRS")) {
		return errors.New("SRS magic 无效")
	}
	if body[3] < 1 || body[3] > 3 {
		return fmt.Errorf("SRS 版本不受支持: %d", body[3])
	}
	reader, err := zlib.NewReader(bytes.NewReader(body[4:]))
	if err != nil {
		return fmt.Errorf("SRS 压缩数据无效: %w", err)
	}
	defer reader.Close()
	decoded, err := readRuleSetProbeLimited(reader, ruleSetProbeMaxDecodedBytes, "SRS 解压内容超过 32 MiB")
	if err != nil {
		return err
	}
	ruleCount, read := binary.Uvarint(decoded)
	if read <= 0 || ruleCount == 0 || len(decoded) <= read {
		return errors.New("SRS 规则内容无效或为空")
	}
	return nil
}

func looksLikeZstd(body []byte) bool {
	return len(body) >= 4 && bytes.Equal(body[:4], []byte{0x28, 0xb5, 0x2f, 0xfd})
}

func validateMRSRuleSet(body []byte) (string, error) {
	decoder, err := zstd.NewReader(bytes.NewReader(body), zstd.WithDecoderMaxMemory(ruleSetProbeMaxDecodedBytes))
	if err != nil {
		return "", fmt.Errorf("MRS zstd 数据无效: %w", err)
	}
	defer decoder.Close()
	decoded, err := readRuleSetProbeLimited(decoder, ruleSetProbeMaxDecodedBytes, "MRS 解压内容超过 32 MiB")
	if err != nil {
		return "", err
	}
	if len(decoded) < 21 || !bytes.Equal(decoded[:4], []byte{'M', 'R', 'S', 1}) {
		return "", errors.New("MRS magic 无效")
	}
	behavior := ""
	switch decoded[4] {
	case 0:
		behavior = "domain"
	case 1:
		behavior = "ip"
	default:
		return "", errors.New("MRS behavior 不受支持")
	}
	count := int64(binary.BigEndian.Uint64(decoded[5:13]))
	extraLength := int64(binary.BigEndian.Uint64(decoded[13:21]))
	if count <= 0 || extraLength < 0 || extraLength > int64(len(decoded)-21) || int64(len(decoded)) <= 21+extraLength {
		return "", errors.New("MRS 规则内容无效或为空")
	}
	return behavior, nil
}

func looksLikeHTML(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	return strings.HasPrefix(lower, "<!doctype html") || strings.HasPrefix(lower, "<html") || strings.Contains(lower[:min(len(lower), 512)], "<body")
}

func classifyTextRuleSet(body []byte, contentType string, finalURL string) (int, int, string, error) {
	trimmed := bytes.TrimSpace(body)
	format := inferTextRuleSetFormat(contentType, finalURL, trimmed)
	var value interface{}
	switch format {
	case "json":
		decoder := json.NewDecoder(bytes.NewReader(trimmed))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil {
			return 0, 0, "", fmt.Errorf("JSON 规则集解析失败: %w", err)
		}
	case "yaml":
		var node yaml.Node
		if err := yaml.Unmarshal(trimmed, &node); err != nil {
			return 0, 0, "", fmt.Errorf("YAML 规则集解析失败: %w", err)
		}
		if err := rejectDuplicateYAMLKeys(&node, "$ruleset"); err != nil {
			return 0, 0, "", err
		}
		if err := node.Decode(&value); err != nil {
			return 0, 0, "", fmt.Errorf("YAML 规则集解析失败: %w", err)
		}
	default:
		value = strings.Split(string(trimmed), "\n")
		format = "text"
	}
	domainCount, ipCount := classifyRuleSetValue(value, "")
	if domainCount == 0 && ipCount == 0 {
		return 0, 0, "", errors.New("规则集中没有可识别的 domain/IP 条目")
	}
	return domainCount, ipCount, format, nil
}

func inferTextRuleSetFormat(contentType string, finalURL string, body []byte) string {
	lowerType := strings.ToLower(contentType)
	extension := strings.ToLower(path.Ext(strings.Split(finalURL, "?")[0]))
	if strings.Contains(lowerType, "json") || extension == ".json" || (len(body) > 0 && (body[0] == '{' || body[0] == '[')) {
		return "json"
	}
	if strings.Contains(lowerType, "yaml") || extension == ".yaml" || extension == ".yml" {
		return "yaml"
	}
	if bytes.HasPrefix(body, []byte("payload:")) || bytes.HasPrefix(body, []byte("rules:")) {
		return "yaml"
	}
	return "text"
}

func classifyRuleSetValue(value interface{}, key string) (int, int) {
	domains := 0
	ips := 0
	switch typed := value.(type) {
	case map[string]interface{}:
		for childKey, child := range typed {
			domainCount, ipCount := classifyRuleSetValue(child, strings.ToLower(strings.TrimSpace(childKey)))
			domains += domainCount
			ips += ipCount
		}
	case map[interface{}]interface{}:
		for childKey, child := range typed {
			domainCount, ipCount := classifyRuleSetValue(child, strings.ToLower(strings.TrimSpace(fmt.Sprint(childKey))))
			domains += domainCount
			ips += ipCount
		}
	case []interface{}:
		for _, child := range typed {
			domainCount, ipCount := classifyRuleSetValue(child, key)
			domains += domainCount
			ips += ipCount
		}
	case []string:
		for _, child := range typed {
			domainCount, ipCount := classifyRuleSetString(child, key)
			domains += domainCount
			ips += ipCount
		}
	case string:
		domains, ips = classifyRuleSetString(typed, key)
	}
	return domains, ips
}

func classifyRuleSetString(raw string, key string) (int, int) {
	line := strings.TrimSpace(strings.TrimPrefix(raw, "-"))
	if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
		return 0, 0
	}
	line = strings.Trim(strings.TrimSpace(line), "\"'")
	lowerKey := strings.ToLower(key)
	if strings.Contains(lowerKey, "ip_cidr") || strings.Contains(lowerKey, "ipcidr") {
		return 0, 1
	}
	if strings.Contains(lowerKey, "domain") || strings.Contains(lowerKey, "geosite") {
		return 1, 0
	}
	upper := strings.ToUpper(line)
	for _, prefix := range []string{"IP-CIDR,", "IP-CIDR6,", "IP-SUFFIX,", "IP-ASN,", "GEOIP,"} {
		if strings.HasPrefix(upper, prefix) {
			return 0, 1
		}
	}
	for _, prefix := range []string{"DOMAIN,", "DOMAIN-SUFFIX,", "DOMAIN-KEYWORD,", "DOMAIN-WILDCARD,", "DOMAIN-REGEX,"} {
		if strings.HasPrefix(upper, prefix) {
			return 1, 0
		}
	}
	first := strings.TrimSpace(strings.Split(line, ",")[0])
	if prefix, err := netip.ParsePrefix(first); err == nil && prefix.IsValid() {
		return 0, 1
	}
	if address, err := netip.ParseAddr(first); err == nil && address.IsValid() {
		return 0, 1
	}
	if strings.Contains(first, ".") || strings.HasPrefix(first, "+.") || strings.HasPrefix(first, "*.") {
		return 1, 0
	}
	return 0, 0
}
