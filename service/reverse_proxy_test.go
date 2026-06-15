package service

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
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
	if len(normalized.listenIPs) != 0 || len(normalized.hosts) != 0 {
		t.Fatalf("expected empty listen match, got ips=%#v hosts=%#v", normalized.listenIPs, normalized.hosts)
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
			name:        "preserve encoded upstream remainder",
			path:        "/88999/tag/mysql/",
			rawPath:     "/88999/tag%2Fmysql/",
			prefix:      "/88999",
			wantPath:    "/tag/mysql/",
			wantRawPath: "/tag%2Fmysql/",
		},
		{
			name:        "encoded local prefix falls back to decoded trim",
			path:        "/88999/tag/mysql/",
			rawPath:     "/%38%38%39%39%39/tag/mysql/",
			prefix:      "/88999",
			wantPath:    "/tag/mysql/",
			wantRawPath: "/tag/mysql/",
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
				Name:                "forward-request-path-" + tc.name,
				Enabled:             true,
				ListenProtocol:      reverseProxyProtocolHTTPS,
				ListenIPs:           "example.com",
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

func TestDecodeReverseProxyListenIPs_FallbackToLegacy(t *testing.T) {
	row := &model.ReverseProxyRule{ListenIP: "1.2.3.4"}
	got := decodeReverseProxyListenIPs(row)
	if len(got) != 1 || got[0] != "1.2.3.4" {
		t.Fatalf("unexpected listen ips: %#v", got)
	}
}

func TestValidateReverseProxyNoObviousLoop(t *testing.T) {
	err := validateReverseProxyNoObviousLoop(reverseProxyNormalizedRule{
		listenProtocol:  "https",
		listenIPs:       []string{"127.0.0.1"},
		listenPort:      443,
		targetProtocol:  "https",
		targetAddresses: []string{"127.0.0.1"},
		targetPort:      443,
	})
	if err == nil {
		t.Fatal("expected obvious loop to fail")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "must not point back") {
		t.Fatalf("unexpected error: %v", err)
	}

	err = validateReverseProxyNoObviousLoop(reverseProxyNormalizedRule{
		listenProtocol:  "https",
		listenIPs:       []string{"127.0.0.1"},
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
			ListenIPList:        `["127.0.0.1"]`,
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
			ListenIPList:        `["127.0.0.1"]`,
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

func TestReverseProxyNoSNIUsesReloadedCertificate(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	t.Cleanup(func() {
		_ = svc.StopRuntime()
	})

	certPEMOld, keyPEMOld := buildReverseProxyTestCertificatePEM(t, []string{"127.0.0.1"})
	oldFingerprint, _, _, err := inspectCertificateFingerprint(certPEMOld, keyPEMOld)
	if err != nil {
		t.Fatalf("inspect old certificate failed: %v", err)
	}
	cert := model.CertificateRecord{
		SourceType:      CertificateSourceSelfSigned,
		SourceRef:       "reverse-proxy-nosni-reload",
		MainDomain:      "127.0.0.1",
		DomainSet:       `["127.0.0.1"]`,
		CertPEM:         certPEMOld,
		KeyPEM:          keyPEMOld,
		FullchainPEM:    certPEMOld,
		Fingerprint:     oldFingerprint,
		ListOrderAt:     time.Now().Unix(),
		CertificateType: "ip",
	}
	if err := database.GetDB().Create(&cert).Error; err != nil {
		t.Fatalf("create certificate record failed: %v", err)
	}

	listenPort := reserveReverseProxyTestPort(t)
	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:                "nosni-reload",
		Enabled:             true,
		ListenProtocol:      reverseProxyProtocolHTTPS,
		ListenPort:          listenPort,
		TargetProtocol:      reverseProxyProtocolHTTP,
		TargetAddresses:     "127.0.0.1",
		TargetPort:          reserveReverseProxyTestPort(t),
		CertificateRecordID: cert.Id,
		IPStrategy:          reverseProxyIPStrategyPreferIPv4,
	}); err != nil {
		t.Fatalf("upsert no-sni reverse proxy rule failed: %v", err)
	}

	if got := reverseProxyDialNoSNIFingerprint(t, listenPort); got != oldFingerprint {
		t.Fatalf("initial no-sni certificate fingerprint = %q, want %q", got, oldFingerprint)
	}

	certPEMNew, keyPEMNew := buildReverseProxyTestCertificatePEM(t, []string{"127.0.0.1"})
	newFingerprint, _, _, err := inspectCertificateFingerprint(certPEMNew, keyPEMNew)
	if err != nil {
		t.Fatalf("inspect new certificate failed: %v", err)
	}
	if newFingerprint == oldFingerprint {
		t.Fatal("test certificates unexpectedly have the same fingerprint")
	}
	if err := database.GetDB().Model(&model.CertificateRecord{}).Where("id = ?", cert.Id).Updates(map[string]interface{}{
		"cert_pem":      certPEMNew,
		"key_pem":       keyPEMNew,
		"fullchain_pem": certPEMNew,
		"fingerprint":   newFingerprint,
		"updated_at":    time.Now().Add(time.Minute),
	}).Error; err != nil {
		t.Fatalf("update certificate record failed: %v", err)
	}
	if err := svc.SyncIfNeeded(0); err != nil {
		t.Fatalf("sync reverse proxy after certificate update failed: %v", err)
	}

	if got := reverseProxyDialNoSNIFingerprint(t, listenPort); got != newFingerprint {
		t.Fatalf("reloaded no-sni certificate fingerprint = %q, want %q", got, newFingerprint)
	}
}

func TestComputeReverseProxyRenderKey_ChangesWhenCertificateOrderChanges(t *testing.T) {
	rows := []model.ReverseProxyRule{
		{
			Id:                    1,
			ListOrder:             1,
			Enabled:               true,
			ListenProtocol:        reverseProxyProtocolHTTPS,
			ListenIPList:          `["example.com"]`,
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

func TestReverseProxyOverviewIncludesAPIPassthrough(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	t.Cleanup(func() {
		_ = svc.StopRuntime()
	})

	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:            "api-passthrough-overview",
		Enabled:         true,
		ListenProtocol:  reverseProxyProtocolHTTP,
		ListenIPs:       "127.0.0.1",
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
		DisplayID:           1,
		ListOrder:           1,
		Name:                "startup-reset",
		Enabled:             true,
		ListenProtocol:      reverseProxyProtocolHTTP,
		ListenIP:            "127.0.0.1",
		ListenIPList:        `["127.0.0.1"]`,
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

func TestReverseProxyGroupRules_GroupsByProtocolAndPortOnly(t *testing.T) {
	rows := []model.ReverseProxyRule{
		{
			Id:             1,
			ListOrder:      1,
			Enabled:        true,
			ListenProtocol: "http",
			ListenIPList:   `["127.0.0.1"]`,
			ListenPort:     18080,
			PathPrefix:     "/a",
		},
		{
			Id:             2,
			ListOrder:      2,
			Enabled:        true,
			ListenProtocol: "http",
			ListenIPList:   `["::1"]`,
			ListenPort:     18080,
			PathPrefix:     "/b",
		},
	}

	grouped := reverseProxyGroupRules(rows)
	group := grouped["http|18080"]
	if len(grouped) != 1 || len(group) != 2 {
		t.Fatalf("expected one protocol/port listener group, got %#v", grouped)
	}
}

func TestReverseProxyRuleServerNameMatch_UsesSNINameList(t *testing.T) {
	rule := &model.ReverseProxyRule{
		ListenIPList: `["1.2.3.4"]`,
		HostList:     `["example.com"]`,
	}
	if !reverseProxyRuleServerNameMatch(rule, "1.2.3.4") {
		t.Fatal("ip should be accepted as sni")
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
		ListenIPList: `["1.2.3.4"]`,
		HostList:     `["example.com"]`,
		PathPrefix:   "",
	}
	if !reverseProxyRuleNamesOverlap(reverseProxyRuleServerNames(existing), []string{"1.2.3.4"}) {
		t.Fatal("ip sni should overlap")
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

func TestReverseProxyGetCertificateRejectsUnknownOrMissingSNI(t *testing.T) {
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
				Id:           1,
				ListenIPList: `["1.2.3.4"]`,
				HostList:     `["example.com"]`,
			},
		},
		certBindingsByRule: map[uint][]*reverseProxyRuleCertificateBinding{
			1: {binding},
		},
		orderedCertBindings: []*reverseProxyRuleCertificateBinding{binding},
		defaultCert:         cert,
		defaultLeaf:         nil,
	}

	got, err := group.getCertificate(&tls.ClientHelloInfo{ServerName: "example.com"})
	if err != nil || got != cert {
		t.Fatalf("expected matching host sni certificate, got cert=%v err=%v", got, err)
	}
	got, err = group.getCertificate(&tls.ClientHelloInfo{ServerName: "1.2.3.4"})
	if err != nil || got != cert {
		t.Fatalf("expected matching ip sni certificate, got cert=%v err=%v", got, err)
	}
	got, err = group.getCertificate(&tls.ClientHelloInfo{ServerName: "other.example.com"})
	if err == nil {
		t.Fatalf("expected unknown sni to be rejected, got cert=%v", got)
	}
	got, err = group.getCertificate(&tls.ClientHelloInfo{})
	if err == nil {
		t.Fatalf("expected missing sni to be rejected, got cert=%v", got)
	}
	got, err = group.getCertificate(nil)
	if err == nil {
		t.Fatalf("expected nil client hello to be rejected, got cert=%v", got)
	}
}

func TestReverseProxyGetCertificateAllowsMissingSNIForEmptyListenMatch(t *testing.T) {
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
		defaultCert:         cert,
		defaultLeaf:         nil,
	}

	got, err := group.getCertificate(&tls.ClientHelloInfo{})
	if err != nil || got != cert {
		t.Fatalf("expected missing sni certificate for empty listen match, got cert=%v err=%v", got, err)
	}
	got, err = group.getCertificate(&tls.ClientHelloInfo{ServerName: "example.com"})
	if err == nil {
		t.Fatalf("expected configured sni to be rejected by empty listen match, got cert=%v", got)
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
				Id:           1,
				ListenIPList: `["example.com"]`,
				HostList:     `["example.com"]`,
			},
			{
				Id:           2,
				ListenIPList: `["127.0.0.1"]`,
			},
		},
		certBindingsByRule: map[uint][]*reverseProxyRuleCertificateBinding{
			1: {domainBinding},
			2: {ipBinding},
		},
		orderedCertBindings: []*reverseProxyRuleCertificateBinding{domainBinding, ipBinding},
		defaultCert:         domainCert,
		defaultLeaf:         nil,
	}

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
				Id:           1,
				ListenIPList: `["example.com"]`,
				HostList:     `["example.com"]`,
			},
		},
		certBindingsByRule: map[uint][]*reverseProxyRuleCertificateBinding{
			1: {domainBinding},
		},
		orderedCertBindings: []*reverseProxyRuleCertificateBinding{domainBinding},
		defaultCert:         domainCert,
	}

	got, err := group.getCertificate(&tls.ClientHelloInfo{})
	if err == nil {
		t.Fatalf("expected missing sni to be rejected, got cert=%v", got)
	}
}

func TestReverseProxyGetCertificateWithSNIUsesLeastActiveBalancedCertificate(t *testing.T) {
	openReverseProxyTestDB(t)

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
	if err := database.GetDB().Create(&model.ReverseProxyCertificateBalanceState{
		ListenerKey:         "https|443",
		SNIBucket:           "example.com",
		CertificateRecordID: 1,
		ActiveConn:          7,
		SelectedTotal:       9,
		LastSelectedAt:      nowUnix - 20,
		UpdatedAtUnix:       nowUnix - 20,
	}).Error; err != nil {
		t.Fatalf("create first balance row failed: %v", err)
	}
	if err := database.GetDB().Create(&model.ReverseProxyCertificateBalanceState{
		ListenerKey:         "https|443",
		SNIBucket:           "example.com",
		CertificateRecordID: 2,
		ActiveConn:          1,
		SelectedTotal:       2,
		LastSelectedAt:      nowUnix - 10,
		UpdatedAtUnix:       nowUnix - 10,
	}).Error; err != nil {
		t.Fatalf("create second balance row failed: %v", err)
	}

	group := &reverseProxyListenerGroup{
		key:     "https|443",
		service: &ReverseProxyService{},
		rules: []*model.ReverseProxyRule{
			{
				Id:           1,
				ListenIPList: `["example.com"]`,
				HostList:     `["example.com"]`,
			},
		},
		certBindingsByRule: map[uint][]*reverseProxyRuleCertificateBinding{
			1: {firstBinding, secondBinding},
		},
		orderedCertBindings: []*reverseProxyRuleCertificateBinding{firstBinding, secondBinding},
		defaultCert:         firstCert,
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

func TestReverseProxyOverviewFallsBackWhenBalanceDiagnosticsFail(t *testing.T) {
	openReverseProxyTestDB(t)

	svc := &ReverseProxyService{}
	certRecordID := createReverseProxyTestCertificateRecord(t, "example.com")
	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:                "overview-balance-diagnostics-fallback",
		Enabled:             true,
		ListenProtocol:      reverseProxyProtocolHTTPS,
		ListenIPs:           "127.0.0.1",
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
	found := false
	for _, warning := range overview.Warnings {
		if strings.Contains(warning, "certificate balance diagnostics unavailable") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected diagnostics warning, got %#v", overview.Warnings)
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
				Id:           1,
				ListenIPList: `["example.com"]`,
				HostList:     `["example.com"]`,
			},
		},
		certBindingsByRule: map[uint][]*reverseProxyRuleCertificateBinding{
			1: {firstBinding, secondBinding},
		},
		orderedCertBindings: []*reverseProxyRuleCertificateBinding{firstBinding, secondBinding},
		defaultCert:         firstCert,
	}

	got, err := group.getCertificate(&tls.ClientHelloInfo{ServerName: "example.com"})
	if err != nil {
		t.Fatalf("expected sni certificate selection to succeed: %v", err)
	}
	if got != secondCert {
		t.Fatalf("expected second certificate to match example.com, got %v", got)
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
			ListenIPList:    `["127.0.0.1"]`,
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
				listenIPs:           []string{"127.0.0.1"},
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
			Name:                "upsert-https",
			Enabled:             false,
			ListenProtocol:      reverseProxyProtocolHTTPS,
			ListenIPs:           "127.0.0.1",
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
		Name:                "https-http11-upstream",
		Enabled:             true,
		ListenProtocol:      reverseProxyProtocolHTTPS,
		ListenIPs:           "example.com",
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
		Name:                "https-h3-listener",
		Enabled:             true,
		ListenProtocol:      reverseProxyProtocolHTTPS,
		ListenIPs:           "example.com",
		Hosts:               "example.com",
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
		ListenIPs:                 "example.com",
		Hosts:                     "example.com",
		ListenPort:                listenPort,
		TargetProtocol:            reverseProxyProtocolHTTP,
		TargetAddresses:           upstreamHost,
		TargetPort:                upstreamPort,
		CertificateRecordID:       certRecordID,
		IPStrategy:                reverseProxyIPStrategyPreferIPv4,
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
		ListenIPs:                 "example.com",
		Hosts:                     "example.com",
		ListenPort:                listenPort,
		TargetProtocol:            reverseProxyProtocolHTTP,
		TargetAddresses:           upstreamHost,
		TargetPort:                upstreamPort,
		CertificateRecordID:       certRecordID,
		IPStrategy:                reverseProxyIPStrategyPreferIPv4,
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
		Name:                "virtual-h2-to-h3",
		Enabled:             true,
		ListenProtocol:      "h2",
		ListenIPs:           "example.com",
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
		Name:                "virtual-h3-to-h2",
		Enabled:             true,
		ListenProtocol:      "h3",
		ListenIPs:           "example.com",
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

func TestReverseProxyRejectsMixedHTTPSLocalVersionStrategyOnSamePort(t *testing.T) {
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
		ListenIPs:                 "example.com",
		Hosts:                     "example.com",
		ListenPort:                listenPort,
		PathPrefix:                "/a",
		TargetProtocol:            reverseProxyProtocolHTTP,
		TargetAddresses:           "127.0.0.1",
		TargetPort:                18080,
		CertificateRecordID:       certRecordID,
		IPStrategy:                reverseProxyIPStrategyPreferIPv4,
	}); err != nil {
		t.Fatalf("create h2-only rule failed: %v", err)
	}

	err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:                      "https-listener-h3-only",
		Enabled:                   false,
		ListenProtocol:            reverseProxyProtocolHTTPS,
		ListenHTTPVersionStrategy: reverseProxyListenHTTPVersionH3Only,
		ListenIPs:                 "example.com",
		Hosts:                     "example.com",
		ListenPort:                listenPort,
		PathPrefix:                "/b",
		TargetProtocol:            reverseProxyProtocolHTTP,
		TargetAddresses:           "127.0.0.1",
		TargetPort:                18081,
		CertificateRecordID:       certRecordID,
		IPStrategy:                reverseProxyIPStrategyPreferIPv4,
	})
	if err == nil {
		t.Fatal("expected mixed local http version strategies on same https listener to fail")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "same local http version strategy") {
		t.Fatalf("unexpected mixed strategy error: %v", err)
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
		Name:                "rewrite-body",
		Enabled:             true,
		ListenProtocol:      reverseProxyProtocolHTTPS,
		ListenIPs:           "example.com",
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
		Name:            "api-passthrough-body",
		Enabled:         true,
		ListenProtocol:  reverseProxyProtocolHTTP,
		ListenIPs:       "127.0.0.1",
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
		Name:            "api-passthrough-sse",
		Enabled:         true,
		ListenProtocol:  reverseProxyProtocolHTTP,
		ListenIPs:       "127.0.0.1",
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
		Name:                "rewrite-prefix-links",
		Enabled:             true,
		ListenProtocol:      reverseProxyProtocolHTTPS,
		ListenIPs:           "example.com",
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
		Name:                "rewrite-asset-paths",
		Enabled:             true,
		ListenProtocol:      reverseProxyProtocolHTTPS,
		ListenIPs:           "example.com",
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
		Name:                "route-apad",
		Enabled:             true,
		ListenProtocol:      reverseProxyProtocolHTTPS,
		ListenIPs:           "example.com",
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
		Name:                "route-google",
		Enabled:             true,
		ListenProtocol:      reverseProxyProtocolHTTPS,
		ListenIPs:           "example.com",
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
		path        string
		wantBody    string
		wantStatus  []int
		allowReject bool
	}{
		{path: "/88999/tag/mysql/", wantBody: "apad:/tag/mysql/", wantStatus: []int{http.StatusOK}},
		{path: "/aaa", wantBody: "google:/", wantStatus: []int{http.StatusOK}},
		{path: "/bbb", wantBody: "", wantStatus: []int{http.StatusNotFound, http.StatusUnauthorized}, allowReject: true},
	} {
		req, err := http.NewRequest(http.MethodGet, "https://127.0.0.1:"+strconv.Itoa(listenPort)+tc.path, nil)
		if err != nil {
			t.Fatalf("build routed request failed: %v", err)
		}
		req.Host = "example.com"

		resp, err := client.Do(req)
		if err != nil {
			if tc.allowReject {
				continue
			}
			t.Fatalf("routed request failed for %s: %v", tc.path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		statusOK := false
		for _, wantStatus := range tc.wantStatus {
			if resp.StatusCode == wantStatus {
				statusOK = true
				break
			}
		}
		if !statusOK {
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
		Name:                "reject-mismatch",
		Enabled:             true,
		ListenProtocol:      reverseProxyProtocolHTTPS,
		ListenIPs:           "example.com",
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
		name       string
		serverName string
		host       string
	}{
		{name: "bad_sni", serverName: "wrong.example.com", host: "example.com"},
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
		if err == nil {
			_, _ = io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			t.Fatalf("expected silent reject for %s", tc.name)
		}
	}

	if got := atomic.LoadInt32(&upstreamHits); got != 0 {
		t.Fatalf("mismatched requests must not reach upstream, got %d hits", got)
	}
}

func TestReverseProxyHTTPSRuleRejectsRequestWithoutSNI(t *testing.T) {
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
		Name:                "no-sni-allowed",
		Enabled:             true,
		ListenProtocol:      reverseProxyProtocolHTTPS,
		ListenIPs:           "example.com",
		ListenPort:          listenPort,
		Hosts:               "example.com",
		PathPrefix:          "/000",
		TargetProtocol:      reverseProxyProtocolHTTP,
		TargetAddresses:     upstreamHost,
		TargetPort:          upstreamPort,
		CertificateRecordID: certRecordID,
		IPStrategy:          reverseProxyIPStrategyPreferIPv4,
	}); err != nil {
		t.Fatalf("upsert no-sni rule failed: %v", err)
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
		_, _ = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		t.Fatal("expected no-sni request to be rejected")
	}
}

func TestReverseProxyHTTPSIPRuleWithoutSNIIsRejected(t *testing.T) {
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
		Name:                "domain-first",
		Enabled:             true,
		ListenProtocol:      reverseProxyProtocolHTTPS,
		ListenIPs:           "example.com",
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
	if err := svc.UpsertRule(ReverseProxyRulePayload{
		Name:                "ip-direct",
		Enabled:             true,
		ListenProtocol:      reverseProxyProtocolHTTPS,
		ListenIPs:           "127.0.0.1",
		ListenPort:          listenPort,
		PathPrefix:          "/88999",
		TargetProtocol:      reverseProxyProtocolHTTP,
		TargetAddresses:     upstreamIPHost,
		TargetPort:          upstreamIPPort,
		CertificateRecordID: ipCertID,
		IPStrategy:          reverseProxyIPStrategyPreferIPv4,
	}); err != nil {
		t.Fatalf("upsert ip rule failed: %v", err)
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
	req, err := http.NewRequest(http.MethodGet, "https://127.0.0.1:"+strconv.Itoa(listenPort)+"/88999", nil)
	if err != nil {
		t.Fatalf("build ip-direct request failed: %v", err)
	}
	req.Host = "127.0.0.1"

	resp, err := client.Do(req)
	if err == nil {
		_, _ = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		t.Fatal("expected no-sni ip-direct request to be rejected")
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
		Name:            "shutdown-timeout",
		Enabled:         true,
		ListenProtocol:  reverseProxyProtocolHTTP,
		ListenIPs:       "127.0.0.1",
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
		ID:                saved.Id,
		Name:              saved.Name,
		Enabled:           false,
		ListenProtocol:    saved.ListenProtocol,
		ListenIPs:         strings.Join(decodeReverseProxyListenIPs(&saved), ","),
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
		Name:                "rewrite-headers",
		Enabled:             true,
		ListenProtocol:      reverseProxyProtocolHTTPS,
		ListenIPs:           "example.com",
		ListenPort:          listenPort,
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
	req, err := http.NewRequest(http.MethodGet, "https://127.0.0.1:"+strconv.Itoa(listenPort)+"/redirect", nil)
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
	if got := resp.Header.Get("Location"); got != "https://example.com/next" {
		t.Fatalf("unexpected rewritten location header: %q", got)
	}
	cookieValues := resp.Header.Values("Set-Cookie")
	if len(cookieValues) == 0 {
		t.Fatal("expected rewritten set-cookie header")
	}
	if !strings.Contains(cookieValues[0], "Domain=example.com") {
		t.Fatalf("expected rewritten cookie domain, got %q", cookieValues[0])
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
		Name:                "rewrite-headers-api-passthrough",
		Enabled:             true,
		ListenProtocol:      reverseProxyProtocolHTTPS,
		ListenIPs:           "example.com",
		ListenPort:          listenPort,
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
	req, err := http.NewRequest(http.MethodGet, "https://127.0.0.1:"+strconv.Itoa(listenPort)+"/redirect", nil)
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
	if got := resp.Header.Get("Location"); got != "https://example.com/next" {
		t.Fatalf("unexpected rewritten location header in api passthrough mode: %q", got)
	}
	cookieValues := resp.Header.Values("Set-Cookie")
	if len(cookieValues) == 0 {
		t.Fatal("expected rewritten set-cookie header in api passthrough mode")
	}
	if !strings.Contains(cookieValues[0], "Domain=example.com") {
		t.Fatalf("expected rewritten cookie domain in api passthrough mode, got %q", cookieValues[0])
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
		Name:                "https-upstream-failure",
		Enabled:             true,
		ListenProtocol:      reverseProxyProtocolHTTPS,
		ListenIPs:           "example.com",
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

func reverseProxyDialNoSNIFingerprint(t *testing.T, listenPort int) string {
	t.Helper()

	conn, err := tls.DialWithDialer(
		&net.Dialer{Timeout: 5 * time.Second},
		"tcp",
		"127.0.0.1:"+strconv.Itoa(listenPort),
		&tls.Config{InsecureSkipVerify: true},
	)
	if err != nil {
		t.Fatalf("dial reverse proxy without sni failed: %v", err)
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
