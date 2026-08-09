package service

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"reflect"
	"strings"
	"testing"
	"time"

	dnsproxy "github.com/AdguardTeam/dnsproxy/proxy"
	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/miekg/dns"
)

type reverseProxyTestRoundTripper func(*http.Request) (*http.Response, error)

func (f reverseProxyTestRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type reverseProxyTestDNSResolver func(context.Context, string, string) ([]netip.Addr, error)

func (f reverseProxyTestDNSResolver) LookupNetIP(ctx context.Context, network string, host string) ([]netip.Addr, error) {
	return f(ctx, network, host)
}

func TestNormalizeReverseProxyPayloadAlwaysUsesStrictDomainConditions(t *testing.T) {
	service := &ReverseProxyService{}
	base := ReverseProxyRulePayload{
		Name:            "strict-default",
		Enabled:         true,
		ListenProtocol:  reverseProxyProtocolHTTP,
		ListenPort:      18080,
		Hosts:           "example.com",
		TargetProtocol:  reverseProxyProtocolHTTP,
		TargetAddresses: "127.0.0.1",
		TargetPort:      18081,
		IPStrategy:      reverseProxyIPStrategyPreferIPv4,
	}

	normalized, err := service.normalizeRulePayload(base)
	if err != nil {
		t.Fatalf("normalize new domain rule failed: %v", err)
	}
	if len(normalized.hosts) != 1 || normalized.hosts[0] != "example.com" {
		t.Fatalf("new domain rule lost its strict domain condition: %#v", normalized.hosts)
	}

	base.ID = 99
	normalized, err = service.normalizeRulePayload(base)
	if err != nil {
		t.Fatalf("normalize historical domain rule failed: %v", err)
	}
	if len(normalized.hosts) != 1 || normalized.hosts[0] != "example.com" {
		t.Fatalf("historical rule must use the same strict domain condition: %#v", normalized.hosts)
	}
}

func TestReverseProxyJSONPayloadDefaultsTLSVerificationWhenOmitted(t *testing.T) {
	decode := func(raw string) reverseProxyNormalizedRule {
		t.Helper()
		var payload ReverseProxyRulePayload
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			t.Fatalf("decode reverse proxy payload failed: %v", err)
		}
		normalized, err := (&ReverseProxyService{}).normalizeRulePayload(payload)
		if err != nil {
			t.Fatalf("normalize reverse proxy payload failed: %v", err)
		}
		return normalized
	}

	omitted := decode(`{"name":"tls-default","enabled":true,"listenProtocol":"http","listenPort":18080,"targetProtocol":"https","targetAddresses":"upstream.example","targetPort":443,"ipStrategy":"prefer_ipv4"}`)
	if !omitted.upstreamTLSVerify {
		t.Fatal("omitted upstreamTlsVerify must default to TLS verification")
	}

	explicitlyDisabled := decode(`{"name":"tls-disabled","enabled":true,"listenProtocol":"http","listenPort":18080,"targetProtocol":"https","targetAddresses":"upstream.example","targetPort":443,"ipStrategy":"prefer_ipv4","upstreamTlsVerify":false}`)
	if explicitlyDisabled.upstreamTLSVerify {
		t.Fatal("explicit upstreamTlsVerify=false must remain disabled")
	}
}

func TestReverseProxyNormalizePayloadStripsHTTPPrefixFromHostAndTargetInputs(t *testing.T) {
	normalized, err := (&ReverseProxyService{}).normalizeRulePayload(ReverseProxyRulePayload{
		Name:            "strip-http-prefix",
		Enabled:         true,
		ListenProtocol:  reverseProxyProtocolHTTP,
		ListenPort:      18080,
		Hosts:           " https://Example.com , http://*.Example.com ",
		TargetProtocol:  reverseProxyProtocolHTTP,
		TargetAddresses: " https://UPSTREAM.example , http://[2001:db8::1] ",
		TargetPort:      18081,
		IPStrategy:      reverseProxyIPStrategyPreferIPv4,
	})
	if err != nil {
		t.Fatalf("normalize reverse proxy payload failed: %v", err)
	}
	if !reflect.DeepEqual(normalized.hosts, []string{"example.com", "*.example.com"}) {
		t.Fatalf("normalized hosts mismatch: %#v", normalized.hosts)
	}
	if !reflect.DeepEqual(normalized.targetAddresses, []string{"upstream.example", "2001:db8::1"}) {
		t.Fatalf("normalized target addresses mismatch: %#v", normalized.targetAddresses)
	}
}

func TestReverseProxyDNSAdmissionEnforcesACLRateAndConcurrency(t *testing.T) {
	makeContext := func(value string) *dnsproxy.DNSContext {
		message := new(dns.Msg)
		message.SetQuestion("example.com.", dns.TypeA)
		return &dnsproxy.DNSContext{Req: message, Addr: netip.MustParseAddrPort(value)}
	}

	aclAdmission, err := buildReverseProxyDNSAdmission(&model.ReverseProxyRule{

		DNSAllowedCIDRs:         `["198.51.100.0/24"]`,
		DNSRateLimitQPS:         100,
		DNSMaxConcurrentQueries: 2,
	})
	if err != nil {
		t.Fatalf("build dns acl admission failed: %v", err)
	}
	release, rejected := aclAdmission.acquire(makeContext("198.51.100.8:53000"))
	if rejected != "" || release == nil {
		t.Fatalf("allowed dns client was rejected: %q", rejected)
	}
	release()
	if release, rejected = aclAdmission.acquire(makeContext("203.0.113.8:53000")); release != nil || rejected != "dns_acl_denied" {
		t.Fatalf("unexpected dns acl result: release=%v rejected=%q", release != nil, rejected)
	}

	rateAdmission, err := buildReverseProxyDNSAdmission(&model.ReverseProxyRule{
		DNSAllowedCIDRs:         `["127.0.0.0/8"]`,
		DNSRateLimitQPS:         1,
		DNSMaxConcurrentQueries: 2,
	})
	if err != nil {
		t.Fatalf("build dns rate admission failed: %v", err)
	}
	release, rejected = rateAdmission.acquire(makeContext("127.0.0.1:53001"))
	if rejected != "" || release == nil {
		t.Fatalf("first dns request was rejected: %q", rejected)
	}
	release()
	if release, rejected = rateAdmission.acquire(makeContext("127.0.0.1:53001")); release != nil || rejected != "dns_rate_limited" {
		t.Fatalf("unexpected dns rate result: release=%v rejected=%q", release != nil, rejected)
	}

	concurrencyAdmission, err := buildReverseProxyDNSAdmission(&model.ReverseProxyRule{
		DNSAllowedCIDRs:         `["127.0.0.0/8"]`,
		DNSRateLimitQPS:         100,
		DNSMaxConcurrentQueries: 1,
	})
	if err != nil {
		t.Fatalf("build dns concurrency admission failed: %v", err)
	}
	release, rejected = concurrencyAdmission.acquire(makeContext("127.0.0.1:53002"))
	if rejected != "" || release == nil {
		t.Fatalf("first dns concurrent request was rejected: %q", rejected)
	}
	if nextRelease, nextRejected := concurrencyAdmission.acquire(makeContext("127.0.0.2:53003")); nextRelease != nil || nextRejected != "dns_concurrency_limited" {
		t.Fatalf("unexpected dns concurrency result: release=%v rejected=%q", nextRelease != nil, nextRejected)
	}
	release()
}

func TestReverseProxyDNSBootstrapRejectsResolvedListenerLoop(t *testing.T) {
	resolver := reverseProxyDNSIPStrategyResolver{
		base: reverseProxyTestDNSResolver(func(context.Context, string, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
		}),
		loopGuard: func(address netip.Addr) bool {
			return address == netip.MustParseAddr("127.0.0.1")
		},
	}
	if _, err := resolver.LookupNetIP(context.Background(), "ip", "upstream.example"); err == nil || !strings.Contains(err.Error(), "points back") {
		t.Fatalf("expected resolved dns loop to be rejected, got %v", err)
	}
}

func TestValidateReverseProxyResolvedLoopRejectsWildcardDomainTarget(t *testing.T) {
	err := (&ReverseProxyService{}).validateReverseProxyResolvedLoop(reverseProxyNormalizedRule{
		listenPort:      19443,
		targetAddresses: []string{"localhost"},
		targetPort:      19443,
		ipStrategy:      reverseProxyIPStrategyPreferIPv4,
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "resolves back") {
		t.Fatalf("expected wildcard listener loop to be rejected, got %v", err)
	}
}

func TestReverseProxyRuntimeRequestStateDoesNotPersistToSQLite(t *testing.T) {
	openReverseProxyTestDB(t)
	reverseProxyRuntime.resetRuleStates()
	t.Cleanup(reverseProxyRuntime.resetRuleStates)

	row := model.ReverseProxyRule{Name: "runtime-memory", RuntimeStatus: "pending", LastError: ""}
	if err := database.GetDB().Create(&row).Error; err != nil {
		t.Fatalf("create reverse proxy row failed: %v", err)
	}
	reverseProxyRuntime.reportRuleState(row.Id, "proxy_error", "upstream timeout")

	var stored model.ReverseProxyRule
	if err := database.GetDB().Where("id = ?", row.Id).First(&stored).Error; err != nil {
		t.Fatalf("reload reverse proxy row failed: %v", err)
	}
	if stored.RuntimeStatus != "pending" || stored.LastError != "" {
		t.Fatalf("request runtime state must not write sqlite, got status=%q error=%q", stored.RuntimeStatus, stored.LastError)
	}
	state := reverseProxyRuntime.snapshotRuleStates()[row.Id]
	if state.status != "proxy_error" || state.lastError != "upstream timeout" {
		t.Fatalf("runtime state was not retained in memory: %#v", state)
	}
}

func TestCertificateRecordListProjectionExcludesMaterialBLOBs(t *testing.T) {
	columns := certificateRecordListProjectionColumns()
	set := make(map[string]struct{}, len(columns))
	for _, column := range columns {
		set[column] = struct{}{}
	}
	for _, materialColumn := range []string{"cert_pem", "key_pem", "fullchain_pem", "chain_pem"} {
		if _, exists := set[materialColumn]; exists {
			t.Fatalf("certificate list projection must not select %s", materialColumn)
		}
	}
	for _, metadataColumn := range []string{"issued_key_algorithm", "issued_signature_algorithm"} {
		if _, exists := set[metadataColumn]; !exists {
			t.Fatalf("certificate list projection must select %s", metadataColumn)
		}
	}
}

func TestReverseProxyResponseHeaderTimeoutKeepsStreamingBodyAlive(t *testing.T) {
	reader, writer := io.Pipe()
	transport := reverseProxyResponseHeaderTimeoutTransport{
		timeout: 20 * time.Millisecond,
		base: reverseProxyTestRoundTripper(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: reader}, nil
		}),
	}
	request := httptest.NewRequest(http.MethodGet, "http://listener.example/stream", nil)
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatalf("stream response headers should succeed: %v", err)
	}
	defer response.Body.Close()

	time.Sleep(60 * time.Millisecond)
	go func() {
		_, _ = writer.Write([]byte("data: still-open\n\n"))
		_ = writer.Close()
	}()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("stream body must remain readable after header timeout window: %v", err)
	}
	if string(body) != "data: still-open\n\n" {
		t.Fatalf("unexpected stream body: %q", string(body))
	}
}

func TestReverseProxyListenerErrorRecoversAfterExternalPortRelease(t *testing.T) {
	openReverseProxyTestDB(t)
	svc := &ReverseProxyService{}
	t.Cleanup(func() {
		_ = svc.StopRuntime()
	})

	blocker, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve external listener port failed: %v", err)
	}
	listenPort := blocker.Addr().(*net.TCPAddr).Port

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()
	upstreamHost, upstreamPort := splitReverseProxyTestServerAddress(t, upstream.URL)
	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:           "recover-external-port",
		Enabled:        true,
		ListenProtocol: reverseProxyProtocolHTTP,

		ListenPort:      listenPort,
		TargetProtocol:  reverseProxyProtocolHTTP,
		TargetAddresses: upstreamHost,
		TargetPort:      upstreamPort,
		IPStrategy:      reverseProxyIPStrategyPreferIPv4,
	}); err != nil {
		t.Fatalf("save rule with occupied port should preserve configuration: %v", err)
	}

	var row model.ReverseProxyRule
	if err := database.GetDB().Where("name = ?", "recover-external-port").First(&row).Error; err != nil {
		t.Fatalf("load saved occupied-port rule failed: %v", err)
	}
	if state := reverseProxyRuntime.snapshotRuleStates()[row.Id]; state.status != "listener_error" {
		t.Fatalf("occupied listener must be reported as listener_error, got %#v", state)
	}

	if err := blocker.Close(); err != nil {
		t.Fatalf("release external listener port failed: %v", err)
	}
	if err := svc.SyncIfNeeded(0); err != nil {
		t.Fatalf("retry after external port release failed: %v", err)
	}
	if state := reverseProxyRuntime.snapshotRuleStates()[row.Id]; state.status != "running" || state.lastError != "" {
		t.Fatalf("released listener must recover automatically, got %#v", state)
	}
}
