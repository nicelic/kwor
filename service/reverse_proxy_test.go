package service

import (
	"bufio"
	"bytes"
	"container/list"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	dnsproxy "github.com/AdguardTeam/dnsproxy/proxy"
	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/network"
	"github.com/coder/websocket"
	"github.com/miekg/dns"
	"github.com/quic-go/quic-go/http3"
	"golang.org/x/net/http2"
	"gorm.io/gorm"
)

func TestNormalizeReverseProxyTokens_IPsAndHosts(t *testing.T) {
	ips, err := normalizeReverseProxyTokens(" 1.1.1.1, example.com,  ,\n::1 ", reverseProxyTokenModeServerName)
	if err != nil {
		t.Fatalf("normalize sni names failed: %v", err)
	}
	if len(ips) != 3 || ips[0] != "1.1.1.1" || ips[1] != "example.com" || ips[2] != "::1" {
		t.Fatalf("unexpected sni names: %#v", ips)
	}

	hosts, err := normalizeReverseProxyTokens(" example.com, *.example.com,  api.example.com ", reverseProxyTokenModeHost)
	if err != nil {
		t.Fatalf("normalize hosts failed: %v", err)
	}
	if len(hosts) != 3 {
		t.Fatalf("unexpected hosts: %#v", hosts)
	}

	if _, err := normalizeReverseProxyTokens("*a.example.com", reverseProxyTokenModeHost); err == nil {
		t.Fatal("expected invalid wildcard host to fail")
	}
}

func TestReverseProxyHTTP3WebSocketRequestDetection(t *testing.T) {
	tests := []struct {
		name string
		req  *http.Request
		want bool
	}{
		{
			name: "http3 websocket extended connect",
			req: &http.Request{
				Method:     http.MethodConnect,
				Proto:      "websocket",
				ProtoMajor: 3,
			},
			want: true,
		},
		{
			name: "http3 ordinary connect",
			req: &http.Request{
				Method:     http.MethodConnect,
				Proto:      "connect-udp",
				ProtoMajor: 3,
			},
			want: false,
		},
		{
			name: "http2 websocket extended connect",
			req: &http.Request{
				Method:     http.MethodConnect,
				Proto:      "websocket",
				ProtoMajor: 2,
			},
			want: false,
		},
		{
			name: "http3 websocket get",
			req: &http.Request{
				Method:     http.MethodGet,
				Proto:      "websocket",
				ProtoMajor: 3,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := reverseProxyIsHTTP3WebSocketRequest(tt.req); got != tt.want {
				t.Fatalf("reverseProxyIsHTTP3WebSocketRequest() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestReverseProxyWSSPayloadDisablesHTTP3Advertisement(t *testing.T) {
	normalized, err := (&ReverseProxyService{}).normalizeRulePayload(ReverseProxyRulePayload{
		Name:           "wss-no-http3-advertisement",
		Enabled:        true,
		ListenProtocol: "wss",

		ListenPort:          443,
		TargetProtocol:      reverseProxyProtocolHTTP,
		TargetAddresses:     "127.0.0.1",
		TargetPort:          8080,
		CertificateRecordID: 1,
		AdvertiseHTTP3:      true,
		IPStrategy:          reverseProxyIPStrategyPreferIPv4,
	})
	if err != nil {
		t.Fatalf("normalize wss rule failed: %v", err)
	}
	if normalized.advertiseHTTP3 {
		t.Fatal("wss rule must disable http3 advertisement")
	}
}

func TestReverseProxyResponseRewriteBodyRespectsStreamingAndSizeBounds(t *testing.T) {
	const upstreamOrigin = "https://upstream.example"
	const externalOrigin = "https://panel.example"
	plan := reverseProxyResponseRewritePlan{
		Enabled: true,
		Replacements: []reverseProxyStringReplacement{{
			Old: upstreamOrigin,
			New: externalOrigin,
		}},
	}

	tests := []struct {
		name            string
		contentType     string
		contentEncoding string
		contentLength   int64
		body            string
		want            string
	}{
		{
			name:          "rewrites bounded text response",
			contentType:   "text/html; charset=utf-8",
			contentLength: int64(len(upstreamOrigin)),
			body:          upstreamOrigin,
			want:          externalOrigin,
		},
		{
			name:          "skips unknown length response",
			contentType:   "text/plain",
			contentLength: -1,
			body:          upstreamOrigin,
			want:          upstreamOrigin,
		},
		{
			name:          "skips event stream",
			contentType:   "text/event-stream",
			contentLength: -1,
			body:          upstreamOrigin,
			want:          upstreamOrigin,
		},
		{
			name:            "skips compressed response",
			contentType:     "text/html",
			contentEncoding: "gzip",
			contentLength:   int64(len(upstreamOrigin)),
			body:            upstreamOrigin,
			want:            upstreamOrigin,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{
				Header:        make(http.Header),
				Body:          io.NopCloser(strings.NewReader(tt.body)),
				ContentLength: tt.contentLength,
			}
			resp.Header.Set("Content-Type", tt.contentType)
			if tt.contentEncoding != "" {
				resp.Header.Set("Content-Encoding", tt.contentEncoding)
			}

			if err := reverseProxyRewriteResponseBody(resp, plan); err != nil {
				t.Fatalf("rewrite response body failed: %v", err)
			}
			got, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read rewritten response body failed: %v", err)
			}
			if err := resp.Body.Close(); err != nil {
				t.Fatalf("close rewritten response body failed: %v", err)
			}
			if string(got) != tt.want {
				t.Fatalf("unexpected response body: got %q, want %q", string(got), tt.want)
			}
		})
	}

	oversized := strings.Repeat("x", int(reverseProxyResources.current().ResponseRewriteInputBytes)+1)
	resp := &http.Response{
		Header:        make(http.Header),
		Body:          io.NopCloser(strings.NewReader(oversized)),
		ContentLength: 0,
	}
	resp.Header.Set("Content-Type", "text/plain")
	resp.Header.Set("Content-Length", "0")
	if err := reverseProxyRewriteResponseBody(resp, plan); err != nil {
		t.Fatalf("rewrite malformed oversized response failed: %v", err)
	}
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read preserved oversized response failed: %v", err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatalf("close preserved oversized response failed: %v", err)
	}
	if string(got) != oversized {
		t.Fatal("oversized response body was not preserved")
	}
	if resp.ContentLength != -1 || resp.Header.Get("Content-Length") != "" {
		t.Fatal("oversized response should be forwarded without a declared content length")
	}
}

func TestNormalizeReverseProxyTokens_RejectInlinePorts(t *testing.T) {
	if _, err := normalizeReverseProxyTokens("example.com:8443", reverseProxyTokenModeHost); err == nil {
		t.Fatal("expected host token with inline port to fail")
	}
	if _, err := normalizeReverseProxyTokens("api.example.com:8443", reverseProxyTokenModeTarget); err == nil {
		t.Fatal("expected target token with inline port to fail")
	}
}

func TestNormalizeReverseProxyProtocol_AcceptsWSAliases(t *testing.T) {
	gotWS, err := normalizeReverseProxyProtocol("ws")
	if err != nil {
		t.Fatalf("normalize ws protocol failed: %v", err)
	}
	if gotWS != reverseProxyProtocolHTTP {
		t.Fatalf("expected ws to map to http, got %q", gotWS)
	}

	gotWSS, err := normalizeReverseProxyProtocol("wss")
	if err != nil {
		t.Fatalf("normalize wss protocol failed: %v", err)
	}
	if gotWSS != reverseProxyProtocolHTTPS {
		t.Fatalf("expected wss to map to https, got %q", gotWSS)
	}

	gotDNS, err := normalizeReverseProxyProtocol(reverseProxyDNSProtocolDoH)
	if err != nil {
		t.Fatalf("normalize dns doh protocol failed: %v", err)
	}
	if gotDNS != reverseProxyProtocolDNS {
		t.Fatalf("expected dns_doh to map to dns, got %q", gotDNS)
	}
}

func TestReverseProxySplitSNICertificateCandidates_PrefersExactBeforeWildcard(t *testing.T) {
	wildcardCertPEM, wildcardKeyPEM := buildReverseProxyTestCertificatePEM(t, []string{"*.example.com"})
	wildcardCert, wildcardLeaf, err := loadReverseProxyBindingForTest(wildcardCertPEM, wildcardKeyPEM)
	if err != nil {
		t.Fatalf("load wildcard certificate failed: %v", err)
	}
	exactCertPEM, exactKeyPEM := buildReverseProxyTestCertificatePEM(t, []string{"api.example.com"})
	exactCert, exactLeaf, err := loadReverseProxyBindingForTest(exactCertPEM, exactKeyPEM)
	if err != nil {
		t.Fatalf("load exact certificate failed: %v", err)
	}
	wildcard := &reverseProxyRuleCertificateBinding{CertificateRecordID: 1, Certificate: wildcardCert, Leaf: wildcardLeaf}
	exact := &reverseProxyRuleCertificateBinding{CertificateRecordID: 2, Certificate: exactCert, Leaf: exactLeaf}

	exactMatched, wildcardMatched := reverseProxySplitSNICertificateCandidates([]*reverseProxyRuleCertificateBinding{wildcard, exact}, "api.example.com")
	if len(exactMatched) != 1 || exactMatched[0] != exact {
		t.Fatalf("expected exact cert in exact bucket, got %#v", exactMatched)
	}
	if len(wildcardMatched) != 1 || wildcardMatched[0] != wildcard {
		t.Fatalf("expected wildcard cert in wildcard bucket, got %#v", wildcardMatched)
	}
}

func TestBuildReverseProxyDNSServerTLSConfigAllowsEmptySNIWithExactIPCertificate(t *testing.T) {
	openReverseProxyTestDB(t)

	domainCertID := createReverseProxyTestCertificateRecord(t, "example.com")
	ipCertID := createReverseProxyTestCertificateRecord(t, "127.0.0.1")

	svc := &ReverseProxyService{}
	config, err := buildReverseProxyDNSServerTLSConfig(svc, []model.ReverseProxyRule{
		{
			Id:                    1,
			Enabled:               true,
			ListenProtocol:        reverseProxyProtocolDNS,
			ListenProtocolAlias:   reverseProxyDNSProtocolDoH,
			ListenPort:            443,
			CertificateRecordList: encodeReverseProxyUintList([]uint{domainCertID, ipCertID}),
			CertificateRecordID:   domainCertID,
		},
	}, []string{"h2", "http/1.1"})
	if err != nil {
		t.Fatalf("build dns tls config failed: %v", err)
	}

	got, err := config.GetCertificate(&tls.ClientHelloInfo{
		Conn: reverseProxyTestConn{
			local: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 443},
		},
	})
	if err != nil || got == nil {
		t.Fatalf("empty SNI must select the exact local IP certificate, certificate=%v err=%v", got != nil, err)
	}
}

func TestNormalizeReverseProxyPayloadAllowsEmptyListenMatch(t *testing.T) {
	svc := &ReverseProxyService{}
	normalized, err := svc.normalizeRulePayload(ReverseProxyRulePayload{
		Enabled:         true,
		ListenProtocol:  reverseProxyProtocolHTTP,
		ListenPort:      18080,
		TargetProtocol:  reverseProxyProtocolHTTP,
		TargetAddresses: "127.0.0.1",
		TargetPort:      80,
		IPStrategy:      reverseProxyIPStrategyPreferIPv4,
	})
	if err != nil {
		t.Fatalf("empty listen match should be accepted: %v", err)
	}
	if len(normalized.hosts) != 0 {
		t.Fatalf("expected empty domain condition, got hosts=%#v", normalized.hosts)
	}
}

func TestReverseProxyPayloadPreservesAPIPassthroughForWSAliases(t *testing.T) {
	svc := &ReverseProxyService{}
	cases := []struct {
		name     string
		listen   string
		target   string
		expected bool
	}{
		{
			name:     "ws_with_passthrough_true",
			listen:   "ws",
			target:   "ws",
			expected: true,
		},
		{
			name:     "wss_with_passthrough_false",
			listen:   "wss",
			target:   "wss",
			expected: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			certificateRecordID := uint(0)
			if tc.listen == "wss" {
				certificateRecordID = 1
			}
			normalized, err := svc.normalizeRulePayload(ReverseProxyRulePayload{
				Name:                tc.name,
				Enabled:             true,
				ListenProtocol:      tc.listen,
				ListenPort:          18080,
				TargetProtocol:      tc.target,
				TargetAddresses:     "127.0.0.1",
				TargetPort:          18081,
				CertificateRecordID: certificateRecordID,
				IPStrategy:          reverseProxyIPStrategyPreferIPv4,
				ApiPassthrough:      tc.expected,
			})
			if err != nil {
				t.Fatalf("normalize rule payload failed: %v", err)
			}
			if normalized.apiPassthrough != tc.expected {
				t.Fatalf("expected apiPassthrough=%v got %v", tc.expected, normalized.apiPassthrough)
			}
		})
	}
}

func TestNormalizeReverseProxyPayloadSupportsDNSProtocols(t *testing.T) {
	svc := &ReverseProxyService{}
	certIDs := []uint{3, 5}
	normalized, err := svc.normalizeRulePayload(ReverseProxyRulePayload{
		Enabled:                true,
		ListenProtocol:         reverseProxyDNSProtocolDoH,
		ListenPort:             4443,
		ListenDNSPath:          "/custom-dns",
		TargetProtocol:         reverseProxyDNSProtocolDoQ,
		TargetAddresses:        "1.1.1.1, 8.8.8.8",
		TargetPort:             853,
		EDNSEnabled:            true,
		EDNSMode:               reverseProxyEDNSModeCustom,
		EDNSCustomIP:           "14.119.184.123",
		EDNSClientSubnetPolicy: reverseProxyEDNSClientSubnetPolicyPreferRequestPublic,
		DisableIPv4Answer:      true,
		DisableIPv6Answer:      false,
		IPStrategy:             reverseProxyIPStrategyPreferIPv4,
		UpstreamTLSVerify:      true,
		CertificateRecordIDs:   certIDs,
	})
	if err != nil {
		t.Fatalf("normalize dns payload failed: %v", err)
	}
	if normalized.listenProtocol != reverseProxyProtocolDNS {
		t.Fatalf("expected listen protocol dns, got %q", normalized.listenProtocol)
	}
	if normalized.listenProtocolAlias != reverseProxyDNSProtocolDoH {
		t.Fatalf("expected listen alias dns_doh, got %q", normalized.listenProtocolAlias)
	}
	if normalized.targetProtocolAlias != reverseProxyDNSProtocolDoQ {
		t.Fatalf("expected target alias dns_doq, got %q", normalized.targetProtocolAlias)
	}
	if normalized.listenDNSPath != "/custom-dns" {
		t.Fatalf("expected custom doh path, got %q", normalized.listenDNSPath)
	}
	if normalized.targetDNSPath != "" {
		t.Fatalf("expected empty doq target path, got %q", normalized.targetDNSPath)
	}
	if normalized.ipStrategy != reverseProxyIPStrategyPreferIPv4 {
		t.Fatalf("expected dns ip strategy to be preserved, got %q", normalized.ipStrategy)
	}
	if !normalized.ednsEnabled || normalized.ednsMode != reverseProxyEDNSModeCustom || normalized.ednsCustomIP != "14.119.184.1" {
		t.Fatalf("expected edns settings to be preserved, got enabled=%v mode=%q ip=%q", normalized.ednsEnabled, normalized.ednsMode, normalized.ednsCustomIP)
	}
	if normalized.ednsClientSubnetPolicy != reverseProxyEDNSClientSubnetPolicyPreferRequestPublic {
		t.Fatalf("expected edns client subnet policy to be preserved, got %q", normalized.ednsClientSubnetPolicy)
	}
	if !normalized.disableIPv4Answer || normalized.disableIPv6Answer {
		t.Fatalf("expected ipv4 disable only, got v4=%v v6=%v", normalized.disableIPv4Answer, normalized.disableIPv6Answer)
	}
	if len(normalized.certificateRecordIDs) != len(certIDs) || normalized.certificateRecordID != certIDs[0] {
		t.Fatalf("expected dns tls listener certificates to be preserved, got ids=%v first=%d", normalized.certificateRecordIDs, normalized.certificateRecordID)
	}
}

func TestNormalizeReverseProxyPayloadRejectsInvalidEDNSCustomIP(t *testing.T) {
	svc := &ReverseProxyService{}
	_, err := svc.normalizeRulePayload(ReverseProxyRulePayload{
		Enabled:         true,
		ListenProtocol:  reverseProxyDNSProtocolUDP,
		ListenPort:      5353,
		TargetProtocol:  reverseProxyDNSProtocolTCP,
		TargetAddresses: "1.1.1.1",
		TargetPort:      53,
		EDNSEnabled:     true,
		EDNSMode:        reverseProxyEDNSModeCustom,
		EDNSCustomIP:    "not-an-ip",
		IPStrategy:      reverseProxyIPStrategyPreferIPv4,
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "edns custom ip") {
		t.Fatalf("expected invalid edns custom ip error, got %v", err)
	}
}

func TestNormalizeReverseProxyPayloadRejectsIPv6EDNSCustomIP(t *testing.T) {
	svc := &ReverseProxyService{}
	_, err := svc.normalizeRulePayload(ReverseProxyRulePayload{
		Enabled:         true,
		ListenProtocol:  reverseProxyDNSProtocolUDP,
		ListenPort:      5353,
		TargetProtocol:  reverseProxyDNSProtocolTCP,
		TargetAddresses: "1.1.1.1",
		TargetPort:      53,
		EDNSEnabled:     true,
		EDNSMode:        reverseProxyEDNSModeCustom,
		EDNSCustomIP:    "2408:1234::abcd",
		IPStrategy:      reverseProxyIPStrategyPreferIPv4,
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "only ipv4") {
		t.Fatalf("expected ipv6 edns custom ip to be rejected, got %v", err)
	}
}

func TestNormalizeReverseProxyPayloadMasksCustomEDNSIPv4ToDotOne(t *testing.T) {
	svc := &ReverseProxyService{}
	normalized, err := svc.normalizeRulePayload(ReverseProxyRulePayload{
		Enabled:         true,
		ListenProtocol:  reverseProxyDNSProtocolUDP,
		ListenPort:      5353,
		TargetProtocol:  reverseProxyDNSProtocolTCP,
		TargetAddresses: "1.1.1.1",
		TargetPort:      53,
		EDNSEnabled:     true,
		EDNSMode:        reverseProxyEDNSModeCustom,
		EDNSCustomIP:    "14.119.184.123",
		IPStrategy:      reverseProxyIPStrategyPreferIPv4,
	})
	if err != nil {
		t.Fatalf("normalize dns payload failed: %v", err)
	}
	if normalized.ednsCustomIP != "14.119.184.1" {
		t.Fatalf("expected custom edns ipv4 to be normalized to dot one, got %q", normalized.ednsCustomIP)
	}
}

func TestNormalizeReverseProxyPayloadIgnoresInvalidEDNSCustomIPForNonDNSRules(t *testing.T) {
	svc := &ReverseProxyService{}
	normalized, err := svc.normalizeRulePayload(ReverseProxyRulePayload{
		Enabled:         true,
		ListenProtocol:  reverseProxyProtocolHTTP,
		ListenPort:      8080,
		TargetProtocol:  reverseProxyProtocolHTTP,
		TargetAddresses: "example.com",
		TargetPort:      80,
		EDNSEnabled:     true,
		EDNSMode:        reverseProxyEDNSModeCustom,
		EDNSCustomIP:    "not-an-ip",
		IPStrategy:      reverseProxyIPStrategyPreferIPv4,
	})
	if err != nil {
		t.Fatalf("expected non-dns payload to ignore edns custom ip, got %v", err)
	}
	if normalized.ednsEnabled || normalized.ednsCustomIP != "" {
		t.Fatalf("expected non-dns payload to clear edns fields, got enabled=%v ip=%q", normalized.ednsEnabled, normalized.ednsCustomIP)
	}
}

func TestNormalizeReverseProxyPayloadClearsDNSOnlyFieldsForNonDNSRules(t *testing.T) {
	svc := &ReverseProxyService{}
	normalized, err := svc.normalizeRulePayload(ReverseProxyRulePayload{
		Enabled:                true,
		ListenProtocol:         reverseProxyProtocolHTTP,
		ListenPort:             8080,
		Hosts:                  "example.com",
		PathPrefix:             "/app",
		ListenDNSPath:          "/stale-listen-dns",
		TargetProtocol:         reverseProxyProtocolHTTPS,
		TargetAddresses:        "backend.local",
		TargetPort:             443,
		TargetPath:             "/api",
		TargetDNSPath:          "/stale-target-dns",
		EDNSEnabled:            true,
		EDNSMode:               reverseProxyEDNSModeCustom,
		EDNSCustomIP:           "14.119.184.1",
		EDNSClientSubnetPolicy: reverseProxyEDNSClientSubnetPolicyPreferRequestPublic,
		DisableIPv4Answer:      true,
		DisableIPv6Answer:      true,
		IPStrategy:             reverseProxyIPStrategyPreferIPv4,
		HTTPVersionStrategy:    reverseProxyHTTPVersionPreferH2,
		UpstreamTLSVerify:      true,
	})
	if err != nil {
		t.Fatalf("normalize non-dns payload failed: %v", err)
	}
	if normalized.listenDNSPath != "" || normalized.targetDNSPath != "" {
		t.Fatalf("expected non-dns payload to clear dns paths, got listen=%q target=%q", normalized.listenDNSPath, normalized.targetDNSPath)
	}
	if normalized.ednsEnabled || normalized.disableIPv4Answer || normalized.disableIPv6Answer {
		t.Fatalf("expected non-dns payload to clear dns-only flags, got edns=%v v4=%v v6=%v", normalized.ednsEnabled, normalized.disableIPv4Answer, normalized.disableIPv6Answer)
	}
}

func TestNormalizeReverseProxyPayloadClearsHTTPOnlyFieldsForDNSRules(t *testing.T) {
	svc := &ReverseProxyService{}
	normalized, err := svc.normalizeRulePayload(ReverseProxyRulePayload{
		Enabled:                true,
		ListenProtocol:         reverseProxyDNSProtocolDoH,
		ListenPort:             443,
		Hosts:                  "stale.example.com",
		PathPrefix:             "/stale-http-prefix",
		ListenDNSPath:          "/dns-query",
		TargetProtocol:         reverseProxyDNSProtocolTCP,
		TargetAddresses:        "1.1.1.1",
		TargetPort:             53,
		TargetPath:             "/stale-http-target",
		EDNSEnabled:            true,
		EDNSMode:               reverseProxyEDNSModeAuto,
		EDNSClientSubnetPolicy: reverseProxyEDNSClientSubnetPolicyClientIP,
		IPStrategy:             reverseProxyIPStrategyPreferIPv4,
		CertificateRecordIDs:   []uint{3},
	})
	if err != nil {
		t.Fatalf("normalize dns payload failed: %v", err)
	}
	if len(normalized.hosts) != 0 || normalized.pathPrefix != "" || normalized.targetPath != "" {
		t.Fatalf("expected dns payload to clear http-only fields, got hosts=%v path=%q targetPath=%q", normalized.hosts, normalized.pathPrefix, normalized.targetPath)
	}
}

func TestReverseProxyDNSResolveAutoEDNSIPPrefersPublicRequestECS(t *testing.T) {
	req := new(dns.Msg)
	req.SetQuestion("example.com.", dns.TypeA)
	reverseProxyDNSSetECS(req, net.ParseIP("117.189.81.36"))

	dctx := &dnsproxy.DNSContext{
		Req:  req,
		Addr: netip.MustParseAddrPort("10.0.0.2:12345"),
	}
	rule := &model.ReverseProxyRule{
		EDNSEnabled:            true,
		EDNSMode:               reverseProxyEDNSModeAuto,
		EDNSClientSubnetPolicy: reverseProxyEDNSClientSubnetPolicyPreferRequestPublic,
	}

	ip, ok := reverseProxyDNSResolveAutoEDNSIP(req, dctx, rule)
	if !ok || ip == nil || !ip.Equal(net.ParseIP("117.189.81.36")) {
		t.Fatalf("expected request public ecs to be preferred, got ok=%v ip=%v", ok, ip)
	}
}

func TestReverseProxyDNSExtractUsableRequestECSPreservesClientPrefix(t *testing.T) {
	req := new(dns.Msg)
	req.SetQuestion("example.com.", dns.TypeA)
	opt := &dns.OPT{
		Hdr: dns.RR_Header{Name: ".", Rrtype: dns.TypeOPT},
		Option: []dns.EDNS0{
			&dns.EDNS0_SUBNET{
				Code:          dns.EDNS0SUBNET,
				Family:        1,
				SourceNetmask: 24,
				SourceScope:   0,
				Address:       net.ParseIP("117.189.81.36"),
			},
		},
	}
	opt.SetUDPSize(4096)
	req.Extra = append(req.Extra, opt)

	subnet, ok := reverseProxyDNSExtractUsableRequestECS(req)
	if !ok || subnet == nil {
		t.Fatal("expected usable request ecs to be extracted")
	}
	if subnet.Family != 1 || subnet.SourceNetmask != 24 {
		t.Fatalf("expected client ecs family/prefix to be preserved, got family=%d prefix=%d", subnet.Family, subnet.SourceNetmask)
	}
	if ip := subnet.Address.To4(); ip == nil || !ip.Equal(net.ParseIP("117.189.81.36").To4()) {
		t.Fatalf("expected client ecs address to be preserved, got %v", subnet.Address)
	}
}

func TestReverseProxyDNSResolveAutoEDNSIPIgnoresPrivateRequestECS(t *testing.T) {
	req := new(dns.Msg)
	req.SetQuestion("example.com.", dns.TypeA)
	reverseProxyDNSSetECS(req, net.ParseIP("192.168.1.25"))

	dctx := &dnsproxy.DNSContext{
		Req:  req,
		Addr: netip.MustParseAddrPort("117.189.81.36:12345"),
	}
	rule := &model.ReverseProxyRule{
		EDNSEnabled:            true,
		EDNSMode:               reverseProxyEDNSModeAuto,
		EDNSClientSubnetPolicy: reverseProxyEDNSClientSubnetPolicyPreferRequestPublic,
	}

	ip, ok := reverseProxyDNSResolveAutoEDNSIP(req, dctx, rule)
	if !ok || ip == nil || !ip.Equal(net.ParseIP("117.189.81.36")) {
		t.Fatalf("expected private request ecs to fall back to client ip, got ok=%v ip=%v", ok, ip)
	}
}

func TestReverseProxyDNSApplyEDNSPolicyPrefersPublicRequestECSPrefixWithoutRewriting(t *testing.T) {
	req := new(dns.Msg)
	req.SetQuestion("example.com.", dns.TypeA)
	opt := &dns.OPT{
		Hdr: dns.RR_Header{Name: ".", Rrtype: dns.TypeOPT},
		Option: []dns.EDNS0{
			&dns.EDNS0_SUBNET{
				Code:          dns.EDNS0SUBNET,
				Family:        1,
				SourceNetmask: 24,
				SourceScope:   0,
				Address:       net.ParseIP("117.189.81.36"),
			},
		},
	}
	opt.SetUDPSize(4096)
	req.Extra = append(req.Extra, opt)
	dctx := &dnsproxy.DNSContext{
		Req:  req,
		Addr: netip.MustParseAddrPort("14.119.184.86:12345"),
	}
	rule := &model.ReverseProxyRule{
		EDNSEnabled:            true,
		EDNSMode:               reverseProxyEDNSModeAuto,
		EDNSClientSubnetPolicy: reverseProxyEDNSClientSubnetPolicyPreferRequestPublic,
	}

	reverseProxyDNSApplyEDNSPolicy(req, dctx, rule)

	subnet, ok := reverseProxyDNSExtractUsableRequestECS(req)
	if !ok || subnet == nil {
		t.Fatal("expected request ecs to remain after applying policy")
	}
	if subnet.SourceNetmask != 24 {
		t.Fatalf("expected request ecs prefix to remain 24, got %d", subnet.SourceNetmask)
	}
	if ip := subnet.Address.To4(); ip == nil || !ip.Equal(net.ParseIP("117.189.81.36").To4()) {
		t.Fatalf("expected request ecs address to remain unchanged, got %v", subnet.Address)
	}
}

func TestReverseProxyDNSApplyEDNSPolicyMasksIPv4ToDotOne(t *testing.T) {
	req := new(dns.Msg)
	req.SetQuestion("example.com.", dns.TypeA)
	dctx := &dnsproxy.DNSContext{
		Req:  req,
		Addr: netip.MustParseAddrPort("14.119.184.86:12345"),
	}
	rule := &model.ReverseProxyRule{
		EDNSEnabled:            true,
		EDNSMode:               reverseProxyEDNSModeAuto,
		EDNSClientSubnetPolicy: reverseProxyEDNSClientSubnetPolicyClientIP,
	}

	reverseProxyDNSApplyEDNSPolicy(req, dctx, rule)

	ip, ok := reverseProxyDNSExtractUsableRequestECSIP(req)
	if !ok || ip == nil || !ip.Equal(net.ParseIP("14.119.184.1")) {
		t.Fatalf("expected auto ipv4 ecs to mask to dot one, got ok=%v ip=%v", ok, ip)
	}
}

func TestReverseProxyDNSApplyEDNSPolicyKeepsIPv6ClientAddress(t *testing.T) {
	req := new(dns.Msg)
	req.SetQuestion("example.com.", dns.TypeAAAA)
	dctx := &dnsproxy.DNSContext{
		Req:  req,
		Addr: netip.MustParseAddrPort("[2408:8888::1234]:12345"),
	}
	rule := &model.ReverseProxyRule{
		EDNSEnabled:            true,
		EDNSMode:               reverseProxyEDNSModeAuto,
		EDNSClientSubnetPolicy: reverseProxyEDNSClientSubnetPolicyClientIP,
	}

	reverseProxyDNSApplyEDNSPolicy(req, dctx, rule)

	ip, ok := reverseProxyDNSExtractUsableRequestECSIP(req)
	if !ok || ip == nil || !ip.Equal(net.ParseIP("2408:8888::1234")) {
		t.Fatalf("expected auto ipv6 ecs to preserve the client ipv6, got ok=%v ip=%v", ok, ip)
	}
}

func TestReverseProxyDNSApplyEDNSPolicyMasksCustomIPv4ToDotOne(t *testing.T) {
	req := new(dns.Msg)
	req.SetQuestion("example.com.", dns.TypeA)

	reverseProxyDNSApplyEDNSPolicy(req, &dnsproxy.DNSContext{Req: req}, &model.ReverseProxyRule{
		EDNSEnabled:  true,
		EDNSMode:     reverseProxyEDNSModeCustom,
		EDNSCustomIP: "14.119.184.123",
	})

	ip, ok := reverseProxyDNSExtractUsableRequestECSIP(req)
	if !ok || ip == nil || !ip.Equal(net.ParseIP("14.119.184.1")) {
		t.Fatalf("expected runtime custom edns ipv4 to be masked to dot one, got ok=%v ip=%v", ok, ip)
	}
}

func TestReverseProxyDNSApplyEDNSPolicyRemovesExistingECSWhenDisabled(t *testing.T) {
	req := new(dns.Msg)
	req.SetQuestion("example.com.", dns.TypeA)
	reverseProxyDNSSetECS(req, net.ParseIP("117.189.81.36"))

	reverseProxyDNSApplyEDNSPolicy(req, nil, &model.ReverseProxyRule{
		EDNSEnabled: false,
	})

	if _, ok := reverseProxyDNSExtractUsableRequestECSIP(req); ok {
		t.Fatal("expected existing ecs to be removed when edns is disabled")
	}
}

func TestReverseProxyDNSApplyEDNSPolicyRemovesExistingECSWhenCustomIPInvalid(t *testing.T) {
	req := new(dns.Msg)
	req.SetQuestion("example.com.", dns.TypeA)
	reverseProxyDNSSetECS(req, net.ParseIP("117.189.81.36"))

	reverseProxyDNSApplyEDNSPolicy(req, &dnsproxy.DNSContext{Req: req}, &model.ReverseProxyRule{
		EDNSEnabled:  true,
		EDNSMode:     reverseProxyEDNSModeCustom,
		EDNSCustomIP: "not-an-ip",
	})

	if _, ok := reverseProxyDNSExtractUsableRequestECSIP(req); ok {
		t.Fatal("expected existing ecs to be removed when custom edns ip is invalid")
	}
}

func TestReverseProxyDNSFilterResponseDropsAddressRecordsAndMatchingRRSIG(t *testing.T) {
	resp := &dns.Msg{
		Answer: []dns.RR{
			&dns.A{Hdr: dns.RR_Header{Name: "example.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60}, A: net.ParseIP("1.1.1.1").To4()},
			&dns.AAAA{Hdr: dns.RR_Header{Name: "example.com.", Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: 120}, AAAA: net.ParseIP("2400:3200::1")},
			&dns.RRSIG{Hdr: dns.RR_Header{Name: "example.com.", Rrtype: dns.TypeRRSIG, Class: dns.ClassINET, Ttl: 120}, TypeCovered: dns.TypeA},
			&dns.RRSIG{Hdr: dns.RR_Header{Name: "example.com.", Rrtype: dns.TypeRRSIG, Class: dns.ClassINET, Ttl: 120}, TypeCovered: dns.TypeAAAA},
		},
	}

	reverseProxyDNSFilterResponse(resp, true, false)

	if len(resp.Answer) != 2 {
		t.Fatalf("expected only aaaa and its rrsig to remain, got %d", len(resp.Answer))
	}
	if _, ok := resp.Answer[0].(*dns.AAAA); !ok {
		t.Fatalf("expected first remaining record to be AAAA, got %T", resp.Answer[0])
	}
	if sig, ok := resp.Answer[1].(*dns.RRSIG); !ok || sig.TypeCovered != dns.TypeAAAA {
		t.Fatalf("expected remaining rrsig to cover AAAA, got %#v", resp.Answer[1])
	}
}

func TestReverseProxyDNSFilterResponseDropsNSECAndItsRRSIGWhenAddressTypeIsBlocked(t *testing.T) {
	resp := &dns.Msg{
		Ns: []dns.RR{
			&dns.NSEC{
				Hdr:        dns.RR_Header{Name: "example.com.", Rrtype: dns.TypeNSEC, Class: dns.ClassINET, Ttl: 60},
				NextDomain: "next.example.com.",
				TypeBitMap: []uint16{dns.TypeA, dns.TypeRRSIG},
			},
			&dns.RRSIG{
				Hdr:         dns.RR_Header{Name: "example.com.", Rrtype: dns.TypeRRSIG, Class: dns.ClassINET, Ttl: 60},
				TypeCovered: dns.TypeNSEC,
			},
		},
	}

	reverseProxyDNSFilterResponse(resp, true, false)

	if len(resp.Ns) != 0 {
		t.Fatalf("expected nsec and matching rrsig to be removed, got %d records", len(resp.Ns))
	}
}

func TestReverseProxyDNSFilterResponseDropsNSEC3AndItsRRSIGWhenAddressTypeIsBlocked(t *testing.T) {
	resp := &dns.Msg{
		Ns: []dns.RR{
			&dns.NSEC3{
				Hdr:        dns.RR_Header{Name: "hash.example.com.", Rrtype: dns.TypeNSEC3, Class: dns.ClassINET, Ttl: 60},
				Hash:       dns.SHA1,
				Flags:      0,
				Iterations: 1,
				SaltLength: 0,
				HashLength: 0,
				NextDomain: "",
				TypeBitMap: []uint16{dns.TypeAAAA, dns.TypeRRSIG},
			},
			&dns.RRSIG{
				Hdr:         dns.RR_Header{Name: "hash.example.com.", Rrtype: dns.TypeRRSIG, Class: dns.ClassINET, Ttl: 60},
				TypeCovered: dns.TypeNSEC3,
			},
		},
	}

	reverseProxyDNSFilterResponse(resp, false, true)

	if len(resp.Ns) != 0 {
		t.Fatalf("expected nsec3 and matching rrsig to be removed, got %d records", len(resp.Ns))
	}
}

func TestReverseProxyDNSFilterResponseDropsCrossSectionRRSIGForBlockedAddressType(t *testing.T) {
	resp := &dns.Msg{
		Answer: []dns.RR{
			&dns.RRSIG{Hdr: dns.RR_Header{Name: "example.com.", Rrtype: dns.TypeRRSIG, Class: dns.ClassINET, Ttl: 60}, TypeCovered: dns.TypeA},
		},
		Extra: []dns.RR{
			&dns.A{Hdr: dns.RR_Header{Name: "example.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60}, A: net.ParseIP("1.1.1.1").To4()},
		},
	}

	reverseProxyDNSFilterResponse(resp, true, false)

	if len(resp.Answer) != 0 || len(resp.Extra) != 0 {
		t.Fatalf("expected cross-section a record and rrsig to be removed, got answer=%d extra=%d", len(resp.Answer), len(resp.Extra))
	}
}

func TestReverseProxyDNSFilterResponseDropsHTTPSHintsForBlockedAddressType(t *testing.T) {
	resp := &dns.Msg{
		Answer: []dns.RR{
			&dns.HTTPS{
				SVCB: dns.SVCB{
					Hdr:      dns.RR_Header{Name: "example.com.", Rrtype: dns.TypeHTTPS, Class: dns.ClassINET, Ttl: 60},
					Priority: 1,
					Target:   ".",
					Value: []dns.SVCBKeyValue{
						&dns.SVCBIPv4Hint{Hint: []net.IP{net.ParseIP("1.1.1.1").To4()}},
						&dns.SVCBIPv6Hint{Hint: []net.IP{net.ParseIP("2400:3200::1")}},
					},
				},
			},
			&dns.RRSIG{
				Hdr:         dns.RR_Header{Name: "example.com.", Rrtype: dns.TypeRRSIG, Class: dns.ClassINET, Ttl: 60},
				TypeCovered: dns.TypeHTTPS,
			},
		},
	}

	reverseProxyDNSFilterResponse(resp, true, false)

	if len(resp.Answer) != 1 {
		t.Fatalf("expected only https rr to remain after trimming ipv4 hint, got %d", len(resp.Answer))
	}
	httpsRR, ok := resp.Answer[0].(*dns.HTTPS)
	if !ok {
		t.Fatalf("expected first remaining record to be HTTPS, got %T", resp.Answer[0])
	}
	if len(httpsRR.Value) != 1 {
		t.Fatalf("expected only one https hint to remain, got %d", len(httpsRR.Value))
	}
	if _, ok := httpsRR.Value[0].(*dns.SVCBIPv6Hint); !ok {
		t.Fatalf("expected remaining https hint to be ipv6, got %T", httpsRR.Value[0])
	}
}

func TestReverseProxyDNSFilterResponseClearsAuthenticatedDataWhenResponseChanges(t *testing.T) {
	resp := &dns.Msg{
		Answer: []dns.RR{
			&dns.A{Hdr: dns.RR_Header{Name: "example.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60}, A: net.ParseIP("1.1.1.1").To4()},
		},
	}
	resp.AuthenticatedData = true

	reverseProxyDNSFilterResponse(resp, true, false)

	if resp.AuthenticatedData {
		t.Fatal("expected authenticated data bit to be cleared after response filtering")
	}
}

func TestReverseProxyDNSFilterResponseKeepsAuthenticatedDataWhenUnchanged(t *testing.T) {
	resp := &dns.Msg{
		Answer: []dns.RR{
			&dns.AAAA{Hdr: dns.RR_Header{Name: "example.com.", Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: 60}, AAAA: net.ParseIP("2400:3200::1")},
		},
	}
	resp.AuthenticatedData = true

	reverseProxyDNSFilterResponse(resp, true, false)

	if !resp.AuthenticatedData {
		t.Fatal("expected authenticated data bit to remain when response is unchanged")
	}
}

func TestNormalizeReverseProxyPayloadSupportsFrontendDNSAliasShape(t *testing.T) {
	svc := &ReverseProxyService{}
	certIDs := []uint{3, 5}
	normalized, err := svc.normalizeRulePayload(ReverseProxyRulePayload{
		Enabled:              true,
		ListenProtocol:       reverseProxyProtocolDNS,
		ListenProtocolAlias:  reverseProxyDNSProtocolDoT,
		ListenPort:           853,
		TargetProtocol:       reverseProxyProtocolDNS,
		TargetProtocolAlias:  reverseProxyDNSProtocolDoH,
		TargetAddresses:      "1.1.1.1",
		TargetPort:           443,
		TargetDNSPath:        "/dns-query",
		IPStrategy:           reverseProxyIPStrategyPreferIPv4,
		UpstreamTLSVerify:    true,
		CertificateRecordIDs: certIDs,
	})
	if err != nil {
		t.Fatalf("normalize frontend dns payload failed: %v", err)
	}
	if normalized.listenProtocol != reverseProxyProtocolDNS || normalized.targetProtocol != reverseProxyProtocolDNS {
		t.Fatalf("expected dns protocols, got listen=%q target=%q", normalized.listenProtocol, normalized.targetProtocol)
	}
	if normalized.listenProtocolAlias != reverseProxyDNSProtocolDoT || normalized.targetProtocolAlias != reverseProxyDNSProtocolDoH {
		t.Fatalf("expected dns aliases to be preserved, got listen=%q target=%q", normalized.listenProtocolAlias, normalized.targetProtocolAlias)
	}
	if !normalized.upstreamTLSVerify {
		t.Fatal("expected dns upstream tls verify to be preserved")
	}
	if len(normalized.certificateRecordIDs) != len(certIDs) || normalized.certificateRecordID != certIDs[0] {
		t.Fatalf("expected dns tls listener certificates to be preserved, got ids=%v first=%d", normalized.certificateRecordIDs, normalized.certificateRecordID)
	}
}

func TestNormalizeReverseProxyPayloadRejectsDNSTLSWithoutCertificate(t *testing.T) {
	svc := &ReverseProxyService{}
	_, err := svc.normalizeRulePayload(ReverseProxyRulePayload{
		Enabled:         true,
		ListenProtocol:  reverseProxyDNSProtocolDoT,
		ListenPort:      853,
		TargetProtocol:  reverseProxyDNSProtocolTCP,
		TargetAddresses: "1.1.1.1",
		TargetPort:      53,
		IPStrategy:      reverseProxyIPStrategyPreferIPv4,
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "certificate") {
		t.Fatalf("expected dns tls listener without certificate to fail, got %v", err)
	}
}

func TestReverseProxyDNSWildcardListenerRequiresRestrictedCIDR(t *testing.T) {
	svc := &ReverseProxyService{}
	base := ReverseProxyRulePayload{
		Enabled: true, ListenProtocol: reverseProxyDNSProtocolUDP, ListenPort: 53,
		TargetProtocol: reverseProxyDNSProtocolUDP, TargetAddresses: "1.1.1.1", TargetPort: 53,
		IPStrategy: reverseProxyIPStrategyPreferIPv4,
	}
	if _, err := svc.normalizeRulePayload(base); err == nil || !strings.Contains(strings.ToLower(err.Error()), "allowed cidr") {
		t.Fatalf("DNS wildcard listener without CIDR must be rejected: %v", err)
	}
	base.DNSAllowedCIDRs = "0.0.0.0/0"
	if _, err := svc.normalizeRulePayload(base); err == nil || !strings.Contains(strings.ToLower(err.Error()), "entire internet") {
		t.Fatalf("global CIDR must be rejected: %v", err)
	}
	legacy := model.ReverseProxyRule{
		Id: 1, ListenProtocol: reverseProxyProtocolDNS, ListenProtocolAlias: reverseProxyDNSProtocolUDP,
		ListenPort: 53, TargetProtocol: reverseProxyProtocolDNS, TargetProtocolAlias: reverseProxyDNSProtocolUDP,
		TargetAddresses: `["1.1.1.1"]`, TargetPort: 53,
	}
	if _, err := buildReverseProxyDNSRuleHandler([]model.ReverseProxyRule{legacy}); err == nil || !strings.Contains(strings.ToLower(err.Error()), "allowed cidr") {
		t.Fatalf("legacy DNS rule without CIDR must remain configured but fail safe at runtime: %v", err)
	}
}

func TestValidateNormalizedDNSRuleRejectsPlainDNSCertificateBinding(t *testing.T) {
	openReverseProxyTestDB(t)

	err := (&ReverseProxyService{}).validateNormalizedDNSRule(database.GetDB(), reverseProxyNormalizedRule{
		listenProtocol:       reverseProxyProtocolDNS,
		listenProtocolAlias:  reverseProxyDNSProtocolUDP,
		listenPort:           5353,
		targetProtocol:       reverseProxyProtocolDNS,
		targetProtocolAlias:  reverseProxyDNSProtocolTCP,
		targetAddresses:      []string{"1.1.1.1"},
		targetPort:           53,
		certificateRecordIDs: []uint{1},
		certificateRecordID:  1,
		ipStrategy:           reverseProxyIPStrategyPreferIPv4,
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "plain dns listener cannot bind certificate") {
		t.Fatalf("expected plain dns listener certificate bind to fail, got %v", err)
	}
}

func TestValidateNormalizedDNSRuleAllowsDifferentDoHPathsOnSameSocket(t *testing.T) {
	openReverseProxyTestDB(t)
	firstCertID := createReverseProxyTestCertificateRecord(t, "dns-one.example.com")
	secondCertID := createReverseProxyTestCertificateRecord(t, "dns-two.example.com")

	existing := model.ReverseProxyRule{
		DisplayID:           1,
		ListOrder:           1,
		Name:                "existing-doh",
		Enabled:             true,
		ListenProtocol:      reverseProxyProtocolDNS,
		ListenProtocolAlias: reverseProxyDNSProtocolDoH,

		ListenPort:            4443,
		ListenDNSPath:         "/dns-query",
		TargetProtocol:        reverseProxyProtocolDNS,
		TargetProtocolAlias:   reverseProxyDNSProtocolUDP,
		TargetAddresses:       `["1.1.1.1"]`,
		TargetPort:            53,
		CertificateRecordList: encodeReverseProxyUintList([]uint{firstCertID}),
		CertificateRecordID:   firstCertID,
		IPStrategy:            reverseProxyIPStrategyPreferIPv4,
	}
	if err := database.GetDB().Create(&existing).Error; err != nil {
		t.Fatalf("create dns rule failed: %v", err)
	}

	err := (&ReverseProxyService{}).validateNormalizedDNSRule(database.GetDB(), reverseProxyNormalizedRule{
		listenProtocol:            reverseProxyProtocolDNS,
		listenProtocolAlias:       reverseProxyDNSProtocolDoH,
		listenPort:                4443,
		listenDNSPath:             "/another-path",
		targetProtocol:            reverseProxyProtocolDNS,
		targetProtocolAlias:       reverseProxyDNSProtocolTCP,
		targetAddresses:           []string{"8.8.8.8"},
		targetPort:                53,
		dnsUpstreamTimeoutSeconds: 12,
		dnsCacheSizeBytes:         reverseProxyDNSDefaultCacheSizeBytes,
		dnsAllowedCIDRs:           []string{"192.0.2.0/24"},
		dnsRateLimitQPS:           reverseProxyDNSDefaultRateLimitQPS,
		certificateRecordIDs:      []uint{secondCertID},
		certificateRecordID:       secondCertID,
		ipStrategy:                reverseProxyIPStrategyPreferIPv4,
	})
	if err != nil {
		t.Fatalf("expected different doh paths to share the same listener socket, got %v", err)
	}
}

func TestValidateNormalizedDNSRuleRejectsDuplicateDoHPathOnSameSocket(t *testing.T) {
	openReverseProxyTestDB(t)
	firstCertID := createReverseProxyTestCertificateRecord(t, "dns-one.example.com")
	secondCertID := createReverseProxyTestCertificateRecord(t, "dns-two.example.com")

	existing := model.ReverseProxyRule{
		DisplayID:           1,
		ListOrder:           1,
		Name:                "existing-doh",
		Enabled:             true,
		ListenProtocol:      reverseProxyProtocolDNS,
		ListenProtocolAlias: reverseProxyDNSProtocolDoH,

		ListenPort:            4443,
		ListenDNSPath:         "/dns-query",
		TargetProtocol:        reverseProxyProtocolDNS,
		TargetProtocolAlias:   reverseProxyDNSProtocolUDP,
		TargetAddresses:       `["1.1.1.1"]`,
		TargetPort:            53,
		CertificateRecordList: encodeReverseProxyUintList([]uint{firstCertID}),
		CertificateRecordID:   firstCertID,
		IPStrategy:            reverseProxyIPStrategyPreferIPv4,
	}
	if err := database.GetDB().Create(&existing).Error; err != nil {
		t.Fatalf("create dns rule failed: %v", err)
	}

	err := (&ReverseProxyService{}).validateNormalizedDNSRule(database.GetDB(), reverseProxyNormalizedRule{
		listenProtocol:            reverseProxyProtocolDNS,
		listenProtocolAlias:       reverseProxyDNSProtocolDoH,
		listenPort:                4443,
		listenDNSPath:             "/dns-query",
		targetProtocol:            reverseProxyProtocolDNS,
		targetProtocolAlias:       reverseProxyDNSProtocolTCP,
		targetAddresses:           []string{"8.8.8.8"},
		targetPort:                53,
		dnsUpstreamTimeoutSeconds: 12,
		dnsCacheSizeBytes:         reverseProxyDNSDefaultCacheSizeBytes,
		dnsAllowedCIDRs:           []string{"192.0.2.0/24"},
		dnsRateLimitQPS:           reverseProxyDNSDefaultRateLimitQPS,
		certificateRecordIDs:      []uint{secondCertID},
		certificateRecordID:       secondCertID,
		ipStrategy:                reverseProxyIPStrategyPreferIPv4,
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "conflicts with existing dns listener") {
		t.Fatalf("expected duplicate doh path to conflict, got %v", err)
	}
}

func TestValidateNormalizedDNSRuleAllowsDoH3OnSeparateDoHSocket(t *testing.T) {
	openReverseProxyTestDB(t)
	firstCertID := createReverseProxyTestCertificateRecord(t, "dns-one.example.com")
	secondCertID := createReverseProxyTestCertificateRecord(t, "dns-two.example.com")

	existing := model.ReverseProxyRule{
		DisplayID:           1,
		ListOrder:           1,
		Name:                "existing-doh",
		Enabled:             true,
		ListenProtocol:      reverseProxyProtocolDNS,
		ListenProtocolAlias: reverseProxyDNSProtocolDoH,

		ListenPort:            4443,
		ListenDNSPath:         "/dns-query",
		TargetProtocol:        reverseProxyProtocolDNS,
		TargetProtocolAlias:   reverseProxyDNSProtocolUDP,
		TargetAddresses:       `["1.1.1.1"]`,
		TargetPort:            53,
		CertificateRecordList: encodeReverseProxyUintList([]uint{firstCertID}),
		CertificateRecordID:   firstCertID,
		IPStrategy:            reverseProxyIPStrategyPreferIPv4,
	}
	if err := database.GetDB().Create(&existing).Error; err != nil {
		t.Fatalf("create dns rule failed: %v", err)
	}

	err := (&ReverseProxyService{}).validateNormalizedDNSRule(database.GetDB(), reverseProxyNormalizedRule{
		listenProtocol:            reverseProxyProtocolDNS,
		listenProtocolAlias:       reverseProxyDNSProtocolDoHH3,
		listenPort:                4443,
		listenDNSPath:             "/dns-query",
		targetProtocol:            reverseProxyProtocolDNS,
		targetProtocolAlias:       reverseProxyDNSProtocolTCP,
		targetAddresses:           []string{"8.8.8.8"},
		targetPort:                53,
		dnsUpstreamTimeoutSeconds: 12,
		dnsCacheSizeBytes:         reverseProxyDNSDefaultCacheSizeBytes,
		dnsAllowedCIDRs:           []string{"192.0.2.0/24"},
		dnsRateLimitQPS:           reverseProxyDNSDefaultRateLimitQPS,
		certificateRecordIDs:      []uint{secondCertID},
		certificateRecordID:       secondCertID,
		ipStrategy:                reverseProxyIPStrategyPreferIPv4,
	})
	if err != nil {
		t.Fatalf("DoH3 should coexist with DoH on separate TCP/UDP sockets: %v", err)
	}
}

func TestValidateNormalizedHTTPRuleSharesDoHListenerWhenConditionsAreDisjoint(t *testing.T) {
	openReverseProxyTestDB(t)
	certID := createReverseProxyTestCertificateRecord(t, "dns-one.example.com")

	existing := model.ReverseProxyRule{
		DisplayID:           1,
		ListOrder:           1,
		Name:                "existing-doh",
		Enabled:             true,
		ListenProtocol:      reverseProxyProtocolDNS,
		ListenProtocolAlias: reverseProxyDNSProtocolDoH,

		ListenPort:            4443,
		HostList:              `["dns-one.example.com"]`,
		ListenDNSPath:         "/dns-query",
		TargetProtocol:        reverseProxyProtocolDNS,
		TargetProtocolAlias:   reverseProxyDNSProtocolUDP,
		TargetAddresses:       `["1.1.1.1"]`,
		TargetPort:            53,
		CertificateRecordList: encodeReverseProxyUintList([]uint{certID}),
		CertificateRecordID:   certID,
		IPStrategy:            reverseProxyIPStrategyPreferIPv4,
	}
	if err := database.GetDB().Create(&existing).Error; err != nil {
		t.Fatalf("create dns rule failed: %v", err)
	}

	httpsCertID := createReverseProxyTestCertificateRecord(t, "example.com")
	err := (&ReverseProxyService{}).validateNormalizedRule(database.GetDB(), reverseProxyNormalizedRule{
		listenProtocol:            reverseProxyProtocolHTTPS,
		listenPort:                4443,
		listenHTTPVersionStrategy: reverseProxyListenHTTPVersionH2H3,
		hosts:                     []string{"example.com"},
		pathPrefix:                "/",
		targetProtocol:            reverseProxyProtocolHTTP,
		targetAddresses:           []string{"127.0.0.1"},
		targetPort:                8080,
		certificateRecordIDs:      []uint{httpsCertID},
		certificateRecordID:       httpsCertID,
		ipStrategy:                reverseProxyIPStrategyPreferIPv4,
	})
	if err != nil {
		t.Fatalf("disjoint HTTPS and DoH conditions must share the TCP listener: %v", err)
	}

	err = (&ReverseProxyService{}).validateNormalizedRule(database.GetDB(), reverseProxyNormalizedRule{
		listenProtocol:            reverseProxyProtocolHTTPS,
		listenPort:                4443,
		listenHTTPVersionStrategy: reverseProxyListenHTTPVersionH2Only,
		hosts:                     []string{"dns-one.example.com"},
		pathPrefix:                "/",
		targetProtocol:            reverseProxyProtocolHTTP,
		targetAddresses:           []string{"127.0.0.1"},
		targetPort:                8080,
		certificateRecordIDs:      []uint{httpsCertID},
		certificateRecordID:       httpsCertID,
		ipStrategy:                reverseProxyIPStrategyPreferIPv4,
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "conflicts with existing dns listener") {
		t.Fatalf("same-host HTTPS root path must conflict with /dns-query DoH route, got %v", err)
	}
}

func TestValidateNormalizedHTTPRuleRejectsMixedProtocolsOnSamePort(t *testing.T) {
	openReverseProxyTestDB(t)

	existing := model.ReverseProxyRule{
		DisplayID:      1,
		ListOrder:      1,
		Name:           "existing-http",
		Enabled:        true,
		ListenProtocol: reverseProxyProtocolHTTP,

		ListenPort:        8080,
		HostList:          `["example.com"]`,
		PathPrefix:        "/",
		TargetProtocol:    reverseProxyProtocolHTTP,
		TargetAddresses:   `["127.0.0.1"]`,
		TargetPort:        18080,
		IPStrategy:        reverseProxyIPStrategyPreferIPv4,
		UpstreamTLSVerify: false,
	}
	if err := database.GetDB().Create(&existing).Error; err != nil {
		t.Fatalf("create http rule failed: %v", err)
	}

	certID := createReverseProxyTestCertificateRecord(t, "example.com")
	err := (&ReverseProxyService{}).validateNormalizedRule(database.GetDB(), reverseProxyNormalizedRule{
		listenProtocol:            reverseProxyProtocolHTTPS,
		listenPort:                8080,
		listenHTTPVersionStrategy: reverseProxyListenHTTPVersionH2H3,
		hosts:                     []string{"example.com"},
		pathPrefix:                "/secure",
		targetProtocol:            reverseProxyProtocolHTTP,
		targetAddresses:           []string{"127.0.0.1"},
		targetPort:                18443,
		certificateRecordIDs:      []uint{certID},
		certificateRecordID:       certID,
		ipStrategy:                reverseProxyIPStrategyPreferIPv4,
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "conflicts with existing reverse proxy listener") {
		t.Fatalf("expected https listener to conflict with existing http listener on same port, got %v", err)
	}
}

func TestReverseProxyDNSListenIPSetsOverlap(t *testing.T) {
	cases := []struct {
		name string
		a    []string
		b    []string
		want bool
	}{
		{name: "same ip", a: []string{"127.0.0.1"}, b: []string{"127.0.0.1"}, want: true},
		{name: "wildcard covers concrete", a: []string{"0.0.0.0"}, b: []string{"127.0.0.1"}, want: true},
		{name: "empty means wildcard", a: nil, b: []string{"127.0.0.1"}, want: true},
		{name: "different concrete ips", a: []string{"127.0.0.1"}, b: []string{"127.0.0.2"}, want: false},
		{name: "different families", a: []string{"0.0.0.0"}, b: []string{"::"}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := reverseProxyListenIPSetsOverlap(tc.a, tc.b); got != tc.want {
				t.Fatalf("overlap=%v want %v", got, tc.want)
			}
		})
	}
}

func TestReverseProxyDNSDoH3OnlyRoutesRestrictPath(t *testing.T) {
	called := 0
	mux := http.NewServeMux()
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called++
		w.WriteHeader(http.StatusNoContent)
	})
	for _, route := range buildReverseProxyDNSDoHRoutes("/dns-query") {
		mux.Handle(route, handler)
	}

	okReq := httptest.NewRequest(http.MethodPost, "https://dns.example.com/dns-query", nil)
	okRec := httptest.NewRecorder()
	mux.ServeHTTP(okRec, okReq)
	if okRec.Code != http.StatusNoContent || called != 1 {
		t.Fatalf("expected configured doh3 route to be handled, status=%d called=%d", okRec.Code, called)
	}

	missReq := httptest.NewRequest(http.MethodPost, "https://dns.example.com/other", nil)
	missRec := httptest.NewRecorder()
	mux.ServeHTTP(missRec, missReq)
	if missRec.Code != http.StatusNotFound || called != 1 {
		t.Fatalf("expected unconfigured doh3 route to be rejected, status=%d called=%d", missRec.Code, called)
	}
}

func TestReverseProxyDNSRouteFallsBackAfterPrimaryFailure(t *testing.T) {
	openReverseProxyTestDB(t)
	fallbackPort, fallbackHits := startReverseProxyTestDNSServer(t, 45, "203.0.113.45")
	primaryPort := reserveReverseProxyTestUDPPort(t)
	row := model.ReverseProxyRule{
		Id:                        1,
		ListenProtocol:            reverseProxyProtocolDNS,
		ListenProtocolAlias:       reverseProxyDNSProtocolUDP,
		TargetProtocol:            reverseProxyProtocolDNS,
		TargetProtocolAlias:       reverseProxyDNSProtocolUDP,
		TargetAddresses:           encodeReverseProxyList([]string{"127.0.0.1"}),
		TargetPort:                primaryPort,
		FallbackDNSUpstreams:      "[/fallback.example/]udp://127.0.0.1:" + strconv.Itoa(fallbackPort),
		DNSUpstreamTimeoutSeconds: 1,
		DNSCacheSizeBytes:         reverseProxyDNSDefaultCacheSizeBytes,
		IPStrategy:                reverseProxyIPStrategyPreferIPv4,
	}
	handler, err := buildReverseProxyDNSRuleHandler([]model.ReverseProxyRule{row})
	if err != nil {
		t.Fatalf("build dns handler with fallback failed: %v", err)
	}
	defer closeReverseProxyDNSHandler(handler)

	response, err := resolveReverseProxyTestDNSHandler(handler, "fallback.example.", "8.8.8.8:5300")
	if err != nil {
		t.Fatalf("resolve through fallback failed: %v", err)
	}
	if len(response.Answer) != 1 {
		t.Fatalf("expected fallback answer, got %#v", response.Answer)
	}
	if got := atomic.LoadInt32(fallbackHits); got != 1 {
		t.Fatalf("expected one fallback upstream request, got %d", got)
	}
}

func TestReverseProxyDNSRuleHandlerRejectsUnknownDoHPath(t *testing.T) {
	row := model.ReverseProxyRule{
		Id:                  1,
		ListenProtocol:      reverseProxyProtocolDNS,
		ListenProtocolAlias: reverseProxyDNSProtocolDoH,
		ListenDNSPath:       "/dns-query",
		TargetProtocol:      reverseProxyProtocolDNS,
		TargetProtocolAlias: reverseProxyDNSProtocolUDP,
		TargetAddresses:     encodeReverseProxyList([]string{"127.0.0.1"}),
		TargetPort:          53,
		DNSCacheSizeBytes:   reverseProxyDNSDefaultCacheSizeBytes,
		IPStrategy:          reverseProxyIPStrategyPreferIPv4,
		UpstreamTLSVerify:   true,
	}
	handler, err := buildReverseProxyDNSRuleHandler([]model.ReverseProxyRule{row})
	if err != nil {
		t.Fatalf("build doh handler failed: %v", err)
	}
	defer closeReverseProxyDNSHandler(handler)

	req := new(dns.Msg)
	req.SetQuestion("unknown-path.example.", dns.TypeA)
	dctx := &dnsproxy.DNSContext{
		Req:         req,
		Addr:        netip.MustParseAddrPort("8.8.8.8:5300"),
		Proto:       dnsproxy.ProtoHTTPS,
		HTTPRequest: httptest.NewRequest(http.MethodPost, "https://dns.example/not-configured", nil),
	}
	err = handler.ServeDNS(context.Background(), nil, dctx)
	if err == nil || !strings.Contains(err.Error(), "path is not configured") {
		t.Fatalf("expected unknown doh path to be rejected, got %v", err)
	}
	if dctx.Res != nil {
		t.Fatalf("unknown doh path must not use another rule resolver, got %#v", dctx.Res)
	}
}

func TestReverseProxyDNSRuleHandlerKeepsSamePathRoutesSeparatedByRuleID(t *testing.T) {
	rows := []model.ReverseProxyRule{
		{
			Id: 1, ListenProtocol: reverseProxyProtocolDNS, ListenProtocolAlias: reverseProxyDNSProtocolDoH,
			ListenPort: 443, ListenDNSPath: "/dns-query", TargetProtocol: reverseProxyProtocolDNS,
			TargetProtocolAlias: reverseProxyDNSProtocolUDP, TargetAddresses: `["1.1.1.1"]`, TargetPort: 53,
			DNSAllowedCIDRs: `["192.0.2.0/24"]`, DNSRateLimitQPS: 50,
		},
		{
			Id: 2, ListenProtocol: reverseProxyProtocolDNS, ListenProtocolAlias: reverseProxyDNSProtocolDoH,
			ListenPort: 443, ListenDNSPath: "/dns-query", TargetProtocol: reverseProxyProtocolDNS,
			TargetProtocolAlias: reverseProxyDNSProtocolUDP, TargetAddresses: `["8.8.8.8"]`, TargetPort: 53,
			DNSAllowedCIDRs: `["198.51.100.0/24"]`, DNSRateLimitQPS: 50,
		},
	}
	handler, err := buildReverseProxyDNSRuleHandler(rows)
	if err != nil {
		t.Fatalf("same DoH path on disjoint Host rules must build separate routes: %v", err)
	}
	defer func() { _ = closeReverseProxyDNSHandler(handler) }()
	handler.mu.RLock()
	first := handler.routesByRule[1]
	second := handler.routesByRule[2]
	handler.mu.RUnlock()
	if first == nil || second == nil || first == second || first.rule == nil || second.rule == nil || first.rule.Id != 1 || second.rule.Id != 2 {
		t.Fatalf("DoH rule routes were not isolated by rule ID: first=%p second=%p", first, second)
	}
}

func TestReverseProxyDNSRouteCachesResponsesAndAppliesTTLBounds(t *testing.T) {
	openReverseProxyTestDB(t)
	upstreamPort, upstreamHits := startReverseProxyTestDNSServer(t, 1, "203.0.113.60")
	row := model.ReverseProxyRule{
		Id:                        1,
		ListenProtocol:            reverseProxyProtocolDNS,
		ListenProtocolAlias:       reverseProxyDNSProtocolUDP,
		TargetProtocol:            reverseProxyProtocolDNS,
		TargetProtocolAlias:       reverseProxyDNSProtocolUDP,
		TargetAddresses:           encodeReverseProxyList([]string{"127.0.0.1"}),
		TargetPort:                upstreamPort,
		DNSUpstreamTimeoutSeconds: 2,
		DNSCacheEnabled:           true,
		DNSCacheSizeBytes:         1024 * 1024,
		DNSCacheMinTTL:            60,
		DNSCacheMaxTTL:            120,
		IPStrategy:                reverseProxyIPStrategyPreferIPv4,
	}
	handler, err := buildReverseProxyDNSRuleHandler([]model.ReverseProxyRule{row})
	if err != nil {
		t.Fatalf("build cached dns handler failed: %v", err)
	}
	defer closeReverseProxyDNSHandler(handler)

	first, err := resolveReverseProxyTestDNSHandler(handler, "cache.example.", "8.8.8.8:5300")
	if err != nil {
		t.Fatalf("first dns resolve failed: %v", err)
	}
	if len(first.Answer) != 1 {
		t.Fatalf("expected first dns answer, got %#v", first.Answer)
	}
	if got := first.Answer[0].Header().Ttl; got < 55 || got > 60 {
		t.Fatalf("expected cache minimum ttl near 60 seconds, got %d", got)
	}
	second, err := resolveReverseProxyTestDNSHandler(handler, "cache.example.", "8.8.8.8:5300")
	if err != nil {
		t.Fatalf("second dns resolve failed: %v", err)
	}
	if len(second.Answer) != 1 {
		t.Fatalf("expected cached dns answer, got %#v", second.Answer)
	}
	if got := atomic.LoadInt32(upstreamHits); got != 1 {
		t.Fatalf("expected one primary upstream query after cache hit, got %d", got)
	}
}

func TestReverseProxyDNSRouteAppliesMaximumTTLAndHonorsDisabledCache(t *testing.T) {
	openReverseProxyTestDB(t)

	maxTTLPort, maxTTLHits := startReverseProxyTestDNSServer(t, 600, "203.0.113.61")
	maxTTLRule := model.ReverseProxyRule{
		Id:                        1,
		ListenProtocol:            reverseProxyProtocolDNS,
		ListenProtocolAlias:       reverseProxyDNSProtocolUDP,
		TargetProtocol:            reverseProxyProtocolDNS,
		TargetProtocolAlias:       reverseProxyDNSProtocolUDP,
		TargetAddresses:           encodeReverseProxyList([]string{"127.0.0.1"}),
		TargetPort:                maxTTLPort,
		DNSUpstreamTimeoutSeconds: 2,
		DNSCacheEnabled:           true,
		DNSCacheSizeBytes:         1024 * 1024,
		DNSCacheMaxTTL:            120,
		IPStrategy:                reverseProxyIPStrategyPreferIPv4,
	}
	maxTTLHandler, err := buildReverseProxyDNSRuleHandler([]model.ReverseProxyRule{maxTTLRule})
	if err != nil {
		t.Fatalf("build maximum-ttl dns handler failed: %v", err)
	}
	response, err := resolveReverseProxyTestDNSHandler(maxTTLHandler, "max-ttl.example.", "8.8.8.8:5300")
	if err != nil {
		closeReverseProxyDNSHandler(maxTTLHandler)
		t.Fatalf("maximum-ttl dns resolve failed: %v", err)
	}
	if len(response.Answer) != 1 || response.Answer[0].Header().Ttl != 120 {
		closeReverseProxyDNSHandler(maxTTLHandler)
		t.Fatalf("expected maximum ttl 120, got %#v", response.Answer)
	}
	if _, err = resolveReverseProxyTestDNSHandler(maxTTLHandler, "max-ttl.example.", "8.8.8.8:5300"); err != nil {
		closeReverseProxyDNSHandler(maxTTLHandler)
		t.Fatalf("cached maximum-ttl dns resolve failed: %v", err)
	}
	closeReverseProxyDNSHandler(maxTTLHandler)
	if got := atomic.LoadInt32(maxTTLHits); got != 1 {
		t.Fatalf("expected maximum-ttl response to be cached, got %d upstream requests", got)
	}

	disabledPort, disabledHits := startReverseProxyTestDNSServer(t, 600, "203.0.113.62")
	disabledRule := maxTTLRule
	disabledRule.Id = 2
	disabledRule.TargetPort = disabledPort
	disabledRule.DNSCacheEnabled = false
	disabledRule.DNSCacheMinTTL = 60
	disabledRule.DNSCacheMaxTTL = 120
	disabledHandler, err := buildReverseProxyDNSRuleHandler([]model.ReverseProxyRule{disabledRule})
	if err != nil {
		t.Fatalf("build disabled-cache dns handler failed: %v", err)
	}
	defer closeReverseProxyDNSHandler(disabledHandler)
	for i := 0; i < 2; i++ {
		response, resolveErr := resolveReverseProxyTestDNSHandler(disabledHandler, "disabled-cache.example.", "8.8.8.8:5300")
		if resolveErr != nil {
			t.Fatalf("disabled-cache dns resolve %d failed: %v", i+1, resolveErr)
		}
		if len(response.Answer) != 1 || response.Answer[0].Header().Ttl != 600 {
			t.Fatalf("disabled cache must preserve upstream ttl, got %#v", response.Answer)
		}
	}
	if got := atomic.LoadInt32(disabledHits); got != 2 {
		t.Fatalf("disabled cache must query upstream every time, got %d requests", got)
	}
}

func TestReverseProxyDNSRouteCacheSeparatesEDNSClientSubnets(t *testing.T) {
	openReverseProxyTestDB(t)
	packetConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen ecs test dns upstream failed: %v", err)
	}
	var upstreamHits int32
	server := &dns.Server{
		PacketConn: packetConn,
		Handler: dns.HandlerFunc(func(w dns.ResponseWriter, req *dns.Msg) {
			atomic.AddInt32(&upstreamHits, 1)
			resp := new(dns.Msg)
			resp.SetReply(req)

			answer := "203.0.113.1"
			var requestECS *dns.EDNS0_SUBNET
			if opt := req.IsEdns0(); opt != nil {
				for _, option := range opt.Option {
					subnet, ok := option.(*dns.EDNS0_SUBNET)
					if !ok || subnet.Family != 1 || subnet.Address.To4() == nil {
						continue
					}
					requestECS = subnet
					answer = "203.0.113." + strconv.Itoa(int(subnet.Address.To4()[0]))
					break
				}
			}
			if len(req.Question) > 0 {
				resp.Answer = []dns.RR{&dns.A{
					Hdr: dns.RR_Header{
						Name:   req.Question[0].Name,
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    60,
					},
					A: net.ParseIP(answer).To4(),
				}}
			}
			if requestECS != nil {
				resp.SetEdns0(1232, false)
				resp.IsEdns0().Option = append(resp.IsEdns0().Option, &dns.EDNS0_SUBNET{
					Code:          dns.EDNS0SUBNET,
					Family:        requestECS.Family,
					SourceNetmask: requestECS.SourceNetmask,
					SourceScope:   requestECS.SourceNetmask,
					Address:       append(net.IP(nil), requestECS.Address...),
				})
			}
			_ = w.WriteMsg(resp)
		}),
	}
	go func() {
		_ = server.ActivateAndServe()
	}()
	t.Cleanup(func() {
		_ = server.Shutdown()
		_ = packetConn.Close()
	})

	row := model.ReverseProxyRule{
		Id:                        1,
		ListenProtocol:            reverseProxyProtocolDNS,
		ListenProtocolAlias:       reverseProxyDNSProtocolUDP,
		TargetProtocol:            reverseProxyProtocolDNS,
		TargetProtocolAlias:       reverseProxyDNSProtocolUDP,
		TargetAddresses:           encodeReverseProxyList([]string{"127.0.0.1"}),
		TargetPort:                packetConn.LocalAddr().(*net.UDPAddr).Port,
		DNSUpstreamTimeoutSeconds: 2,
		DNSCacheEnabled:           true,
		DNSCacheSizeBytes:         1024 * 1024,
		EDNSEnabled:               true,
		EDNSMode:                  reverseProxyEDNSModeAuto,
		EDNSClientSubnetPolicy:    reverseProxyEDNSClientSubnetPolicyPreferRequestPublic,
		IPStrategy:                reverseProxyIPStrategyPreferIPv4,
	}
	handler, err := buildReverseProxyDNSRuleHandler([]model.ReverseProxyRule{row})
	if err != nil {
		t.Fatalf("build ecs cached dns handler failed: %v", err)
	}
	defer closeReverseProxyDNSHandler(handler)

	requestForECS := func(ip string) *dns.Msg {
		req := new(dns.Msg)
		req.SetQuestion("ecs-cache.example.", dns.TypeA)
		req.SetEdns0(1232, false)
		req.IsEdns0().Option = append(req.IsEdns0().Option, &dns.EDNS0_SUBNET{
			Code:          dns.EDNS0SUBNET,
			Family:        1,
			SourceNetmask: 32,
			SourceScope:   0,
			Address:       net.ParseIP(ip).To4(),
		})
		return req
	}
	resolve := func(ip string) *dns.Msg {
		response, resolveErr := resolveReverseProxyTestDNSHandlerRequest(handler, requestForECS(ip), "9.9.9.9:5300")
		if resolveErr != nil {
			t.Fatalf("resolve ecs %s failed: %v", ip, resolveErr)
		}
		return response
	}

	if got := reverseProxyTestDNSAnswerIPv4(t, resolve("8.8.8.8")); got != "203.0.113.8" {
		t.Fatalf("unexpected first ecs answer: %s", got)
	}
	if got := reverseProxyTestDNSAnswerIPv4(t, resolve("1.1.1.1")); got != "203.0.113.1" {
		t.Fatalf("unexpected second ecs answer: %s", got)
	}
	if got := reverseProxyTestDNSAnswerIPv4(t, resolve("8.8.8.8")); got != "203.0.113.8" {
		t.Fatalf("unexpected cached ecs answer: %s", got)
	}
	if got := atomic.LoadInt32(&upstreamHits); got != 2 {
		t.Fatalf("expected separate cache entries for two ECS values, got %d upstream requests", got)
	}
}

func TestReverseProxyDNSRouteRefreshClearsOnlyChangedPathCache(t *testing.T) {
	openReverseProxyTestDB(t)
	firstPort, firstHits := startReverseProxyTestDNSServer(t, 30, "203.0.113.101")
	secondPort, secondHits := startReverseProxyTestDNSServer(t, 30, "203.0.113.102")
	rows := []model.ReverseProxyRule{
		{
			Id:                        1,
			ListOrder:                 1,
			ListenProtocol:            reverseProxyProtocolDNS,
			ListenProtocolAlias:       reverseProxyDNSProtocolDoH,
			ListenDNSPath:             "/first",
			TargetProtocol:            reverseProxyProtocolDNS,
			TargetProtocolAlias:       reverseProxyDNSProtocolUDP,
			TargetAddresses:           encodeReverseProxyList([]string{"127.0.0.1"}),
			TargetPort:                firstPort,
			DNSUpstreamTimeoutSeconds: 2,
			DNSCacheEnabled:           true,
			DNSCacheSizeBytes:         1024 * 1024,
			DNSCacheMinTTL:            60,
			IPStrategy:                reverseProxyIPStrategyPreferIPv4,
			UpstreamTLSVerify:         true,
		},
		{
			Id:                        2,
			ListOrder:                 2,
			ListenProtocol:            reverseProxyProtocolDNS,
			ListenProtocolAlias:       reverseProxyDNSProtocolDoH,
			ListenDNSPath:             "/second",
			TargetProtocol:            reverseProxyProtocolDNS,
			TargetProtocolAlias:       reverseProxyDNSProtocolUDP,
			TargetAddresses:           encodeReverseProxyList([]string{"127.0.0.1"}),
			TargetPort:                secondPort,
			DNSUpstreamTimeoutSeconds: 2,
			DNSCacheEnabled:           true,
			DNSCacheSizeBytes:         1024 * 1024,
			DNSCacheMinTTL:            60,
			IPStrategy:                reverseProxyIPStrategyPreferIPv4,
			UpstreamTLSVerify:         true,
		},
	}
	handler, err := buildReverseProxyDNSRuleHandler(rows)
	if err != nil {
		t.Fatalf("build shared doh handler failed: %v", err)
	}
	defer closeReverseProxyDNSHandler(handler)

	stateKey := reverseProxyDNSRuntimeStateKey(rows, nil)
	listenerStateKey := reverseProxyDNSListenerRuntimeStateKey(rows, nil)
	instance := &reverseProxyDNSInstance{
		handler:          handler,
		rules:            cloneReverseProxyRules(rows),
		runtimeStateKey:  stateKey,
		listenerStateKey: listenerStateKey,
	}
	resolve := func(path string) *dns.Msg {
		req := new(dns.Msg)
		req.SetQuestion("cache-refresh.example.", dns.TypeA)
		dctx := &dnsproxy.DNSContext{
			Req:         req,
			Addr:        netip.MustParseAddrPort("8.8.8.8:5300"),
			Proto:       dnsproxy.ProtoHTTPS,
			HTTPRequest: httptest.NewRequest(http.MethodPost, "https://dns.example"+path, nil),
		}
		if resolveErr := handler.ServeDNS(context.Background(), nil, dctx); resolveErr != nil {
			t.Fatalf("resolve %s failed: %v", path, resolveErr)
		}
		if dctx.Res == nil {
			t.Fatalf("resolve %s returned no dns response", path)
		}
		return dctx.Res
	}

	resolve("/first")
	resolve("/first")
	resolve("/second")
	resolve("/second")
	if got := atomic.LoadInt32(firstHits); got != 1 {
		t.Fatalf("expected first route cache hit before refresh, got %d upstream requests", got)
	}
	if got := atomic.LoadInt32(secondHits); got != 1 {
		t.Fatalf("expected second route cache hit before refresh, got %d upstream requests", got)
	}

	handler.mu.RLock()
	firstBefore := handler.routes["/first"]
	secondBefore := handler.routes["/second"]
	handler.mu.RUnlock()
	rows[0].DNSCacheSizeBytes = 2 * 1024 * 1024
	if got := reverseProxyDNSListenerRuntimeStateKey(rows, nil); got != listenerStateKey {
		t.Fatal("cache change must not require a shared doh listener restart")
	}
	refreshed, err := refreshReverseProxyDNSInstanceRoutes(instance, rows, reverseProxyDNSRuntimeStateKey(rows, nil))
	if err != nil || !refreshed {
		t.Fatalf("refresh changed dns route failed: refreshed=%v err=%v", refreshed, err)
	}
	handler.mu.RLock()
	firstAfter := handler.routes["/first"]
	secondAfter := handler.routes["/second"]
	handler.mu.RUnlock()
	if firstAfter == nil || firstAfter == firstBefore {
		t.Fatalf("expected changed route resolver to be rebuilt: old=%p new=%p", firstBefore, firstAfter)
	}
	if secondAfter == nil || secondAfter != secondBefore {
		t.Fatalf("unchanged shared doh route must retain its resolver: old=%p new=%p", secondBefore, secondAfter)
	}

	resolve("/first")
	resolve("/second")
	if got := atomic.LoadInt32(firstHits); got != 2 {
		t.Fatalf("changed route cache should be cleared, got %d upstream requests", got)
	}
	if got := atomic.LoadInt32(secondHits); got != 1 {
		t.Fatalf("unchanged route cache should remain warm, got %d upstream requests", got)
	}
}

func TestReverseProxyDNSRuntimeStateKeyChangesForCacheSettings(t *testing.T) {
	rows := []model.ReverseProxyRule{{
		Id:                        1,
		ListOrder:                 1,
		ListenProtocol:            reverseProxyProtocolDNS,
		ListenProtocolAlias:       reverseProxyDNSProtocolUDP,
		TargetProtocol:            reverseProxyProtocolDNS,
		TargetProtocolAlias:       reverseProxyDNSProtocolUDP,
		TargetAddresses:           encodeReverseProxyList([]string{"1.1.1.1"}),
		TargetPort:                53,
		DNSUpstreamTimeoutSeconds: 12,
		DNSCacheSizeBytes:         reverseProxyDNSDefaultCacheSizeBytes,
	}}
	before := reverseProxyDNSRuntimeStateKey(rows, nil)
	rows[0].DNSCacheEnabled = true
	rows[0].DNSCacheMinTTL = 60
	rows[0].DNSCacheMaxTTL = 120
	after := reverseProxyDNSRuntimeStateKey(rows, nil)
	if before == after {
		t.Fatal("dns cache settings must rebuild the affected runtime and clear its cache")
	}
}

func TestNormalizeReverseProxyPayloadAppliesDNSCacheDefaultsAndValidation(t *testing.T) {
	svc := &ReverseProxyService{}
	payload := ReverseProxyRulePayload{
		ListenProtocol:  reverseProxyDNSProtocolUDP,
		ListenPort:      53,
		TargetProtocol:  reverseProxyDNSProtocolUDP,
		TargetAddresses: "1.1.1.1",
		TargetPort:      53,
	}
	normalized, err := svc.normalizeRulePayload(payload)
	if err != nil {
		t.Fatalf("normalize dns cache defaults failed: %v", err)
	}
	if normalized.dnsUpstreamTimeoutSeconds != reverseProxyDNSDefaultUpstreamTimeoutSeconds {
		t.Fatalf("unexpected dns timeout default: %d", normalized.dnsUpstreamTimeoutSeconds)
	}
	if normalized.dnsCacheSizeBytes != reverseProxyDNSDefaultCacheSizeBytes || normalized.dnsCacheEnabled {
		t.Fatalf("unexpected dns cache defaults: size=%d enabled=%v", normalized.dnsCacheSizeBytes, normalized.dnsCacheEnabled)
	}

	var explicitZero ReverseProxyRulePayload
	if err = json.Unmarshal([]byte(`{"dnsUpstreamTimeoutSeconds":0,"dnsCacheSizeBytes":0}`), &explicitZero); err != nil {
		t.Fatalf("decode explicit dns zero values failed: %v", err)
	}
	if explicitZero.DNSUpstreamTimeoutSeconds == nil || explicitZero.DNSCacheSizeBytes == nil {
		t.Fatal("explicit dns zero values must remain distinguishable from missing fields")
	}
	payload.DNSUpstreamTimeoutSeconds = explicitZero.DNSUpstreamTimeoutSeconds
	if _, err = svc.normalizeRulePayload(payload); err == nil {
		t.Fatal("expected explicit zero dns upstream timeout to be rejected")
	}
	validTimeout := reverseProxyDNSDefaultUpstreamTimeoutSeconds
	payload.DNSUpstreamTimeoutSeconds = &validTimeout
	payload.DNSCacheSizeBytes = explicitZero.DNSCacheSizeBytes
	if _, err = svc.normalizeRulePayload(payload); err == nil {
		t.Fatal("expected explicit zero dns cache size to be rejected")
	}
	retainedCacheSize := 999999
	payload.DNSCacheSizeBytes = &retainedCacheSize
	payload.DNSCacheMinTTL = 60
	payload.DNSCacheMaxTTL = 120
	normalized, err = svc.normalizeRulePayload(payload)
	if err != nil {
		t.Fatalf("normalize disabled dns cache with valid retained size failed: %v", err)
	}
	if normalized.dnsCacheEnabled || normalized.dnsCacheSizeBytes != retainedCacheSize || normalized.dnsCacheMinTTL != 60 || normalized.dnsCacheMaxTTL != 120 {
		t.Fatalf("disabled dns cache must retain valid settings, got enabled=%v size=%d min=%d max=%d", normalized.dnsCacheEnabled, normalized.dnsCacheSizeBytes, normalized.dnsCacheMinTTL, normalized.dnsCacheMaxTTL)
	}

	payload.DNSCacheMinTTL = reverseProxyDNSMaxCacheTTLSeconds
	payload.DNSCacheMaxTTL = reverseProxyDNSMaxCacheTTLSeconds
	if _, err = svc.normalizeRulePayload(payload); err != nil {
		t.Fatalf("maximum uint32 dns ttl must be accepted: %v", err)
	}
	payload.DNSCacheMinTTL = reverseProxyDNSMaxCacheTTLSeconds + 1
	payload.DNSCacheMaxTTL = 0
	if _, err = svc.normalizeRulePayload(payload); err == nil {
		t.Fatal("expected dns ttl above uint32 maximum to be rejected")
	}
	payload.DNSCacheMinTTL = 121
	payload.DNSCacheMaxTTL = 120
	if _, err = svc.normalizeRulePayload(payload); err == nil {
		t.Fatal("expected invalid dns ttl range to be rejected")
	}
	payload.DNSCacheMinTTL = 0
	payload.DNSCacheMaxTTL = 0
	payload.FallbackDNSUpstreams = "https://%"
	normalized, err = svc.normalizeRulePayload(payload)
	if err != nil {
		t.Fatalf("normalize invalid fallback input should defer syntax validation: %v", err)
	}
	openReverseProxyTestDB(t)
	if err = svc.preflightNormalizedRule(normalized); err == nil {
		t.Fatal("expected invalid fallback dns upstream syntax to be rejected")
	}
}

func TestReverseProxyDNSFallbackParserAcceptsAdGuardStyleEntries(t *testing.T) {
	config, err := buildReverseProxyDNSFallbackUpstreamConfig(&model.ReverseProxyRule{
		FallbackDNSUpstreams: strings.Join([]string{
			"# fallback resolvers",
			"udp://1.1.1.1:53",
			"[/example.com/]tls://1.0.0.1:853",
			"https://dns.google/dns-query",
			"h3://dns.google/dns-query",
		}, "\n"),
		DNSUpstreamTimeoutSeconds: 12,
		IPStrategy:                reverseProxyIPStrategyPreferIPv4,
		UpstreamTLSVerify:         true,
	})
	if err != nil {
		t.Fatalf("parse fallback upstream entries failed: %v", err)
	}
	defer config.Close()
	if len(config.Upstreams) != 3 {
		t.Fatalf("unexpected default fallback upstream count: %d", len(config.Upstreams))
	}
	if len(config.DomainReservedUpstreams) == 0 {
		t.Fatal("expected domain-specific fallback upstream rule")
	}
}

func TestReverseProxyDNSFallbackParserAllowsDomainOnlyEntry(t *testing.T) {
	config, err := buildReverseProxyDNSFallbackUpstreamConfig(&model.ReverseProxyRule{
		FallbackDNSUpstreams:      "[/fallback.example/]udp://1.1.1.1:53",
		DNSUpstreamTimeoutSeconds: 12,
		IPStrategy:                reverseProxyIPStrategyPreferIPv4,
		UpstreamTLSVerify:         true,
	})
	if err != nil {
		t.Fatalf("parse domain-only fallback upstream failed: %v", err)
	}
	defer config.Close()
	if len(config.Upstreams) != 0 || len(config.DomainReservedUpstreams) == 0 {
		t.Fatalf("unexpected domain-only fallback configuration: %#v", config)
	}
}

func TestReverseProxyDNSIPStrategyResolverFiltersAddressFamilies(t *testing.T) {
	resolver := reverseProxyDNSIPStrategyResolver{
		base: reverseProxyTestResolver{
			addrs: []netip.Addr{
				netip.MustParseAddr("1.1.1.1"),
				netip.MustParseAddr("2001:db8::1"),
			},
		},
		strategy: reverseProxyIPStrategyIPv4Only,
	}
	got, err := resolver.LookupNetIP(context.Background(), "ip", "dns.example.com")
	if err != nil {
		t.Fatalf("lookup ipv4 only failed: %v", err)
	}
	if len(got) != 1 || !got[0].Is4() {
		t.Fatalf("unexpected ipv4 only result: %#v", got)
	}

	resolver.strategy = reverseProxyIPStrategyIPv6Only
	got, err = resolver.LookupNetIP(context.Background(), "ip", "dns.example.com")
	if err != nil {
		t.Fatalf("lookup ipv6 only failed: %v", err)
	}
	if len(got) != 1 || !got[0].Is6() {
		t.Fatalf("unexpected ipv6 only result: %#v", got)
	}

	resolver.strategy = reverseProxyIPStrategyPreferIPv6
	got, err = resolver.LookupNetIP(context.Background(), "ip", "dns.example.com")
	if err != nil {
		t.Fatalf("lookup prefer ipv6 failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("prefer strategies should keep both address families, got %#v", got)
	}
}

func TestReverseProxyHostPatternMatches(t *testing.T) {
	tests := []struct {
		pattern string
		host    string
		want    bool
	}{
		{pattern: "example.com", host: "example.com", want: true},
		{pattern: "*.example.com", host: "api.example.com", want: true},
		{pattern: "*.example.com", host: "example.com", want: false},
		{pattern: "*.example.com", host: "a.b.example.com", want: false},
	}
	for _, tc := range tests {
		if got := reverseProxyHostPatternMatches(tc.pattern, tc.host); got != tc.want {
			t.Fatalf("pattern match (%s,%s)=%v want %v", tc.pattern, tc.host, got, tc.want)
		}
	}
}

func TestReverseProxyRulePathMatch_UsesStrictPrefixBoundaries(t *testing.T) {
	rule := &model.ReverseProxyRule{PathPrefix: "/88999"}
	if !reverseProxyRulePathMatch(rule, "/88999") {
		t.Fatal("expected prefix root to match")
	}
	if !reverseProxyRulePathMatch(rule, "/88999/888") {
		t.Fatal("expected child path to match")
	}
	if !reverseProxyRulePathMatch(rule, "/88999/tag/mysql/") {
		t.Fatal("expected nested path to match")
	}
	if reverseProxyRulePathMatch(rule, "/88999x") {
		t.Fatal("expected sibling path to fail")
	}
	if !reverseProxyRulePathMatch(&model.ReverseProxyRule{PathPrefix: "/88999/"}, "/88999") {
		t.Fatal("expected trailing slash rule to normalize to the same prefix")
	}
	if !reverseProxyRulePathMatch(&model.ReverseProxyRule{}, "/anything") {
		t.Fatal("empty path should skip url validation")
	}
}

func TestReverseProxyTrimMatchedPathPrefix_UsesStrictPrefixBoundaries(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		rawPath     string
		prefix      string
		wantPath    string
		wantRawPath string
	}{
		{
			name:        "strip child path",
			path:        "/88999/tag/mysql/",
			prefix:      "/88999",
			wantPath:    "/tag/mysql/",
			wantRawPath: "/tag/mysql/",
		},
		{
			name:        "prefix root maps to slash",
			path:        "/88999",
			prefix:      "/88999",
			wantPath:    "/",
			wantRawPath: "/",
		},
		{
			name:        "sibling path is not stripped",
			path:        "/88999x/tag/mysql/",
			prefix:      "/88999",
			wantPath:    "/88999x/tag/mysql/",
			wantRawPath: "/88999x/tag/mysql/",
		},
		{
			name:        "normalize encoded slash in upstream remainder",
			path:        "/88999/tag/mysql/",
			rawPath:     "/88999/tag%2Fmysql/",
			prefix:      "/88999",
			wantPath:    "/tag/mysql/",
			wantRawPath: "/tag/mysql/",
		},
		{
			name:        "encoded local prefix falls back to decoded trim",
			path:        "/88999/tag/mysql/",
			rawPath:     "/%38%38%39%39%39/tag/mysql/",
			prefix:      "/88999",
			wantPath:    "/tag/mysql/",
			wantRawPath: "/tag/mysql/",
		},
		{
			name:        "normalize encoded local prefix and encoded slash",
			path:        "/app/file/name",
			rawPath:     "/%61pp/file%2Fname",
			prefix:      "/app",
			wantPath:    "/file/name",
			wantRawPath: "/file/name",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotPath, gotRawPath := reverseProxyTrimMatchedPathPrefix(tc.path, tc.rawPath, tc.prefix)
			if gotPath != tc.wantPath || gotRawPath != tc.wantRawPath {
				t.Fatalf("unexpected trim result: got path=%q raw=%q want path=%q raw=%q", gotPath, gotRawPath, tc.wantPath, tc.wantRawPath)
			}
		})
	}
}

func TestReverseProxyHTTPSRuleForwardsRequestPathRelativeToTargetBase(t *testing.T) {
	cases := []struct {
		name        string
		pathPrefix  string
		targetPath  string
		requestPath string
		wantPath    string
	}{
		{
			name:        "preserve_request_path",
			pathPrefix:  "",
			targetPath:  "",
			requestPath: "/wp-content/cache/site.css",
			wantPath:    "/wp-content/cache/site.css",
		},
		{
			name:        "strip_local_prefix",
			pathPrefix:  "/88999",
			targetPath:  "",
			requestPath: "/88999/tag/mysql/",
			wantPath:    "/tag/mysql/",
		},
		{
			name:        "join_target_base_after_stripping_local_prefix",
			pathPrefix:  "/88999",
			targetPath:  "/base",
			requestPath: "/88999/wp-content/cache/site.css",
			wantPath:    "/base/wp-content/cache/site.css",
		},
		{
			name:        "prefix_root_maps_to_upstream_root",
			pathPrefix:  "/88999",
			targetPath:  "",
			requestPath: "/88999",
			wantPath:    "/",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			openReverseProxyTestDB(t)

			svc := &ReverseProxyService{}
			t.Cleanup(func() {
				_ = svc.StopRuntime()
			})

			upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				_, _ = w.Write([]byte(r.URL.Path))
			}))
			defer upstream.Close()

			upstreamHost, upstreamPort := splitReverseProxyTestServerAddress(t, upstream.URL)
			listenPort := reserveReverseProxyTestPort(t)
			certRecordID := createReverseProxyTestCertificateRecord(t, "example.com")

			if err := svc.UpsertRule(ReverseProxyRulePayload{
				Name:           "forward-request-path-" + tc.name,
				Enabled:        true,
				ListenProtocol: reverseProxyProtocolHTTPS,

				ListenPort:          listenPort,
				PathPrefix:          tc.pathPrefix,
				TargetProtocol:      reverseProxyProtocolHTTPS,
				TargetAddresses:     upstreamHost,
				TargetPort:          upstreamPort,
				TargetPath:          tc.targetPath,
				CertificateRecordID: certRecordID,
				IPStrategy:          reverseProxyIPStrategyPreferIPv4,
				HTTPVersionStrategy: reverseProxyHTTPVersionPreferH2,
				UpstreamTLSVerify:   false,
			}); err != nil {
				t.Fatalf("upsert path forwarding rule failed: %v", err)
			}

			client := &http.Client{
				Transport: &http.Transport{
					Proxy: nil,
					TLSClientConfig: &tls.Config{
						InsecureSkipVerify: true,
						ServerName:         "example.com",
					},
				},
				Timeout: 15 * time.Second,
			}
			req, err := http.NewRequest(http.MethodGet, "https://127.0.0.1:"+strconv.Itoa(listenPort)+tc.requestPath, nil)
			if err != nil {
				t.Fatalf("build path forwarding request failed: %v", err)
			}
			req.Host = "example.com"

			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("path forwarding request failed: %v", err)
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read path forwarding body failed: %v", err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("unexpected path forwarding status: %d body=%q", resp.StatusCode, string(body))
			}
			if got := string(body); got != tc.wantPath {
				t.Fatalf("unexpected upstream path: got %q want %q", got, tc.wantPath)
			}
		})
	}
}

func TestReverseProxyRuntimeListenIPsAlwaysUseIPv4AndIPv6Wildcards(t *testing.T) {
	got := reverseProxyHTTPRuntimeListenIPs(&model.ReverseProxyRule{})
	if !reflect.DeepEqual(got, []string{"0.0.0.0", "::"}) {
		t.Fatalf("unexpected runtime listen ips: %#v", got)
	}
}

func TestValidateReverseProxyNoObviousLoopDefersToResolvedGuard(t *testing.T) {
	err := validateReverseProxyNoObviousLoop(reverseProxyNormalizedRule{
		listenProtocol:  "https",
		listenPort:      443,
		targetProtocol:  "https",
		targetAddresses: []string{"127.0.0.1"},
		targetPort:      443,
	})
	if err != nil {
		t.Fatalf("metadata-only loop validation must defer to resolved target guard: %v", err)
	}

	err = validateReverseProxyNoObviousLoop(reverseProxyNormalizedRule{
		listenProtocol:  "https",
		listenPort:      443,
		targetProtocol:  "https",
		targetAddresses: []string{"127.0.0.1"},
		targetPort:      8443,
	})
	if err != nil {
		t.Fatalf("different port should pass: %v", err)
	}
}

func TestComputeReverseProxyRenderKey_IgnoresRuntimeFields(t *testing.T) {
	rows := []model.ReverseProxyRule{
		{
			Id:                  1,
			ListOrder:           1,
			Enabled:             true,
			ListenProtocol:      "https",
			ListenPort:          8443,
			HostList:            `["example.com"]`,
			PathPrefix:          "/app",
			TargetProtocol:      "https",
			TargetAddresses:     `["backend.local"]`,
			TargetPort:          443,
			TargetPath:          "/api",
			CertificateRecordID: 7,
			IPStrategy:          reverseProxyIPStrategyPreferIPv4,
			HTTPVersionStrategy: reverseProxyHTTPVersionPreferH2,
			UpstreamTLSVerify:   true,
			LastError:           "dial error",
			RuntimeStatus:       "proxy_error",
		},
	}

	base := computeReverseProxyRenderKey(nil, rows)
	rows[0].LastError = "updated later"
	rows[0].RuntimeStatus = "running"
	rows[0].UpdatedAt = time.Now()
	rows[0].CreatedAt = time.Now().Add(-time.Hour)
	if got := computeReverseProxyRenderKey(nil, rows); got != base {
		t.Fatalf("runtime-only change should not affect render key: %q vs %q", got, base)
	}

	rows[0].TargetPort = 8444
	if got := computeReverseProxyRenderKey(nil, rows); got == base {
		t.Fatalf("config change should affect render key")
	}

	rows[0].TargetPort = 443
	rows[0].ApiPassthrough = true
	if got := computeReverseProxyRenderKey(nil, rows); got == base {
		t.Fatalf("api passthrough change should affect render key")
	}
}

func TestComputeReverseProxyRenderKeyIncludesRuntimeConfigurationFields(t *testing.T) {
	base := model.ReverseProxyRule{
		Id:             1,
		ListOrder:      1,
		Enabled:        true,
		ListenProtocol: reverseProxyProtocolHTTPS,

		ListenPort:                 8443,
		HostList:                   `["example.com"]`,
		TargetProtocol:             reverseProxyProtocolHTTPS,
		TargetAddresses:            `["upstream.example"]`,
		TargetPort:                 443,
		IPStrategy:                 reverseProxyIPStrategyPreferIPv4,
		UpstreamTLSVerify:          true,
		MaxConcurrentRequests:      10,
		MaxConcurrentConnections:   11,
		UpstreamMaxConnections:     12,
		UpstreamMaxIdleConnections: 13,
		MemoryLimitBytes:           14 * 1024 * 1024,
	}
	mutations := []struct {
		name   string
		mutate func(*model.ReverseProxyRule)
	}{
		{name: "local_connections", mutate: func(row *model.ReverseProxyRule) { row.MaxConcurrentConnections++ }},
		{name: "requests", mutate: func(row *model.ReverseProxyRule) { row.MaxConcurrentRequests++ }},
		{name: "upstream_connections", mutate: func(row *model.ReverseProxyRule) { row.UpstreamMaxConnections++ }},
		{name: "upstream_idle", mutate: func(row *model.ReverseProxyRule) { row.UpstreamMaxIdleConnections++ }},
		{name: "memory", mutate: func(row *model.ReverseProxyRule) { row.MemoryLimitBytes++ }},
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			before := computeReverseProxyRenderKey(nil, []model.ReverseProxyRule{base})
			changed := base
			tc.mutate(&changed)
			if after := computeReverseProxyRenderKey(nil, []model.ReverseProxyRule{changed}); after == before {
				t.Fatalf("%s change must refresh the runtime", tc.name)
			}
		})
	}
}

func TestReverseProxyDoHUsesOnlyTCPSocket(t *testing.T) {
	if !reverseProxyDNSProtocolUsesTCP(reverseProxyDNSProtocolDoH) {
		t.Fatal("ordinary DoH must use TCP")
	}
	if reverseProxyDNSProtocolUsesUDP(reverseProxyDNSProtocolDoH) {
		t.Fatal("ordinary DoH must not claim UDP")
	}
	if tcp, udp := reverseProxyListenerUsesUnderlyingSockets(reverseProxyProtocolDNS, "", reverseProxyDNSProtocolDoH); !tcp || udp {
		t.Fatalf("unexpected DoH socket footprint: tcp=%v udp=%v", tcp, udp)
	}
	if tcp, udp := reverseProxyListenerUsesUnderlyingSockets(reverseProxyProtocolHTTPS, reverseProxyListenHTTPVersionH2H3, "wss"); !tcp || udp {
		t.Fatalf("WSS must remain TCP-only: tcp=%v udp=%v", tcp, udp)
	}
}

func TestReverseProxyDNSAndHTTPListenerSocketConflictMatrix(t *testing.T) {
	tests := []struct {
		name      string
		dnsAlias  string
		httpMode  string
		conflicts bool
	}{
		{name: "dot_with_h2", dnsAlias: reverseProxyDNSProtocolDoT, httpMode: reverseProxyListenHTTPVersionH2Only, conflicts: true},
		{name: "dot_with_h3", dnsAlias: reverseProxyDNSProtocolDoT, httpMode: reverseProxyListenHTTPVersionH3Only, conflicts: false},
		{name: "doq_with_h2", dnsAlias: reverseProxyDNSProtocolDoQ, httpMode: reverseProxyListenHTTPVersionH2Only, conflicts: false},
		{name: "doq_with_h3", dnsAlias: reverseProxyDNSProtocolDoQ, httpMode: reverseProxyListenHTTPVersionH3Only, conflicts: true},
		{name: "doh_with_h2", dnsAlias: reverseProxyDNSProtocolDoH, httpMode: reverseProxyListenHTTPVersionH2Only, conflicts: true},
		{name: "doh_with_h3", dnsAlias: reverseProxyDNSProtocolDoH, httpMode: reverseProxyListenHTTPVersionH3Only, conflicts: false},
		{name: "doh3_with_h2", dnsAlias: reverseProxyDNSProtocolDoHH3, httpMode: reverseProxyListenHTTPVersionH2Only, conflicts: false},
		{name: "doh3_with_h3", dnsAlias: reverseProxyDNSProtocolDoHH3, httpMode: reverseProxyListenHTTPVersionH3Only, conflicts: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := reverseProxyProtocolsShareUnderlyingSocket(
				reverseProxyProtocolDNS, "", reverseProxyProtocolHTTPS, tc.httpMode,
				tc.dnsAlias, reverseProxyProtocolHTTPS,
			)
			if got != tc.conflicts {
				t.Fatalf("socket conflict=%v want %v", got, tc.conflicts)
			}
		})
	}
}

func TestReverseProxyWebSocketAliasRejectsOrdinaryHTTP(t *testing.T) {
	group := &reverseProxyListenerGroup{
		protocol: reverseProxyProtocolHTTP,
		rules: []*model.ReverseProxyRule{{
			Id:                  1,
			Enabled:             true,
			ListenProtocol:      reverseProxyProtocolHTTP,
			ListenProtocolAlias: "ws",
			HostList:            encodeReverseProxyList([]string{"example.com"}),
			PathPrefix:          "/",
			TargetProtocol:      reverseProxyProtocolHTTP,
			TargetProtocolAlias: "ws",
		}},
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "http://example.com/socket", nil)
	request.Host = "example.com"
	group.newHandler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUpgradeRequired {
		t.Fatalf("ordinary HTTP request to WS alias returned %d", recorder.Code)
	}
	if got := recorder.Header().Get("Upgrade"); !strings.EqualFold(got, "websocket") {
		t.Fatalf("missing websocket upgrade response header: %q", got)
	}
}

func TestReverseProxyDNSRouteRetiresAfterActiveLease(t *testing.T) {
	cache := &reverseProxyDNSResponseCache{
		entries: make(map[string]*reverseProxyDNSCacheEntry),
		lru:     list.New(),
		expiry:  make(reverseProxyDNSExpiryHeap, 0),
	}
	route := &reverseProxyDNSRoute{cache: cache}
	lease, ok := route.acquire()
	if !ok {
		t.Fatal("route lease was rejected")
	}
	started := time.Now()
	if err := route.close(); err != nil {
		t.Fatalf("retire route failed: %v", err)
	}
	if time.Since(started) > 100*time.Millisecond {
		t.Fatal("route retirement blocked on an active query")
	}
	cache.mu.Lock()
	closedBeforeRelease := cache.closed
	cache.mu.Unlock()
	if closedBeforeRelease {
		t.Fatal("active route resources closed before the lease was released")
	}
	lease.release()
	cache.mu.Lock()
	closedAfterRelease := cache.closed
	cache.mu.Unlock()
	if !closedAfterRelease {
		t.Fatal("retired route resources were not closed after the final lease")
	}
}

func TestComputeReverseProxyRenderKey_IgnoresDNSFieldsForHTTPRules(t *testing.T) {
	rows := []model.ReverseProxyRule{
		{
			Id:                  1,
			ListOrder:           1,
			Enabled:             true,
			ListenProtocol:      reverseProxyProtocolHTTPS,
			ListenPort:          8443,
			HostList:            `["example.com"]`,
			PathPrefix:          "/app",
			TargetProtocol:      reverseProxyProtocolHTTPS,
			TargetAddresses:     `["backend.local"]`,
			TargetPort:          443,
			TargetPath:          "/api",
			CertificateRecordID: 7,
			IPStrategy:          reverseProxyIPStrategyPreferIPv4,
			HTTPVersionStrategy: reverseProxyHTTPVersionPreferH2,
			UpstreamTLSVerify:   true,
		},
	}

	base := computeReverseProxyRenderKey(nil, rows)
	rows[0].EDNSEnabled = true
	rows[0].EDNSMode = reverseProxyEDNSModeCustom
	rows[0].EDNSCustomIP = "14.119.184.1"
	rows[0].EDNSClientSubnetPolicy = reverseProxyEDNSClientSubnetPolicyPreferRequestPublic
	rows[0].DisableIPv4Answer = true
	rows[0].DisableIPv6Answer = true

	if got := computeReverseProxyRenderKey(nil, rows); got != base {
		t.Fatalf("dns-only fields should not affect http render key: %q vs %q", got, base)
	}
}

func TestComputeReverseProxyRenderKey_IgnoresSeparateDNSRules(t *testing.T) {
	rows := []model.ReverseProxyRule{
		{
			Id:                  1,
			ListOrder:           1,
			Enabled:             true,
			ListenProtocol:      reverseProxyProtocolHTTPS,
			ListenPort:          8443,
			HostList:            `["example.com"]`,
			PathPrefix:          "/app",
			TargetProtocol:      reverseProxyProtocolHTTPS,
			TargetAddresses:     `["backend.local"]`,
			TargetPort:          443,
			TargetPath:          "/api",
			IPStrategy:          reverseProxyIPStrategyPreferIPv4,
			HTTPVersionStrategy: reverseProxyHTTPVersionPreferH2,
			UpstreamTLSVerify:   true,
		},
		{
			Id:                     2,
			ListOrder:              2,
			Enabled:                true,
			ListenProtocol:         reverseProxyProtocolDNS,
			ListenProtocolAlias:    reverseProxyDNSProtocolDoH,
			ListenPort:             443,
			ListenDNSPath:          "/dns-query",
			TargetProtocol:         reverseProxyProtocolDNS,
			TargetProtocolAlias:    reverseProxyDNSProtocolTCP,
			TargetAddresses:        `["1.1.1.1"]`,
			TargetPort:             53,
			EDNSEnabled:            true,
			EDNSMode:               reverseProxyEDNSModeAuto,
			EDNSClientSubnetPolicy: reverseProxyEDNSClientSubnetPolicyClientIP,
			DisableIPv4Answer:      true,
			IPStrategy:             reverseProxyIPStrategyPreferIPv4,
		},
	}

	base := computeReverseProxyRenderKey(nil, rows)
	rows[1].EDNSEnabled = false
	rows[1].EDNSMode = reverseProxyEDNSModeCustom
	rows[1].EDNSCustomIP = "14.119.184.1"
	rows[1].DisableIPv6Answer = true
	rows[1].TargetAddresses = `["8.8.8.8"]`

	if got := computeReverseProxyRenderKey(nil, rows); got != base {
		t.Fatalf("separate dns rule change should not affect http render key: %q vs %q", got, base)
	}
}

func TestComputeReverseProxyRenderKey_ChangesWhenCertificateContentChanges(t *testing.T) {
	openReverseProxyTestDB(t)

	cert := model.CertificateRecord{
		SourceType:      CertificateSourceSelfSigned,
		SourceRef:       "reverse-proxy-render-key",
		MainDomain:      "127.0.0.1",
		DomainSet:       `["127.0.0.1"]`,
		CertPEM:         []byte("cert"),
		KeyPEM:          []byte("key"),
		FullchainPEM:    []byte("cert"),
		Fingerprint:     "fingerprint-old",
		ListOrderAt:     time.Now().Unix(),
		CertificateType: "ip",
	}
	if err := database.GetDB().Create(&cert).Error; err != nil {
		t.Fatalf("create certificate record failed: %v", err)
	}

	rows := []model.ReverseProxyRule{
		{
			Id:                  1,
			ListOrder:           1,
			Enabled:             true,
			ListenProtocol:      reverseProxyProtocolHTTPS,
			ListenPort:          8443,
			TargetProtocol:      reverseProxyProtocolHTTP,
			TargetAddresses:     `["127.0.0.1"]`,
			TargetPort:          8080,
			CertificateRecordID: cert.Id,
			IPStrategy:          reverseProxyIPStrategyPreferIPv4,
		},
	}

	base := computeReverseProxyRenderKey(database.GetDB(), rows)
	if err := database.GetDB().Model(&model.CertificateRecord{}).Where("id = ?", cert.Id).Updates(map[string]interface{}{
		"fingerprint": "fingerprint-new",
		"updated_at":  time.Now().Add(time.Minute),
	}).Error; err != nil {
		t.Fatalf("update certificate record failed: %v", err)
	}

	if got := computeReverseProxyRenderKey(database.GetDB(), rows); got == base {
		t.Fatal("certificate content change should affect render key")
	}
}

func TestReverseProxySNIUsesReloadedCertificate(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	t.Cleanup(func() {
		_ = svc.StopRuntime()
	})

	certPEMOld, keyPEMOld := buildReverseProxyTestCertificatePEM(t, []string{"example.com"})
	oldFingerprint, _, _, err := inspectCertificateFingerprint(certPEMOld, keyPEMOld)
	if err != nil {
		t.Fatalf("inspect old certificate failed: %v", err)
	}
	cert := model.CertificateRecord{
		SourceType:      CertificateSourceSelfSigned,
		SourceRef:       "reverse-proxy-sni-reload",
		MainDomain:      "example.com",
		DomainSet:       `["example.com"]`,
		CertPEM:         certPEMOld,
		KeyPEM:          keyPEMOld,
		FullchainPEM:    certPEMOld,
		Fingerprint:     oldFingerprint,
		ListOrderAt:     time.Now().Unix(),
		CertificateType: "domain",
	}
	if err := database.GetDB().Create(&cert).Error; err != nil {
		t.Fatalf("create certificate record failed: %v", err)
	}

	listenPort := reserveReverseProxyTestPort(t)
	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:           "sni-reload",
		Enabled:        true,
		ListenProtocol: reverseProxyProtocolHTTPS,

		ListenPort:          listenPort,
		Hosts:               "example.com",
		TargetProtocol:      reverseProxyProtocolHTTP,
		TargetAddresses:     "127.0.0.1",
		TargetPort:          reserveReverseProxyTestPort(t),
		CertificateRecordID: cert.Id,
		IPStrategy:          reverseProxyIPStrategyPreferIPv4,
	}); err != nil {
		t.Fatalf("upsert sni reverse proxy rule failed: %v", err)
	}

	if got := reverseProxyDialWithSNIFingerprint(t, listenPort, "example.com"); got != oldFingerprint {
		t.Fatalf("initial sni certificate fingerprint = %q, want %q", got, oldFingerprint)
	}

	certPEMNew, keyPEMNew := buildReverseProxyTestCertificatePEM(t, []string{"example.com"})
	newFingerprint, _, _, err := inspectCertificateFingerprint(certPEMNew, keyPEMNew)
	if err != nil {
		t.Fatalf("inspect new certificate failed: %v", err)
	}
	if newFingerprint == oldFingerprint {
		t.Fatal("test certificates unexpectedly have the same fingerprint")
	}
	updated, err := certificateInventory.Upsert(CertificateUpsertPayload{
		SourceType:      cert.SourceType,
		SourceRef:       cert.SourceRef,
		MainDomain:      cert.MainDomain,
		Domains:         []string{"example.com"},
		CertificateType: cert.CertificateType,
		CertPEM:         certPEMNew,
		KeyPEM:          keyPEMNew,
		FullchainPEM:    certPEMNew,
		Fingerprint:     newFingerprint,
		ListOrderAt:     cert.ListOrderAt,
		LastRenewedAt:   time.Now().Unix(),
	})
	if err != nil {
		t.Fatalf("update certificate inventory failed: %v", err)
	}
	warnings := (&AcmeService{}).applyCertificateRecordPostActions(updated, "", "", false)
	for _, warning := range warnings {
		if strings.Contains(warning, "刷新反向代理证书运行态") {
			t.Fatalf("certificate post action did not refresh reverse proxy runtime: %s", warning)
		}
	}

	if got := reverseProxyDialWithSNIFingerprint(t, listenPort, "example.com"); got != newFingerprint {
		t.Fatalf("reloaded sni certificate fingerprint = %q, want %q", got, newFingerprint)
	}
}

func TestReverseProxyCertificateDeletionStopsTLSListenerAndRemovesOption(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	t.Cleanup(func() {
		_ = svc.StopRuntime()
	})

	certRecordID := createReverseProxyTestCertificateRecord(t, "delete-runtime.example.com")
	var certificate model.CertificateRecord
	if err := database.GetDB().Where("id = ?", certRecordID).First(&certificate).Error; err != nil {
		t.Fatalf("load certificate record failed: %v", err)
	}
	listenPort := reserveReverseProxyTestPort(t)
	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:                      "certificate-delete-runtime",
		Enabled:                   true,
		ListenProtocol:            reverseProxyProtocolHTTPS,
		ListenHTTPVersionStrategy: reverseProxyListenHTTPVersionH2Only,

		ListenPort:          listenPort,
		Hosts:               "delete-runtime.example.com",
		TargetProtocol:      reverseProxyProtocolHTTP,
		TargetAddresses:     "127.0.0.1",
		TargetPort:          reserveReverseProxyTestPort(t),
		CertificateRecordID: certRecordID,
		IPStrategy:          reverseProxyIPStrategyPreferIPv4,
	}); err != nil {
		t.Fatalf("create certificate deletion runtime rule failed: %v", err)
	}

	if got := reverseProxyDialWithSNIFingerprint(t, listenPort, "delete-runtime.example.com"); got == "" {
		t.Fatal("TLS listener did not serve its initial certificate")
	}
	if _, err := (&AcmeService{}).Delete(AcmeDeletePayload{ID: certRecordID}); err != nil {
		t.Fatalf("delete referenced certificate failed: %v", err)
	}

	overview, err := svc.GetOverview()
	if err != nil {
		t.Fatalf("load overview after certificate deletion failed: %v", err)
	}
	for _, option := range overview.Certificates {
		if option.ID == certRecordID {
			t.Fatal("deleted certificate remained in reverse proxy certificate options")
		}
	}
	foundRule := false
	for _, rule := range overview.Rules {
		if rule.Name != "certificate-delete-runtime" {
			continue
		}
		foundRule = true
		if rule.Enabled || len(rule.CertificateRecordIDs) != 0 || rule.CertificateRecordID != 0 {
			t.Fatalf("rule without a remaining TLS certificate must be disabled and detached: %#v", rule)
		}
	}
	if !foundRule {
		t.Fatal("reverse proxy rule disappeared after certificate deletion")
	}

	conn, err := tls.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(listenPort)), &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         "delete-runtime.example.com",
	})
	if err == nil {
		_ = conn.Close()
		t.Fatal("deleted certificate listener remained reachable after runtime reconciliation")
	}
}

func TestComputeReverseProxyRenderKey_ChangesWhenCertificateOrderChanges(t *testing.T) {
	rows := []model.ReverseProxyRule{
		{
			Id:                    1,
			ListOrder:             1,
			Enabled:               true,
			ListenProtocol:        reverseProxyProtocolHTTPS,
			ListenPort:            8443,
			HostList:              `["example.com"]`,
			TargetProtocol:        reverseProxyProtocolHTTP,
			TargetAddresses:       `["127.0.0.1"]`,
			TargetPort:            8080,
			CertificateRecordID:   1,
			CertificateRecordList: `[1,2]`,
			IPStrategy:            reverseProxyIPStrategyPreferIPv4,
		},
	}

	base := computeReverseProxyRenderKey(nil, rows)
	rows[0].CertificateRecordList = `[2,1]`
	if got := computeReverseProxyRenderKey(nil, rows); got == base {
		t.Fatal("certificate order change should affect render key")
	}
}

func TestComputeReverseProxyRenderKeyChangesWhenDomainConditionChanges(t *testing.T) {
	rows := []model.ReverseProxyRule{
		{
			Id:                  1,
			ListOrder:           1,
			Enabled:             true,
			ListenProtocol:      reverseProxyProtocolHTTPS,
			ListenPort:          8443,
			HostList:            `["example.com"]`,
			TargetProtocol:      reverseProxyProtocolHTTP,
			TargetAddresses:     `["127.0.0.1"]`,
			TargetPort:          8080,
			CertificateRecordID: 1,
			IPStrategy:          reverseProxyIPStrategyPreferIPv4,
		},
	}

	base := computeReverseProxyRenderKey(nil, rows)
	rows[0].HostList = `["updated.example.com"]`
	if got := computeReverseProxyRenderKey(nil, rows); got == base {
		t.Fatal("domain condition change should affect render key")
	}
}

func TestReverseProxyOverviewIncludesAPIPassthrough(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	t.Cleanup(func() {
		_ = svc.StopRuntime()
	})

	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:           "api-passthrough-overview",
		Enabled:        true,
		ListenProtocol: reverseProxyProtocolHTTP,

		ListenPort:      reserveReverseProxyTestPort(t),
		Hosts:           "example.com",
		TargetProtocol:  reverseProxyProtocolHTTP,
		TargetAddresses: "127.0.0.1",
		TargetPort:      18080,
		IPStrategy:      reverseProxyIPStrategyPreferIPv4,
		ApiPassthrough:  true,
	}); err != nil {
		t.Fatalf("upsert api passthrough rule failed: %v", err)
	}

	var saved model.ReverseProxyRule
	if err := database.GetDB().Where("name = ?", "api-passthrough-overview").First(&saved).Error; err != nil {
		t.Fatalf("load api passthrough rule failed: %v", err)
	}
	if !saved.ApiPassthrough {
		t.Fatalf("expected saved api passthrough=true, got %#v", saved.ApiPassthrough)
	}

	overview, err := svc.GetOverview()
	if err != nil {
		t.Fatalf("get reverse proxy overview failed: %v", err)
	}
	if len(overview.Rules) != 1 {
		t.Fatalf("expected exactly one overview rule, got %d", len(overview.Rules))
	}
	if !overview.Rules[0].ApiPassthrough {
		t.Fatalf("expected overview apiPassthrough=true, got %#v", overview.Rules[0].ApiPassthrough)
	}
}

func TestReverseProxyOverviewPreservesWSWSSAliases(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	t.Cleanup(func() {
		_ = svc.StopRuntime()
	})

	httpsListenPort := reserveReverseProxyTestPort(t)
	httpsCertRecordID := createReverseProxyTestCertificateRecord(t, "example.com")
	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:                      "listen-wss-target-ws",
		Enabled:                   true,
		ListenProtocol:            reverseProxyProtocolHTTPS,
		ListenProtocolAlias:       "wss",
		ListenPort:                httpsListenPort,
		Hosts:                     "example.com",
		TargetProtocol:            reverseProxyProtocolHTTP,
		TargetProtocolAlias:       "ws",
		TargetAddresses:           "127.0.0.1",
		TargetPort:                18080,
		CertificateRecordID:       httpsCertRecordID,
		ListenHTTPVersionStrategy: reverseProxyListenHTTPVersionH2H3,
		IPStrategy:                reverseProxyIPStrategyPreferIPv4,
		UpstreamTLSVerify:         false,
	}); err != nil {
		t.Fatalf("upsert wss->ws alias rule failed: %v", err)
	}

	httpListenPort := reserveReverseProxyTestPort(t)
	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:                "listen-ws-target-wss",
		Enabled:             true,
		ListenProtocol:      reverseProxyProtocolHTTP,
		ListenProtocolAlias: "ws",
		ListenPort:          httpListenPort,
		Hosts:               "example.net",
		TargetProtocol:      reverseProxyProtocolHTTPS,
		TargetProtocolAlias: "wss",
		TargetAddresses:     "127.0.0.1",
		TargetPort:          18443,
		IPStrategy:          reverseProxyIPStrategyPreferIPv4,
		HTTPVersionStrategy: reverseProxyHTTPVersionPreferH2,
		UpstreamTLSVerify:   true,
	}); err != nil {
		t.Fatalf("upsert ws->wss alias rule failed: %v", err)
	}

	type aliasRow struct {
		Name                string
		ListenProtocol      string
		ListenProtocolAlias string
		TargetProtocol      string
		TargetProtocolAlias string
	}
	var rows []aliasRow
	if err := database.GetDB().
		Model(&model.ReverseProxyRule{}).
		Select("name, listen_protocol, listen_protocol_alias, target_protocol, target_protocol_alias").
		Order("id asc").
		Find(&rows).Error; err != nil {
		t.Fatalf("query alias persistence rows failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected two alias rows, got %d (%#v)", len(rows), rows)
	}
	if rows[0].ListenProtocolAlias != "wss" || rows[0].TargetProtocolAlias != "ws" {
		t.Fatalf("unexpected first alias row: %#v", rows[0])
	}
	if rows[1].ListenProtocolAlias != "ws" || rows[1].TargetProtocolAlias != "wss" {
		t.Fatalf("unexpected second alias row: %#v", rows[1])
	}

	overview, err := svc.GetOverview()
	if err != nil {
		t.Fatalf("get reverse proxy overview failed: %v", err)
	}
	if len(overview.Rules) != 2 {
		t.Fatalf("expected two overview rules, got %d", len(overview.Rules))
	}
	byName := make(map[string]ReverseProxyRuleView, len(overview.Rules))
	for _, rule := range overview.Rules {
		byName[rule.Name] = rule
	}

	first, ok := byName["listen-wss-target-ws"]
	if !ok {
		t.Fatalf("overview missing first alias rule: %#v", overview.Rules)
	}
	if first.ListenProtocolAlias != "wss" || first.TargetProtocolAlias != "ws" {
		t.Fatalf("unexpected first overview alias fields: %#v", first)
	}
	if first.ListenProtocol != reverseProxyProtocolHTTPS || first.TargetProtocol != reverseProxyProtocolHTTP {
		t.Fatalf("unexpected first overview protocols: %#v", first)
	}

	second, ok := byName["listen-ws-target-wss"]
	if !ok {
		t.Fatalf("overview missing second alias rule: %#v", overview.Rules)
	}
	if second.ListenProtocolAlias != "ws" || second.TargetProtocolAlias != "wss" {
		t.Fatalf("unexpected second overview alias fields: %#v", second)
	}
	if second.ListenProtocol != reverseProxyProtocolHTTP || second.TargetProtocol != reverseProxyProtocolHTTPS {
		t.Fatalf("unexpected second overview protocols: %#v", second)
	}
}

func TestReverseProxyStartRuntimeResetsStaleRuntimeState(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "reverse-proxy-start-runtime.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	if sqlDB, err := database.GetDB().DB(); err == nil && sqlDB != nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}

	svc := &ReverseProxyService{}
	t.Cleanup(func() {
		_ = svc.StopRuntime()
	})

	listenPort := reserveReverseProxyTestPort(t)
	row := model.ReverseProxyRule{
		DisplayID:      1,
		ListOrder:      1,
		Name:           "startup-reset",
		Enabled:        true,
		ListenProtocol: reverseProxyProtocolHTTP,

		ListenPort:          listenPort,
		HostList:            `["example.com"]`,
		PathPrefix:          "/",
		TargetProtocol:      reverseProxyProtocolHTTP,
		TargetAddresses:     `["127.0.0.1"]`,
		TargetPort:          8080,
		TargetPath:          "/",
		IPStrategy:          reverseProxyIPStrategyPreferIPv4,
		HTTPVersionStrategy: reverseProxyHTTPVersionPreferH3,
		UpstreamTLSVerify:   false,
		LastError:           "stale error",
		RuntimeStatus:       "proxy_error",
	}
	if err := database.GetDB().Create(&row).Error; err != nil {
		t.Fatalf("create reverse proxy rule failed: %v", err)
	}

	if err := svc.StartRuntime(); err != nil {
		t.Fatalf("start runtime failed: %v", err)
	}

	var reloaded model.ReverseProxyRule
	if err := database.GetDB().Where("id = ?", row.Id).First(&reloaded).Error; err != nil {
		t.Fatalf("reload reverse proxy rule failed: %v", err)
	}
	if reloaded.LastError != "" {
		t.Fatalf("expected last_error to be cleared, got %q", reloaded.LastError)
	}
	if reloaded.RuntimeStatus != "pending" {
		t.Fatalf("expected runtime_status to be pending, got %q", reloaded.RuntimeStatus)
	}
}

func TestReverseProxyGroupRulesGroupsAllRulesOnWildcardPort(t *testing.T) {
	rows := []model.ReverseProxyRule{
		{
			Id:             1,
			ListOrder:      1,
			Enabled:        true,
			ListenProtocol: "http",
			ListenPort:     18080,
			PathPrefix:     "/a",
		},
		{
			Id:             2,
			ListOrder:      2,
			Enabled:        true,
			ListenProtocol: "http",
			ListenPort:     18080,
			PathPrefix:     "/b",
		},
	}

	grouped := reverseProxyGroupRules(rows)
	if len(grouped) != 1 || len(grouped["http|18080|tcp|0.0.0.0,::"]) != 2 {
		t.Fatalf("expected one wildcard listener group, got %#v", grouped)
	}
}

func TestReverseProxyGroupRulesSkipsNonHTTPDNSListeners(t *testing.T) {
	rows := []model.ReverseProxyRule{
		{
			Id:                  1,
			ListOrder:           1,
			Enabled:             true,
			ListenProtocol:      reverseProxyProtocolDNS,
			ListenProtocolAlias: reverseProxyDNSProtocolUDP,
			ListenPort:          53,
		},
		{
			Id:             2,
			ListOrder:      2,
			Enabled:        true,
			ListenProtocol: reverseProxyProtocolHTTP,
			ListenPort:     8080,
		},
	}

	grouped := reverseProxyGroupRules(rows)
	if len(grouped) != 1 {
		t.Fatalf("expected dns listener to be skipped from http runtime groups, got %#v", grouped)
	}
	if _, exists := grouped["dns|53"]; exists {
		t.Fatalf("dns listener should not be grouped into http runtime: %#v", grouped)
	}
}

func TestReverseProxyGroupRulesSharesDoHWithHTTPSAndDoH3WithH3(t *testing.T) {
	rows := []model.ReverseProxyRule{
		{Id: 1, Enabled: true, ListenProtocol: reverseProxyProtocolHTTPS, ListenHTTPVersionStrategy: reverseProxyListenHTTPVersionH2Only, ListenPort: 443},
		{Id: 2, Enabled: true, ListenProtocol: reverseProxyProtocolDNS, ListenProtocolAlias: reverseProxyDNSProtocolDoH, ListenPort: 443},
		{Id: 3, Enabled: true, ListenProtocol: reverseProxyProtocolHTTPS, ListenHTTPVersionStrategy: reverseProxyListenHTTPVersionH3Only, ListenPort: 8443},
		{Id: 4, Enabled: true, ListenProtocol: reverseProxyProtocolDNS, ListenProtocolAlias: reverseProxyDNSProtocolDoHH3, ListenPort: 8443},
	}
	grouped := reverseProxyGroupRules(rows)
	if got := grouped["https|443|tcp|0.0.0.0,::"]; len(got) != 2 {
		t.Fatalf("DoH and H2 must share one TCP listener group: %#v", grouped)
	}
	if got := grouped["https|8443|udp|0.0.0.0,::"]; len(got) != 2 {
		t.Fatalf("DoH3 and H3 must share one UDP listener group: %#v", grouped)
	}
}

func TestUpsertDoHRuleSyncsSharedHTTPRuntimeImmediately(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	t.Cleanup(func() {
		_ = svc.StopRuntime()
	})

	listenPort := reserveReverseProxyTestPort(t)
	certID := createReverseProxyTestCertificateRecord(t, "dns-runtime.example.com")

	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:                "dns-runtime-sync",
		Enabled:             true,
		ListenProtocol:      reverseProxyDNSProtocolDoH,
		ListenPort:          listenPort,
		Hosts:               "dns-runtime.example.com",
		ListenDNSPath:       "/dns-query",
		TargetProtocol:      reverseProxyDNSProtocolUDP,
		TargetAddresses:     "1.1.1.1",
		TargetPort:          53,
		CertificateRecordID: certID,
		IPStrategy:          reverseProxyIPStrategyPreferIPv4,
		DNSAllowedCIDRs:     "127.0.0.0/8",
	}); err != nil {
		t.Fatalf("upsert dns rule failed: %v", err)
	}

	var row model.ReverseProxyRule
	if err := database.GetDB().Where("name = ?", "dns-runtime-sync").First(&row).Error; err != nil {
		t.Fatalf("load saved DoH rule failed: %v", err)
	}
	key := reverseProxyListenerGroupKey(&row, reverseProxySocketKindTCP)
	reverseProxyRuntime.mu.Lock()
	group := reverseProxyRuntime.groups[key]
	reverseProxyRuntime.mu.Unlock()
	if group == nil || group.dnsHandler == nil {
		t.Fatalf("expected DoH to start in the shared HTTP listener group %q", key)
	}
	reverseProxyDNSRuntime.mu.Lock()
	standaloneCount := len(reverseProxyDNSRuntime.running)
	reverseProxyDNSRuntime.mu.Unlock()
	if standaloneCount != 0 {
		t.Fatalf("DoH must not start a standalone DNS listener, got %d instances", standaloneCount)
	}
}

func TestReverseProxyDoHAndH2ShareListenerAndRouteByPath(t *testing.T) {
	openReverseProxyTestDB(t)
	svc := &ReverseProxyService{}
	t.Cleanup(func() { _ = svc.StopRuntime() })

	dnsPort, dnsHits := startReverseProxyTestDNSServer(t, 60, "203.0.113.60")
	httpUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("http:" + r.URL.Path))
	}))
	defer httpUpstream.Close()
	httpHost, httpPort := splitReverseProxyTestServerAddress(t, httpUpstream.URL)
	listenPort := reserveReverseProxyTestPort(t)
	certificateID := createReverseProxyTestCertificateRecord(t, "shared.example.com")

	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name: "shared-h2", Enabled: true, ListenProtocol: reverseProxyProtocolHTTPS,
		ListenHTTPVersionStrategy: reverseProxyListenHTTPVersionH2Only, ListenPort: listenPort,
		Hosts: "shared.example.com", PathPrefix: "/app", TargetProtocol: reverseProxyProtocolHTTP,
		TargetAddresses: httpHost, TargetPort: httpPort, CertificateRecordID: certificateID,
		IPStrategy: reverseProxyIPStrategyPreferIPv4,
	}); err != nil {
		t.Fatalf("create shared H2 rule failed: %v", err)
	}
	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name: "shared-doh", Enabled: true, ListenProtocol: reverseProxyDNSProtocolDoH,
		ListenPort: listenPort, Hosts: "shared.example.com", ListenDNSPath: "/dns-query",
		TargetProtocol: reverseProxyDNSProtocolUDP, TargetAddresses: "127.0.0.1", TargetPort: dnsPort,
		CertificateRecordID: certificateID, IPStrategy: reverseProxyIPStrategyPreferIPv4,
		DNSAllowedCIDRs: "127.0.0.0/8",
	}); err != nil {
		t.Fatalf("create shared DoH rule failed: %v", err)
	}

	client := &http.Client{Transport: &http.Transport{
		Proxy: nil, ForceAttemptHTTP2: true,
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true, ServerName: "shared.example.com"},
	}, Timeout: 15 * time.Second}
	httpRequest, err := http.NewRequest(http.MethodGet, "https://127.0.0.1:"+strconv.Itoa(listenPort)+"/app/value", nil)
	if err != nil {
		t.Fatalf("build shared H2 request failed: %v", err)
	}
	httpRequest.Host = "shared.example.com"
	httpResponse, err := client.Do(httpRequest)
	if err != nil {
		t.Fatalf("shared H2 request failed: %v", err)
	}
	httpBody, _ := io.ReadAll(httpResponse.Body)
	_ = httpResponse.Body.Close()
	if httpResponse.StatusCode != http.StatusOK || string(httpBody) != "http:/value" {
		t.Fatalf("unexpected shared H2 response: status=%d body=%q", httpResponse.StatusCode, string(httpBody))
	}

	query := new(dns.Msg)
	query.SetQuestion("shared.example.", dns.TypeA)
	wire, err := query.Pack()
	if err != nil {
		t.Fatalf("pack DoH query failed: %v", err)
	}
	dohRequest, err := http.NewRequest(http.MethodPost, "https://127.0.0.1:"+strconv.Itoa(listenPort)+"/dns-query", bytes.NewReader(wire))
	if err != nil {
		t.Fatalf("build shared DoH request failed: %v", err)
	}
	dohRequest.Host = "shared.example.com"
	dohRequest.Header.Set("Content-Type", "application/dns-message")
	dohResponse, err := client.Do(dohRequest)
	if err != nil {
		t.Fatalf("shared DoH request failed: %v", err)
	}
	dohWire, _ := io.ReadAll(dohResponse.Body)
	_ = dohResponse.Body.Close()
	message := new(dns.Msg)
	if err := message.Unpack(dohWire); err != nil {
		t.Fatalf("unpack shared DoH response failed: status=%d body=%q err=%v", dohResponse.StatusCode, string(dohWire), err)
	}
	if dohResponse.StatusCode != http.StatusOK || len(message.Answer) != 1 || atomic.LoadInt32(dnsHits) != 1 {
		t.Fatalf("unexpected shared DoH response: status=%d answers=%d hits=%d", dohResponse.StatusCode, len(message.Answer), atomic.LoadInt32(dnsHits))
	}

	reverseProxyRuntime.mu.Lock()
	groupCount := len(reverseProxyRuntime.groups)
	reverseProxyRuntime.mu.Unlock()
	reverseProxyDNSRuntime.mu.Lock()
	standaloneCount := len(reverseProxyDNSRuntime.running)
	reverseProxyDNSRuntime.mu.Unlock()
	if groupCount != 1 || standaloneCount != 0 {
		t.Fatalf("shared DoH/H2 must own one HTTP group and no standalone DNS listener: http=%d dns=%d", groupCount, standaloneCount)
	}
}

func TestReverseProxyDoH3AndH3ShareListenerAndRouteByPath(t *testing.T) {
	openReverseProxyTestDB(t)
	svc := &ReverseProxyService{}
	t.Cleanup(func() { _ = svc.StopRuntime() })

	dnsPort, dnsHits := startReverseProxyTestDNSServer(t, 60, "203.0.113.61")
	httpUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("h3:" + r.URL.Path))
	}))
	defer httpUpstream.Close()
	httpHost, httpPort := splitReverseProxyTestServerAddress(t, httpUpstream.URL)
	listenPort := reserveReverseProxyTestUDPPort(t)
	certificateID := createReverseProxyTestCertificateRecord(t, "shared-h3.example.com")

	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name: "shared-h3", Enabled: true, ListenProtocol: reverseProxyProtocolHTTPS,
		ListenHTTPVersionStrategy: reverseProxyListenHTTPVersionH3Only, ListenPort: listenPort,
		Hosts: "shared-h3.example.com", PathPrefix: "/app", TargetProtocol: reverseProxyProtocolHTTP,
		TargetAddresses: httpHost, TargetPort: httpPort, CertificateRecordID: certificateID,
		IPStrategy: reverseProxyIPStrategyPreferIPv4,
	}); err != nil {
		t.Fatalf("create shared H3 rule failed: %v", err)
	}
	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name: "shared-doh3", Enabled: true, ListenProtocol: reverseProxyDNSProtocolDoHH3,
		ListenPort: listenPort, Hosts: "shared-h3.example.com", ListenDNSPath: "/dns-query",
		TargetProtocol: reverseProxyDNSProtocolUDP, TargetAddresses: "127.0.0.1", TargetPort: dnsPort,
		CertificateRecordID: certificateID, IPStrategy: reverseProxyIPStrategyPreferIPv4,
		DNSAllowedCIDRs: "127.0.0.0/8",
	}); err != nil {
		t.Fatalf("create shared DoH3 rule failed: %v", err)
	}

	transport := &http3.Transport{TLSClientConfig: &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         "shared-h3.example.com",
	}}
	defer func() { _ = transport.Close() }()
	client := &http.Client{Transport: transport, Timeout: 15 * time.Second}
	httpRequest, err := http.NewRequest(http.MethodGet, "https://127.0.0.1:"+strconv.Itoa(listenPort)+"/app/value", nil)
	if err != nil {
		t.Fatalf("build shared H3 request failed: %v", err)
	}
	httpRequest.Host = "shared-h3.example.com"
	httpResponse, err := client.Do(httpRequest)
	if err != nil {
		t.Fatalf("shared H3 request failed: %v", err)
	}
	httpBody, _ := io.ReadAll(httpResponse.Body)
	_ = httpResponse.Body.Close()
	if httpResponse.StatusCode != http.StatusOK || string(httpBody) != "h3:/value" {
		t.Fatalf("unexpected shared H3 response: status=%d body=%q", httpResponse.StatusCode, string(httpBody))
	}

	query := new(dns.Msg)
	query.SetQuestion("shared-h3.example.", dns.TypeA)
	wire, err := query.Pack()
	if err != nil {
		t.Fatalf("pack DoH3 query failed: %v", err)
	}
	dohRequest, err := http.NewRequest(http.MethodPost, "https://127.0.0.1:"+strconv.Itoa(listenPort)+"/dns-query", bytes.NewReader(wire))
	if err != nil {
		t.Fatalf("build shared DoH3 request failed: %v", err)
	}
	dohRequest.Host = "shared-h3.example.com"
	dohRequest.Header.Set("Content-Type", "application/dns-message")
	dohResponse, err := client.Do(dohRequest)
	if err != nil {
		t.Fatalf("shared DoH3 request failed: %v", err)
	}
	dohWire, _ := io.ReadAll(dohResponse.Body)
	_ = dohResponse.Body.Close()
	message := new(dns.Msg)
	if err := message.Unpack(dohWire); err != nil {
		t.Fatalf("unpack shared DoH3 response failed: status=%d body=%q err=%v", dohResponse.StatusCode, string(dohWire), err)
	}
	if dohResponse.StatusCode != http.StatusOK || len(message.Answer) != 1 || atomic.LoadInt32(dnsHits) != 1 {
		t.Fatalf("unexpected shared DoH3 response: status=%d answers=%d hits=%d", dohResponse.StatusCode, len(message.Answer), atomic.LoadInt32(dnsHits))
	}

	reverseProxyRuntime.mu.Lock()
	groupCount := len(reverseProxyRuntime.groups)
	reverseProxyRuntime.mu.Unlock()
	reverseProxyDNSRuntime.mu.Lock()
	standaloneCount := len(reverseProxyDNSRuntime.running)
	reverseProxyDNSRuntime.mu.Unlock()
	if groupCount != 1 || standaloneCount != 0 {
		t.Fatalf("shared DoH3/H3 must own one HTTP group and no standalone DNS listener: http=%d dns=%d", groupCount, standaloneCount)
	}
}

func TestReverseProxyHTTPRuntimeRebindsOverlappingListenIPChange(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("rebound"))
	}))
	defer upstream.Close()
	upstreamHost, upstreamPort := splitReverseProxyTestServerAddress(t, upstream.URL)
	listenPort := reserveReverseProxyTestPort(t)

	revision, err := svc.CurrentRevision()
	if err != nil {
		t.Fatalf("load initial revision failed: %v", err)
	}
	if err := svc.UpsertRule(ReverseProxyRulePayload{
		ExpectedRevision: &revision,
		Name:             "http-overlapping-rebind",
		Enabled:          true,
		ListenProtocol:   reverseProxyProtocolHTTP,

		ListenPort:      listenPort,
		TargetProtocol:  reverseProxyProtocolHTTP,
		TargetAddresses: upstreamHost,
		TargetPort:      upstreamPort,
		IPStrategy:      reverseProxyIPStrategyPreferIPv4,
	}); err != nil {
		t.Fatalf("create wildcard http listener failed: %v", err)
	}

	previous := model.ReverseProxyRule{}
	if err := database.GetDB().Where("name = ?", "http-overlapping-rebind").First(&previous).Error; err != nil {
		t.Fatalf("load wildcard http rule failed: %v", err)
	}
	previousKey := reverseProxyListenerGroupKey(&previous, reverseProxySocketKindTCP)
	revision, err = svc.CurrentRevision()
	if err != nil {
		t.Fatalf("load rebind revision failed: %v", err)
	}
	if err := svc.UpsertRule(ReverseProxyRulePayload{
		ExpectedRevision: &revision,
		ID:               previous.Id,
		Name:             previous.Name,
		Enabled:          true,
		ListenProtocol:   reverseProxyProtocolHTTP,

		ListenPort:      listenPort,
		TargetProtocol:  reverseProxyProtocolHTTP,
		TargetAddresses: upstreamHost,
		TargetPort:      upstreamPort,
		IPStrategy:      reverseProxyIPStrategyPreferIPv4,
	}); err != nil {
		t.Fatalf("rebind http listener failed: %v", err)
	}

	current := model.ReverseProxyRule{}
	if err := database.GetDB().Where("id = ?", previous.Id).First(&current).Error; err != nil {
		t.Fatalf("load rebound http rule failed: %v", err)
	}
	currentKey := reverseProxyListenerGroupKey(&current, reverseProxySocketKindTCP)
	reverseProxyRuntime.mu.Lock()
	_, hasCurrent := reverseProxyRuntime.groups[currentKey]
	_, hasPrevious := reverseProxyRuntime.groups[previousKey]
	reverseProxyRuntime.mu.Unlock()
	if !hasCurrent || hasPrevious {
		t.Fatalf("http runtime did not replace overlapping binding: current=%q present=%v previous=%q present=%v", currentKey, hasCurrent, previousKey, hasPrevious)
	}

	client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 5 * time.Second}
	response, err := client.Get("http://127.0.0.1:" + strconv.Itoa(listenPort) + "/")
	if err != nil {
		t.Fatalf("request rebound http listener failed: %v", err)
	}
	body, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || string(body) != "rebound" {
		t.Fatalf("unexpected rebound http response: status=%d body=%q", response.StatusCode, string(body))
	}
}

func TestReverseProxyDNSRuntimeRebindsOverlappingListenIPChange(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	upstreamPort, _ := startReverseProxyTestDNSServer(t, 30, "203.0.113.90")
	listenPort := reserveReverseProxyTestPort(t)
	revision, err := svc.CurrentRevision()
	if err != nil {
		t.Fatalf("load initial revision failed: %v", err)
	}
	if err := svc.UpsertRule(ReverseProxyRulePayload{
		ExpectedRevision: &revision,
		Name:             "dns-overlapping-rebind",
		Enabled:          true,
		ListenProtocol:   reverseProxyDNSProtocolTCP,

		ListenPort:      listenPort,
		TargetProtocol:  reverseProxyDNSProtocolUDP,
		TargetAddresses: "127.0.0.1",
		TargetPort:      upstreamPort,
		DNSAllowedCIDRs: "127.0.0.0/8",
		IPStrategy:      reverseProxyIPStrategyPreferIPv4,
	}); err != nil {
		t.Fatalf("create wildcard dns listener failed: %v", err)
	}

	previous := model.ReverseProxyRule{}
	if err := database.GetDB().Where("name = ?", "dns-overlapping-rebind").First(&previous).Error; err != nil {
		t.Fatalf("load wildcard dns rule failed: %v", err)
	}
	previousKey := reverseProxyDNSInstanceKey(&previous)
	revision, err = svc.CurrentRevision()
	if err != nil {
		t.Fatalf("load dns rebind revision failed: %v", err)
	}
	if err := svc.UpsertRule(ReverseProxyRulePayload{
		ExpectedRevision: &revision,
		ID:               previous.Id,
		Name:             previous.Name,
		Enabled:          true,
		ListenProtocol:   reverseProxyDNSProtocolTCP,

		ListenPort:      listenPort,
		TargetProtocol:  reverseProxyDNSProtocolUDP,
		TargetAddresses: "127.0.0.1",
		TargetPort:      upstreamPort,
		DNSAllowedCIDRs: "127.0.0.0/8",
		IPStrategy:      reverseProxyIPStrategyPreferIPv4,
	}); err != nil {
		t.Fatalf("rebind dns listener failed: %v", err)
	}

	current := model.ReverseProxyRule{}
	if err := database.GetDB().Where("id = ?", previous.Id).First(&current).Error; err != nil {
		t.Fatalf("load rebound dns rule failed: %v", err)
	}
	currentKey := reverseProxyDNSInstanceKey(&current)
	reverseProxyDNSRuntime.mu.Lock()
	_, hasCurrent := reverseProxyDNSRuntime.running[currentKey]
	_, hasPrevious := reverseProxyDNSRuntime.running[previousKey]
	reverseProxyDNSRuntime.mu.Unlock()
	if !hasCurrent || hasPrevious {
		t.Fatalf("dns runtime did not replace overlapping binding: current=%q present=%v previous=%q present=%v", currentKey, hasCurrent, previousKey, hasPrevious)
	}
}

func TestReverseProxyRuntimeSwitchesDNSToHTTPOnSameSocketImmediately(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	dnsUpstreamPort, _ := startReverseProxyTestDNSServer(t, 30, "203.0.113.91")
	httpUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("dns-to-http"))
	}))
	defer httpUpstream.Close()
	httpUpstreamHost, httpUpstreamPort := splitReverseProxyTestServerAddress(t, httpUpstream.URL)
	listenPort := reserveReverseProxyTestPort(t)

	revision, err := svc.CurrentRevision()
	if err != nil {
		t.Fatalf("load initial revision failed: %v", err)
	}
	if err := svc.UpsertRule(ReverseProxyRulePayload{
		ExpectedRevision: &revision,
		Name:             "dns-to-http-same-socket",
		Enabled:          true,
		ListenProtocol:   reverseProxyDNSProtocolTCP,

		ListenPort:      listenPort,
		TargetProtocol:  reverseProxyDNSProtocolUDP,
		TargetAddresses: "127.0.0.1",
		TargetPort:      dnsUpstreamPort,
		IPStrategy:      reverseProxyIPStrategyPreferIPv4,
	}); err != nil {
		t.Fatalf("create dns listener failed: %v", err)
	}

	previous := model.ReverseProxyRule{}
	if err := database.GetDB().Where("name = ?", "dns-to-http-same-socket").First(&previous).Error; err != nil {
		t.Fatalf("load dns rule failed: %v", err)
	}
	revision, err = svc.CurrentRevision()
	if err != nil {
		t.Fatalf("load protocol switch revision failed: %v", err)
	}
	if err := svc.UpsertRule(ReverseProxyRulePayload{
		ExpectedRevision: &revision,
		ID:               previous.Id,
		Name:             previous.Name,
		Enabled:          true,
		ListenProtocol:   reverseProxyProtocolHTTP,

		ListenPort:      listenPort,
		TargetProtocol:  reverseProxyProtocolHTTP,
		TargetAddresses: httpUpstreamHost,
		TargetPort:      httpUpstreamPort,
		IPStrategy:      reverseProxyIPStrategyPreferIPv4,
	}); err != nil {
		t.Fatalf("switch dns listener to http failed: %v", err)
	}

	current := model.ReverseProxyRule{}
	if err := database.GetDB().Where("id = ?", previous.Id).First(&current).Error; err != nil {
		t.Fatalf("load switched http rule failed: %v", err)
	}
	currentKey := reverseProxyListenerGroupKey(&current, reverseProxySocketKindTCP)
	reverseProxyRuntime.mu.Lock()
	_, hasHTTP := reverseProxyRuntime.groups[currentKey]
	pendingHTTP := reverseProxyRuntime.state.lastRenderKey == ""
	reverseProxyRuntime.mu.Unlock()
	reverseProxyDNSRuntime.mu.Lock()
	dnsCount := len(reverseProxyDNSRuntime.running)
	reverseProxyDNSRuntime.mu.Unlock()
	if !hasHTTP || pendingHTTP || dnsCount != 0 {
		t.Fatalf("dns to http switch was not applied immediately: http=%v pending=%v dnsInstances=%d", hasHTTP, pendingHTTP, dnsCount)
	}

	client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 5 * time.Second}
	response, err := client.Get("http://127.0.0.1:" + strconv.Itoa(listenPort) + "/")
	if err != nil {
		t.Fatalf("request switched http listener failed: %v", err)
	}
	body, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || string(body) != "dns-to-http" {
		t.Fatalf("unexpected switched http response: status=%d body=%q", response.StatusCode, string(body))
	}
}

func TestReverseProxyRuleServerNameMatch_UsesSNINameList(t *testing.T) {
	rule := &model.ReverseProxyRule{

		HostList: `["example.com"]`,
	}
	if reverseProxyRuleServerNameMatch(rule, "1.2.3.4") {
		t.Fatal("ip should bypass domain sni matching instead of counting as configured sni")
	}
	if !reverseProxyRuleServerNameMatch(rule, "example.com") {
		t.Fatal("host should be accepted as sni")
	}
	if reverseProxyRuleServerNameMatch(rule, "other.example.com") {
		t.Fatal("unexpected sni match")
	}
}

func TestReverseProxyRuleNamesAndPathsOverlap(t *testing.T) {
	existing := &model.ReverseProxyRule{

		HostList:   `["example.com"]`,
		PathPrefix: "",
	}
	if reverseProxyRuleNamesOverlap(reverseProxyRuleServerNames(existing), []string{"1.2.3.4"}) {
		t.Fatal("ip should not be treated as an explicit configured domain overlap")
	}
	if !reverseProxyRuleNamesOverlap(reverseProxyRuleServerNames(existing), []string{"example.com"}) {
		t.Fatal("host sni should overlap")
	}
	if !reverseProxyRulePathsOverlap(existing.PathPrefix, "/api") {
		t.Fatal("empty url rule should overlap a concrete url")
	}
	if !reverseProxyRulePathsOverlap("/api", "/api/child") {
		t.Fatal("ancestor prefix should overlap descendant prefix")
	}
	if !reverseProxyRulePathsOverlap("/api/", "/api") {
		t.Fatal("trailing slash should normalize to the same prefix")
	}
	if reverseProxyRulePathsOverlap("/api", "/apix") {
		t.Fatal("sibling prefix must not overlap")
	}
	if reverseProxyRulePathsOverlap("/api", "/aaa") {
		t.Fatal("different concrete prefixes must not overlap")
	}
}

func TestReverseProxyGetCertificateRejectsUnknownAndMissingSNI(t *testing.T) {
	cert := &tls.Certificate{}
	binding := &reverseProxyRuleCertificateBinding{
		RuleID:              1,
		CertificateRecordID: 1,
		Certificate:         cert,
		Leaf:                reverseProxyTestLeafState("example.com", "1.2.3.4"),
	}
	group := &reverseProxyListenerGroup{
		rules: []*model.ReverseProxyRule{
			{
				Id:       1,
				HostList: `["example.com"]`,
			},
		},
		certBindingsByRule: map[uint][]*reverseProxyRuleCertificateBinding{
			1: {binding},
		},
		orderedCertBindings: []*reverseProxyRuleCertificateBinding{binding},
	}
	group.configureIPCertificateIndexesLocked()

	got, err := group.getCertificate(&tls.ClientHelloInfo{ServerName: "example.com"})
	if err != nil || got != cert {
		t.Fatalf("expected matching host sni certificate, got cert=%v err=%v", got, err)
	}
	got, err = group.getCertificate(&tls.ClientHelloInfo{
		ServerName: "1.2.3.4",
		Conn: reverseProxyTestConn{
			local: &net.TCPAddr{IP: net.ParseIP("1.2.3.4"), Port: 443},
		},
	})
	if err != nil || got != cert {
		t.Fatalf("expected matching ip sni certificate, got cert=%v err=%v", got, err)
	}
	got, err = group.getCertificate(&tls.ClientHelloInfo{ServerName: "other.example.com"})
	if err == nil || got != nil {
		t.Fatalf("unknown sni must be rejected, got cert=%v err=%v", got, err)
	}
	got, err = group.getCertificate(&tls.ClientHelloInfo{})
	if err == nil || got != nil {
		t.Fatalf("missing sni without a local target must be rejected, got cert=%v err=%v", got, err)
	}
	got, err = group.getCertificate(nil)
	if err == nil || got != nil {
		t.Fatalf("nil client hello must be rejected, got cert=%v err=%v", got, err)
	}
}

func TestReverseProxyGetCertificateRejectsDomainSNIForEmptyDomainCondition(t *testing.T) {
	cert := &tls.Certificate{}
	binding := &reverseProxyRuleCertificateBinding{
		RuleID:              1,
		CertificateRecordID: 1,
		Certificate:         cert,
		Leaf:                reverseProxyTestLeafState("example.com"),
	}
	group := &reverseProxyListenerGroup{
		rules: []*model.ReverseProxyRule{
			{
				Id:         1,
				PathPrefix: "/",
			},
		},
		certBindingsByRule: map[uint][]*reverseProxyRuleCertificateBinding{
			1: {binding},
		},
		orderedCertBindings: []*reverseProxyRuleCertificateBinding{binding},
	}

	got, err := group.getCertificate(&tls.ClientHelloInfo{})
	if err == nil || got != nil {
		t.Fatalf("missing sni must be rejected even with an empty listen match, got cert=%v err=%v", got, err)
	}
	got, err = group.getCertificate(&tls.ClientHelloInfo{ServerName: "example.com"})
	if err == nil || got != nil {
		t.Fatalf("empty domain condition must reject every domain sni, got cert=%v err=%v", got, err)
	}
}

func TestReverseProxyDomainRuleAlsoRequiresCertificateDNSSANCoverage(t *testing.T) {
	wildcardCertificate := &tls.Certificate{}
	wildcardBinding := &reverseProxyRuleCertificateBinding{
		RuleID: 1, CertificateRecordID: 1, Certificate: wildcardCertificate,
		Leaf: reverseProxyTestLeafState("*.aa.cc"),
	}
	rule := &model.ReverseProxyRule{Id: 1, HostList: `["z.a.aa.cc"]`}
	group := &reverseProxyListenerGroup{
		rules: []*model.ReverseProxyRule{rule},
		certBindingsByRule: map[uint][]*reverseProxyRuleCertificateBinding{
			1: {wildcardBinding},
		},
		orderedCertBindings: []*reverseProxyRuleCertificateBinding{wildcardBinding},
	}
	if got, err := group.getCertificate(&tls.ClientHelloInfo{ServerName: "z.a.aa.cc"}); err == nil || got != nil {
		t.Fatalf("*.aa.cc certificate must not cover two-label z.a.aa.cc: cert=%v err=%v", got, err)
	}
	exactCertificate := &tls.Certificate{}
	exactBinding := &reverseProxyRuleCertificateBinding{
		RuleID: 1, CertificateRecordID: 2, Certificate: exactCertificate,
		Leaf: reverseProxyTestLeafState("z.a.aa.cc"),
	}
	group.certBindingsByRule[1] = []*reverseProxyRuleCertificateBinding{exactBinding}
	group.orderedCertBindings = []*reverseProxyRuleCertificateBinding{exactBinding}
	if got, err := group.getCertificate(&tls.ClientHelloInfo{ServerName: "z.a.aa.cc"}); err != nil || got != exactCertificate {
		t.Fatalf("exact certificate must cover exact domain rule: cert=%v err=%v", got, err)
	}
}

func TestReverseProxyGetCertificateUsesIPSNIForIPRules(t *testing.T) {
	ipCert := &tls.Certificate{}
	domainCert := &tls.Certificate{}
	domainBinding := &reverseProxyRuleCertificateBinding{
		RuleID:              1,
		CertificateRecordID: 1,
		Certificate:         domainCert,
		Leaf:                reverseProxyTestLeafState("example.com"),
	}
	ipBinding := &reverseProxyRuleCertificateBinding{
		RuleID:              2,
		CertificateRecordID: 2,
		Certificate:         ipCert,
		Leaf:                reverseProxyTestLeafState("127.0.0.1"),
	}
	group := &reverseProxyListenerGroup{
		rules: []*model.ReverseProxyRule{
			{
				Id:       1,
				HostList: `["example.com"]`,
			},
			{
				Id: 2,
			},
		},
		certBindingsByRule: map[uint][]*reverseProxyRuleCertificateBinding{
			1: {domainBinding},
			2: {ipBinding},
		},
		orderedCertBindings: []*reverseProxyRuleCertificateBinding{domainBinding, ipBinding},
	}
	group.configureIPCertificateIndexesLocked()

	got, err := group.getCertificate(&tls.ClientHelloInfo{
		ServerName: "127.0.0.1",
		Conn: reverseProxyTestConn{
			local: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 443},
		},
	})
	if err != nil || got != ipCert {
		t.Fatalf("expected ip sni certificate, got cert=%v err=%v", got, err)
	}
}

func TestReverseProxyGetCertificateAllowsMissingSNIForExactIPCertificate(t *testing.T) {
	ipCert := &tls.Certificate{}
	ipBinding := &reverseProxyRuleCertificateBinding{
		RuleID:              1,
		CertificateRecordID: 1,
		Certificate:         ipCert,
		Leaf:                reverseProxyTestLeafState("127.0.0.1"),
	}
	group := &reverseProxyListenerGroup{
		rules: []*model.ReverseProxyRule{
			{
				Id:       1,
				HostList: `["example.com"]`,
			},
		},
		certBindingsByRule: map[uint][]*reverseProxyRuleCertificateBinding{
			1: {ipBinding},
		},
		orderedCertBindings: []*reverseProxyRuleCertificateBinding{ipBinding},
	}
	group.configureIPCertificateIndexesLocked()

	got, err := group.getCertificate(&tls.ClientHelloInfo{
		Conn: reverseProxyTestConn{
			local: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 443},
		},
	})
	if err != nil || got != ipCert {
		t.Fatalf("missing sni must select the certificate covering the visible target ip, got cert=%v err=%v", got, err)
	}
	got, err = group.getCertificate(&tls.ClientHelloInfo{
		ServerName: "127.0.0.2",
		Conn: reverseProxyTestConn{
			local: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 443},
		},
	})
	if err == nil || got != nil {
		t.Fatalf("strict ip rule must reject unmatched ip sni, got cert=%v err=%v", got, err)
	}
}

func TestReverseProxyGetCertificateNATFallbackRequiresOneCertificateCoveringAllIPs(t *testing.T) {
	firstCert := &tls.Certificate{}
	secondCert := &tls.Certificate{}
	umbrellaCert := &tls.Certificate{}
	first := &reverseProxyRuleCertificateBinding{
		RuleID: 1, CertificateRecordID: 1, Certificate: firstCert,
		Leaf: reverseProxyTestLeafState("198.51.100.1"),
	}
	second := &reverseProxyRuleCertificateBinding{
		RuleID: 2, CertificateRecordID: 2, Certificate: secondCert,
		Leaf: reverseProxyTestLeafState("198.51.100.2"),
	}
	hello := &tls.ClientHelloInfo{Conn: reverseProxyTestConn{local: &net.TCPAddr{IP: net.ParseIP("10.0.0.5"), Port: 443}}}
	group := &reverseProxyListenerGroup{orderedCertBindings: []*reverseProxyRuleCertificateBinding{first}}
	group.configureIPCertificateIndexesLocked()
	if got, err := group.getCertificate(hello); err != nil || got != firstCert {
		t.Fatalf("one public IPv4 behind NAT must work with its ordinary single-IP certificate: cert=%v err=%v", got, err)
	}
	ipv6Cert := &tls.Certificate{}
	ipv6 := &reverseProxyRuleCertificateBinding{
		RuleID: 4, CertificateRecordID: 4, Certificate: ipv6Cert,
		Leaf: reverseProxyTestLeafState("2001:db8::1"),
	}
	group.orderedCertBindings = []*reverseProxyRuleCertificateBinding{first, ipv6}
	group.configureIPCertificateIndexesLocked()
	if got, err := group.getCertificate(hello); err != nil || got != firstCert {
		t.Fatalf("a separate IPv6 certificate must not block the single IPv4 NAT fallback: cert=%v err=%v", got, err)
	}
	hintedHello := reverseProxyClientHelloWithLocalIPHint(&tls.ClientHelloInfo{}, "0.0.0.0")
	if got, err := group.getCertificate(hintedHello); err != nil || got != firstCert {
		t.Fatalf("QUIC IPv4 listener hint must select the single IPv4 NAT fallback: cert=%v err=%v", got, err)
	}

	group.orderedCertBindings = []*reverseProxyRuleCertificateBinding{first, second}
	group.configureIPCertificateIndexesLocked()
	exactHello := &tls.ClientHelloInfo{Conn: reverseProxyTestConn{local: &net.TCPAddr{IP: net.ParseIP("198.51.100.1"), Port: 443}}}
	if got, err := group.getCertificate(exactHello); err != nil || got != firstCert {
		t.Fatalf("visible IP1 must select IP1 certificate and never IP2: cert=%v err=%v", got, err)
	}
	if got, err := group.getCertificate(hello); err == nil || got != nil {
		t.Fatalf("disjoint single-IP certificates must not be guessed behind NAT: cert=%v err=%v", got, err)
	}

	umbrella := &reverseProxyRuleCertificateBinding{
		RuleID: 3, CertificateRecordID: 3, Certificate: umbrellaCert,
		Leaf: reverseProxyTestLeafState("198.51.100.1", "198.51.100.2"),
	}
	group.orderedCertBindings = []*reverseProxyRuleCertificateBinding{first, second, umbrella}
	group.configureIPCertificateIndexesLocked()
	if got, err := group.getCertificate(hello); err != nil || got != umbrellaCert {
		t.Fatalf("full-cover multi-IP SAN certificate must be the NAT fallback: cert=%v err=%v", got, err)
	}
}

func TestReverseProxyGetCertificateRejectsVisibleUnconfiguredPublicIP(t *testing.T) {
	umbrellaCert := &tls.Certificate{}
	umbrella := &reverseProxyRuleCertificateBinding{
		RuleID: 1, CertificateRecordID: 1, Certificate: umbrellaCert,
		Leaf: reverseProxyTestLeafState("198.51.100.1", "198.51.100.2"),
	}
	group := &reverseProxyListenerGroup{orderedCertBindings: []*reverseProxyRuleCertificateBinding{umbrella}}
	group.configureIPCertificateIndexesLocked()
	hello := &tls.ClientHelloInfo{Conn: reverseProxyTestConn{local: &net.TCPAddr{IP: net.ParseIP("198.51.100.5"), Port: 443}}}
	if got, err := group.getCertificate(hello); err == nil || got != nil {
		t.Fatalf("visible public IP5 must not use the IP1-IP2 NAT fallback certificate: cert=%v err=%v", got, err)
	}
}

func TestReverseProxyGetCertificateNormalizesIPv6Target(t *testing.T) {
	certificate := &tls.Certificate{}
	binding := &reverseProxyRuleCertificateBinding{
		RuleID: 1, CertificateRecordID: 1, Certificate: certificate,
		Leaf: reverseProxyTestLeafState("2001:db8::1"),
	}
	group := &reverseProxyListenerGroup{orderedCertBindings: []*reverseProxyRuleCertificateBinding{binding}}
	group.configureIPCertificateIndexesLocked()
	hello := &tls.ClientHelloInfo{Conn: reverseProxyTestConn{local: &net.TCPAddr{IP: net.ParseIP("2001:0db8:0:0:0:0:0:1"), Port: 443}}}
	if got, err := group.getCertificate(hello); err != nil || got != certificate {
		t.Fatalf("equivalent IPv6 spellings must select the same IP SAN certificate: cert=%v err=%v", got, err)
	}
}

func TestReverseProxyFindRuleRejectsIPHostOutsideSelectedCertificate(t *testing.T) {
	certificate := &tls.Certificate{}
	rule := &model.ReverseProxyRule{Id: 1, PathPrefix: "/"}
	binding := &reverseProxyRuleCertificateBinding{
		RuleID: 1, CertificateRecordID: 1, Certificate: certificate,
		Leaf: reverseProxyTestLeafState("198.51.100.1", "198.51.100.2"),
	}
	group := &reverseProxyListenerGroup{
		protocol:            reverseProxyProtocolHTTPS,
		rules:               []*model.ReverseProxyRule{rule},
		certBindingsByRule:  map[uint][]*reverseProxyRuleCertificateBinding{1: {binding}},
		orderedCertBindings: []*reverseProxyRuleCertificateBinding{binding},
	}
	group.configureIPCertificateIndexesLocked()
	selection := reverseProxyCertificateSelection{CertificateRecordID: 1, ClientKind: reverseProxyTLSClientNoSNI}
	if got, _ := group.findRuleWithSelection("198.51.100.5", "", "/", selection, true); got != nil {
		t.Fatalf("IP5 must not enter a rule whose selected certificate only covers IP1-IP2: %#v", got)
	}
	if got, _ := group.findRuleWithSelection("198.51.100.1", "", "/", selection, true); got != rule {
		t.Fatalf("IP1 should enter the rule covered by the selected certificate: %#v", got)
	}
}

func TestReverseProxyTLSAndRuleMatchingHotPathDoesNotQuerySQLite(t *testing.T) {
	openReverseProxyTestDB(t)
	certificateID := createReverseProxyTestCertificateRecord(t, "hot-path.example.com")
	rule := &model.ReverseProxyRule{
		Id: 1, ListenProtocol: reverseProxyProtocolHTTPS, ListenPort: 443,
		HostList: `["hot-path.example.com"]`, PathPrefix: "/",
		CertificateRecordID: certificateID, CertificateRecordList: encodeReverseProxyUintList([]uint{certificateID}),
	}
	bindingsByRule, bindings, err := (&ReverseProxyService{}).loadRuleCertificates([]*model.ReverseProxyRule{rule})
	if err != nil {
		t.Fatalf("load TLS snapshot failed: %v", err)
	}
	group := &reverseProxyListenerGroup{
		key: "https|443|tcp", protocol: reverseProxyProtocolHTTPS, rules: []*model.ReverseProxyRule{rule},
		certBindingsByRule: bindingsByRule, orderedCertBindings: bindings,
	}
	group.configureIPCertificateIndexesLocked()

	var queries atomic.Int32
	callbackName := "reverse_proxy_hot_path_query_counter"
	db := database.GetDB()
	if err := db.Callback().Query().Before("gorm:query").Register(callbackName, func(*gorm.DB) {
		queries.Add(1)
	}); err != nil {
		t.Fatalf("register query counter failed: %v", err)
	}
	defer func() { _ = db.Callback().Query().Remove(callbackName) }()

	for index := 0; index < 5; index++ {
		certificate, err := group.getCertificate(&tls.ClientHelloInfo{ServerName: "hot-path.example.com"})
		if err != nil || certificate == nil {
			t.Fatalf("hot-path certificate selection %d failed: cert=%v err=%v", index, certificate != nil, err)
		}
		selection := reverseProxyCertificateSelection{
			CertificateRecordID: certificateID,
			ClientKind:          reverseProxyTLSClientDomain,
		}
		if matched, _ := group.findRuleWithSelection("hot-path.example.com", "hot-path.example.com", "/", selection, true); matched != rule {
			t.Fatalf("hot-path rule match %d failed: %#v", index, matched)
		}
	}
	if got := queries.Load(); got != 0 {
		t.Fatalf("TLS and request matching hot path issued %d SQLite queries", got)
	}
}

func TestReverseProxyFindRuleSkipsStrictDomainRuleForIPDirect(t *testing.T) {
	domainCert := &tls.Certificate{}
	ipCert := &tls.Certificate{}
	domainRule := &model.ReverseProxyRule{
		Id:         1,
		HostList:   `["example.com"]`,
		PathPrefix: "/direct",
	}
	ipRule := &model.ReverseProxyRule{
		Id:         2,
		HostList:   `["api.example.com"]`,
		PathPrefix: "/direct",
	}
	group := &reverseProxyListenerGroup{
		protocol: reverseProxyProtocolHTTPS,
		rules:    []*model.ReverseProxyRule{domainRule, ipRule},
		certBindingsByRule: map[uint][]*reverseProxyRuleCertificateBinding{
			1: {{RuleID: 1, CertificateRecordID: 1, Certificate: domainCert, Leaf: reverseProxyTestLeafState("example.com")}},
			2: {{RuleID: 2, CertificateRecordID: 2, Certificate: ipCert, Leaf: reverseProxyTestLeafState("127.0.0.1")}},
		},
	}

	for _, sni := range []string{"", "127.0.0.1"} {
		rule, nameMatched := group.findRule("127.0.0.1", sni, "/direct")
		if !nameMatched || rule == nil || rule.Id != ipRule.Id {
			t.Fatalf("expected strict ip certificate rule for sni %q, got rule=%v matched=%v", sni, rule, nameMatched)
		}
	}
	if rule, nameMatched := group.findRule("127.0.0.1", "example.com", "/direct"); rule != nil || nameMatched {
		t.Fatalf("strict ip direct request must reject mismatched domain sni, got rule=%v matched=%v", rule, nameMatched)
	}
}

func TestReverseProxyGetCertificateRejectsMissingSNIWhenOnlyDomainCertificate(t *testing.T) {
	domainCert := &tls.Certificate{}
	domainBinding := &reverseProxyRuleCertificateBinding{
		RuleID:              1,
		CertificateRecordID: 1,
		Certificate:         domainCert,
		Leaf:                reverseProxyTestLeafState("example.com"),
	}
	group := &reverseProxyListenerGroup{
		rules: []*model.ReverseProxyRule{
			{
				Id:       1,
				HostList: `["example.com"]`,
			},
		},
		certBindingsByRule: map[uint][]*reverseProxyRuleCertificateBinding{
			1: {domainBinding},
		},
		orderedCertBindings: []*reverseProxyRuleCertificateBinding{domainBinding},
	}

	got, err := group.getCertificate(&tls.ClientHelloInfo{})
	if err == nil || got != nil {
		t.Fatalf("missing sni must be rejected, got cert=%v err=%v", got, err)
	}
}

func TestReverseProxyGetCertificateWithSNIUsesLeastActiveBalancedCertificate(t *testing.T) {
	firstCert := &tls.Certificate{}
	secondCert := &tls.Certificate{}
	firstBinding := &reverseProxyRuleCertificateBinding{
		RuleID:              1,
		CertificateRecordID: 1,
		Certificate:         firstCert,
		Leaf:                reverseProxyTestLeafState("example.com"),
	}
	secondBinding := &reverseProxyRuleCertificateBinding{
		RuleID:              1,
		CertificateRecordID: 2,
		Certificate:         secondCert,
		Leaf:                reverseProxyTestLeafState("example.com"),
	}

	nowUnix := time.Now().Unix()

	group := &reverseProxyListenerGroup{
		key:     "https|443",
		service: &ReverseProxyService{},
		rules: []*model.ReverseProxyRule{
			{
				Id:       1,
				HostList: `["example.com"]`,
			},
		},
		certBindingsByRule: map[uint][]*reverseProxyRuleCertificateBinding{
			1: {firstBinding, secondBinding},
		},
		orderedCertBindings: []*reverseProxyRuleCertificateBinding{firstBinding, secondBinding},
	}
	shard := group.certificateBalanceShard("example.com")
	shard.states = map[string]map[uint]*reverseProxyCertificateBalanceRuntimeState{
		"example.com": {
			1: {ActiveConn: 7, SelectedTotal: 9, LastSelectedAt: nowUnix - 20, UpdatedAtUnix: nowUnix - 20},
			2: {ActiveConn: 1, SelectedTotal: 2, LastSelectedAt: nowUnix - 10, UpdatedAtUnix: nowUnix - 10},
		},
	}

	got, err := group.getCertificate(&tls.ClientHelloInfo{ServerName: "example.com"})
	if err != nil {
		t.Fatalf("expected sni certificate selection to succeed: %v", err)
	}
	if got != secondCert {
		t.Fatalf("expected least-active certificate to be selected, got %v", got)
	}
}

func TestMaintainCertificateBalanceCleansOrphanAndStaleRows(t *testing.T) {
	openReverseProxyTestDB(t)

	certID := createReverseProxyTestCertificateRecord(t, "example.com")
	rule := &model.ReverseProxyRule{
		Name:                  "balance-maintenance",
		Enabled:               true,
		ListenProtocol:        reverseProxyProtocolHTTPS,
		ListenPort:            443,
		CertificateRecordList: encodeReverseProxyUintList([]uint{certID}),
	}
	if err := database.GetDB().Create(rule).Error; err != nil {
		t.Fatalf("create reverse proxy rule failed: %v", err)
	}

	nowUnix := time.Now().Unix()
	staleUnix := nowUnix - int64((reverseProxyCertBalanceStaleTTL/time.Second)+10)
	rows := []model.ReverseProxyCertificateBalanceState{
		{
			ListenerKey:         "https|443",
			SNIBucket:           "example.com",
			CertificateRecordID: certID,
			ActiveConn:          -3,
			SelectedTotal:       -8,
			LastSelectedAt:      -1,
			UpdatedAtUnix:       nowUnix,
		},
		{
			ListenerKey:         "https|443",
			SNIBucket:           reverseProxyCertBalanceNoSNIBucket,
			CertificateRecordID: certID,
			ActiveConn:          1,
			SelectedTotal:       4,
			LastSelectedAt:      staleUnix,
			UpdatedAtUnix:       staleUnix,
		},
		{
			ListenerKey:         "https|443",
			SNIBucket:           "active.example.com",
			CertificateRecordID: certID,
			ActiveConn:          2,
			SelectedTotal:       8,
			LastSelectedAt:      staleUnix,
			UpdatedAtUnix:       staleUnix,
		},
		{
			ListenerKey:         "https|443",
			SNIBucket:           "example.com",
			CertificateRecordID: certID + 9999,
			ActiveConn:          1,
			SelectedTotal:       1,
			LastSelectedAt:      nowUnix,
			UpdatedAtUnix:       nowUnix,
		},
	}
	if err := database.GetDB().Create(&rows).Error; err != nil {
		t.Fatalf("create balance rows failed: %v", err)
	}

	if err := (&ReverseProxyService{}).MaintainCertificateBalance(true); err != nil {
		t.Fatalf("maintain certificate balance failed: %v", err)
	}

	remaining := make([]model.ReverseProxyCertificateBalanceState, 0)
	if err := database.GetDB().Find(&remaining).Error; err != nil {
		t.Fatalf("query remaining balance rows failed: %v", err)
	}
	if len(remaining) != 3 {
		t.Fatalf("expected exactly three balance rows after cleanup, got %d (%#v)", len(remaining), remaining)
	}
	kept := make(map[string]model.ReverseProxyCertificateBalanceState, 3)
	for _, row := range remaining {
		kept[row.SNIBucket] = row
	}
	base := kept["example.com"]
	if base.CertificateRecordID != certID {
		t.Fatalf("unexpected remaining cert id: got %d want %d", base.CertificateRecordID, certID)
	}
	if base.ActiveConn < 0 || base.SelectedTotal < 0 || base.LastSelectedAt < 0 || base.UpdatedAtUnix < 0 {
		t.Fatalf("expected non-negative counters after cleanup, got %#v", base)
	}
	active := kept["active.example.com"]
	if active.CertificateRecordID != certID {
		t.Fatalf("expected active stale row to remain, got %#v", active)
	}
	if active.ActiveConn != 2 {
		t.Fatalf("active stale row should keep active_conn, got %d want 2", active.ActiveConn)
	}
	noSNI := kept[reverseProxyCertBalanceNoSNIBucket]
	if noSNI.CertificateRecordID != certID || noSNI.ActiveConn != 1 {
		t.Fatalf("expected stale nosni active row to remain, got %#v", noSNI)
	}
}

func TestReserveCertificateBalanceSelectionConcurrentStaysBalanced(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	candidates := []*reverseProxyRuleCertificateBinding{
		{
			RuleID:              1,
			CertificateRecordID: 11,
			Certificate:         &tls.Certificate{},
			Leaf:                reverseProxyTestLeafState("example.com"),
		},
		{
			RuleID:              1,
			CertificateRecordID: 22,
			Certificate:         &tls.Certificate{},
			Leaf:                reverseProxyTestLeafState("example.com"),
		},
	}

	const runs = 40
	errCh := make(chan error, runs)
	var wg sync.WaitGroup
	for i := 0; i < runs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			selected, selection, err := svc.reserveCertificateBalanceSelection("https|9443", "example.com", candidates)
			if err != nil {
				errCh <- err
				return
			}
			if selected == nil || selection.CertificateRecordID == 0 {
				errCh <- errors.New("unexpected empty selection")
				return
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("reserve concurrent failed: %v", err)
		}
	}

	rows := make([]model.ReverseProxyCertificateBalanceState, 0)
	if err := database.GetDB().
		Where("listener_key = ? AND sni_bucket = ?", "https|9443", "example.com").
		Order("certificate_record_id asc").
		Find(&rows).Error; err != nil {
		t.Fatalf("query balance rows failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected two cert rows, got %d (%#v)", len(rows), rows)
	}
	total := rows[0].ActiveConn + rows[1].ActiveConn
	if total != runs {
		t.Fatalf("active_conn total mismatch: got %d want %d", total, runs)
	}
	diff := rows[0].ActiveConn - rows[1].ActiveConn
	if diff < 0 {
		diff = -diff
	}
	if diff > 1 {
		t.Fatalf("expected near-even distribution, got diff=%d rows=%#v", diff, rows)
	}
}

func TestReverseProxyCertificateSelectionSkipsExpiredCertificates(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	expired := &reverseProxyRuleCertificateBinding{
		RuleID:              1,
		CertificateRecordID: 11,
		Certificate:         &tls.Certificate{},
		Leaf:                reverseProxyTestLeafState("example.com"),
	}
	expired.Leaf.NotAfter = time.Now().Add(-time.Minute)
	expired.Leaf.Leaf.NotAfter = expired.Leaf.NotAfter
	valid := &reverseProxyRuleCertificateBinding{
		RuleID:              1,
		CertificateRecordID: 22,
		Certificate:         &tls.Certificate{},
		Leaf:                reverseProxyTestLeafState("example.com"),
	}
	valid.Leaf.NotAfter = time.Now().Add(time.Hour)
	valid.Leaf.Leaf.NotAfter = valid.Leaf.NotAfter

	filtered := reverseProxyUniqueCertificateBindings([]*reverseProxyRuleCertificateBinding{expired, valid})
	if len(filtered) != 1 || filtered[0] != valid {
		t.Fatalf("expected only valid reverse proxy certificate candidate, got=%#v", filtered)
	}

	selected, selection, err := svc.reserveCertificateBalanceSelection("https|9443", "example.com", []*reverseProxyRuleCertificateBinding{expired, valid})
	if err != nil {
		t.Fatalf("reserve certificate failed: %v", err)
	}
	if selected != valid || selection.CertificateRecordID != valid.CertificateRecordID {
		t.Fatalf("expected valid certificate selection, got selected=%#v selection=%#v", selected, selection)
	}
	if reverseProxyCertificateBindingMatchesServerName(expired, "example.com") {
		t.Fatal("expired certificate should not match sni")
	}
}

func TestReverseProxyOverviewDoesNotReadCertificateBalanceDatabaseTable(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	certRecordID := createReverseProxyTestCertificateRecord(t, "example.com")
	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:           "overview-balance-diagnostics-memory",
		Enabled:        true,
		ListenProtocol: reverseProxyProtocolHTTPS,

		ListenPort:          reserveReverseProxyTestPort(t),
		Hosts:               "example.com",
		TargetProtocol:      reverseProxyProtocolHTTP,
		TargetAddresses:     "127.0.0.1",
		TargetPort:          18080,
		CertificateRecordID: certRecordID,
		IPStrategy:          reverseProxyIPStrategyPreferIPv4,
	}); err != nil {
		t.Fatalf("upsert overview fallback rule failed: %v", err)
	}

	if err := database.GetDB().Exec("DROP TABLE IF EXISTS reverse_proxy_certificate_balance_states").Error; err != nil {
		t.Fatalf("drop balance table failed: %v", err)
	}

	overview, err := svc.GetOverview()
	if err != nil {
		t.Fatalf("get overview should not fail when diagnostics fail: %v", err)
	}
	if len(overview.Rules) == 0 {
		t.Fatalf("expected overview rules to be present, got %#v", overview)
	}
	for _, warning := range overview.Warnings {
		if strings.Contains(warning, "certificate balance diagnostics unavailable") {
			t.Fatalf("overview must use in-memory certificate diagnostics, got %#v", overview.Warnings)
		}
	}
}

func TestReverseProxyGetCertificateWithSNISelectsFirstCoveringCertInConfiguredOrder(t *testing.T) {
	firstCert := &tls.Certificate{}
	secondCert := &tls.Certificate{}
	firstBinding := &reverseProxyRuleCertificateBinding{
		RuleID:              1,
		CertificateRecordID: 1,
		Certificate:         firstCert,
		Leaf:                reverseProxyTestLeafState("aaa.example.com"),
	}
	secondBinding := &reverseProxyRuleCertificateBinding{
		RuleID:              1,
		CertificateRecordID: 2,
		Certificate:         secondCert,
		Leaf:                reverseProxyTestLeafState("example.com"),
	}
	group := &reverseProxyListenerGroup{
		rules: []*model.ReverseProxyRule{
			{
				Id:       1,
				HostList: `["example.com"]`,
			},
		},
		certBindingsByRule: map[uint][]*reverseProxyRuleCertificateBinding{
			1: {firstBinding, secondBinding},
		},
		orderedCertBindings: []*reverseProxyRuleCertificateBinding{firstBinding, secondBinding},
	}

	got, err := group.getCertificate(&tls.ClientHelloInfo{ServerName: "example.com"})
	if err != nil {
		t.Fatalf("expected sni certificate selection to succeed: %v", err)
	}
	if got != secondCert {
		t.Fatalf("expected second certificate to match example.com, got %v", got)
	}
}

func TestBuildReverseProxyCertificateHintsKeepsHostWarningsWhenOnlyIPSANCertPresent(t *testing.T) {
	hints := buildReverseProxyCertificateHints(
		[]string{"dns.example.com"},
		[]ReverseProxyCertificateOption{
			{
				MainDomain: "127.0.0.1",
				Domains:    []string{"127.0.0.1"},
			},
		},
	)
	if len(hints) != 1 || hints[0] != "证书未覆盖域名: dns.example.com" {
		t.Fatalf("ip-san certificate must not suppress domain warning, got %#v", hints)
	}
}

func TestReverseProxyGetCertificateNoFallbackCertFails(t *testing.T) {
	group := &reverseProxyListenerGroup{}
	if _, err := group.getCertificate(nil); err == nil {
		t.Fatal("expected missing default certificate to fail")
	}
}

func TestReverseProxyListenBindsUseIPv4AndIPv6Wildcards(t *testing.T) {
	binds := reverseProxyTCPListenBinds(18080)
	if len(binds) != 2 {
		t.Fatalf("expected ipv4 and ipv6 listen binds, got %#v", binds)
	}
	if binds[0].network != "tcp4" || binds[0].listenIP != "0.0.0.0" || binds[0].optional {
		t.Fatalf("unexpected ipv4 bind: %#v", binds[0])
	}
	if binds[1].network != "tcp6" || binds[1].listenIP != "::" || !binds[1].optional {
		t.Fatalf("unexpected ipv6 bind: %#v", binds[1])
	}
}

func TestReverseProxyUDPListenBindsUseIPv4AndIPv6Wildcards(t *testing.T) {
	binds := reverseProxyUDPListenBinds(18080)
	if len(binds) != 2 {
		t.Fatalf("expected ipv4 and ipv6 udp listen binds, got %#v", binds)
	}
	if binds[0].network != "udp4" || binds[0].listenIP != "0.0.0.0" || binds[0].optional {
		t.Fatalf("unexpected udp ipv4 bind: %#v", binds[0])
	}
	if binds[1].network != "udp6" || binds[1].listenIP != "::" || !binds[1].optional {
		t.Fatalf("unexpected udp ipv6 bind: %#v", binds[1])
	}
}

func TestReverseProxyDNSRuntimeListenIPsAlwaysUseWildcards(t *testing.T) {
	row := &model.ReverseProxyRule{
		ListenProtocol:      reverseProxyProtocolDNS,
		ListenProtocolAlias: reverseProxyDNSProtocolDoH,
	}

	got := reverseProxyDNSRuntimeListenIPsForAlias(row, reverseProxyDNSProtocolDoH)
	if len(got) != 2 || got[0] != "0.0.0.0" || got[1] != "::" {
		t.Fatalf("expected wildcard dns runtime, got %#v", got)
	}
}

func TestReverseProxyDNSRuntimeListenIPsIgnoreLegacyAddressSelection(t *testing.T) {
	row := &model.ReverseProxyRule{
		ListenProtocol:      reverseProxyProtocolDNS,
		ListenProtocolAlias: reverseProxyDNSProtocolDoH,
	}

	got := reverseProxyDNSRuntimeListenIPsForAlias(row, reverseProxyDNSProtocolDoH)
	if len(got) != 2 || got[0] != "0.0.0.0" || got[1] != "::" {
		t.Fatalf("expected legacy address selection to be ignored, got %#v", got)
	}
}

func TestReverseProxyListenerCountCountsSockets(t *testing.T) {
	groups := map[string]*reverseProxyListenerGroup{
		"http|80": {
			listeners: []net.Listener{nil, nil},
		},
		"http|81": {},
	}
	if got := reverseProxyListenerCount(groups); got != 2 {
		t.Fatalf("listener count = %d, want 2", got)
	}
}

func TestShutdownReverseProxyListenerGroups_ReleasesPort(t *testing.T) {
	hold, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port failed: %v", err)
	}
	port := hold.Addr().(*net.TCPAddr).Port
	if err := hold.Close(); err != nil {
		t.Fatalf("close reservation failed: %v", err)
	}

	group, err := (&ReverseProxyService{}).newListenerGroup("http|"+strconv.Itoa(port), []*model.ReverseProxyRule{
		{
			ListenProtocol:  "http",
			ListenPort:      port,
			PathPrefix:      "/",
			TargetProtocol:  "http",
			TargetAddresses: `["127.0.0.1"]`,
			TargetPort:      80,
		},
	})
	if err != nil {
		t.Fatalf("create listener group failed: %v", err)
	}

	if err := shutdownReverseProxyListenerGroups(map[string]*reverseProxyListenerGroup{"test": group}); err != nil {
		t.Fatalf("shutdown listener group failed: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		probe, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err == nil {
			_ = probe.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("port should be released after shutdown: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestValidateReverseProxyHTTPSCertificateUsesCurrentTransaction(t *testing.T) {
	openReverseProxyTestDB(t)

	cert := model.CertificateRecord{
		SourceType:      CertificateSourceSelfSigned,
		SourceRef:       "reverse-proxy-test",
		MainDomain:      "example.com",
		DomainSet:       `["example.com"]`,
		CertPEM:         []byte("cert"),
		KeyPEM:          []byte("key"),
		FullchainPEM:    []byte("cert"),
		ListOrderAt:     time.Now().Unix(),
		CertificateType: "domain",
	}
	if err := database.GetDB().Create(&cert).Error; err != nil {
		t.Fatalf("create certificate record failed: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- database.GetDB().Transaction(func(tx *gorm.DB) error {
			return (&ReverseProxyService{}).validateNormalizedRule(tx, reverseProxyNormalizedRule{
				listenProtocol:      reverseProxyProtocolHTTPS,
				listenPort:          8443,
				hosts:               []string{"example.com"},
				pathPrefix:          "/",
				targetProtocol:      reverseProxyProtocolHTTP,
				targetAddresses:     []string{"127.0.0.1"},
				targetPort:          8080,
				certificateRecordID: cert.Id,
				ipStrategy:          reverseProxyIPStrategyPreferIPv4,
			})
		})
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("validate rule failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("validate rule blocked while checking certificate inside a transaction")
	}
}

func TestUpsertReverseProxyHTTPSRuleReturnsAfterCertificateValidation(t *testing.T) {
	openReverseProxyTestDB(t)
	defer func() {
		_ = (&ReverseProxyService{}).StopRuntime()
	}()

	cert := model.CertificateRecord{
		SourceType:      CertificateSourceSelfSigned,
		SourceRef:       "reverse-proxy-upsert-test",
		MainDomain:      "example.com",
		DomainSet:       `["example.com"]`,
		CertPEM:         []byte("cert"),
		KeyPEM:          []byte("key"),
		FullchainPEM:    []byte("cert"),
		ListOrderAt:     time.Now().Unix(),
		CertificateType: "domain",
	}
	if err := database.GetDB().Create(&cert).Error; err != nil {
		t.Fatalf("create certificate record failed: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate listen port failed: %v", err)
	}
	listenPort := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	done := make(chan error, 1)
	go func() {
		done <- (&ReverseProxyService{}).UpsertRule(ReverseProxyRulePayload{
			Name:           "upsert-https",
			Enabled:        false,
			ListenProtocol: reverseProxyProtocolHTTPS,

			ListenPort:          listenPort,
			Hosts:               "example.com",
			PathPrefix:          "",
			TargetProtocol:      reverseProxyProtocolHTTP,
			TargetAddresses:     "127.0.0.1",
			TargetPort:          8080,
			CertificateRecordID: cert.Id,
			IPStrategy:          reverseProxyIPStrategyPreferIPv4,
		})
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("upsert rule failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("upsert rule blocked while saving an https reverse proxy rule")
	}

	var saved model.ReverseProxyRule
	if err := database.GetDB().Where("name = ?", "upsert-https").First(&saved).Error; err != nil {
		t.Fatalf("load saved rule failed: %v", err)
	}
	if saved.Enabled {
		t.Fatal("new disabled reverse proxy rule was saved as enabled")
	}
}

func TestReverseProxyHTTPSRuleProxiesHTTP11TLSUpstream(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	t.Cleanup(func() {
		_ = svc.StopRuntime()
	})

	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(r.Proto))
	}))
	upstream.EnableHTTP2 = false
	upstream.StartTLS()
	defer upstream.Close()

	upstreamHost, upstreamPort := splitReverseProxyTestServerAddress(t, upstream.URL)
	packetConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate udp listen port failed: %v", err)
	}
	listenPort := packetConn.LocalAddr().(*net.UDPAddr).Port
	if err := packetConn.Close(); err != nil {
		t.Fatalf("close udp port reservation failed: %v", err)
	}
	certRecordID := createReverseProxyTestCertificateRecord(t, "example.com")

	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:           "https-http11-upstream",
		Enabled:        true,
		ListenProtocol: reverseProxyProtocolHTTPS,

		Hosts:               "example.com",
		ListenPort:          listenPort,
		TargetProtocol:      reverseProxyProtocolHTTPS,
		TargetAddresses:     upstreamHost,
		TargetPort:          upstreamPort,
		CertificateRecordID: certRecordID,
		IPStrategy:          reverseProxyIPStrategyPreferIPv4,
		HTTPVersionStrategy: reverseProxyHTTPVersionPreferH3,
		UpstreamTLSVerify:   false,
	}); err != nil {
		t.Fatalf("upsert https reverse proxy rule failed: %v", err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: nil,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				ServerName:         "example.com",
			},
		},
		Timeout: 15 * time.Second,
	}
	req, err := http.NewRequest(http.MethodGet, "https://127.0.0.1:"+strconv.Itoa(listenPort)+"/", nil)
	if err != nil {
		t.Fatalf("build request failed: %v", err)
	}
	req.Host = "example.com"

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected proxy status: %d body=%q", resp.StatusCode, string(body))
	}
	if got := string(body); got != "HTTP/1.1" {
		t.Fatalf("expected upstream HTTP/1.1 response, got %q", got)
	}
}

func TestReverseProxyHTTPSListenerAcceptsHTTP3Client(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	t.Cleanup(func() {
		_ = svc.StopRuntime()
	})

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok:" + r.URL.Path))
	}))
	defer upstream.Close()

	upstreamHost, upstreamPort := splitReverseProxyTestServerAddress(t, upstream.URL)
	listenPort := reserveReverseProxyTestPort(t)
	certRecordID := createReverseProxyTestCertificateRecord(t, "example.com")

	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:           "https-h3-listener",
		Enabled:        true,
		ListenProtocol: reverseProxyProtocolHTTPS,

		Hosts: "example.com",

		ListenPort:          listenPort,
		TargetProtocol:      reverseProxyProtocolHTTP,
		TargetAddresses:     upstreamHost,
		TargetPort:          upstreamPort,
		CertificateRecordID: certRecordID,
		IPStrategy:          reverseProxyIPStrategyPreferIPv4,
	}); err != nil {
		t.Fatalf("upsert https-h3-listener rule failed: %v", err)
	}

	transport := &http3.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         "example.com",
		},
	}
	defer func() {
		_ = transport.Close()
	}()

	client := &http.Client{
		Transport: transport,
		Timeout:   15 * time.Second,
	}
	req, err := http.NewRequest(http.MethodGet, "https://127.0.0.1:"+strconv.Itoa(listenPort)+"/h3/ping", nil)
	if err != nil {
		t.Fatalf("build h3 request failed: %v", err)
	}
	req.Host = "example.com"

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("h3 request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read h3 response failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected h3 proxy status: %d body=%q", resp.StatusCode, string(body))
	}
	if got := string(body); got != "ok:/h3/ping" {
		t.Fatalf("unexpected h3 body: got %q", got)
	}
}

func TestReverseProxyHTTPSListenerRejectsHTTP3WebSocket(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	t.Cleanup(func() {
		_ = svc.StopRuntime()
	})

	upstreamRequests := make(chan string, 2)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamRequests <- r.Method + " " + r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok:" + r.URL.Path))
	}))
	defer upstream.Close()

	upstreamHost, upstreamPort := splitReverseProxyTestServerAddress(t, upstream.URL)
	listenPort := reserveReverseProxyTestPort(t)
	certRecordID := createReverseProxyTestCertificateRecord(t, "example.com")
	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:           "https-h3-reject-websocket",
		Enabled:        true,
		ListenProtocol: reverseProxyProtocolHTTPS,

		Hosts:               "example.com",
		ListenPort:          listenPort,
		TargetProtocol:      reverseProxyProtocolHTTP,
		TargetAddresses:     upstreamHost,
		TargetPort:          upstreamPort,
		CertificateRecordID: certRecordID,
		IPStrategy:          reverseProxyIPStrategyPreferIPv4,
	}); err != nil {
		t.Fatalf("upsert https listener failed: %v", err)
	}

	transport := &http3.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         "example.com",
		},
	}
	defer func() {
		_ = transport.Close()
	}()
	client := &http.Client{Transport: transport, Timeout: 15 * time.Second}

	pageRequest, err := http.NewRequest(http.MethodGet, "https://127.0.0.1:"+strconv.Itoa(listenPort)+"/page", nil)
	if err != nil {
		t.Fatalf("build h3 page request failed: %v", err)
	}
	pageRequest.Host = "example.com"
	pageResponse, err := client.Do(pageRequest)
	if err != nil {
		t.Fatalf("h3 page request failed: %v", err)
	}
	pageBody, err := io.ReadAll(pageResponse.Body)
	_ = pageResponse.Body.Close()
	if err != nil {
		t.Fatalf("read h3 page response failed: %v", err)
	}
	if pageResponse.StatusCode != http.StatusOK || string(pageBody) != "ok:/page" {
		t.Fatalf("unexpected h3 page response: status=%d body=%q", pageResponse.StatusCode, string(pageBody))
	}
	select {
	case got := <-upstreamRequests:
		if got != "GET /page" {
			t.Fatalf("unexpected h3 page upstream request: %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("h3 page request did not reach upstream")
	}

	websocketRequest, err := http.NewRequest(http.MethodConnect, "https://127.0.0.1:"+strconv.Itoa(listenPort)+"/socket", nil)
	if err != nil {
		t.Fatalf("build h3 websocket request failed: %v", err)
	}
	websocketRequest.Host = "example.com"
	websocketRequest.Proto = "websocket"
	websocketRequest.ProtoMajor = 3
	websocketRequest.ProtoMinor = 0
	websocketResponse, err := client.Do(websocketRequest)
	if err != nil {
		t.Fatalf("h3 websocket request failed: %v", err)
	}
	defer websocketResponse.Body.Close()
	if websocketResponse.StatusCode != http.StatusNotImplemented {
		body, _ := io.ReadAll(websocketResponse.Body)
		t.Fatalf("unexpected h3 websocket status: %d body=%q", websocketResponse.StatusCode, string(body))
	}
	select {
	case got := <-upstreamRequests:
		t.Fatalf("h3 websocket request must not reach upstream: %q", got)
	case <-time.After(250 * time.Millisecond):
	}
}

func TestReverseProxyHTTPSListenerMovesBetweenTCPAndUDPGroups(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	t.Cleanup(func() {
		_ = svc.StopRuntime()
	})

	listenPort := reserveReverseProxyTestPort(t)
	certRecordID := createReverseProxyTestCertificateRecord(t, "example.com")
	payload := ReverseProxyRulePayload{
		Name:                      "https-listener-runtime-rebind",
		Enabled:                   true,
		ListenProtocol:            reverseProxyProtocolHTTPS,
		ListenHTTPVersionStrategy: reverseProxyListenHTTPVersionH2Only,

		Hosts:               "example.com",
		ListenPort:          listenPort,
		TargetProtocol:      reverseProxyProtocolHTTP,
		TargetAddresses:     "127.0.0.1",
		TargetPort:          18080,
		CertificateRecordID: certRecordID,
		IPStrategy:          reverseProxyIPStrategyPreferIPv4,
	}
	if err := svc.UpsertRule(payload); err != nil {
		t.Fatalf("create h2-only listener failed: %v", err)
	}

	var saved model.ReverseProxyRule
	if err := database.GetDB().Where("name = ?", payload.Name).First(&saved).Error; err != nil {
		t.Fatalf("load saved listener rule failed: %v", err)
	}
	payload.ID = saved.Id
	groupFor := func(kind string) (*reverseProxyListenerGroup, string, int, int) {
		reverseProxyRuntime.mu.Lock()
		defer reverseProxyRuntime.mu.Unlock()
		listenerKey := reverseProxyListenerGroupKey(&saved, kind)
		group := reverseProxyRuntime.groups[listenerKey]
		if group == nil {
			return nil, "", 0, 0
		}
		group.mu.RLock()
		defer group.mu.RUnlock()
		return group, group.listenHTTPVersionStrategy, len(group.listeners), len(group.packetConns)
	}

	initial, strategy, tcpListeners, udpListeners := groupFor(reverseProxySocketKindTCP)
	if initial == nil || strategy != reverseProxyListenHTTPVersionH2Only || tcpListeners == 0 || udpListeners != 0 {
		t.Fatalf("unexpected initial h2-only listener state: group=%p strategy=%q tcp=%d udp=%d", initial, strategy, tcpListeners, udpListeners)
	}

	payload.ListenHTTPVersionStrategy = reverseProxyListenHTTPVersionH2H3
	if err := svc.UpsertRule(payload); err != nil {
		t.Fatalf("switch listener to h2+h3 failed: %v", err)
	}
	if err := database.GetDB().Where("id = ?", saved.Id).First(&saved).Error; err != nil {
		t.Fatalf("reload h2+h3 listener rule failed: %v", err)
	}
	h2h3TCP, strategy, tcpListeners, udpListeners := groupFor(reverseProxySocketKindTCP)
	h2h3UDP, udpStrategy, udpTCPListeners, udpPacketConns := groupFor(reverseProxySocketKindUDP)
	if h2h3TCP == nil || h2h3TCP != initial || strategy != reverseProxyListenHTTPVersionH2H3 || tcpListeners == 0 || udpListeners != 0 || h2h3UDP == nil || udpStrategy != reverseProxyListenHTTPVersionH2H3 || udpTCPListeners != 0 || udpPacketConns == 0 {
		t.Fatalf("h2+h3 strategy must retain tcp and create udp group: tcp=%p/%q/%d/%d udp=%p/%q/%d/%d", h2h3TCP, strategy, tcpListeners, udpListeners, h2h3UDP, udpStrategy, udpTCPListeners, udpPacketConns)
	}

	payload.ListenHTTPVersionStrategy = reverseProxyListenHTTPVersionH3Only
	if err := svc.UpsertRule(payload); err != nil {
		t.Fatalf("switch listener to h3-only failed: %v", err)
	}
	if err := database.GetDB().Where("id = ?", saved.Id).First(&saved).Error; err != nil {
		t.Fatalf("reload h3-only listener rule failed: %v", err)
	}
	h3TCP, _, h3TCPListeners, h3TCPPackets := groupFor(reverseProxySocketKindTCP)
	h3Only, strategy, tcpListeners, udpListeners := groupFor(reverseProxySocketKindUDP)
	if h3TCP != nil || h3TCPListeners != 0 || h3TCPPackets != 0 || h3Only == nil || h3Only != h2h3UDP || strategy != reverseProxyListenHTTPVersionH3Only || tcpListeners != 0 || udpListeners == 0 {
		t.Fatalf("h3-only strategy must retain only udp group: tcp=%p/%d/%d udp=%p/%q/%d/%d", h3TCP, h3TCPListeners, h3TCPPackets, h3Only, strategy, tcpListeners, udpListeners)
	}
}

func TestReverseProxyHTTPSH2OnlyIPCertificateAllowsDirectRequestWithoutSNI(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	t.Cleanup(func() {
		_ = svc.StopRuntime()
	})

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ip-direct:" + r.URL.Path))
	}))
	defer upstream.Close()

	upstreamHost, upstreamPort := splitReverseProxyTestServerAddress(t, upstream.URL)
	listenPort := reserveReverseProxyTestPort(t)
	certRecordID := createReverseProxyTestCertificateRecord(t, "127.0.0.1")

	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:                      "https-h2-only-strict-ip-direct",
		Enabled:                   true,
		ListenProtocol:            reverseProxyProtocolHTTPS,
		ListenHTTPVersionStrategy: reverseProxyListenHTTPVersionH2Only,

		Hosts: "example.com",

		ListenPort:          listenPort,
		TargetProtocol:      reverseProxyProtocolHTTP,
		TargetAddresses:     upstreamHost,
		TargetPort:          upstreamPort,
		CertificateRecordID: certRecordID,
		IPStrategy:          reverseProxyIPStrategyPreferIPv4,
	}); err != nil {
		t.Fatalf("upsert strict ip direct rule failed: %v", err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			Proxy:             nil,
			ForceAttemptHTTP2: true,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
		Timeout: 15 * time.Second,
	}
	req, err := http.NewRequest(http.MethodGet, "https://127.0.0.1:"+strconv.Itoa(listenPort)+"/ip-direct", nil)
	if err != nil {
		t.Fatalf("build strict ip direct request failed: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("IP SAN certificate must allow direct access without SNI: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read ip direct response failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK || string(body) != "ip-direct:/ip-direct" {
		t.Fatalf("unexpected ip direct response: status=%d body=%q", resp.StatusCode, string(body))
	}
}

func TestReverseProxyHTTPSStrictDomainCertificateRejectsIPDirect(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	t.Cleanup(func() {
		_ = svc.StopRuntime()
	})

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("domain:" + r.URL.Path))
	}))
	defer upstream.Close()

	upstreamHost, upstreamPort := splitReverseProxyTestServerAddress(t, upstream.URL)
	listenPort := reserveReverseProxyTestPort(t)
	certRecordID := createReverseProxyTestCertificateRecord(t, "example.com")

	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:           "https-strict-domain-only",
		Enabled:        true,
		ListenProtocol: reverseProxyProtocolHTTPS,

		Hosts: "example.com",

		ListenPort:          listenPort,
		TargetProtocol:      reverseProxyProtocolHTTP,
		TargetAddresses:     upstreamHost,
		TargetPort:          upstreamPort,
		CertificateRecordID: certRecordID,
		IPStrategy:          reverseProxyIPStrategyPreferIPv4,
	}); err != nil {
		t.Fatalf("upsert strict domain rule failed: %v", err)
	}

	domainClient := &http.Client{
		Transport: &http.Transport{
			Proxy: nil,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				ServerName:         "example.com",
			},
		},
		Timeout: 15 * time.Second,
	}
	domainReq, err := http.NewRequest(http.MethodGet, "https://127.0.0.1:"+strconv.Itoa(listenPort)+"/domain", nil)
	if err != nil {
		t.Fatalf("build strict domain request failed: %v", err)
	}
	domainReq.Host = "example.com"
	domainResp, err := domainClient.Do(domainReq)
	if err != nil {
		t.Fatalf("strict domain request failed: %v", err)
	}
	domainBody, _ := io.ReadAll(domainResp.Body)
	_ = domainResp.Body.Close()
	if domainResp.StatusCode != http.StatusOK || string(domainBody) != "domain:/domain" {
		t.Fatalf("unexpected strict domain response: status=%d body=%q", domainResp.StatusCode, string(domainBody))
	}

	ipClient := &http.Client{
		Transport: &http.Transport{
			Proxy: nil,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
		Timeout: 15 * time.Second,
	}
	ipReq, err := http.NewRequest(http.MethodGet, "https://127.0.0.1:"+strconv.Itoa(listenPort)+"/domain", nil)
	if err != nil {
		t.Fatalf("build strict domain ip request failed: %v", err)
	}
	if ipResp, err := ipClient.Do(ipReq); err == nil {
		_ = ipResp.Body.Close()
		t.Fatal("strict domain certificate must reject direct ip access")
	}
}

func TestReverseProxyHTTPSListenerH2OnlyRejectsHTTP3Client(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	t.Cleanup(func() {
		_ = svc.StopRuntime()
	})

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok:" + r.URL.Path))
	}))
	defer upstream.Close()

	upstreamHost, upstreamPort := splitReverseProxyTestServerAddress(t, upstream.URL)
	listenPort := reserveReverseProxyTestPort(t)
	certRecordID := createReverseProxyTestCertificateRecord(t, "example.com")

	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:                      "https-h2-only-listener",
		Enabled:                   true,
		ListenProtocol:            reverseProxyProtocolHTTPS,
		ListenHTTPVersionStrategy: reverseProxyListenHTTPVersionH2Only,

		Hosts:               "example.com",
		ListenPort:          listenPort,
		TargetProtocol:      reverseProxyProtocolHTTP,
		TargetAddresses:     upstreamHost,
		TargetPort:          upstreamPort,
		CertificateRecordID: certRecordID,
		IPStrategy:          reverseProxyIPStrategyPreferIPv4,
	}); err != nil {
		t.Fatalf("upsert https-h2-only-listener rule failed: %v", err)
	}

	tcpClient := &http.Client{
		Transport: &http.Transport{
			Proxy: nil,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				ServerName:         "example.com",
			},
		},
		Timeout: 15 * time.Second,
	}
	tcpReq, err := http.NewRequest(http.MethodGet, "https://127.0.0.1:"+strconv.Itoa(listenPort)+"/h2/ping", nil)
	if err != nil {
		t.Fatalf("build h2 request failed: %v", err)
	}
	tcpReq.Host = "example.com"
	tcpResp, err := tcpClient.Do(tcpReq)
	if err != nil {
		t.Fatalf("h2 request failed: %v", err)
	}
	defer tcpResp.Body.Close()
	if tcpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(tcpResp.Body)
		t.Fatalf("unexpected h2 proxy status: %d body=%q", tcpResp.StatusCode, string(body))
	}

	transport := &http3.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         "example.com",
		},
	}
	defer func() {
		_ = transport.Close()
	}()
	h3Client := &http.Client{
		Transport: transport,
		Timeout:   8 * time.Second,
	}
	h3Req, err := http.NewRequest(http.MethodGet, "https://127.0.0.1:"+strconv.Itoa(listenPort)+"/h3/ping", nil)
	if err != nil {
		t.Fatalf("build h3 request failed: %v", err)
	}
	h3Req.Host = "example.com"
	h3Resp, err := h3Client.Do(h3Req)
	if err == nil {
		_, _ = io.ReadAll(h3Resp.Body)
		_ = h3Resp.Body.Close()
		t.Fatal("expected h3 request to fail for h2-only listener")
	}
}

func TestReverseProxyHTTPSListenerH3OnlyRejectsTCPHTTPSClient(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	t.Cleanup(func() {
		_ = svc.StopRuntime()
	})

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok:" + r.URL.Path))
	}))
	defer upstream.Close()

	upstreamHost, upstreamPort := splitReverseProxyTestServerAddress(t, upstream.URL)
	listenPort := reserveReverseProxyTestPort(t)
	certRecordID := createReverseProxyTestCertificateRecord(t, "example.com")

	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:                      "https-h3-only-listener",
		Enabled:                   true,
		ListenProtocol:            reverseProxyProtocolHTTPS,
		ListenHTTPVersionStrategy: reverseProxyListenHTTPVersionH3Only,

		Hosts:               "example.com",
		ListenPort:          listenPort,
		TargetProtocol:      reverseProxyProtocolHTTP,
		TargetAddresses:     upstreamHost,
		TargetPort:          upstreamPort,
		CertificateRecordID: certRecordID,
		IPStrategy:          reverseProxyIPStrategyPreferIPv4,
	}); err != nil {
		t.Fatalf("upsert https-h3-only-listener rule failed: %v", err)
	}

	transport := &http3.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         "example.com",
		},
	}
	defer func() {
		_ = transport.Close()
	}()
	h3Client := &http.Client{
		Transport: transport,
		Timeout:   15 * time.Second,
	}
	h3Req, err := http.NewRequest(http.MethodGet, "https://127.0.0.1:"+strconv.Itoa(listenPort)+"/h3/ping", nil)
	if err != nil {
		t.Fatalf("build h3 request failed: %v", err)
	}
	h3Req.Host = "example.com"
	h3Resp, err := h3Client.Do(h3Req)
	if err != nil {
		t.Fatalf("h3 request failed: %v", err)
	}
	defer h3Resp.Body.Close()
	if h3Resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(h3Resp.Body)
		t.Fatalf("unexpected h3 proxy status: %d body=%q", h3Resp.StatusCode, string(body))
	}

	tcpClient := &http.Client{
		Transport: &http.Transport{
			Proxy: nil,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				ServerName:         "example.com",
			},
		},
		Timeout: 8 * time.Second,
	}
	tcpReq, err := http.NewRequest(http.MethodGet, "https://127.0.0.1:"+strconv.Itoa(listenPort)+"/h2/ping", nil)
	if err != nil {
		t.Fatalf("build tcp tls request failed: %v", err)
	}
	tcpReq.Host = "example.com"
	tcpResp, err := tcpClient.Do(tcpReq)
	if err == nil {
		_, _ = io.ReadAll(tcpResp.Body)
		_ = tcpResp.Body.Close()
		t.Fatal("expected tcp https request to fail for h3-only listener")
	}
}

func TestReverseProxyVirtualH2ListenerProxiesToVirtualH3Upstream(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	t.Cleanup(func() {
		_ = svc.StopRuntime()
	})

	upstreamHost, upstreamPort := startReverseProxyTestHTTP3Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(r.Proto))
	}))
	listenPort := reserveReverseProxyTestPort(t)
	certRecordID := createReverseProxyTestCertificateRecord(t, "example.com")

	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:           "virtual-h2-to-h3",
		Enabled:        true,
		ListenProtocol: "h2",

		Hosts:               "example.com",
		ListenPort:          listenPort,
		TargetProtocol:      "h3",
		TargetAddresses:     upstreamHost,
		TargetPort:          upstreamPort,
		CertificateRecordID: certRecordID,
		IPStrategy:          reverseProxyIPStrategyPreferIPv4,
		UpstreamTLSVerify:   false,
	}); err != nil {
		t.Fatalf("upsert virtual h2->h3 rule failed: %v", err)
	}

	h2Transport := &http2.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         "example.com",
		},
	}
	t.Cleanup(h2Transport.CloseIdleConnections)
	client := &http.Client{
		Transport: h2Transport,
		Timeout:   15 * time.Second,
	}
	req, err := http.NewRequest(http.MethodGet, "https://127.0.0.1:"+strconv.Itoa(listenPort)+"/bridge/h2-to-h3", nil)
	if err != nil {
		t.Fatalf("build h2 client request failed: %v", err)
	}
	req.Host = "example.com"

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("h2 listener request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read h2->h3 response failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected h2->h3 proxy status: %d body=%q", resp.StatusCode, string(body))
	}
	if got := string(body); !strings.HasPrefix(got, "HTTP/3") {
		t.Fatalf("expected HTTP/3 upstream response, got %q", got)
	}
}

func TestReverseProxyVirtualH3ListenerProxiesToVirtualH2Upstream(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	t.Cleanup(func() {
		_ = svc.StopRuntime()
	})

	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(r.Proto))
	}))
	upstream.EnableHTTP2 = true
	upstream.StartTLS()
	defer upstream.Close()

	upstreamHost, upstreamPort := splitReverseProxyTestServerAddress(t, upstream.URL)
	listenPort := reserveReverseProxyTestPort(t)
	certRecordID := createReverseProxyTestCertificateRecord(t, "example.com")

	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:           "virtual-h3-to-h2",
		Enabled:        true,
		ListenProtocol: "h3",

		Hosts:               "example.com",
		ListenPort:          listenPort,
		TargetProtocol:      "h2",
		TargetAddresses:     upstreamHost,
		TargetPort:          upstreamPort,
		CertificateRecordID: certRecordID,
		IPStrategy:          reverseProxyIPStrategyPreferIPv4,
		UpstreamTLSVerify:   false,
	}); err != nil {
		t.Fatalf("upsert virtual h3->h2 rule failed: %v", err)
	}

	transport := &http3.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         "example.com",
		},
	}
	defer func() {
		_ = transport.Close()
	}()
	client := &http.Client{
		Transport: transport,
		Timeout:   15 * time.Second,
	}
	req, err := http.NewRequest(http.MethodGet, "https://127.0.0.1:"+strconv.Itoa(listenPort)+"/bridge/h3-to-h2", nil)
	if err != nil {
		t.Fatalf("build h3 client request failed: %v", err)
	}
	req.Host = "example.com"

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("h3 listener request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read h3->h2 response failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected h3->h2 proxy status: %d body=%q", resp.StatusCode, string(body))
	}
	if got := string(body); !strings.HasPrefix(got, "HTTP/2") {
		t.Fatalf("expected HTTP/2 upstream response, got %q", got)
	}
}

func TestReverseProxyAllowsH2AndH3OnlyOnSamePort(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	t.Cleanup(func() {
		_ = svc.StopRuntime()
	})

	certRecordID := createReverseProxyTestCertificateRecord(t, "example.com")
	listenPort := reserveReverseProxyTestPort(t)

	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:                      "https-listener-h2-only",
		Enabled:                   false,
		ListenProtocol:            reverseProxyProtocolHTTPS,
		ListenHTTPVersionStrategy: reverseProxyListenHTTPVersionH2Only,

		Hosts:               "example.com",
		ListenPort:          listenPort,
		PathPrefix:          "/a",
		TargetProtocol:      reverseProxyProtocolHTTP,
		TargetAddresses:     "127.0.0.1",
		TargetPort:          18080,
		CertificateRecordID: certRecordID,
		IPStrategy:          reverseProxyIPStrategyPreferIPv4,
	}); err != nil {
		t.Fatalf("create h2-only rule failed: %v", err)
	}

	err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:                      "https-listener-h3-only",
		Enabled:                   false,
		ListenProtocol:            reverseProxyProtocolHTTPS,
		ListenHTTPVersionStrategy: reverseProxyListenHTTPVersionH3Only,

		Hosts:               "example.com",
		ListenPort:          listenPort,
		PathPrefix:          "/b",
		TargetProtocol:      reverseProxyProtocolHTTP,
		TargetAddresses:     "127.0.0.1",
		TargetPort:          18081,
		CertificateRecordID: certRecordID,
		IPStrategy:          reverseProxyIPStrategyPreferIPv4,
	})
	if err != nil {
		t.Fatalf("h2-only and h3-only should coexist on separate tcp/udp sockets: %v", err)
	}
}

func TestReverseProxyRewritesAbsoluteOriginsToListenerHost(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	t.Cleanup(func() {
		_ = svc.StopRuntime()
	})

	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		upstreamOrigin := "https://" + r.Host
		escapedUpstreamOrigin := strings.ReplaceAll(upstreamOrigin, "/", `\/`)
		_, _ = w.Write([]byte(`<a href="` + upstreamOrigin + `/article">link</a><script>const api="` + escapedUpstreamOrigin + `\/wp-json";</script>`))
	}))
	defer upstream.Close()

	upstreamHost, upstreamPort := splitReverseProxyTestServerAddress(t, upstream.URL)
	listenPort := reserveReverseProxyTestPort(t)
	certRecordID := createReverseProxyTestCertificateRecord(t, "example.com")

	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:           "rewrite-body",
		Enabled:        true,
		ListenProtocol: reverseProxyProtocolHTTPS,

		ListenPort:          listenPort,
		TargetProtocol:      reverseProxyProtocolHTTPS,
		TargetAddresses:     upstreamHost,
		TargetPort:          upstreamPort,
		CertificateRecordID: certRecordID,
		IPStrategy:          reverseProxyIPStrategyPreferIPv4,
		HTTPVersionStrategy: reverseProxyHTTPVersionPreferH2,
		UpstreamTLSVerify:   false,
	}); err != nil {
		t.Fatalf("upsert rewrite-body rule failed: %v", err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: nil,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				ServerName:         "example.com",
			},
		},
		Timeout: 15 * time.Second,
	}
	req, err := http.NewRequest(http.MethodGet, "https://127.0.0.1:"+strconv.Itoa(listenPort)+"/", nil)
	if err != nil {
		t.Fatalf("build rewrite request failed: %v", err)
	}
	req.Host = "example.com"

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("rewrite request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read rewrite body failed: %v", err)
	}
	text := string(body)
	if strings.Contains(text, "https://"+upstreamHost) {
		t.Fatalf("expected upstream origin to be removed from body, got %q", text)
	}
	if !strings.Contains(text, `https://example.com/article`) {
		t.Fatalf("expected rewritten external link in body, got %q", text)
	}
	if !strings.Contains(text, `https:\/\/example.com\/wp-json`) {
		t.Fatalf("expected rewritten escaped external origin in body, got %q", text)
	}
}

func TestReverseProxyAPIPassthroughPreservesResponseBodyAndAcceptEncoding(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	t.Cleanup(func() {
		_ = svc.StopRuntime()
	})

	acceptEncoding := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		acceptEncoding <- r.Header.Get("Accept-Encoding")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<a href="http://` + r.Host + `/article">link</a>`))
	}))
	defer upstream.Close()

	upstreamHost, upstreamPort := splitReverseProxyTestServerAddress(t, upstream.URL)
	listenPort := reserveReverseProxyTestPort(t)

	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:           "api-passthrough-body",
		Enabled:        true,
		ListenProtocol: reverseProxyProtocolHTTP,

		ListenPort:      listenPort,
		Hosts:           "example.com",
		TargetProtocol:  reverseProxyProtocolHTTP,
		TargetAddresses: upstreamHost,
		TargetPort:      upstreamPort,
		IPStrategy:      reverseProxyIPStrategyPreferIPv4,
		ApiPassthrough:  true,
	}); err != nil {
		t.Fatalf("upsert api passthrough body rule failed: %v", err)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(listenPort)+"/", nil)
	if err != nil {
		t.Fatalf("build api passthrough request failed: %v", err)
	}
	req.Host = "example.com"
	req.Header.Set("Accept-Encoding", "gzip")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("api passthrough request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read api passthrough body failed: %v", err)
	}
	text := string(body)
	expectedUpstreamOrigin := "http://" + net.JoinHostPort(upstreamHost, strconv.Itoa(upstreamPort)) + "/article"
	if !strings.Contains(text, expectedUpstreamOrigin) {
		t.Fatalf("expected upstream origin to remain in passthrough body, got %q", text)
	}
	if strings.Contains(text, "http://example.com/article") {
		t.Fatalf("expected passthrough body to avoid external origin rewrite, got %q", text)
	}

	select {
	case got := <-acceptEncoding:
		if got != "gzip" {
			t.Fatalf("expected upstream accept-encoding to remain gzip, got %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for upstream accept-encoding")
	}
}

func TestReverseProxyPreservesQueryWhenRewritingPath(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	t.Cleanup(func() {
		_ = svc.StopRuntime()
	})

	type upstreamRequest struct {
		path  string
		query string
	}
	requests := make(chan upstreamRequest, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- upstreamRequest{path: r.URL.EscapedPath(), query: r.URL.RawQuery}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	upstreamHost, upstreamPort := splitReverseProxyTestServerAddress(t, upstream.URL)
	listenPort := reserveReverseProxyTestPort(t)
	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:            "query-preserved-under-path-prefix",
		Enabled:         true,
		ListenProtocol:  reverseProxyProtocolHTTP,
		ListenPort:      listenPort,
		Hosts:           "example.com",
		PathPrefix:      "/app",
		TargetProtocol:  reverseProxyProtocolHTTP,
		TargetAddresses: upstreamHost,
		TargetPort:      upstreamPort,
		TargetPath:      "/base",
		IPStrategy:      reverseProxyIPStrategyPreferIPv4,
	}); err != nil {
		t.Fatalf("upsert query preservation rule failed: %v", err)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(listenPort)+"/%61pp/file%2Fname?q=one%2Ftwo&empty=&flag", nil)
	if err != nil {
		t.Fatalf("build query preservation request failed: %v", err)
	}
	req.Host = "example.com"
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("query preservation request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("unexpected query preservation status: %d", resp.StatusCode)
	}

	select {
	case got := <-requests:
		if got.path != "/base/file/name" || got.query != "q=one%2Ftwo&empty=&flag" {
			t.Fatalf("unexpected rewritten upstream request: path=%q query=%q", got.path, got.query)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("upstream did not receive query preservation request")
	}
}

func TestReverseProxyForwardsWebSocketUpgradeAndPath(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	t.Cleanup(func() {
		_ = svc.StopRuntime()
	})

	upstreamRequests := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamRequests <- r.URL.EscapedPath() + "?" + r.URL.RawQuery
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept upstream websocket failed: %v", err)
			return
		}
		defer func() {
			_ = conn.CloseNow()
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		messageType, payload, err := conn.Read(ctx)
		if err != nil {
			t.Errorf("read upstream websocket payload failed: %v", err)
			return
		}
		if err := conn.Write(ctx, messageType, append([]byte("echo:"), payload...)); err != nil {
			t.Errorf("write upstream websocket payload failed: %v", err)
		}
	}))
	defer upstream.Close()

	upstreamHost, upstreamPort := splitReverseProxyTestServerAddress(t, upstream.URL)
	listenPort := reserveReverseProxyTestPort(t)
	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:                "websocket-upgrade-through-prefix",
		Enabled:             true,
		ListenProtocol:      reverseProxyProtocolHTTP,
		ListenProtocolAlias: "ws",
		ListenPort:          listenPort,
		Hosts:               "example.com",
		PathPrefix:          "/app",
		TargetProtocol:      reverseProxyProtocolHTTP,
		TargetProtocolAlias: "ws",
		TargetAddresses:     upstreamHost,
		TargetPort:          upstreamPort,
		IPStrategy:          reverseProxyIPStrategyPreferIPv4,
	}); err != nil {
		t.Fatalf("upsert websocket proxy rule failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, handshakeResponse, err := websocket.Dial(ctx, "ws://127.0.0.1:"+strconv.Itoa(listenPort)+"/app/socket?token=abc%2Fdef", &websocket.DialOptions{
		HTTPClient: &http.Client{
			Transport: &http.Transport{Proxy: nil},
		},
		Host: "example.com",
	})
	if err != nil {
		body := ""
		if handshakeResponse != nil && handshakeResponse.Body != nil {
			content, _ := io.ReadAll(handshakeResponse.Body)
			_ = handshakeResponse.Body.Close()
			body = string(content)
		}
		t.Fatalf("dial websocket through reverse proxy failed: %v body=%q runtime=%#v", err, body, reverseProxyRuntime.snapshotRuleStates())
	}
	defer func() {
		_ = conn.CloseNow()
	}()
	if err := conn.Write(ctx, websocket.MessageText, []byte("hello")); err != nil {
		t.Fatalf("write websocket payload failed: %v", err)
	}
	messageType, payload, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read websocket payload failed: %v", err)
	}
	if messageType != websocket.MessageText || string(payload) != "echo:hello" {
		t.Fatalf("unexpected websocket echo: type=%v payload=%q", messageType, string(payload))
	}
	select {
	case got := <-upstreamRequests:
		if got != "/socket?token=abc%2Fdef" {
			t.Fatalf("unexpected websocket upstream path: %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("upstream did not receive websocket upgrade")
	}
}

func TestReverseProxyAPIPassthroughStreamsSSE(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	t.Cleanup(func() {
		_ = svc.StopRuntime()
	})

	firstChunkSent := make(chan struct{})
	releaseSecondChunk := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("expected http flusher for SSE upstream")
			return
		}
		_, _ = io.WriteString(w, "data: hello\n\n")
		flusher.Flush()
		close(firstChunkSent)
		<-releaseSecondChunk
		_, _ = io.WriteString(w, "data: world\n\n")
		flusher.Flush()
	}))
	defer upstream.Close()

	upstreamHost, upstreamPort := splitReverseProxyTestServerAddress(t, upstream.URL)
	listenPort := reserveReverseProxyTestPort(t)

	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:           "api-passthrough-sse",
		Enabled:        true,
		ListenProtocol: reverseProxyProtocolHTTP,

		ListenPort:      listenPort,
		PathPrefix:      "/12345",
		Hosts:           "example.com",
		TargetProtocol:  reverseProxyProtocolHTTP,
		TargetAddresses: upstreamHost,
		TargetPort:      upstreamPort,
		IPStrategy:      reverseProxyIPStrategyPreferIPv4,
		ApiPassthrough:  true,
	}); err != nil {
		t.Fatalf("upsert api passthrough sse rule failed: %v", err)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(listenPort)+"/12345/v1/responses", nil)
	if err != nil {
		t.Fatalf("build api passthrough sse request failed: %v", err)
	}
	req.Host = "example.com"

	respCh := make(chan *http.Response, 1)
	errCh := make(chan error, 1)
	go func() {
		resp, err := client.Do(req)
		if err != nil {
			errCh <- err
			return
		}
		respCh <- resp
	}()

	select {
	case <-firstChunkSent:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for upstream first SSE chunk")
	}

	var resp *http.Response
	select {
	case err := <-errCh:
		t.Fatalf("api passthrough sse request failed early: %v", err)
	case resp = <-respCh:
	case <-time.After(2 * time.Second):
		t.Fatal("expected proxy response headers before upstream stream completed")
	}
	defer resp.Body.Close()

	lineCh := make(chan string, 1)
	readErrCh := make(chan error, 1)
	go func() {
		line, err := bufio.NewReader(resp.Body).ReadString('\n')
		if err != nil {
			readErrCh <- err
			return
		}
		lineCh <- line
	}()

	select {
	case err := <-readErrCh:
		t.Fatalf("read first SSE line failed: %v", err)
	case line := <-lineCh:
		if line != "data: hello\n" {
			t.Fatalf("unexpected first SSE line: %q", line)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected first SSE line to reach client before upstream finished")
	}

	close(releaseSecondChunk)
}

func TestReverseProxyRewritesRootRelativeLinksWithLocalPathPrefix(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	t.Cleanup(func() {
		_ = svc.StopRuntime()
	})

	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<a href="/tag/mysql/">tag</a><img src="/wp-content/app.css"><script>const api="/wp-json";const escaped="\/feed\/";</script><style>.hero{background-image:url(/images/bg.png)}</style>`))
	}))
	defer upstream.Close()

	upstreamHost, upstreamPort := splitReverseProxyTestServerAddress(t, upstream.URL)
	listenPort := reserveReverseProxyTestPort(t)
	certRecordID := createReverseProxyTestCertificateRecord(t, "example.com")

	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:           "rewrite-prefix-links",
		Enabled:        true,
		ListenProtocol: reverseProxyProtocolHTTPS,

		ListenPort:          listenPort,
		PathPrefix:          "/88999",
		TargetProtocol:      reverseProxyProtocolHTTPS,
		TargetAddresses:     upstreamHost,
		TargetPort:          upstreamPort,
		CertificateRecordID: certRecordID,
		IPStrategy:          reverseProxyIPStrategyPreferIPv4,
		HTTPVersionStrategy: reverseProxyHTTPVersionPreferH2,
		UpstreamTLSVerify:   false,
	}); err != nil {
		t.Fatalf("upsert rewrite-prefix-links rule failed: %v", err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: nil,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				ServerName:         "example.com",
			},
		},
		Timeout: 15 * time.Second,
	}
	req, err := http.NewRequest(http.MethodGet, "https://127.0.0.1:"+strconv.Itoa(listenPort)+"/88999", nil)
	if err != nil {
		t.Fatalf("build prefix rewrite request failed: %v", err)
	}
	req.Host = "example.com"

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("prefix rewrite request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read prefix rewrite body failed: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, `href="/88999/tag/mysql/"`) {
		t.Fatalf("expected prefixed anchor path, got %q", text)
	}
	if !strings.Contains(text, `src="/88999/wp-content/app.css"`) {
		t.Fatalf("expected prefixed asset path, got %q", text)
	}
	if !strings.Contains(text, `const api="/88999/wp-json"`) {
		t.Fatalf("expected prefixed javascript path, got %q", text)
	}
	if !strings.Contains(text, `const escaped="/88999\/feed\/"`) {
		t.Fatalf("expected prefixed escaped path, got %q", text)
	}
	if !strings.Contains(text, `url(/88999/images/bg.png)`) {
		t.Fatalf("expected prefixed css url path, got %q", text)
	}
}

func TestReverseProxyRewrittenAssetURLsLoadCorrectAssetPath(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	t.Cleanup(func() {
		_ = svc.StopRuntime()
	})

	requestPaths := make(chan string, 4)
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPaths <- r.URL.Path
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<link rel="stylesheet" href="https://` + r.Host + `/wp-content/app.css">`))
		case "/wp-content/app.css":
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
			_, _ = w.Write([]byte("body{background:#fff}"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	upstreamHost, upstreamPort := splitReverseProxyTestServerAddress(t, upstream.URL)
	listenPort := reserveReverseProxyTestPort(t)
	certRecordID := createReverseProxyTestCertificateRecord(t, "example.com")

	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:           "rewrite-asset-paths",
		Enabled:        true,
		ListenProtocol: reverseProxyProtocolHTTPS,

		ListenPort:          listenPort,
		PathPrefix:          "/88999",
		TargetProtocol:      reverseProxyProtocolHTTPS,
		TargetAddresses:     upstreamHost,
		TargetPort:          upstreamPort,
		CertificateRecordID: certRecordID,
		IPStrategy:          reverseProxyIPStrategyPreferIPv4,
		HTTPVersionStrategy: reverseProxyHTTPVersionPreferH2,
		UpstreamTLSVerify:   false,
	}); err != nil {
		t.Fatalf("upsert rewrite-asset-paths rule failed: %v", err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: nil,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				ServerName:         "example.com",
			},
		},
		Timeout: 15 * time.Second,
	}

	rootReq, err := http.NewRequest(http.MethodGet, "https://127.0.0.1:"+strconv.Itoa(listenPort)+"/88999", nil)
	if err != nil {
		t.Fatalf("build root request failed: %v", err)
	}
	rootReq.Host = "example.com"

	rootResp, err := client.Do(rootReq)
	if err != nil {
		t.Fatalf("root request failed: %v", err)
	}
	rootBody, err := io.ReadAll(rootResp.Body)
	_ = rootResp.Body.Close()
	if err != nil {
		t.Fatalf("read root body failed: %v", err)
	}
	if rootResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected root status: %d body=%q", rootResp.StatusCode, string(rootBody))
	}
	rootText := string(rootBody)
	if !strings.Contains(rootText, `https://example.com/88999/wp-content/app.css`) {
		t.Fatalf("expected rewritten asset url in root html, got %q", rootText)
	}

	assetReq, err := http.NewRequest(http.MethodGet, "https://127.0.0.1:"+strconv.Itoa(listenPort)+"/88999/wp-content/app.css", nil)
	if err != nil {
		t.Fatalf("build asset request failed: %v", err)
	}
	assetReq.Host = "example.com"

	assetResp, err := client.Do(assetReq)
	if err != nil {
		t.Fatalf("asset request failed: %v", err)
	}
	defer assetResp.Body.Close()

	assetBody, err := io.ReadAll(assetResp.Body)
	if err != nil {
		t.Fatalf("read asset body failed: %v", err)
	}
	if assetResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected asset status: %d body=%q", assetResp.StatusCode, string(assetBody))
	}
	if got := string(assetBody); got != "body{background:#fff}" {
		t.Fatalf("expected css asset body, got %q", got)
	}
	if got := reverseProxyReadTestRequestPath(t, requestPaths); got != "/" {
		t.Fatalf("unexpected first upstream request path: %q", got)
	}
	if got := reverseProxyReadTestRequestPath(t, requestPaths); got != "/wp-content/app.css" {
		t.Fatalf("unexpected asset upstream request path: %q", got)
	}
}

func TestReverseProxyRoutesByConfiguredPathPrefixes(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	t.Cleanup(func() {
		_ = svc.StopRuntime()
	})

	upstreamA := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("apad:" + r.URL.Path))
	}))
	defer upstreamA.Close()
	upstreamB := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("google:" + r.URL.Path))
	}))
	defer upstreamB.Close()

	upstreamAHost, upstreamAPort := splitReverseProxyTestServerAddress(t, upstreamA.URL)
	upstreamBHost, upstreamBPort := splitReverseProxyTestServerAddress(t, upstreamB.URL)
	listenPort := reserveReverseProxyTestPort(t)
	certRecordID := createReverseProxyTestCertificateRecord(t, "example.com")

	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:           "route-apad",
		Enabled:        true,
		ListenProtocol: reverseProxyProtocolHTTPS,

		ListenPort:          listenPort,
		Hosts:               "example.com",
		PathPrefix:          "/88999",
		TargetProtocol:      reverseProxyProtocolHTTPS,
		TargetAddresses:     upstreamAHost,
		TargetPort:          upstreamAPort,
		CertificateRecordID: certRecordID,
		IPStrategy:          reverseProxyIPStrategyPreferIPv4,
		HTTPVersionStrategy: reverseProxyHTTPVersionPreferH2,
		UpstreamTLSVerify:   false,
	}); err != nil {
		t.Fatalf("upsert route-apad rule failed: %v", err)
	}
	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:           "route-google",
		Enabled:        true,
		ListenProtocol: reverseProxyProtocolHTTPS,

		ListenPort:          listenPort,
		Hosts:               "example.com",
		PathPrefix:          "/aaa",
		TargetProtocol:      reverseProxyProtocolHTTPS,
		TargetAddresses:     upstreamBHost,
		TargetPort:          upstreamBPort,
		CertificateRecordID: certRecordID,
		IPStrategy:          reverseProxyIPStrategyPreferIPv4,
		HTTPVersionStrategy: reverseProxyHTTPVersionPreferH2,
		UpstreamTLSVerify:   false,
	}); err != nil {
		t.Fatalf("upsert route-google rule failed: %v", err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: nil,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				ServerName:         "example.com",
			},
		},
		Timeout: 15 * time.Second,
	}

	for _, tc := range []struct {
		path       string
		wantBody   string
		wantStatus int
	}{
		{path: "/88999/tag/mysql/", wantBody: "apad:/tag/mysql/", wantStatus: http.StatusOK},
		{path: "/aaa", wantBody: "google:/", wantStatus: http.StatusOK},
		{path: "/bbb", wantBody: "", wantStatus: http.StatusNotFound},
	} {
		req, err := http.NewRequest(http.MethodGet, "https://127.0.0.1:"+strconv.Itoa(listenPort)+tc.path, nil)
		if err != nil {
			t.Fatalf("build routed request failed: %v", err)
		}
		req.Host = "example.com"

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("routed request failed for %s: %v", tc.path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode != tc.wantStatus {
			t.Fatalf("unexpected routed status for %s: %d body=%q", tc.path, resp.StatusCode, string(body))
		}
		if tc.wantBody != "" && string(body) != tc.wantBody {
			t.Fatalf("unexpected routed body for %s: got %q want %q", tc.path, string(body), tc.wantBody)
		}
	}
}

func TestReverseProxyHTTPSRuleRejectsMismatchedSNIOrHost(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	t.Cleanup(func() {
		_ = svc.StopRuntime()
	})

	var upstreamHits int32
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&upstreamHits, 1)
		_, _ = w.Write([]byte("unexpected"))
	}))
	defer upstream.Close()

	upstreamHost, upstreamPort := splitReverseProxyTestServerAddress(t, upstream.URL)
	listenPort := reserveReverseProxyTestPort(t)
	certRecordID := createReverseProxyTestCertificateRecord(t, "example.com")

	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:           "reject-mismatch",
		Enabled:        true,
		ListenProtocol: reverseProxyProtocolHTTPS,

		ListenPort:          listenPort,
		Hosts:               "example.com",
		PathPrefix:          "/88999",
		TargetProtocol:      reverseProxyProtocolHTTPS,
		TargetAddresses:     upstreamHost,
		TargetPort:          upstreamPort,
		CertificateRecordID: certRecordID,
		IPStrategy:          reverseProxyIPStrategyPreferIPv4,
		HTTPVersionStrategy: reverseProxyHTTPVersionPreferH2,
		UpstreamTLSVerify:   false,
	}); err != nil {
		t.Fatalf("upsert reject-mismatch rule failed: %v", err)
	}

	for _, tc := range []struct {
		name            string
		serverName      string
		host            string
		handshakeReject bool
	}{
		{name: "bad_sni", serverName: "wrong.example.com", host: "example.com", handshakeReject: true},
		{name: "bad_host", serverName: "example.com", host: "wrong.example.com"},
	} {
		client := &http.Client{
			Transport: &http.Transport{
				Proxy: nil,
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
					ServerName:         tc.serverName,
				},
			},
			Timeout: 15 * time.Second,
		}
		req, err := http.NewRequest(http.MethodGet, "https://127.0.0.1:"+strconv.Itoa(listenPort)+"/88999", nil)
		if err != nil {
			t.Fatalf("build mismatch request failed: %v", err)
		}
		req.Host = tc.host

		resp, err := client.Do(req)
		if tc.handshakeReject {
			if err == nil {
				_ = resp.Body.Close()
				t.Fatalf("strict tls listener must reject %s during handshake", tc.name)
			}
			continue
		}
		if err != nil {
			t.Fatalf("mismatched request failed for %s: %v", tc.name, err)
		}
		_, _ = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusMisdirectedRequest {
			t.Fatalf("expected 421 for %s, got %d", tc.name, resp.StatusCode)
		}
	}

	if got := atomic.LoadInt32(&upstreamHits); got != 0 {
		t.Fatalf("mismatched requests must not reach upstream, got %d hits", got)
	}
}

func TestReverseProxyHTTPSRuleRejectsIPAccessWithoutSNI(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	t.Cleanup(func() {
		_ = svc.StopRuntime()
	})

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok:" + r.URL.Path))
	}))
	defer upstream.Close()

	upstreamHost, upstreamPort := splitReverseProxyTestServerAddress(t, upstream.URL)
	listenPort := reserveReverseProxyTestPort(t)
	certRecordID := createReverseProxyTestCertificateRecord(t, "example.com")

	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:           "no-sni-rejected",
		Enabled:        true,
		ListenProtocol: reverseProxyProtocolHTTPS,

		ListenPort: listenPort,
		Hosts:      "example.com",

		PathPrefix:          "/000",
		TargetProtocol:      reverseProxyProtocolHTTP,
		TargetAddresses:     upstreamHost,
		TargetPort:          upstreamPort,
		CertificateRecordID: certRecordID,
		IPStrategy:          reverseProxyIPStrategyPreferIPv4,
	}); err != nil {
		t.Fatalf("upsert no-sni rejection rule failed: %v", err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: nil,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
		Timeout: 15 * time.Second,
	}

	req, err := http.NewRequest(http.MethodGet, "https://127.0.0.1:"+strconv.Itoa(listenPort)+"/000/app/img", nil)
	if err != nil {
		t.Fatalf("build no-sni request failed: %v", err)
	}
	req.Host = "127.0.0.1"

	resp, err := client.Do(req)
	if err == nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		t.Fatal("ip access without sni must fail during the TLS handshake")
	}
}

func TestReverseProxyHTTPSIPRuleCoexistsWithDomainRuleOnSameListener(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	t.Cleanup(func() {
		_ = svc.StopRuntime()
	})

	upstreamDomain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("domain:" + r.URL.Path))
	}))
	defer upstreamDomain.Close()
	upstreamIP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ip:" + r.URL.Path))
	}))
	defer upstreamIP.Close()

	upstreamDomainHost, upstreamDomainPort := splitReverseProxyTestServerAddress(t, upstreamDomain.URL)
	upstreamIPHost, upstreamIPPort := splitReverseProxyTestServerAddress(t, upstreamIP.URL)
	listenPort := reserveReverseProxyTestPort(t)
	domainCertID := createReverseProxyTestCertificateRecord(t, "example.com")
	ipCertID := createReverseProxyTestCertificateRecord(t, "127.0.0.1")

	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:           "domain-first",
		Enabled:        true,
		ListenProtocol: reverseProxyProtocolHTTPS,

		ListenPort:          listenPort,
		Hosts:               "example.com",
		PathPrefix:          "",
		TargetProtocol:      reverseProxyProtocolHTTP,
		TargetAddresses:     upstreamDomainHost,
		TargetPort:          upstreamDomainPort,
		CertificateRecordID: domainCertID,
		IPStrategy:          reverseProxyIPStrategyPreferIPv4,
	}); err != nil {
		t.Fatalf("upsert domain rule failed: %v", err)
	}
	err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:           "ip-direct",
		Enabled:        true,
		ListenProtocol: reverseProxyProtocolHTTPS,

		ListenPort:          listenPort,
		PathPrefix:          "/88999",
		TargetProtocol:      reverseProxyProtocolHTTP,
		TargetAddresses:     upstreamIPHost,
		TargetPort:          upstreamIPPort,
		CertificateRecordID: ipCertID,
		IPStrategy:          reverseProxyIPStrategyPreferIPv4,
	})
	if err != nil {
		t.Fatalf("IP and domain connection classes must coexist on the same listener: %v", err)
	}
}

func TestReverseProxyDisableRuleDoesNotFailWhenActiveRequestBlocksShutdown(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	t.Cleanup(func() {
		_ = svc.StopRuntime()
	})

	block := make(chan struct{})
	started := make(chan struct{}, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case started <- struct{}{}:
		default:
		}
		select {
		case <-block:
		case <-r.Context().Done():
		}
	}))
	defer upstream.Close()

	upstreamHost, upstreamPort := splitReverseProxyTestServerAddress(t, upstream.URL)
	listenPort := reserveReverseProxyTestPort(t)

	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:           "shutdown-timeout",
		Enabled:        true,
		ListenProtocol: reverseProxyProtocolHTTP,

		ListenPort:      listenPort,
		PathPrefix:      "",
		TargetProtocol:  reverseProxyProtocolHTTP,
		TargetAddresses: upstreamHost,
		TargetPort:      upstreamPort,
		IPStrategy:      reverseProxyIPStrategyPreferIPv4,
	}); err != nil {
		t.Fatalf("upsert blocking rule failed: %v", err)
	}

	var saved model.ReverseProxyRule
	if err := database.GetDB().Where("name = ?", "shutdown-timeout").First(&saved).Error; err != nil {
		t.Fatalf("load blocking rule failed: %v", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(listenPort)+"/", nil)
	if err != nil {
		t.Fatalf("build blocking request failed: %v", err)
	}
	req.Host = "127.0.0.1"

	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		resp, err := client.Do(req)
		if err == nil && resp != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for blocking upstream request")
	}

	begin := time.Now()
	err = svc.UpsertRule(ReverseProxyRulePayload{
		ID:             saved.Id,
		Name:           saved.Name,
		Enabled:        false,
		ListenProtocol: saved.ListenProtocol,

		ListenPort:        saved.ListenPort,
		Hosts:             strings.Join(decodeReverseProxyList(saved.HostList), ","),
		PathPrefix:        saved.PathPrefix,
		TargetProtocol:    saved.TargetProtocol,
		TargetAddresses:   strings.Join(decodeReverseProxyList(saved.TargetAddresses), ","),
		TargetPort:        saved.TargetPort,
		TargetPath:        saved.TargetPath,
		IPStrategy:        saved.IPStrategy,
		UpstreamTLSVerify: saved.UpstreamTLSVerify,
		ApiPassthrough:    saved.ApiPassthrough,
	})
	close(block)
	<-requestDone
	if err != nil {
		t.Fatalf("disable rule should force close active listeners instead of failing: %v", err)
	}
	if elapsed := time.Since(begin); elapsed > reverseProxyShutdownTimeout+2*time.Second {
		t.Fatalf("disable rule took too long: %v", elapsed)
	}
}

func TestReverseProxyRewritesLocationAndCookieDomain(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	t.Cleanup(func() {
		_ = svc.StopRuntime()
	})

	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "https://"+r.Host+"/next")
		w.Header().Add("Set-Cookie", "sid=1; Path=/; Domain="+strings.Split(r.Host, ":")[0]+"; HttpOnly")
		w.WriteHeader(http.StatusFound)
	}))
	defer upstream.Close()

	upstreamHost, upstreamPort := splitReverseProxyTestServerAddress(t, upstream.URL)
	listenPort := reserveReverseProxyTestPort(t)
	certRecordID := createReverseProxyTestCertificateRecord(t, "example.com")

	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:           "rewrite-headers",
		Enabled:        true,
		ListenProtocol: reverseProxyProtocolHTTPS,

		ListenPort:          listenPort,
		PathPrefix:          "/panel",
		TargetProtocol:      reverseProxyProtocolHTTPS,
		TargetAddresses:     upstreamHost,
		TargetPort:          upstreamPort,
		CertificateRecordID: certRecordID,
		IPStrategy:          reverseProxyIPStrategyPreferIPv4,
		HTTPVersionStrategy: reverseProxyHTTPVersionPreferH2,
		UpstreamTLSVerify:   false,
	}); err != nil {
		t.Fatalf("upsert rewrite-headers rule failed: %v", err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: nil,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				ServerName:         "example.com",
			},
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: 15 * time.Second,
	}
	req, err := http.NewRequest(http.MethodGet, "https://127.0.0.1:"+strconv.Itoa(listenPort)+"/panel/redirect", nil)
	if err != nil {
		t.Fatalf("build redirect request failed: %v", err)
	}
	req.Host = "example.com"

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("redirect request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		t.Fatalf("unexpected redirect status: %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Location"); got != "https://example.com/panel/next" {
		t.Fatalf("unexpected rewritten location header: %q", got)
	}
	cookieValues := resp.Header.Values("Set-Cookie")
	if len(cookieValues) == 0 {
		t.Fatal("expected rewritten set-cookie header")
	}
	if !strings.Contains(cookieValues[0], "Domain=example.com") {
		t.Fatalf("expected rewritten cookie domain, got %q", cookieValues[0])
	}
	if !strings.Contains(cookieValues[0], "Path=/panel") {
		t.Fatalf("expected rewritten cookie path, got %q", cookieValues[0])
	}
}

func TestReverseProxyAPIPassthroughStillRewritesLocationAndCookieDomain(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	t.Cleanup(func() {
		_ = svc.StopRuntime()
	})

	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "https://"+r.Host+"/next")
		w.Header().Add("Set-Cookie", "sid=1; Path=/; Domain="+strings.Split(r.Host, ":")[0]+"; HttpOnly")
		w.WriteHeader(http.StatusFound)
	}))
	defer upstream.Close()

	upstreamHost, upstreamPort := splitReverseProxyTestServerAddress(t, upstream.URL)
	listenPort := reserveReverseProxyTestPort(t)
	certRecordID := createReverseProxyTestCertificateRecord(t, "example.com")

	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:           "rewrite-headers-api-passthrough",
		Enabled:        true,
		ListenProtocol: reverseProxyProtocolHTTPS,

		ListenPort:          listenPort,
		PathPrefix:          "/panel",
		TargetProtocol:      reverseProxyProtocolHTTPS,
		TargetAddresses:     upstreamHost,
		TargetPort:          upstreamPort,
		CertificateRecordID: certRecordID,
		IPStrategy:          reverseProxyIPStrategyPreferIPv4,
		HTTPVersionStrategy: reverseProxyHTTPVersionPreferH2,
		UpstreamTLSVerify:   false,
		ApiPassthrough:      true,
	}); err != nil {
		t.Fatalf("upsert rewrite-headers-api-passthrough rule failed: %v", err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: nil,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				ServerName:         "example.com",
			},
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: 15 * time.Second,
	}
	req, err := http.NewRequest(http.MethodGet, "https://127.0.0.1:"+strconv.Itoa(listenPort)+"/panel/redirect", nil)
	if err != nil {
		t.Fatalf("build api passthrough redirect request failed: %v", err)
	}
	req.Host = "example.com"

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("api passthrough redirect request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		t.Fatalf("unexpected api passthrough redirect status: %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Location"); got != "https://example.com/panel/next" {
		t.Fatalf("unexpected rewritten location header in api passthrough mode: %q", got)
	}
	cookieValues := resp.Header.Values("Set-Cookie")
	if len(cookieValues) == 0 {
		t.Fatal("expected rewritten set-cookie header in api passthrough mode")
	}
	if !strings.Contains(cookieValues[0], "Domain=example.com") {
		t.Fatalf("expected rewritten cookie domain in api passthrough mode, got %q", cookieValues[0])
	}
	if !strings.Contains(cookieValues[0], "Path=/panel") {
		t.Fatalf("expected rewritten cookie path in api passthrough mode, got %q", cookieValues[0])
	}
}

func TestReverseProxyRewritesParentCookieDomain(t *testing.T) {
	got := reverseProxyRewriteSetCookieHeader(
		"sid=1; Path=/; Domain=.example.com; HttpOnly",
		"sub.example.com",
		"example.com",
	)
	if !strings.Contains(got, "Domain=example.com") {
		t.Fatalf("expected parent-domain cookie rewrite, got %q", got)
	}
	if strings.Contains(got, "Domain=.example.com") {
		t.Fatalf("expected leading-dot domain to be removed, got %q", got)
	}
}

func TestReverseProxyRewritesCookiePathWithinProxyPrefix(t *testing.T) {
	got := reverseProxyRewriteSetCookieHeaderWithPath(
		"sid=1; Path=/api/session; Domain=upstream.example; Secure; HttpOnly; SameSite=Lax",
		"upstream.example",
		"panel.example",
		"/api",
		"/panel",
	)
	if !strings.Contains(got, "Path=/panel/session") {
		t.Fatalf("expected rewritten cookie path, got %q", got)
	}
	if !strings.Contains(got, "Domain=panel.example") {
		t.Fatalf("expected rewritten cookie domain, got %q", got)
	}
	for _, attribute := range []string{"Secure", "HttpOnly", "SameSite=Lax"} {
		if !strings.Contains(got, attribute) {
			t.Fatalf("expected cookie attribute %q to be preserved, got %q", attribute, got)
		}
	}

	hostCookie := reverseProxyRewriteSetCookieHeaderWithPath(
		"__Host-session=1; Path=/; Domain=upstream.example; Secure; HttpOnly",
		"upstream.example",
		"panel.example",
		"",
		"/panel",
	)
	if !strings.Contains(hostCookie, "Path=/") || strings.Contains(hostCookie, "Domain=") {
		t.Fatalf("expected __Host- cookie to remain valid, got %q", hostCookie)
	}
}

func TestReverseProxyHTTP3AdvertisementHeaderAggregatesByOrigin(t *testing.T) {
	disabledPath := &model.ReverseProxyRule{
		ListenProtocol:            reverseProxyProtocolHTTPS,
		ListenHTTPVersionStrategy: reverseProxyListenHTTPVersionH2H3,
		HostList:                  encodeReverseProxyList([]string{"example.com"}),
		PathPrefix:                "/off",
	}
	enabledPath := &model.ReverseProxyRule{
		ListenProtocol:            reverseProxyProtocolHTTPS,
		ListenHTTPVersionStrategy: reverseProxyListenHTTPVersionH2H3,
		HostList:                  encodeReverseProxyList([]string{"example.com"}),
		PathPrefix:                "/on",
		AdvertiseHTTP3:            true,
	}
	otherOrigin := &model.ReverseProxyRule{
		ListenProtocol:            reverseProxyProtocolHTTPS,
		ListenHTTPVersionStrategy: reverseProxyListenHTTPVersionH2H3,
		HostList:                  encodeReverseProxyList([]string{"other.example"}),
	}
	group := &reverseProxyListenerGroup{
		listenPort:                8443,
		protocol:                  reverseProxyProtocolHTTPS,
		listenHTTPVersionStrategy: reverseProxyListenHTTPVersionH2H3,
		rules:                     []*model.ReverseProxyRule{disabledPath, enabledPath, otherOrigin},
	}

	if got := group.http3AdvertisementHeader("example.com", "example.com", 1); got != `h3=":8443"; ma=300` {
		t.Fatalf("same-origin enabled rule must advertise for every path, got %q", got)
	}
	if got := group.http3AdvertisementHeader("example.com", "example.com", 3); got != "" {
		t.Fatalf("http3 response should not advertise itself again, got %q", got)
	}
	if got := group.http3AdvertisementHeader("other.example", "other.example", 1); got != "clear" {
		t.Fatalf("another origin must keep its independent clear policy, got %q", got)
	}
	if got := group.http3AdvertisementHeader("unknown.example", "unknown.example", 1); got != "" {
		t.Fatalf("unknown origin must not receive an Alt-Svc policy, got %q", got)
	}

	enabledPath.AdvertiseHTTP3 = false
	if got := group.http3AdvertisementHeader("example.com", "example.com", 1); got != "clear" {
		t.Fatalf("origin with every rule disabled must clear cached h3 routes, got %q", got)
	}
	group.listenHTTPVersionStrategy = reverseProxyListenHTTPVersionH2Only
	if got := group.http3AdvertisementHeader("example.com", "example.com", 1); got != "clear" {
		t.Fatalf("h2-only listener must clear a previously advertised h3 route, got %q", got)
	}
	group.listenHTTPVersionStrategy = reverseProxyListenHTTPVersionH3Only
	if got := group.http3AdvertisementHeader("example.com", "example.com", 3); got != "" {
		t.Fatalf("h3-only listener must retain forced-h3 behavior without Alt-Svc, got %q", got)
	}
}

func TestReverseProxyHTTP3AdvertisementHeaderIgnoresWSSRules(t *testing.T) {
	legacyWSSRule := &model.ReverseProxyRule{
		ListenProtocol:            reverseProxyProtocolHTTPS,
		ListenProtocolAlias:       "wss",
		ListenHTTPVersionStrategy: reverseProxyListenHTTPVersionH2H3,
		HostList:                  encodeReverseProxyList([]string{"example.com"}),
		AdvertiseHTTP3:            true,
	}
	group := &reverseProxyListenerGroup{
		listenPort:                8443,
		protocol:                  reverseProxyProtocolHTTPS,
		listenHTTPVersionStrategy: reverseProxyListenHTTPVersionH2H3,
		rules:                     []*model.ReverseProxyRule{legacyWSSRule},
	}

	if got := group.http3AdvertisementHeader("example.com", "example.com", 1); got != "clear" {
		t.Fatalf("legacy wss rule must clear cached h3 routes, got %q", got)
	}

	pageRule := &model.ReverseProxyRule{
		ListenProtocol:            reverseProxyProtocolHTTPS,
		ListenHTTPVersionStrategy: reverseProxyListenHTTPVersionH2H3,
		HostList:                  encodeReverseProxyList([]string{"example.com"}),
		AdvertiseHTTP3:            true,
	}
	group.rules = []*model.ReverseProxyRule{legacyWSSRule, pageRule}
	if got := group.http3AdvertisementHeader("example.com", "example.com", 1); got != `h3=":8443"; ma=300` {
		t.Fatalf("regular https rule must continue advertising h3, got %q", got)
	}
}

func TestReverseProxyHTTPHandlerReturnsExact404ForUnknownHost(t *testing.T) {
	group := &reverseProxyListenerGroup{
		protocol: reverseProxyProtocolHTTP,
		rules: []*model.ReverseProxyRule{{
			ListenProtocol: reverseProxyProtocolHTTP,
			HostList:       encodeReverseProxyList([]string{"example.com"}),
		}},
	}
	req := httptest.NewRequest(http.MethodGet, "http://wrong.example/", nil)
	req.Host = "wrong.example"
	recorder := httptest.NewRecorder()
	group.newHandler().ServeHTTP(recorder, req)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("unknown plain-http host must return 404, got %d", recorder.Code)
	}
}

func TestReverseProxyHTTP3AdvertisementCoversOriginResponsesOnce(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	t.Cleanup(func() {
		_ = svc.StopRuntime()
	})

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Alt-Svc", `h3=":9999"; ma=86400`)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()
	upstreamHost, upstreamPort := splitReverseProxyTestServerAddress(t, upstream.URL)
	unavailablePort := reserveReverseProxyTestPort(t)
	listenPort := reserveReverseProxyTestPort(t)
	exampleCertID := createReverseProxyTestCertificateRecord(t, "example.com")
	isolatedCertID := createReverseProxyTestCertificateRecord(t, "isolated.example")

	upsert := func(name string, host string, path string, targetPort int, certificateID uint, advertise bool) {
		t.Helper()
		if err := svc.UpsertRule(ReverseProxyRulePayload{
			Name:           name,
			Enabled:        true,
			ListenProtocol: reverseProxyProtocolHTTPS,

			ListenPort:          listenPort,
			Hosts:               host,
			PathPrefix:          path,
			TargetProtocol:      reverseProxyProtocolHTTP,
			TargetAddresses:     upstreamHost,
			TargetPort:          targetPort,
			CertificateRecordID: certificateID,
			IPStrategy:          reverseProxyIPStrategyPreferIPv4,
			AdvertiseHTTP3:      advertise,
		}); err != nil {
			t.Fatalf("upsert %s rule failed: %v", name, err)
		}
	}
	upsert("same-origin-off", "example.com", "/off", upstreamPort, exampleCertID, false)
	upsert("same-origin-on", "example.com", "/on", upstreamPort, exampleCertID, true)
	upsert("same-origin-error", "example.com", "/error", unavailablePort, exampleCertID, false)
	upsert("isolated-origin", "isolated.example", "", upstreamPort, isolatedCertID, false)

	request := func(serverName string, host string, path string) *http.Response {
		t.Helper()
		client := &http.Client{
			Transport: &http.Transport{
				Proxy: nil,
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
					ServerName:         serverName,
				},
			},
			Timeout: 15 * time.Second,
		}
		req, err := http.NewRequest(http.MethodGet, "https://127.0.0.1:"+strconv.Itoa(listenPort)+path, nil)
		if err != nil {
			t.Fatalf("build Alt-Svc request failed: %v", err)
		}
		req.Host = host
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Alt-Svc request failed for %s%s: %v", host, path, err)
		}
		return resp
	}
	assertResponse := func(resp *http.Response, wantStatus int, wantAltSvc string) {
		t.Helper()
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
		if resp.StatusCode != wantStatus {
			t.Fatalf("unexpected response status: got %d want %d", resp.StatusCode, wantStatus)
		}
		values := resp.Header.Values("Alt-Svc")
		if wantAltSvc == "" {
			if len(values) != 0 {
				t.Fatalf("unexpected Alt-Svc values: %#v", values)
			}
			return
		}
		if len(values) != 1 || values[0] != wantAltSvc {
			t.Fatalf("expected exactly one Alt-Svc value %q, got %#v", wantAltSvc, values)
		}
	}

	advertisement := reverseProxyAltSvcValue(listenPort)
	assertResponse(request("example.com", "example.com", "/off"), http.StatusNoContent, advertisement)
	assertResponse(request("example.com", "example.com", "/missing"), http.StatusNotFound, advertisement)
	assertResponse(request("example.com", "example.com", "/error"), http.StatusBadGateway, advertisement)
	assertResponse(request("isolated.example", "isolated.example", "/"), http.StatusNoContent, "clear")
	assertResponse(request("example.com", "unknown.example", "/off"), http.StatusMisdirectedRequest, "")
}

func TestReverseProxyForwardsStandardXForwardedHeadersOnce(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	t.Cleanup(func() {
		_ = svc.StopRuntime()
	})

	type forwardedHeaders struct {
		forValue  string
		forwarded string
		host      string
		proto     string
		port      string
		realIP    string
	}
	headerCh := make(chan forwardedHeaders, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headerCh <- forwardedHeaders{
			forValue:  r.Header.Get("X-Forwarded-For"),
			forwarded: r.Header.Get("Forwarded"),
			host:      r.Header.Get("X-Forwarded-Host"),
			proto:     r.Header.Get("X-Forwarded-Proto"),
			port:      r.Header.Get("X-Forwarded-Port"),
			realIP:    r.Header.Get("X-Real-IP"),
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	upstreamHost, upstreamPort := splitReverseProxyTestServerAddress(t, upstream.URL)
	listenPort := reserveReverseProxyTestPort(t)
	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:            "forwarded-headers",
		Enabled:         true,
		ListenProtocol:  reverseProxyProtocolHTTP,
		ListenPort:      listenPort,
		Hosts:           "example.com",
		TargetProtocol:  reverseProxyProtocolHTTP,
		TargetAddresses: upstreamHost,
		TargetPort:      upstreamPort,
		IPStrategy:      reverseProxyIPStrategyPreferIPv4,
	}); err != nil {
		t.Fatalf("upsert forwarded header rule failed: %v", err)
	}

	client := &http.Client{
		Transport: &http.Transport{Proxy: nil},
		Timeout:   15 * time.Second,
	}
	for _, host := range []string{"example.com", "example.com:12345"} {
		req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(listenPort)+"/", nil)
		if err != nil {
			t.Fatalf("build forwarded request failed: %v", err)
		}
		req.Host = host
		req.Header.Set("X-Forwarded-For", "198.51.100.10")
		req.Header.Set("Forwarded", "for=198.51.100.10;proto=https")
		req.Header.Set("X-Real-IP", "198.51.100.10")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("forwarded request failed: %v", err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("unexpected forwarded request status: %d", resp.StatusCode)
		}

		select {
		case got := <-headerCh:
			if got.forValue != "127.0.0.1" {
				t.Fatalf("unexpected X-Forwarded-For value: %q", got.forValue)
			}
			if got.forwarded != `for=127.0.0.1;host="`+req.Host+`";proto=http` {
				t.Fatalf("unexpected Forwarded value: %q", got.forwarded)
			}
			if got.host != req.Host || got.proto != "http" || got.port != strconv.Itoa(listenPort) || got.realIP != "127.0.0.1" {
				t.Fatalf("unexpected forwarded headers for Host %q: %#v", req.Host, got)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("upstream did not receive forwarded headers")
		}
	}
}

func TestComputeReverseProxyRenderKeyIncludesHTTPProtocolAliases(t *testing.T) {
	rows := []model.ReverseProxyRule{{
		Id:                  1,
		Enabled:             true,
		ListenProtocol:      reverseProxyProtocolHTTPS,
		ListenProtocolAlias: "wss",
		ListenPort:          443,
		TargetProtocol:      reverseProxyProtocolHTTPS,
		TargetProtocolAlias: "wss",
		TargetAddresses:     encodeReverseProxyList([]string{"upstream.example"}),
		TargetPort:          443,
		IPStrategy:          reverseProxyIPStrategyPreferIPv4,
	}}
	before := computeReverseProxyRenderKey(nil, rows)
	rows[0].TargetProtocolAlias = ""
	afterTargetAlias := computeReverseProxyRenderKey(nil, rows)
	if before == afterTargetAlias {
		t.Fatal("target protocol alias change must refresh the http runtime")
	}
	rows[0].ListenProtocolAlias = ""
	afterListenAlias := computeReverseProxyRenderKey(nil, rows)
	if afterTargetAlias == afterListenAlias {
		t.Fatal("listen protocol alias change must refresh the http runtime")
	}
}

func TestReverseProxyRewriteResponseBodySkipsProtocolUpgrade(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusSwitchingProtocols,
		Header:     http.Header{"Content-Type": []string{"text/html"}},
		Body:       io.NopCloser(strings.NewReader("https://upstream.example/socket")),
	}
	plan := reverseProxyResponseRewritePlan{
		Enabled: true,
		Replacements: []reverseProxyStringReplacement{{
			Old: "https://upstream.example",
			New: "https://panel.example",
		}},
	}
	if err := reverseProxyRewriteResponseBody(resp, plan); err != nil {
		t.Fatalf("skip upgrade response rewrite failed: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read upgrade response body failed: %v", err)
	}
	if got := string(body); got != "https://upstream.example/socket" {
		t.Fatalf("upgrade response must not be buffered or rewritten, got %q", got)
	}
}

func TestReverseProxyCachedUpstreamExpiresAfterResolutionTTL(t *testing.T) {
	var cleanupCalls int32
	upstream := &reverseProxyCachedUpstream{
		ResolvedAt:   time.Now().Add(-reverseProxyUpstreamResolveCacheTTL - time.Second),
		RoundTripper: http.DefaultTransport,
		Cleanup: func() {
			atomic.AddInt32(&cleanupCalls, 1)
		},
	}
	group := &reverseProxyListenerGroup{
		upstreamByRule: map[uint]*reverseProxyCachedUpstream{1: upstream},
	}
	if got := group.acquireCachedUpstream(1); got != nil {
		t.Fatalf("expected expired upstream cache miss, got %#v", got)
	}
	if got := atomic.LoadInt32(&cleanupCalls); got != 1 {
		t.Fatalf("expected expired transport cleanup once, got %d", got)
	}
	if _, exists := group.upstreamByRule[1]; exists {
		t.Fatal("expected expired upstream to be removed from cache")
	}
}

func TestReverseProxyUpstreamFailureReturnsBadGateway(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	t.Cleanup(func() {
		_ = svc.StopRuntime()
	})

	listenPort := reserveReverseProxyTestPort(t)
	targetPort := reserveReverseProxyTestPort(t)
	certRecordID := createReverseProxyTestCertificateRecord(t, "example.com")

	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:           "https-upstream-failure",
		Enabled:        true,
		ListenProtocol: reverseProxyProtocolHTTPS,

		ListenPort:          listenPort,
		TargetProtocol:      reverseProxyProtocolHTTP,
		TargetAddresses:     "127.0.0.1",
		TargetPort:          targetPort,
		CertificateRecordID: certRecordID,
		IPStrategy:          reverseProxyIPStrategyPreferIPv4,
	}); err != nil {
		t.Fatalf("upsert failure reverse proxy rule failed: %v", err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: nil,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				ServerName:         "example.com",
			},
		},
		Timeout: 15 * time.Second,
	}
	req, err := http.NewRequest(http.MethodGet, "https://127.0.0.1:"+strconv.Itoa(listenPort)+"/", nil)
	if err != nil {
		t.Fatalf("build request failed: %v", err)
	}
	req.Host = "example.com"

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("proxy failure request should return gateway status, got transport err: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway && resp.StatusCode != http.StatusGatewayTimeout {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected proxy failure status: %d body=%q", resp.StatusCode, string(body))
	}
}

func createReverseProxyTestCertificateRecord(t *testing.T, name string) uint {
	t.Helper()

	certPEM, keyPEM := buildReverseProxyTestCertificatePEM(t, []string{name})
	certificateType := "domain"
	if net.ParseIP(name) != nil {
		certificateType = "ip"
	}
	row := model.CertificateRecord{
		SourceType:      CertificateSourceSelfSigned,
		SourceRef:       "reverse-proxy-test-" + name,
		MainDomain:      name,
		DomainSet:       `["` + name + `"]`,
		CertPEM:         certPEM,
		KeyPEM:          keyPEM,
		FullchainPEM:    certPEM,
		ListOrderAt:     time.Now().Unix(),
		CertificateType: certificateType,
	}
	if err := database.GetDB().Create(&row).Error; err != nil {
		t.Fatalf("create reverse proxy certificate record failed: %v", err)
	}
	return row.Id
}

func TestReverseProxyParsedCertificateCacheIsScopedToDatabaseInstance(t *testing.T) {
	openReverseProxyTestDB(t)

	firstID := createReverseProxyTestCertificateRecord(t, "first-cache.example.com")
	firstMaterials, err := loadReverseProxyParsedCertificateMaterials([]uint{firstID}, false)
	if err != nil {
		t.Fatalf("load first database certificate material failed: %v", err)
	}
	firstMaterial := firstMaterials[firstID]
	if firstMaterial.Leaf == nil || firstMaterial.Leaf.Leaf == nil {
		t.Fatal("first database certificate material is incomplete")
	}

	secondDBPath := filepath.Join(t.TempDir(), "reverse-proxy-cache-second.db")
	if err := database.InitDB(secondDBPath); err != nil {
		t.Fatalf("init second database failed: %v", err)
	}
	secondSQLDB, err := database.GetDB().DB()
	if err != nil {
		t.Fatalf("open second database handle failed: %v", err)
	}
	t.Cleanup(func() {
		_ = secondSQLDB.Close()
	})

	secondID := createReverseProxyTestCertificateRecord(t, "second-cache.example.com")
	if secondID != firstID {
		t.Fatalf("test requires the same certificate id across databases: first=%d second=%d", firstID, secondID)
	}
	secondMaterials, err := loadReverseProxyParsedCertificateMaterials([]uint{secondID}, false)
	if err != nil {
		t.Fatalf("load second database certificate material failed: %v", err)
	}
	secondMaterial := secondMaterials[secondID]
	if secondMaterial.Leaf == nil || secondMaterial.Leaf.Leaf == nil {
		t.Fatal("second database certificate material is incomplete")
	}
	if err := secondMaterial.Leaf.Leaf.VerifyHostname("second-cache.example.com"); err != nil {
		t.Fatalf("second database certificate was not loaded: %v", err)
	}
	if err := secondMaterial.Leaf.Leaf.VerifyHostname("first-cache.example.com"); err == nil {
		t.Fatal("certificate cache reused material from the previous database instance")
	}
}

func TestReverseProxyCertificatePreparationUsesSingleBatchQuery(t *testing.T) {
	openReverseProxyTestDB(t)

	certificateIDs := []uint{
		createReverseProxyTestCertificateRecord(t, "batch-one.example.com"),
		createReverseProxyTestCertificateRecord(t, "batch-two.example.com"),
		createReverseProxyTestCertificateRecord(t, "batch-three.example.com"),
	}
	rows := make([]model.ReverseProxyRule, 0, 30)
	for index := 0; index < 30; index++ {
		certificateID := certificateIDs[index%len(certificateIDs)]
		rows = append(rows, model.ReverseProxyRule{
			Id:                    uint(index + 1),
			Enabled:               true,
			ListenProtocol:        reverseProxyProtocolHTTPS,
			CertificateRecordID:   certificateID,
			CertificateRecordList: encodeReverseProxyUintList([]uint{certificateID}),
		})
	}

	var certificateQueries atomic.Int64
	callbackName := "reverse_proxy_test_certificate_batch_query"
	db := database.GetDB()
	if err := db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if strings.EqualFold(strings.TrimSpace(tx.Statement.Table), "certificate_records") {
			certificateQueries.Add(1)
		}
	}); err != nil {
		t.Fatalf("register certificate query counter failed: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Callback().Query().Remove(callbackName)
	})

	if err := prepareReverseProxyParsedCertificateMaterials(rows); err != nil {
		t.Fatalf("prepare referenced certificate materials failed: %v", err)
	}
	if got := certificateQueries.Load(); got != 1 {
		t.Fatalf("referenced certificates must be loaded by one batch query, got %d", got)
	}
}

func TestReverseProxyRuntimeOverviewDoesNotQuerySQLite(t *testing.T) {
	openReverseProxyTestDB(t)

	var queries atomic.Int64
	callbackName := "reverse_proxy_test_runtime_overview_query"
	db := database.GetDB()
	if err := db.Callback().Query().Before("gorm:query").Register(callbackName, func(*gorm.DB) {
		queries.Add(1)
	}); err != nil {
		t.Fatalf("register runtime query counter failed: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Callback().Query().Remove(callbackName)
	})

	if _, err := (&ReverseProxyService{}).GetRuntimeOverview(); err != nil {
		t.Fatalf("load runtime overview failed: %v", err)
	}
	if got := queries.Load(); got != 0 {
		t.Fatalf("runtime overview must remain SQLite-free, got %d queries", got)
	}
}

type reverseProxyTestResolver struct {
	addrs []netip.Addr
	err   error
}

func (r reverseProxyTestResolver) LookupNetIP(_ context.Context, _ string, _ string) ([]netip.Addr, error) {
	if r.err != nil {
		return nil, r.err
	}
	out := make([]netip.Addr, len(r.addrs))
	copy(out, r.addrs)
	return out, nil
}

func reserveReverseProxyTestUDPPort(t *testing.T) int {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve udp port failed: %v", err)
	}
	port := conn.LocalAddr().(*net.UDPAddr).Port
	if err := conn.Close(); err != nil {
		t.Fatalf("release udp port reservation failed: %v", err)
	}
	return port
}

func startReverseProxyTestDNSServer(t *testing.T, ttl uint32, answer string) (int, *int32) {
	t.Helper()
	packetConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen test dns udp server failed: %v", err)
	}
	var hits int32
	server := &dns.Server{
		PacketConn: packetConn,
		Handler: dns.HandlerFunc(func(w dns.ResponseWriter, req *dns.Msg) {
			atomic.AddInt32(&hits, 1)
			resp := new(dns.Msg)
			resp.SetReply(req)
			if len(req.Question) > 0 {
				resp.Answer = []dns.RR{&dns.A{
					Hdr: dns.RR_Header{
						Name:   req.Question[0].Name,
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    ttl,
					},
					A: net.ParseIP(answer).To4(),
				}}
			}
			_ = w.WriteMsg(resp)
		}),
	}
	go func() {
		_ = server.ActivateAndServe()
	}()
	t.Cleanup(func() {
		_ = server.Shutdown()
		_ = packetConn.Close()
	})
	return packetConn.LocalAddr().(*net.UDPAddr).Port, &hits
}

func resolveReverseProxyTestDNSHandler(handler *reverseProxyDNSRuleHandler, name string, clientAddr string) (*dns.Msg, error) {
	req := new(dns.Msg)
	req.SetQuestion(name, dns.TypeA)
	return resolveReverseProxyTestDNSHandlerRequest(handler, req, clientAddr)
}

func resolveReverseProxyTestDNSHandlerRequest(handler *reverseProxyDNSRuleHandler, req *dns.Msg, clientAddr string) (*dns.Msg, error) {
	addr, err := netip.ParseAddrPort(clientAddr)
	if err != nil {
		return nil, err
	}
	dctx := &dnsproxy.DNSContext{
		Req:   req,
		Addr:  addr,
		Proto: dnsproxy.ProtoUDP,
	}
	err = handler.ServeDNS(context.Background(), nil, dctx)
	if err != nil {
		return dctx.Res, err
	}
	if dctx.Res == nil {
		return nil, errors.New("dns handler returned no response")
	}
	return dctx.Res, nil
}

func reverseProxyTestDNSAnswerIPv4(t *testing.T, response *dns.Msg) string {
	t.Helper()
	if response == nil || len(response.Answer) != 1 {
		t.Fatalf("expected one DNS answer, got %#v", response)
	}
	answer, ok := response.Answer[0].(*dns.A)
	if !ok || answer.A.To4() == nil {
		t.Fatalf("expected IPv4 DNS answer, got %#v", response.Answer[0])
	}
	return answer.A.To4().String()
}

func startReverseProxyTestHTTP3Server(t *testing.T, handler http.Handler) (string, int) {
	t.Helper()

	packetConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen test http3 udp port failed: %v", err)
	}
	certPEM, keyPEM := buildReverseProxyTestCertificatePEM(t, []string{"127.0.0.1"})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		_ = packetConn.Close()
		t.Fatalf("load test http3 certificate failed: %v", err)
	}

	port := packetConn.LocalAddr().(*net.UDPAddr).Port
	server := &http3.Server{
		Handler: handler,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS13,
		},
		Port: port,
	}
	serveErrCh := make(chan error, 1)
	go func() {
		serveErrCh <- server.Serve(packetConn)
	}()
	t.Cleanup(func() {
		_ = server.Close()
		_ = packetConn.Close()
	})

	deadline := time.Now().Add(2 * time.Second)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		probeErr := probeReverseProxyHTTP3(ctx, "127.0.0.1", port, "127.0.0.1", false)
		cancel()
		if probeErr == nil {
			return "127.0.0.1", port
		}
		select {
		case serveErr := <-serveErrCh:
			if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) && !errors.Is(serveErr, net.ErrClosed) {
				t.Fatalf("test http3 server exited early: %v", serveErr)
			}
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("test http3 server did not become ready: %v", probeErr)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func buildReverseProxyTestCertificatePEM(t *testing.T, names []string) ([]byte, []byte) {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate reverse proxy test key failed: %v", err)
	}

	dnsNames := make([]string, 0, len(names))
	ipAddresses := make([]net.IP, 0, len(names))
	for _, name := range names {
		if ip := net.ParseIP(strings.TrimSpace(name)); ip != nil {
			ipAddresses = append(ipAddresses, ip)
			continue
		}
		dnsNames = append(dnsNames, strings.TrimSpace(name))
	}
	commonName := strings.TrimSpace(names[0])

	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			CommonName: commonName,
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              dnsNames,
		IPAddresses:           ipAddresses,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("create reverse proxy test certificate failed: %v", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		t.Fatalf("marshal reverse proxy test private key failed: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM
}

func loadReverseProxyBindingForTest(certPEM []byte, keyPEM []byte) (*tls.Certificate, *x509LeafState, error) {
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, nil, err
	}
	leaf, err := network.ParseLeafCertificate(&pair)
	if err != nil {
		return nil, nil, err
	}
	return &pair, &x509LeafState{
		Certificate: &pair,
		Leaf:        leaf,
		Fingerprint: "",
		NotAfter:    leaf.NotAfter,
		HasIPSAN:    len(leaf.IPAddresses) > 0,
	}, nil
}

func reverseProxyDialWithSNIFingerprint(t *testing.T, listenPort int, serverName string) string {
	t.Helper()

	conn, err := tls.DialWithDialer(
		&net.Dialer{Timeout: 5 * time.Second},
		"tcp",
		"127.0.0.1:"+strconv.Itoa(listenPort),
		&tls.Config{InsecureSkipVerify: true, ServerName: serverName},
	)
	if err != nil {
		t.Fatalf("dial reverse proxy with sni failed: %v", err)
	}
	defer conn.Close()

	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		t.Fatal("reverse proxy did not present a certificate")
	}
	sum := sha256.Sum256(state.PeerCertificates[0].Raw)
	return hex.EncodeToString(sum[:])
}

func splitReverseProxyTestServerAddress(t *testing.T, rawURL string) (string, int) {
	t.Helper()

	hostPort := strings.TrimPrefix(strings.TrimSpace(rawURL), "https://")
	hostPort = strings.TrimPrefix(hostPort, "http://")
	host, portText, err := net.SplitHostPort(hostPort)
	if err != nil {
		t.Fatalf("split test server address failed: %v", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("parse test server port failed: %v", err)
	}
	return host, port
}

func reverseProxyTestLeafState(names ...string) *x509LeafState {
	dnsNames := make([]string, 0, len(names))
	ipNames := make([]net.IP, 0, len(names))
	for _, name := range names {
		cleaned := strings.TrimSpace(name)
		if cleaned == "" {
			continue
		}
		if parsed := net.ParseIP(cleaned); parsed != nil {
			ipNames = append(ipNames, parsed)
			continue
		}
		dnsNames = append(dnsNames, cleaned)
	}
	leaf := &x509.Certificate{
		DNSNames:    dnsNames,
		IPAddresses: ipNames,
	}
	return &x509LeafState{
		Leaf:     leaf,
		HasIPSAN: len(ipNames) > 0,
	}
}

type reverseProxyTestConn struct {
	local net.Addr
}

func (c reverseProxyTestConn) Read(_ []byte) (int, error)         { return 0, io.EOF }
func (c reverseProxyTestConn) Write(b []byte) (int, error)        { return len(b), nil }
func (c reverseProxyTestConn) Close() error                       { return nil }
func (c reverseProxyTestConn) LocalAddr() net.Addr                { return c.local }
func (c reverseProxyTestConn) RemoteAddr() net.Addr               { return &net.TCPAddr{} }
func (c reverseProxyTestConn) SetDeadline(_ time.Time) error      { return nil }
func (c reverseProxyTestConn) SetReadDeadline(_ time.Time) error  { return nil }
func (c reverseProxyTestConn) SetWriteDeadline(_ time.Time) error { return nil }

func reserveReverseProxyTestPort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve test port failed: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	return port
}

func reverseProxyReadTestRequestPath(t *testing.T, paths <-chan string) string {
	t.Helper()

	select {
	case path := <-paths:
		return path
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for upstream request path")
		return ""
	}
}

func openReverseProxyTestDB(t *testing.T) {
	t.Helper()

	_ = (&ReverseProxyService{}).StopRuntime()
	t.Cleanup(func() {
		_ = (&ReverseProxyService{}).StopRuntime()
	})

	dbPath := filepath.Join(t.TempDir(), "reverse-proxy.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("init db failed: %v", err)
	}

	sqlDB, err := database.GetDB().DB()
	if err == nil && sqlDB != nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}
}
