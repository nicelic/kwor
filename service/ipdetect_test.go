package service

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestGetOutboundIPsForceRefreshBypassesCompletedCache(t *testing.T) {
	var responseMu sync.Mutex
	response := "198.51.100.10\n"
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		responseMu.Lock()
		defer responseMu.Unlock()
		requests++
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	originalIPv4APIs := ipv4CheckAPIs
	originalIPv6APIs := ipv6CheckAPIs
	originalAPIs := ipCheckAPIs
	ipv4CheckAPIs = []string{server.URL}
	ipv6CheckAPIs = nil
	ipCheckAPIs = nil
	t.Cleanup(func() {
		ipv4CheckAPIs = originalIPv4APIs
		ipv6CheckAPIs = originalIPv6APIs
		ipCheckAPIs = originalAPIs
	})

	cachedOutboundIPs.mu.Lock()
	originalIPs := append([]string(nil), cachedOutboundIPs.ips...)
	originalExpiresAt := cachedOutboundIPs.expiresAt
	originalLoading := cachedOutboundIPs.loading
	originalWait := cachedOutboundIPs.wait
	cachedOutboundIPs.ips = nil
	cachedOutboundIPs.expiresAt = time.Time{}
	cachedOutboundIPs.loading = false
	cachedOutboundIPs.wait = nil
	cachedOutboundIPs.mu.Unlock()
	t.Cleanup(func() {
		cachedOutboundIPs.mu.Lock()
		cachedOutboundIPs.ips = originalIPs
		cachedOutboundIPs.expiresAt = originalExpiresAt
		cachedOutboundIPs.loading = originalLoading
		cachedOutboundIPs.wait = originalWait
		cachedOutboundIPs.mu.Unlock()
	})

	svc := &IPDetectService{}
	if got := svc.GetOutboundIPs(false); len(got) != 1 || got[0] != "198.51.100.10" {
		t.Fatalf("unexpected first outbound IP result: %#v", got)
	}

	responseMu.Lock()
	response = "203.0.113.20\n"
	responseMu.Unlock()
	if got := svc.GetOutboundIPs(false); len(got) != 1 || got[0] != "198.51.100.10" {
		t.Fatalf("completed cache should be reused: %#v", got)
	}
	if got := svc.GetOutboundIPs(true); len(got) != 1 || got[0] != "203.0.113.20" {
		t.Fatalf("force refresh should bypass completed cache: %#v", got)
	}

	responseMu.Lock()
	defer responseMu.Unlock()
	if requests != 2 {
		t.Fatalf("unexpected external probe count: got %d want 2", requests)
	}
}
