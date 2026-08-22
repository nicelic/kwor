package api

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alireza0/s-ui/service"
	"github.com/klauspost/compress/zstd"
)

type probeTestResolver struct {
	mu        sync.Mutex
	addresses [][]netip.Addr
	calls     int
}

func (resolver *probeTestResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	index := resolver.calls
	resolver.calls++
	if index >= len(resolver.addresses) {
		index = len(resolver.addresses) - 1
	}
	return append([]netip.Addr(nil), resolver.addresses[index]...), nil
}

func (resolver *probeTestResolver) callCount() int {
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	return resolver.calls
}

func TestRuleSetProbeHTTPStatusScopeFormatAndCache(t *testing.T) {
	var domainRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/403.json":
			writer.WriteHeader(http.StatusForbidden)
		case "/404.json":
			writer.WriteHeader(http.StatusNotFound)
		case "/domain.json":
			domainRequests.Add(1)
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"rules":[{"domain_suffix":["example.com"]}]}`))
		case "/ip.json":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"rules":[{"ip_cidr":["1.1.1.0/24"]}]}`))
		case "/wrong.json":
			writer.Header().Set("Content-Type", "application/yaml")
			_, _ = writer.Write([]byte("payload:\n  - example.com\n"))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	resolver := &probeTestResolver{addresses: [][]netip.Addr{{netip.MustParseAddr("127.0.0.1")}}}
	engine := newLocalRuleSetProbeEngine(resolver, nil, time.Second, 4, time.Minute)
	baseURL := probeTestURL(t, server.URL, "rules.test")

	for _, statusPath := range []string{"/403.json", "/404.json"} {
		result := probeTestOne(t, engine, "json", "domain", baseURL+statusPath)
		if result.Valid || !strings.Contains(result.Error, "HTTP") {
			t.Fatalf("status probe %s = %#v", statusPath, result)
		}
	}
	valid := probeTestOne(t, engine, "json", "domain", baseURL+"/domain.json")
	if !valid.Valid || valid.Format != "json" {
		t.Fatalf("valid domain JSON probe = %#v", valid)
	}
	cached := probeTestOne(t, engine, "json", "domain", baseURL+"/domain.json")
	if !cached.Valid || !cached.Cached || domainRequests.Load() != 1 {
		t.Fatalf("cached probe = %#v, requests=%d", cached, domainRequests.Load())
	}
	scopeMismatch := probeTestOne(t, engine, "json", "ip", baseURL+"/domain.json")
	if scopeMismatch.Valid || !strings.Contains(scopeMismatch.Error, "scope") {
		t.Fatalf("scope mismatch probe = %#v", scopeMismatch)
	}
	formatMismatch := probeTestOne(t, engine, "json", "domain", baseURL+"/wrong.json")
	if formatMismatch.Valid || (!strings.Contains(formatMismatch.Error, "格式") && !strings.Contains(formatMismatch.Error, "JSON")) {
		t.Fatalf("format mismatch probe = %#v", formatMismatch)
	}
	ip := probeTestOne(t, engine, "json", "ip", baseURL+"/ip.json")
	if !ip.Valid {
		t.Fatalf("valid IP JSON probe = %#v", ip)
	}
}

func TestRuleSetProbeRedirectRevalidationAndLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/redirect.json":
			step, _ := strconv.Atoi(request.URL.Query().Get("step"))
			http.Redirect(writer, request, "/redirect.json?step="+strconv.Itoa(step+1), http.StatusFound)
		case "/private-redirect.json":
			_, port, _ := net.SplitHostPort(request.Host)
			http.Redirect(writer, request, "http://127.0.0.2:"+port+"/domain.json", http.StatusFound)
		case "/rebind.json":
			http.Redirect(writer, request, "/domain.json", http.StatusFound)
		case "/domain.json":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"rules":[{"domain":["example.com"]}]}`))
		}
	}))
	defer server.Close()
	baseURL := probeTestURL(t, server.URL, "rules.test")
	loopbackOne := netip.MustParseAddr("127.0.0.1")
	resolver := &probeTestResolver{addresses: [][]netip.Addr{{loopbackOne}}}
	allowOnlyServer := func(address netip.Addr) bool { return address == loopbackOne }
	engine := newLocalRuleSetProbeEngine(resolver, allowOnlyServer, time.Second, 4, time.Minute)

	tooMany := probeTestOne(t, engine, "json", "domain", baseURL+"/redirect.json?step=0")
	if tooMany.Valid || !strings.Contains(tooMany.Error, "超过 3 次") {
		t.Fatalf("redirect limit probe = %#v", tooMany)
	}
	privateRedirect := probeTestOne(t, engine, "json", "domain", baseURL+"/private-redirect.json")
	if privateRedirect.Valid || !strings.Contains(privateRedirect.Error, "公网") {
		t.Fatalf("private redirect probe = %#v", privateRedirect)
	}

	rebindingResolver := &probeTestResolver{addresses: [][]netip.Addr{
		{loopbackOne},
		{netip.MustParseAddr("127.0.0.2")},
	}}
	rebindingEngine := newLocalRuleSetProbeEngine(rebindingResolver, allowOnlyServer, time.Second, 4, time.Minute)
	rebinding := probeTestOne(t, rebindingEngine, "json", "domain", baseURL+"/rebind.json")
	if rebinding.Valid || !strings.Contains(rebinding.Error, "公网") || rebindingResolver.callCount() != 2 {
		t.Fatalf("DNS rebinding probe = %#v, lookups=%d", rebinding, rebindingResolver.callCount())
	}
}

func TestRuleSetProbeTimeoutSizeConcurrencyAndCacheExpiry(t *testing.T) {
	var active atomic.Int32
	var maximum atomic.Int32
	var cacheRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/slow.json":
			select {
			case <-request.Context().Done():
				return
			case <-time.After(500 * time.Millisecond):
			}
		case "/wire.json":
			writer.Header().Set("Content-Length", strconv.Itoa(8*1024*1024+1))
		case "/decoded.json":
			writer.Header().Set("Content-Encoding", "gzip")
			_, _ = writer.Write(oversizedGzipRuleSet(t))
		case "/concurrent.json":
			current := active.Add(1)
			for {
				observed := maximum.Load()
				if current <= observed || maximum.CompareAndSwap(observed, current) {
					break
				}
			}
			time.Sleep(60 * time.Millisecond)
			active.Add(-1)
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"rules":[{"domain":["example.com"]}]}`))
		case "/cache.json":
			cacheRequests.Add(1)
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"rules":[{"domain":["example.com"]}]}`))
		}
	}))
	defer server.Close()
	resolver := &probeTestResolver{addresses: [][]netip.Addr{{netip.MustParseAddr("127.0.0.1")}}}
	baseURL := probeTestURL(t, server.URL, "rules.test")

	timeoutEngine := newLocalRuleSetProbeEngine(resolver, nil, 80*time.Millisecond, 4, time.Minute)
	timedOut := probeTestOne(t, timeoutEngine, "json", "domain", baseURL+"/slow.json")
	lowerTimeoutError := strings.ToLower(timedOut.Error)
	if timedOut.Valid || (!strings.Contains(lowerTimeoutError, "timeout") && !strings.Contains(lowerTimeoutError, "deadline exceeded") && !strings.Contains(timedOut.Error, "超时")) {
		t.Fatalf("timeout probe = %#v", timedOut)
	}
	sizeEngine := newLocalRuleSetProbeEngine(resolver, nil, time.Second, 4, time.Minute)
	for _, sizePath := range []string{"/wire.json", "/decoded.json"} {
		result := probeTestOne(t, sizeEngine, "json", "domain", baseURL+sizePath)
		if result.Valid || !strings.Contains(result.Error, "MiB") {
			t.Fatalf("size probe %s = %#v", sizePath, result)
		}
	}

	concurrencyEngine := newLocalRuleSetProbeEngine(resolver, nil, 2*time.Second, 4, time.Minute)
	items := make([]service.SubscriptionRuleSetProbeItem, 8)
	for index := range items {
		items[index] = service.SubscriptionRuleSetProbeItem{
			ID: "concurrency-" + strconv.Itoa(index), Kind: "json", Scope: "domain",
			URL: baseURL + "/concurrent.json?item=" + strconv.Itoa(index),
		}
	}
	results, err := concurrencyEngine.Probe(context.Background(), service.SubscriptionRuleSetProbeRequest{Items: items})
	if err != nil {
		t.Fatalf("concurrency probe: %v", err)
	}
	for _, result := range results {
		if !result.Valid {
			t.Fatalf("concurrency result = %#v", result)
		}
	}
	if maximum.Load() < 2 || maximum.Load() > 4 {
		t.Fatalf("maximum probe concurrency = %d, want 2..4", maximum.Load())
	}

	expiringEngine := newLocalRuleSetProbeEngine(resolver, nil, time.Second, 4, 30*time.Millisecond)
	first := probeTestOne(t, expiringEngine, "json", "domain", baseURL+"/cache.json")
	second := probeTestOne(t, expiringEngine, "json", "domain", baseURL+"/cache.json")
	time.Sleep(45 * time.Millisecond)
	third := probeTestOne(t, expiringEngine, "json", "domain", baseURL+"/cache.json")
	if !first.Valid || !second.Cached || third.Cached || cacheRequests.Load() != 2 {
		t.Fatalf("cache expiry: first=%#v second=%#v third=%#v requests=%d", first, second, third, cacheRequests.Load())
	}
}

func TestRuleSetProbeRejectsExcessInFlightBatches(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"rules":[{"domain":["example.com"]}]}`))
	}))
	defer server.Close()

	resolver := &probeTestResolver{addresses: [][]netip.Addr{{netip.MustParseAddr("127.0.0.1")}}}
	engine := service.NewSubscriptionRuleSetProbeEngine(service.SubscriptionRuleSetProbeEngineOptions{
		Resolver:           resolver,
		AllowAddress:       func(netip.Addr) bool { return true },
		Timeout:            time.Second,
		Concurrency:        1,
		CacheEntries:       8,
		CacheTTL:           time.Minute,
		MaxInFlightBatches: 1,
	})
	request := service.SubscriptionRuleSetProbeRequest{Items: []service.SubscriptionRuleSetProbeItem{{
		ID: "one", Kind: "json", Scope: "domain", URL: probeTestURL(t, server.URL, "rules.test") + "/domain.json",
	}}}
	firstDone := make(chan error, 1)
	go func() {
		_, err := engine.Probe(context.Background(), request)
		firstDone <- err
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first probe did not occupy the in-flight batch slot")
	}

	if _, err := engine.Probe(context.Background(), request); err == nil || !strings.Contains(err.Error(), "请求过多") {
		t.Fatalf("second probe error = %v, want overload rejection", err)
	}

	close(release)
	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatalf("first probe failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("first probe did not finish after release")
	}
}

func TestRuleSetProbeTLSCertificateValidation(t *testing.T) {
	handler := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"rules":[{"domain":["example.com"]}]}`))
	})
	resolver := &probeTestResolver{addresses: [][]netip.Addr{{netip.MustParseAddr("127.0.0.1")}}}

	selfSigned := httptest.NewTLSServer(handler)
	selfSignedURL := probeTestURL(t, selfSigned.URL, "rules.test") + "/domain.json"
	selfSignedResult := probeTestOne(t, newLocalRuleSetProbeEngine(resolver, nil, time.Second, 4, time.Minute), "json", "domain", selfSignedURL)
	selfSigned.Close()
	if selfSignedResult.Valid {
		t.Fatalf("self-signed certificate unexpectedly accepted: %#v", selfSignedResult)
	}

	ca, caKey, roots := newProbeTestCA(t)
	validServer := newProbeTLSServer(t, handler, ca, caKey, "rules.test", time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	validEngine := newLocalRuleSetProbeEngineWithRoots(resolver, roots)
	valid := probeTestOne(t, validEngine, "json", "domain", probeTestURL(t, validServer.URL, "rules.test")+"/domain.json")
	validServer.Close()
	if !valid.Valid {
		t.Fatalf("valid trusted certificate rejected: %#v", valid)
	}

	expiredServer := newProbeTLSServer(t, handler, ca, caKey, "rules.test", time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour))
	expired := probeTestOne(t, newLocalRuleSetProbeEngineWithRoots(resolver, roots), "json", "domain", probeTestURL(t, expiredServer.URL, "rules.test")+"/domain.json")
	expiredServer.Close()
	if expired.Valid {
		t.Fatalf("expired certificate unexpectedly accepted: %#v", expired)
	}

	mismatchServer := newProbeTLSServer(t, handler, ca, caKey, "other.test", time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	mismatch := probeTestOne(t, newLocalRuleSetProbeEngineWithRoots(resolver, roots), "json", "domain", probeTestURL(t, mismatchServer.URL, "rules.test")+"/domain.json")
	mismatchServer.Close()
	if mismatch.Valid {
		t.Fatalf("hostname-mismatched certificate unexpectedly accepted: %#v", mismatch)
	}
}

func TestRuleSetProbeBinaryMagicAndBehavior(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/valid.srs":
			_, _ = writer.Write(validProbeSRS(t))
		case "/bad-version.srs":
			raw := validProbeSRS(t)
			raw[3] = 9
			_, _ = writer.Write(raw)
		case "/domain.mrs":
			_, _ = writer.Write(validProbeMRS(t, 0))
		case "/ip.mrs":
			_, _ = writer.Write(validProbeMRS(t, 1))
		}
	}))
	defer server.Close()
	resolver := &probeTestResolver{addresses: [][]netip.Addr{{netip.MustParseAddr("127.0.0.1")}}}
	engine := newLocalRuleSetProbeEngine(resolver, nil, time.Second, 4, time.Minute)
	baseURL := probeTestURL(t, server.URL, "rules.test")
	if result := probeTestOne(t, engine, "json", "domain", baseURL+"/valid.srs"); !result.Valid || result.Format != "srs" {
		t.Fatalf("valid SRS probe = %#v", result)
	}
	if result := probeTestOne(t, engine, "json", "domain", baseURL+"/bad-version.srs"); result.Valid {
		t.Fatalf("unsupported SRS version accepted: %#v", result)
	}
	if result := probeTestOne(t, engine, "clash", "domain", baseURL+"/domain.mrs"); !result.Valid {
		t.Fatalf("valid domain MRS probe = %#v", result)
	}
	if result := probeTestOne(t, engine, "clash", "domain", baseURL+"/ip.mrs"); result.Valid || !strings.Contains(result.Error, "behavior") {
		t.Fatalf("MRS behavior mismatch probe = %#v", result)
	}
}

func newLocalRuleSetProbeEngine(
	resolver service.SubscriptionRuleSetProbeResolver,
	allowAddress func(netip.Addr) bool,
	timeout time.Duration,
	concurrency int,
	cacheTTL time.Duration,
) *service.SubscriptionRuleSetProbeEngine {
	if allowAddress == nil {
		allowAddress = func(netip.Addr) bool { return true }
	}
	return service.NewSubscriptionRuleSetProbeEngine(service.SubscriptionRuleSetProbeEngineOptions{
		Resolver: resolver, AllowAddress: allowAddress, Timeout: timeout, Concurrency: concurrency,
		CacheEntries: 256, CacheTTL: cacheTTL,
	})
}

func newLocalRuleSetProbeEngineWithRoots(resolver service.SubscriptionRuleSetProbeResolver, roots *x509.CertPool) *service.SubscriptionRuleSetProbeEngine {
	return service.NewSubscriptionRuleSetProbeEngine(service.SubscriptionRuleSetProbeEngineOptions{
		Resolver:     resolver,
		AllowAddress: func(netip.Addr) bool { return true },
		Timeout:      time.Second,
		Concurrency:  4,
		CacheEntries: 32,
		CacheTTL:     time.Minute,
		RootCAs:      roots,
	})
}

func probeTestOne(t *testing.T, engine *service.SubscriptionRuleSetProbeEngine, kind string, scope string, target string) service.SubscriptionRuleSetProbeResult {
	t.Helper()
	results, err := engine.Probe(context.Background(), service.SubscriptionRuleSetProbeRequest{Items: []service.SubscriptionRuleSetProbeItem{{
		ID: target, Kind: kind, Scope: scope, URL: target,
	}}})
	if err != nil {
		t.Fatalf("probe %s: %v", target, err)
	}
	if len(results) != 1 {
		t.Fatalf("probe %s returned %d results", target, len(results))
	}
	return results[0]
}

func probeTestURL(t *testing.T, serverURL string, hostname string) string {
	t.Helper()
	parsed, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	_, port, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatalf("parse test server host: %v", err)
	}
	return parsed.Scheme + "://" + net.JoinHostPort(hostname, port)
}

func oversizedGzipRuleSet(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := gzip.NewWriter(&buffer)
	chunk := bytes.Repeat([]byte("a"), 1024)
	for written := 0; written <= 32*1024; written++ {
		if _, err := writer.Write(chunk); err != nil {
			t.Fatalf("write gzip fixture: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close gzip fixture: %v", err)
	}
	return buffer.Bytes()
}

func newProbeTestCA(t *testing.T) (*x509.Certificate, *ecdsa.PrivateKey, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	certificate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "probe test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	raw, err := x509.CreateCertificate(rand.Reader, certificate, certificate, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create CA certificate: %v", err)
	}
	parsed, err := x509.ParseCertificate(raw)
	if err != nil {
		t.Fatalf("parse CA certificate: %v", err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(parsed)
	return parsed, key, roots
}

func newProbeTLSServer(
	t *testing.T,
	handler http.Handler,
	ca *x509.Certificate,
	caKey *ecdsa.PrivateKey,
	dnsName string,
	notBefore time.Time,
	notAfter time.Time,
) *httptest.Server {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate server key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: dnsName},
		DNSNames:     []string{dnsName},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	raw, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create server certificate: %v", err)
	}
	keyRaw, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal server key: %v", err)
	}
	pair, err := tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: raw}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyRaw}),
	)
	if err != nil {
		t.Fatalf("load server key pair: %v", err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{Certificates: []tls.Certificate{pair}}
	server.StartTLS()
	return server
}

func validProbeSRS(t *testing.T) []byte {
	t.Helper()
	var output bytes.Buffer
	_, _ = output.Write([]byte{'S', 'R', 'S', 1})
	writer := zlib.NewWriter(&output)
	var count [binary.MaxVarintLen64]byte
	length := binary.PutUvarint(count[:], 1)
	_, _ = writer.Write(count[:length])
	_, _ = writer.Write([]byte{0})
	if err := writer.Close(); err != nil {
		t.Fatalf("close SRS fixture: %v", err)
	}
	return output.Bytes()
}

func validProbeMRS(t *testing.T, behavior byte) []byte {
	t.Helper()
	decoded := make([]byte, 22)
	copy(decoded[:4], []byte{'M', 'R', 'S', 1})
	decoded[4] = behavior
	binary.BigEndian.PutUint64(decoded[5:13], 1)
	binary.BigEndian.PutUint64(decoded[13:21], 0)
	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatalf("create MRS encoder: %v", err)
	}
	defer encoder.Close()
	return encoder.EncodeAll(decoded, nil)
}
