package sub

import (
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database/model"
	"gopkg.in/yaml.v3"
)

func TestParseSubOutboundClashProxy_NormalizesScientificIntegers(t *testing.T) {
	subOutbound := &model.SubOutbound{
		Tag: "hy2_node",
		ClashOptions: []byte(`{
  "name": "hy2_node",
  "type": "hysteria2",
  "server": "1.2.3.4",
  "port": 443,
  "max-connection-receive-window": 2.56e+08,
  "max-stream-receive-window": 8e+07
}`),
	}

	proxy, ok := parseSubOutboundClashProxy(subOutbound)
	if !ok || proxy == nil {
		t.Fatalf("expected proxy parse success")
	}

	if _, isFloat := proxy["max-connection-receive-window"].(float64); isFloat {
		t.Fatalf("expected integer-like value to be normalized, got float64")
	}
	if got, ok := toInt(proxy["max-connection-receive-window"]); !ok || got != 256000000 {
		t.Fatalf("expected max-connection-receive-window=256000000, got %#v", proxy["max-connection-receive-window"])
	}
	if got, ok := toInt(proxy["max-stream-receive-window"]); !ok || got != 80000000 {
		t.Fatalf("expected max-stream-receive-window=80000000, got %#v", proxy["max-stream-receive-window"])
	}
}

func TestRenderClashSubscriptionFromProxies_UsesPlainIntegerOutput(t *testing.T) {
	proxies := []map[string]interface{}{
		{
			"name":                          "hy2_node",
			"type":                          "hysteria2",
			"server":                        "1.2.3.4",
			"port":                          443,
			"max-connection-receive-window": float64(256000000),
			"max-stream-receive-window":     float64(80000000),
		},
	}

	rendered, err := renderClashSubscriptionFromProxies(
		proxies,
		"http://www.gstatic.com/generate_204",
		300,
		50,
		nil,
	)
	if err != nil {
		t.Fatalf("renderClashSubscriptionFromProxies failed: %v", err)
	}

	text := string(rendered)
	if strings.Contains(strings.ToLower(text), "e+0") || strings.Contains(strings.ToLower(text), "e+") {
		t.Fatalf("expected plain integer rendering, got scientific notation:\n%s", text)
	}
	if !strings.Contains(text, "256000000") {
		t.Fatalf("expected max-connection-receive-window plain integer, got:\n%s", text)
	}
	if !strings.Contains(text, "80000000") {
		t.Fatalf("expected max-stream-receive-window plain integer, got:\n%s", text)
	}
}

func TestParseSubOutboundClashProxy_PreservesHysteria2FastOpen(t *testing.T) {
	subOutbound := &model.SubOutbound{
		Tag: "hy2_node",
		ClashOptions: []byte(`{
  "name": "hy2_node",
  "type": "hysteria2",
  "server": "1.2.3.4",
  "port": 443,
  "fast-open": true
}`),
	}

	proxy, ok := parseSubOutboundClashProxy(subOutbound)
	if !ok || proxy == nil {
		t.Fatalf("expected proxy parse success")
	}
	if got, _ := proxy["fast-open"].(bool); !got {
		t.Fatalf("expected hysteria2 fast-open=true, got %#v", proxy["fast-open"])
	}
}

func TestParseSubOutboundClashProxy_PreservesTUICFastOpenAndRemovesNetwork(t *testing.T) {
	subOutbound := &model.SubOutbound{
		Tag: "tuic_node",
		ClashOptions: []byte(`{
  "name": "tuic_node",
  "type": "tuic",
  "server": "1.2.3.4",
  "port": 443,
  "uuid": "00000000-0000-0000-0000-000000000001",
  "password": "pwd",
  "fast-open": true,
  "network": "tcp"
}`),
	}

	proxy, ok := parseSubOutboundClashProxy(subOutbound)
	if !ok || proxy == nil {
		t.Fatalf("expected proxy parse success")
	}
	if got, _ := proxy["fast-open"].(bool); !got {
		t.Fatalf("expected tuic fast-open=true, got %#v", proxy["fast-open"])
	}
	if _, exists := proxy["network"]; exists {
		t.Fatalf("expected tuic network to be omitted, got %#v", proxy["network"])
	}
}

func TestShouldUseStoredClashProxy_UsesManagedClientCaches(t *testing.T) {
	svc := &SubManagerSubService{}

	for _, sourceType := range []string{subManagerSourceClient, subManagerSourceMihomoClient} {
		subOutbound := &model.SubOutbound{
			Tag:          "hy2_node",
			SourceType:   sourceType,
			ClashOptions: []byte(`{"name":"hy2_node","type":"hysteria2"}`),
		}

		if !svc.shouldUseStoredClashProxy(subOutbound) {
			t.Fatalf("expected synced %s node to use stored clash cache", sourceType)
		}
	}
}

func TestShouldUseStoredClashProxy_UsesStoredProxyForImportedSubGroup(t *testing.T) {
	svc := &SubManagerSubService{}
	subOutbound := &model.SubOutbound{
		Tag:          "hy2_node",
		SourceType:   subManagerSourceSubGroup,
		ClashOptions: []byte(`{"name":"hy2_node","type":"hysteria2"}`),
	}

	if !svc.shouldUseStoredClashProxy(subOutbound) {
		t.Fatal("expected imported subgroup nodes to keep using stored clash proxy when available")
	}
}

func TestGetSubManagerClash_ManagedClientNodesPreferStoredClashCache(t *testing.T) {
	for _, tt := range []struct {
		name       string
		dbName     string
		sourceType string
	}{
		{name: "default_client", dbName: "submanager-managed-client-fast-open.db", sourceType: subManagerSourceClient},
		{name: "mihomo_client", dbName: "submanager-managed-mihomo-fast-open.db", sourceType: subManagerSourceMihomoClient},
	} {
		t.Run(tt.name, func(t *testing.T) {
			setupSubscriptionTestDB(t, tt.dbName)

			subTag := "hy2_node"
			createSubOutboundFromMap(
				t,
				map[string]interface{}{
					"type":             "hysteria2",
					"tag":              subTag,
					"server":           "1.2.3.4",
					"server_port":      443,
					"password":         "pwd",
					"mihomo_fast_open": false,
				},
				tt.sourceType,
				1001,
				0,
				map[string]interface{}{
					"name":     subTag,
					"type":     "hysteria2",
					"server":   "1.2.3.4",
					"port":     443,
					"password": "pwd",
					"fast-open": true,
				},
			)

			clashSub, err := (&SubManagerSubService{}).GetSubManagerClash(subTag)
			if err != nil {
				t.Fatalf("GetSubManagerClash failed: %v", err)
			}

			var clashDoc map[string]interface{}
			if err := yaml.Unmarshal([]byte(*clashSub), &clashDoc); err != nil {
				t.Fatalf("yaml.Unmarshal failed: %v", err)
			}

			proxy := findNamedProxy(t, clashDoc["proxies"], subTag)
			if got, _ := proxy["fast-open"].(bool); !got {
				t.Fatalf("expected synced %s node to keep fast-open=true from stored ClashOptions, got %#v", tt.sourceType, proxy["fast-open"])
			}
		})
	}
}
