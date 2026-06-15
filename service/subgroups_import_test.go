package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alireza0/s-ui/database/model"
)

func TestExtractProxyOutboundsInjectsCertificateStoreToTLS(t *testing.T) {
	jsonData := []byte(`{
		"certificate": {"store":"mozilla"},
		"outbounds": [
			{"type":"direct","tag":"direct"},
			{"type":"vless","tag":"node-a","tls":{"enabled":true}}
		]
	}`)

	outbounds, err := extractProxyOutbounds(jsonData)
	if err != nil {
		t.Fatalf("extractProxyOutbounds failed: %v", err)
	}
	if len(outbounds) != 1 {
		t.Fatalf("expected 1 proxy outbound, got %d", len(outbounds))
	}

	tlsMap, ok := outbounds[0]["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected tls map")
	}
	if got, _ := tlsMap["tls_store"].(string); got != "mozilla" {
		t.Fatalf("expected tls_store=mozilla, got %v", tlsMap["tls_store"])
	}
}

func TestExtractProxyOutboundsUsesTLSStoreFromTLSBlockFirst(t *testing.T) {
	jsonData := []byte(`{
		"certificate": {"store":"mozilla"},
		"outbounds": [
			{"type":"trojan","tag":"node-b","tls":{"enabled":true,"store":"chrome"}}
		]
	}`)

	outbounds, err := extractProxyOutbounds(jsonData)
	if err != nil {
		t.Fatalf("extractProxyOutbounds failed: %v", err)
	}
	if len(outbounds) != 1 {
		t.Fatalf("expected 1 proxy outbound, got %d", len(outbounds))
	}

	tlsMap, ok := outbounds[0]["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected tls map")
	}
	if got, _ := tlsMap["tls_store"].(string); got != "chrome" {
		t.Fatalf("expected tls_store=chrome, got %v", tlsMap["tls_store"])
	}
}

func TestExtractProxyOutboundsNormalizesCommonTLSAliases(t *testing.T) {
	jsonData := []byte(`{
		"outbounds": [
			{"type":"vmess","tag":"node-c","tls":{"enabled":true,"client_fingerprint":"firefox","minVersion":"1.2","maxVersion":"1.3"}}
		]
	}`)

	outbounds, err := extractProxyOutbounds(jsonData)
	if err != nil {
		t.Fatalf("extractProxyOutbounds failed: %v", err)
	}
	if len(outbounds) != 1 {
		t.Fatalf("expected 1 proxy outbound, got %d", len(outbounds))
	}

	tlsMap, ok := outbounds[0]["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected tls map")
	}
	if got, _ := tlsMap["min_version"].(string); got != "1.2" {
		t.Fatalf("expected min_version=1.2, got %v", tlsMap["min_version"])
	}
	if got, _ := tlsMap["max_version"].(string); got != "1.3" {
		t.Fatalf("expected max_version=1.3, got %v", tlsMap["max_version"])
	}
	utlsMap, ok := tlsMap["utls"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected utls map")
	}
	if got, _ := utlsMap["fingerprint"].(string); got != "firefox" {
		t.Fatalf("expected fingerprint=firefox, got %v", utlsMap["fingerprint"])
	}
}

func TestExtractProxyOutboundsMergesShadowTLSPairs(t *testing.T) {
	jsonData := []byte(`{
		"outbounds": [
			{
				"type":"shadowsocks",
				"tag":"stls-node",
				"method":"2022-blake3-aes-128-gcm",
				"password":"pw",
				"network":"tcp",
				"udp_over_tcp":true,
				"multiplex":{"enabled":true},
				"detour":"stls-node-out"
			},
			{
				"type":"shadowtls",
				"tag":"stls-node-out",
				"server":"1.2.3.4",
				"server_port":443,
				"version":3,
				"tls":{"enabled":true}
			}
		]
	}`)

	outbounds, err := extractProxyOutboundsWithoutTLSStore(jsonData)
	if err != nil {
		t.Fatalf("extractProxyOutboundsWithoutTLSStore failed: %v", err)
	}
	if len(outbounds) != 1 {
		t.Fatalf("expected 1 outbound after merge, got %d", len(outbounds))
	}

	outbound := outbounds[0]
	if outbound["type"] != "shadowtls" {
		t.Fatalf("expected merged type shadowtls, got %#v", outbound["type"])
	}
	if outbound["tag"] != "stls-node" {
		t.Fatalf("expected merged tag stls-node, got %#v", outbound["tag"])
	}
	if _, ok := outbound["detour"]; ok {
		t.Fatalf("merged outbound should not contain detour: %#v", outbound)
	}

	ssConfig, ok := outbound["ss_config"].(map[string]interface{})
	if !ok || ssConfig == nil {
		t.Fatalf("expected merged ss_config, got %#v", outbound["ss_config"])
	}
	if ssConfig["method"] != "2022-blake3-aes-128-gcm" {
		t.Fatalf("unexpected ss_config.method: %#v", ssConfig["method"])
	}
	if ssConfig["network"] != "tcp" {
		t.Fatalf("unexpected ss_config.network: %#v", ssConfig["network"])
	}
	if ssConfig["password"] != "pw" {
		t.Fatalf("unexpected ss_config.password: %#v", ssConfig["password"])
	}
	if ssConfig["udp_over_tcp"] != true {
		t.Fatalf("unexpected ss_config.udp_over_tcp: %#v", ssConfig["udp_over_tcp"])
	}
	if _, ok := ssConfig["multiplex"]; !ok {
		t.Fatalf("expected ss_config.multiplex, got %#v", ssConfig)
	}
}

func TestExtractProxyOutboundsMergesShadowTLSPairsRegardlessOfOrder(t *testing.T) {
	jsonData := []byte(`{
		"outbounds": [
			{
				"type":"shadowtls",
				"tag":"stls-node-out",
				"server":"1.2.3.4",
				"server_port":443,
				"version":3
			},
			{
				"type":"shadowsocks",
				"tag":"stls-node",
				"method":"2022-blake3-aes-128-gcm",
				"password":"pw",
				"detour":"stls-node-out"
			}
		]
	}`)

	outbounds, err := extractProxyOutboundsWithoutTLSStore(jsonData)
	if err != nil {
		t.Fatalf("extractProxyOutboundsWithoutTLSStore failed: %v", err)
	}
	if len(outbounds) != 1 {
		t.Fatalf("expected 1 outbound after merge, got %d", len(outbounds))
	}
	if outbounds[0]["type"] != "shadowtls" || outbounds[0]["tag"] != "stls-node" {
		t.Fatalf("unexpected merged outbound: %#v", outbounds[0])
	}
}

func TestExtractSubscriptionJSONOutboundsRaw_KeepsNonProxyOutbounds(t *testing.T) {
	jsonData := []byte(`{
		"outbounds": [
			{"type":"selector","tag":"node-select","outbounds":["node-a"]},
			{"type":"direct","tag":"node-direct"},
			{"type":"vmess","tag":"node-a","server":"1.1.1.1","server_port":443}
		]
	}`)

	outbounds, err := extractSubscriptionJSONOutboundsRaw(jsonData)
	if err != nil {
		t.Fatalf("extractSubscriptionJSONOutboundsRaw failed: %v", err)
	}
	if len(outbounds) != 3 {
		t.Fatalf("expected 3 raw outbounds, got %d", len(outbounds))
	}

	if got, _ := outbounds[0]["type"].(string); got != "selector" {
		t.Fatalf("expected first outbound type selector, got %q", got)
	}
	if got, _ := outbounds[1]["type"].(string); got != "direct" {
		t.Fatalf("expected second outbound type direct, got %q", got)
	}
	if got, _ := outbounds[2]["type"].(string); got != "vmess" {
		t.Fatalf("expected third outbound type vmess, got %q", got)
	}
}

func TestExtractSubscriptionJSONOutboundsRaw_DoesNotMergeShadowTLSPairs(t *testing.T) {
	jsonData := []byte(`{
		"outbounds": [
			{
				"type":"shadowsocks",
				"tag":"stls-node",
				"method":"2022-blake3-aes-128-gcm",
				"password":"pw",
				"detour":"stls-node-out"
			},
			{
				"type":"shadowtls",
				"tag":"stls-node-out",
				"server":"1.2.3.4",
				"server_port":443
			}
		]
	}`)

	outbounds, err := extractSubscriptionJSONOutboundsRaw(jsonData)
	if err != nil {
		t.Fatalf("extractSubscriptionJSONOutboundsRaw failed: %v", err)
	}
	if len(outbounds) != 2 {
		t.Fatalf("expected 2 raw outbounds, got %d", len(outbounds))
	}

	if got, _ := outbounds[0]["type"].(string); got != "shadowsocks" {
		t.Fatalf("expected first outbound type shadowsocks, got %q", got)
	}
	if got, _ := outbounds[1]["type"].(string); got != "shadowtls" {
		t.Fatalf("expected second outbound type shadowtls, got %q", got)
	}
}

func TestFetchAutoUpdateJSONPayload_ReturnsRawByTag(t *testing.T) {
	subscriptionJSON := `{
		"outbounds": [
			{"type":"selector","tag":"node-select","outbounds":["node-a"]},
			{"type":"vmess","tag":"node-a","server":"1.1.1.1","server_port":443}
		]
	}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(subscriptionJSON))
	}))
	defer server.Close()

	outbounds, rawByTag, err := (&SubGroupService{}).fetchAutoUpdateJSONPayload(server.URL, false)
	if err != nil {
		t.Fatalf("fetchAutoUpdateJSONPayload failed: %v", err)
	}
	if len(outbounds) != 2 {
		t.Fatalf("expected 2 outbounds, got %d", len(outbounds))
	}
	if len(rawByTag) != 2 {
		t.Fatalf("expected 2 raw outbounds, got %d", len(rawByTag))
	}

	var selectorRaw map[string]interface{}
	if err := json.Unmarshal(rawByTag["node-select"], &selectorRaw); err != nil {
		t.Fatalf("unmarshal selector raw failed: %v", err)
	}
	if got, _ := selectorRaw["type"].(string); got != "selector" {
		t.Fatalf("expected selector raw type, got %q", got)
	}

	var nodeRaw map[string]interface{}
	if err := json.Unmarshal(rawByTag["node-a"], &nodeRaw); err != nil {
		t.Fatalf("unmarshal node raw failed: %v", err)
	}
	if got, _ := nodeRaw["type"].(string); got != "vmess" {
		t.Fatalf("expected vmess raw type, got %q", got)
	}
}

func TestLoadSubscriptionImportNodesWithTimeout_PreservesClashRawYAML(t *testing.T) {
	rawProxyYAML := "  -   name: raw-node\n      type: trojan\n      server: 1.1.1.1\n      port: 443\n      password: \"p,ass\"\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write([]byte("proxies:\n" + rawProxyYAML))
	}))
	defer server.Close()

	nodes, err := (&SubGroupService{}).loadSubscriptionImportNodesWithTimeout("", server.URL, false, time.Second)
	if err != nil {
		t.Fatalf("loadSubscriptionImportNodesWithTimeout failed: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if got := nodes[0].Tag; got != "raw-node" {
		t.Fatalf("expected tag raw-node, got %q", got)
	}
	if got := string(nodes[0].ClashRawYAML); got != rawProxyYAML {
		t.Fatalf("expected raw clash yaml %q, got %q", rawProxyYAML, got)
	}
}

func TestLoadSubscriptionImportNodesWithTimeout_UsesSuccessfulSourceWhenPeerSourceFails(t *testing.T) {
	jsonServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"outbounds": [
				{"type":"vmess","tag":"node-a","server":"1.1.1.1","server_port":443,"uuid":"00000000-0000-0000-0000-000000000001"}
			]
		}`))
	}))
	defer jsonServer.Close()

	clashServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream failed", http.StatusBadGateway)
	}))
	defer clashServer.Close()

	nodes, err := (&SubGroupService{}).loadSubscriptionImportNodesWithTimeout(jsonServer.URL, clashServer.URL, false, time.Second)
	if err != nil {
		t.Fatalf("loadSubscriptionImportNodesWithTimeout should keep successful source, got error: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node from successful json source, got %d", len(nodes))
	}
	if got := nodes[0].Tag; got != "node-a" {
		t.Fatalf("expected node tag node-a, got %q", got)
	}
	if len(nodes[0].JSONRaw) == 0 {
		t.Fatalf("expected JSON raw payload to be preserved")
	}
}

func TestPersistSubscriptionImportNodesRecreatesExistingGroupNode(t *testing.T) {
	db := initOutboundEditMergeTestDB(t, "subgroup-import-recreate.db")
	sourceGroupID := uint(77)
	tag := "node-recreate"

	oldNode := &model.SubOutbound{}
	if err := oldNode.UnmarshalJSON(mustMarshalJSON(t, map[string]interface{}{
		"type":        "hysteria",
		"tag":         tag,
		"server":      "1.1.1.1",
		"server_port": float64(443),
		"auth_str":    "old-secret",
		"old_only":    true,
	})); err != nil {
		t.Fatalf("unmarshal old suboutbound failed: %v", err)
	}
	oldNode.RawOutbound = mustMarshalJSON(t, map[string]interface{}{
		"type":     "hysteria",
		"tag":      tag,
		"old_only": true,
	})
	oldNode.ClashOptions = mustMarshalJSON(t, map[string]interface{}{
		"name":     tag,
		"type":     "hysteria",
		"auth-str": "old-secret",
	})
	oldNode.RawClashYAML = []byte("  - name: node-recreate\n    type: hysteria\n    auth-str: old-secret\n")
	oldNode.SourceType = subOutboundSourceSubGroup
	oldNode.SourceClientId = sourceGroupID
	if err := db.Create(oldNode).Error; err != nil {
		t.Fatalf("create old subgroup node failed: %v", err)
	}
	oldID := oldNode.Id

	if err := db.Create(&model.SubOutbound{Type: "direct", Tag: "zz_subgroup_recreate_blocker"}).Error; err != nil {
		t.Fatalf("create blocker suboutbound failed: %v", err)
	}

	nodes := []subscriptionImportNode{
		{
			Tag: tag,
			JSONOutbound: map[string]interface{}{
				"type":        "hysteria",
				"tag":         tag,
				"server":      "2.2.2.2",
				"server_port": float64(8443),
				"auth_str":    "new-secret",
			},
			JSONRaw: mustMarshalJSON(t, map[string]interface{}{
				"type":        "hysteria",
				"tag":         tag,
				"server":      "2.2.2.2",
				"server_port": float64(8443),
				"auth_str":    "new-secret",
			}),
		},
	}

	BeginManagedRuntimeHookScope(db)
	savedTags, _, err := (&SubGroupService{}).persistSubscriptionImportNodes(db, nodes, sourceGroupID)
	DiscardManagedRuntimeHookScope(db)
	if err != nil {
		t.Fatalf("persistSubscriptionImportNodes returned error: %v", err)
	}
	if len(savedTags) != 1 || savedTags[0] != tag {
		t.Fatalf("expected saved tag %q, got %#v", tag, savedTags)
	}

	reloaded := &model.SubOutbound{}
	if err := db.Where("tag = ?", tag).First(reloaded).Error; err != nil {
		t.Fatalf("reload recreated subgroup node failed: %v", err)
	}
	if reloaded.Id == oldID {
		t.Fatalf("expected subgroup node to be recreated with a new id, still got id=%d", reloaded.Id)
	}
	var oldRecord model.SubOutbound
	if err := db.Where("id = ?", oldID).First(&oldRecord).Error; err == nil {
		t.Fatalf("expected old subgroup node id=%d to be deleted before recreate", oldID)
	}
	rawMap := mustDecodeJSONMap(t, reloaded.RawOutbound)
	if _, exists := rawMap["old_only"]; exists {
		t.Fatalf("expected old raw field to be removed, got %#v", rawMap["old_only"])
	}
	if got, _ := rawMap["server"].(string); got != "2.2.2.2" {
		t.Fatalf("expected recreated raw server=2.2.2.2, got %#v", rawMap["server"])
	}
	if len(reloaded.ClashOptions) != 0 {
		t.Fatalf("expected old ClashOptions not to be preserved, got %s", string(reloaded.ClashOptions))
	}
	if len(reloaded.RawClashYAML) != 0 {
		t.Fatalf("expected old RawClashYAML not to be preserved, got %q", string(reloaded.RawClashYAML))
	}
}

func TestBuildSubscriptionImportNodes_FiltersNonProxyOutbounds(t *testing.T) {
	jsonOutbounds := []map[string]interface{}{
		{"type": "selector", "tag": "node-select", "outbounds": []interface{}{"node-a"}},
		{"type": "direct", "tag": "node-direct"},
		{"type": "vmess", "tag": "node-a", "server": "1.1.1.1", "server_port": float64(443)},
	}

	nodes, err := buildSubscriptionImportNodes(jsonOutbounds, nil)
	if err != nil {
		t.Fatalf("buildSubscriptionImportNodes failed: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 proxy node, got %d", len(nodes))
	}

	if nodes[0].Tag != "node-a" {
		t.Fatalf("expected node tag node-a, got %q", nodes[0].Tag)
	}
	if got, _ := nodes[0].JSONOutbound["type"].(string); got != "vmess" {
		t.Fatalf("expected vmess node, got %q", got)
	}
}

func TestBuildSubscriptionImportNodes_IgnoresJSONNonProxyAndKeepsClashProxy(t *testing.T) {
	jsonOutbounds := []map[string]interface{}{
		{"type": "selector", "tag": "node-x", "outbounds": []interface{}{"node-a"}},
	}
	clashProxies := []map[string]interface{}{
		{
			"name":     "node-x",
			"type":     "hysteria2",
			"server":   "8.8.8.8",
			"port":     443,
			"password": "secret",
		},
	}

	nodes, err := buildSubscriptionImportNodes(jsonOutbounds, clashProxies)
	if err != nil {
		t.Fatalf("buildSubscriptionImportNodes failed: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if nodes[0].Tag != "node-x" {
		t.Fatalf("expected node tag node-x, got %q", nodes[0].Tag)
	}
	if got, _ := nodes[0].JSONOutbound["type"].(string); got != "hysteria2" {
		t.Fatalf("expected clash proxy type hysteria2, got %q", got)
	}
}

func TestAttachSubscriptionJSONRawByTag_AttachesAndClonesRawPayload(t *testing.T) {
	nodes := []subscriptionImportNode{
		{
			Tag:          "node-a",
			JSONOutbound: map[string]interface{}{"type": "vmess", "tag": "node-a"},
		},
		{
			Tag:          "node-b",
			JSONOutbound: map[string]interface{}{"type": "vless", "tag": "node-b"},
		},
	}
	raw := json.RawMessage(`{"type":"vmess","tag":"node-a","custom":"keep"}`)
	rawByTag := map[string]json.RawMessage{
		"node-a": raw,
	}

	attached := attachSubscriptionJSONRawByTag(nodes, rawByTag)
	if len(attached[0].JSONRaw) == 0 {
		t.Fatalf("expected node-a raw payload to be attached")
	}
	if len(attached[1].JSONRaw) != 0 {
		t.Fatalf("expected node-b raw payload to stay empty")
	}

	rawByTag["node-a"][0] = 'x'
	if string(attached[0].JSONRaw) != `{"type":"vmess","tag":"node-a","custom":"keep"}` {
		t.Fatalf("expected attached raw payload to be cloned, got %s", string(attached[0].JSONRaw))
	}
}

func TestAttachSubscriptionJSONRawByTag_SkipsNonProxyRaw(t *testing.T) {
	nodes := []subscriptionImportNode{
		{
			Tag:          "node-a",
			JSONOutbound: map[string]interface{}{"type": "vmess", "tag": "node-a"},
		},
	}
	rawByTag := map[string]json.RawMessage{
		"node-a": json.RawMessage(`{"type":"selector","tag":"node-a","outbounds":["node-a"]}`),
	}

	attached := attachSubscriptionJSONRawByTag(nodes, rawByTag)
	if len(attached[0].JSONRaw) != 0 {
		t.Fatalf("expected selector raw to be skipped, got %s", string(attached[0].JSONRaw))
	}
}
