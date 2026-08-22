package service

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestShadowQUICInboundSanitizerKeepsOnlyOfficialFields(t *testing.T) {
	inbound := model.MihomoInbound{
		Type:  "shadowquic",
		Tag:   "sq-in",
		TlsId: 99,
		Tls:   &model.MihomoTls{Name: "legacy-tls"},
		Options: json.RawMessage(`{
          "listen": "0.0.0.0",
          "listen_port": 10443,
          "routing_mark": 100,
          "rule": "legacy-rule",
          "proxy": "legacy-proxy",
          "tls": {"enabled": true},
          "jls-upstream": {
            "addr": "www.example.com:443",
            "proxy": "upstream-group",
            "rate-limit": 0,
            "quic-version-probe": false,
            "ignored": "no"
          },
          "alpn": ["h3", "unsupported", "h2"],
          "quic-versions": ["v2", "v1"],
          "zero-rtt": false,
          "congestion-controller": "bbr",
          "cwnd": 0,
          "disable-mtu-discovery": false,
          "quic-version-probe": true,
          "unknown": "no"
        }`),
	}

	if err := sanitizeMihomoShadowQUICInboundOptions(&inbound); err != nil {
		t.Fatalf("sanitizeMihomoShadowQUICInboundOptions failed: %v", err)
	}
	if inbound.TlsId != 0 || inbound.Tls != nil {
		t.Fatalf("expected TLS relation to be cleared, got id=%d tls=%#v", inbound.TlsId, inbound.Tls)
	}

	var options map[string]interface{}
	if err := json.Unmarshal(inbound.Options, &options); err != nil {
		t.Fatalf("decode sanitized options failed: %v", err)
	}
	for _, key := range []string{"routing_mark", "routing-mark", "rule", "proxy", "tls", "unknown", "jls-upstream", "quic-version-probe"} {
		if _, exists := options[key]; exists {
			t.Fatalf("unexpected listener field %s: %#v", key, options)
		}
	}
	upstream, ok := options["jls_upstream"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected canonical jls_upstream map, got %#v", options["jls_upstream"])
	}
	if got := upstream["proxy"]; got != "upstream-group" {
		t.Fatalf("expected nested jls proxy preserved, got %#v", got)
	}
	for _, key := range []string{"rate_limit"} {
		if _, exists := upstream[key]; !exists {
			t.Fatalf("expected explicit nested key %s, got %#v", key, upstream)
		}
	}
	if _, exists := upstream["quic_version_probe"]; exists {
		t.Fatalf("quic-version-probe must be removed, got %#v", upstream)
	}
	for _, key := range []string{"zero_rtt", "cwnd", "disable_mtu_discovery"} {
		if _, exists := options[key]; !exists {
			t.Fatalf("expected explicit root key %s, got %#v", key, options)
		}
	}
	if got := options["alpn"]; fmt.Sprint(got) != "[h3 h2]" {
		t.Fatalf("unexpected normalized alpn: %#v", got)
	}
	if got := options["quic_versions"]; fmt.Sprint(got) != "[v2 v1]" {
		t.Fatalf("expected both quic versions, got %#v", got)
	}
	if got := options["congestion_controller"]; got != "bbr" {
		t.Fatalf("unexpected congestion controller: %#v", got)
	}
}

func TestShadowQUICInboundSaveOmitsEmptyOptionalFields(t *testing.T) {
	db := setupMihomoSyncTestDB(t, "shadowquic-inbound-empty-options.db")
	payload := mustJSONRaw(t, map[string]interface{}{
		"type":        "shadowquic",
		"tag":         "sq-empty-options",
		"listen":      "0.0.0.0",
		"listen_port": 10822,
		"jls_upstream": map[string]interface{}{
			"addr":       "www.example.com:443",
			"sni":        "",
			"rate_limit": "",
		},
		"alpn":                    []string{},
		"quic_versions":           []string{},
		"congestion_controller":   "",
		"up":                      "",
		"down":                    "",
		"cwnd":                    "",
		"bbr_profile":             "",
		"max_idle_time":           "",
		"max_datagram_frame_size": "",
		"recv_window_conn":        "",
		"recv_window":             "",
	})

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin ShadowQUIC save transaction failed: %v", tx.Error)
	}
	if _, err := (&MihomoInboundService{}).Save(tx, "new", payload, "", "panel.example.com"); err != nil {
		tx.Rollback()
		t.Fatalf("save ShadowQUIC inbound with empty optional fields failed: %v", err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit ShadowQUIC save failed: %v", err)
	}

	var stored model.MihomoInbound
	if err := db.Where("tag = ?", "sq-empty-options").First(&stored).Error; err != nil {
		t.Fatalf("reload saved ShadowQUIC inbound failed: %v", err)
	}
	options := map[string]interface{}{}
	if err := json.Unmarshal(stored.Options, &options); err != nil {
		t.Fatalf("decode saved ShadowQUIC options failed: %v", err)
	}
	for _, key := range []string{
		"alpn", "quic_versions", "congestion_controller", "up", "down", "cwnd",
		"bbr_profile", "max_idle_time", "max_datagram_frame_size", "recv_window_conn", "recv_window",
	} {
		if _, exists := options[key]; exists {
			t.Fatalf("empty optional field %s must not be stored: %#v", key, options)
		}
	}
	upstream, ok := options["jls_upstream"].(map[string]interface{})
	if !ok || upstream["sni"] != "www.example.com" {
		t.Fatalf("expected addr-derived required SNI, got %#v", options["jls_upstream"])
	}
	if _, exists := upstream["rate_limit"]; exists {
		t.Fatalf("empty JLS rate-limit must not be stored: %#v", upstream)
	}
	template := map[string]interface{}{}
	if err := json.Unmarshal(stored.OutJson, &template); err != nil {
		t.Fatalf("decode saved ShadowQUIC client template failed: %v", err)
	}
	if template["sni"] != "www.example.com" {
		t.Fatalf("expected addr-derived SNI in client template, got %#v", template)
	}
	for _, key := range []string{
		"alpn", "quic_versions", "zero_rtt", "congestion_controller", "cwnd", "bbr_profile",
		"max_datagram_frame_size", "recv_window_conn", "recv_window", "disable_mtu_discovery",
	} {
		if _, exists := template[key]; exists {
			t.Fatalf("empty optional field %s must not enter client template: %#v", key, template)
		}
	}

	document, err := NewMihomoManagerService().GenerateServerDocument()
	if err != nil {
		t.Fatalf("generate ShadowQUIC server document failed: %v", err)
	}
	listener := findMihomoListenerByTag(document, stored.Tag)
	if listener == nil {
		t.Fatalf("ShadowQUIC listener %q was not generated", stored.Tag)
	}
	for _, key := range []string{
		"alpn", "quic-versions", "congestion-controller", "up", "down", "cwnd",
		"bbr-profile", "max-idle-time", "max-datagram-frame-size", "recv-window-conn", "recv-window",
	} {
		if _, exists := listener[key]; exists {
			t.Fatalf("empty optional field %s must not be generated: %#v", key, listener)
		}
	}
}

func TestShadowQUICInboundSaveRoundTripPreservesALPN(t *testing.T) {
	db := setupMihomoSyncTestDB(t, "shadowquic-inbound-alpn-roundtrip.db")
	payload := mustJSONRaw(t, map[string]interface{}{
		"type":        "shadowquic",
		"tag":         "sq-alpn-roundtrip",
		"listen":      "::",
		"listen_port": 10443,
		"jls_upstream": map[string]interface{}{
			"addr": "upstream.example.com:443",
			"sni":  "cdn.example.com",
		},
		"alpn":                    []string{"h3", "h2"},
		"quic_versions":           []string{"v2", "v1"},
		"zero_rtt":                true,
		"max_datagram_frame_size": 1400,
	})

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin ShadowQUIC save transaction failed: %v", tx.Error)
	}
	if _, err := (&MihomoInboundService{}).Save(tx, "new", payload, "", "panel.example.com"); err != nil {
		tx.Rollback()
		t.Fatalf("save ShadowQUIC inbound failed: %v", err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit ShadowQUIC inbound save failed: %v", err)
	}

	var stored model.MihomoInbound
	if err := db.Where("tag = ?", "sq-alpn-roundtrip").First(&stored).Error; err != nil {
		t.Fatalf("reload saved ShadowQUIC inbound failed: %v", err)
	}
	storedOptions := map[string]interface{}{}
	if err := json.Unmarshal(stored.Options, &storedOptions); err != nil {
		t.Fatalf("decode saved ShadowQUIC options failed: %v", err)
	}
	if got := fmt.Sprint(storedOptions["alpn"]); got != "[h3 h2]" {
		t.Fatalf("saved ShadowQUIC alpn was lost: %#v", storedOptions)
	}

	views, err := (&MihomoInboundService{}).Get(fmt.Sprint(stored.Id))
	if err != nil || views == nil || len(*views) != 1 {
		t.Fatalf("read saved ShadowQUIC inbound failed: views=%#v err=%v", views, err)
	}
	if got := fmt.Sprint((*views)[0]["alpn"]); got != "[h3 h2]" {
		t.Fatalf("ShadowQUIC alpn was not returned to the editor: %#v", (*views)[0])
	}

	clientTemplate := map[string]interface{}{}
	if err := json.Unmarshal(stored.OutJson, &clientTemplate); err != nil {
		t.Fatalf("decode saved ShadowQUIC client template failed: %v", err)
	}
	if got := fmt.Sprint(clientTemplate["alpn"]); got != "[h3 h2]" {
		t.Fatalf("ShadowQUIC alpn was not copied to the client template: %#v", clientTemplate)
	}

	document, err := NewMihomoManagerService().GenerateServerDocument()
	if err != nil {
		t.Fatalf("generate ShadowQUIC server document failed: %v", err)
	}
	listener := findMihomoListenerByTag(document, stored.Tag)
	if listener == nil || fmt.Sprint(listener["alpn"]) != "[h3 h2]" {
		t.Fatalf("ShadowQUIC alpn was not rendered to the listener: %#v", listener)
	}
}

func TestShadowQUICInboundSanitizerAddsDefaultPortAndSNI(t *testing.T) {
	inbound := model.MihomoInbound{Type: "shadowquic", Options: json.RawMessage(`{"jls_upstream":{"addr":"[2001:db8::1]"}}`)}
	if err := sanitizeMihomoShadowQUICInboundOptions(&inbound); err != nil {
		t.Fatalf("sanitize failed: %v", err)
	}
	var options map[string]interface{}
	if err := json.Unmarshal(inbound.Options, &options); err != nil {
		t.Fatalf("decode sanitized options failed: %v", err)
	}
	upstream, ok := options["jls_upstream"].(map[string]interface{})
	if !ok || upstream["addr"] != "[2001:db8::1]:443" || upstream["sni"] != "2001:db8::1" {
		t.Fatalf("unexpected normalized upstream: %#v", options["jls_upstream"])
	}
}

func TestShadowQUICInboundSaveRoundTripPreservesAllServerControls(t *testing.T) {
	db := setupMihomoSyncTestDB(t, "shadowquic-inbound-control-roundtrip.db")
	payload := mustJSONRaw(t, map[string]interface{}{
		"type":        "shadowquic",
		"tag":         "sq-control-roundtrip",
		"listen":      "::",
		"listen_port": 10443,
		"jls_upstream": map[string]interface{}{
			"addr":       "upstream.example.com:443",
			"sni":        "cdn.example.com",
			"rate_limit": 0,
		},
		"alpn":                    []string{"h3", "h2", "http/1.1"},
		"quic_versions":           []string{"v2", "v1"},
		"zero_rtt":                false,
		"congestion_controller":   "new_reno",
		"up":                      "100 Mbps",
		"down":                    "200 Mbps",
		"ignore_client_bandwidth": false,
		"cwnd":                    0,
		"bbr_profile":             "conservative",
		"max_idle_time":           0,
		"max_datagram_frame_size": 0,
		"recv_window_conn":        0,
		"recv_window":             0,
		"disable_mtu_discovery":   false,
	})

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin ShadowQUIC save transaction failed: %v", tx.Error)
	}
	if _, err := (&MihomoInboundService{}).Save(tx, "new", payload, "", "panel.example.com"); err != nil {
		tx.Rollback()
		t.Fatalf("save ShadowQUIC inbound failed: %v", err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit ShadowQUIC inbound save failed: %v", err)
	}

	var stored model.MihomoInbound
	if err := db.Where("tag = ?", "sq-control-roundtrip").First(&stored).Error; err != nil {
		t.Fatalf("reload saved ShadowQUIC inbound failed: %v", err)
	}
	options := map[string]interface{}{}
	if err := json.Unmarshal(stored.Options, &options); err != nil {
		t.Fatalf("decode saved ShadowQUIC options failed: %v", err)
	}
	for key, want := range map[string]interface{}{
		"zero_rtt":                false,
		"congestion_controller":   "new_reno",
		"up":                      "100 Mbps",
		"down":                    "200 Mbps",
		"ignore_client_bandwidth": false,
		"cwnd":                    float64(0),
		"bbr_profile":             "conservative",
		"max_idle_time":           float64(0),
		"max_datagram_frame_size": float64(0),
		"recv_window_conn":        float64(0),
		"recv_window":             float64(0),
		"disable_mtu_discovery":   false,
	} {
		if got, exists := options[key]; !exists || got != want {
			t.Fatalf("stored option %s = %#v (exists=%v), want %#v; options=%#v", key, got, exists, want, options)
		}
	}
	if got := fmt.Sprint(options["alpn"]); got != "[h3 h2 http/1.1]" {
		t.Fatalf("stored alpn = %#v", options["alpn"])
	}
	if got := fmt.Sprint(options["quic_versions"]); got != "[v2 v1]" {
		t.Fatalf("stored quic_versions = %#v", options["quic_versions"])
	}
	upstream, ok := options["jls_upstream"].(map[string]interface{})
	if !ok || upstream["sni"] != "cdn.example.com" || upstream["rate_limit"] != float64(0) {
		t.Fatalf("stored jls_upstream = %#v", options["jls_upstream"])
	}

	template := map[string]interface{}{}
	if err := json.Unmarshal(stored.OutJson, &template); err != nil {
		t.Fatalf("decode saved ShadowQUIC client template failed: %v", err)
	}
	for key, want := range map[string]interface{}{
		"sni":                     "cdn.example.com",
		"zero_rtt":                false,
		"congestion_controller":   "new_reno",
		"cwnd":                    float64(0),
		"bbr_profile":             "conservative",
		"max_datagram_frame_size": float64(0),
		"recv_window_conn":        float64(0),
		"recv_window":             float64(0),
		"disable_mtu_discovery":   false,
	} {
		if got, exists := template[key]; !exists || got != want {
			t.Fatalf("client template %s = %#v (exists=%v), want %#v; template=%#v", key, got, exists, want, template)
		}
	}
	for _, key := range []string{"up", "down", "ignore_client_bandwidth", "max_idle_time", "rate_limit"} {
		if _, exists := template[key]; exists {
			t.Fatalf("server-only field %s leaked to client template: %#v", key, template)
		}
	}

	document, err := NewMihomoManagerService().GenerateServerDocument()
	if err != nil {
		t.Fatalf("generate ShadowQUIC server document failed: %v", err)
	}
	listener := findMihomoListenerByTag(document, stored.Tag)
	if listener == nil {
		t.Fatalf("ShadowQUIC listener %q was not generated", stored.Tag)
	}
	for key, want := range map[string]interface{}{
		"zero-rtt":                false,
		"congestion-controller":   "new_reno",
		"up":                      "100 Mbps",
		"down":                    "200 Mbps",
		"ignore-client-bandwidth": false,
		"cwnd":                    0,
		"bbr-profile":             "conservative",
		"max-idle-time":           0,
		"max-datagram-frame-size": 0,
		"recv-window-conn":        0,
		"recv-window":             0,
		"disable-mtu-discovery":   false,
	} {
		if got, exists := listener[key]; !exists || got != want {
			t.Fatalf("listener %s = %#v (exists=%v), want %#v; listener=%#v", key, got, exists, want, listener)
		}
	}
	if got := fmt.Sprint(listener["alpn"]); got != "[h3 h2 http/1.1]" {
		t.Fatalf("listener alpn = %#v", listener["alpn"])
	}
	if got := fmt.Sprint(listener["quic-versions"]); got != "[v2 v1]" {
		t.Fatalf("listener quic-versions = %#v", listener["quic-versions"])
	}
	listenerUpstream, ok := listener["jls-upstream"].(map[string]interface{})
	if !ok || listenerUpstream["sni"] != "cdn.example.com" || listenerUpstream["rate-limit"] != 0 {
		t.Fatalf("listener jls-upstream = %#v", listener["jls-upstream"])
	}
}

func TestShadowQUICJLSDirectNormalizesForStorageAndListener(t *testing.T) {
	inbound := model.MihomoInbound{
		Type: "shadowquic",
		Options: json.RawMessage(`{
          "listen": "0.0.0.0",
          "listen_port": 10443,
          "jls_upstream": {"addr": "www.example.com:443", "proxy": "direct"}
        }`),
	}
	if err := sanitizeMihomoShadowQUICInboundOptions(&inbound); err != nil {
		t.Fatalf("sanitizeMihomoShadowQUICInboundOptions failed: %v", err)
	}

	options := map[string]interface{}{}
	if err := json.Unmarshal(inbound.Options, &options); err != nil {
		t.Fatalf("decode sanitized options failed: %v", err)
	}
	upstream, ok := options["jls_upstream"].(map[string]interface{})
	if !ok || upstream["proxy"] != "DIRECT" {
		t.Fatalf("expected stored JLS proxy DIRECT, got %#v", options["jls_upstream"])
	}

	listener := buildMihomoListener(
		model.MihomoInbound{Type: "shadowquic", Tag: "sq-direct"},
		map[string]interface{}{
			"type":          "shadowquic",
			"listen":        "0.0.0.0",
			"listen_port":   10443,
			"quic_versions": []string{"v2", "v1"},
			"jls_upstream": map[string]interface{}{
				"addr":  "www.example.com:443",
				"proxy": "direct",
			},
		},
		mihomoInboundRouteRef{},
	)
	rendered, ok := listener["jls-upstream"].(map[string]interface{})
	if !ok || rendered["proxy"] != "DIRECT" {
		t.Fatalf("expected legacy JLS proxy to render as DIRECT, got %#v", listener["jls-upstream"])
	}
	if got := listener["quic-versions"]; fmt.Sprint(got) != "[v2 v1]" {
		t.Fatalf("expected listener quic versions to remain ordered, got %#v", got)
	}
}

func TestShadowQUICInboundSanitizerRejectsMissingJLSAddress(t *testing.T) {
	inbound := model.MihomoInbound{
		Type:    "shadowquic",
		Options: json.RawMessage(`{"listen_port":10443,"jls_upstream":{"addr":"bad-address"}}`),
	}
	if err := sanitizeMihomoShadowQUICInboundOptions(&inbound); err == nil {
		t.Fatal("expected invalid JLS upstream address to be rejected")
	}
}

func TestShadowQUICListenerDoesNotInjectTopLevelRouteFields(t *testing.T) {
	payload := map[string]interface{}{
		"type":                "shadowquic",
		"listen":              "0.0.0.0",
		"listen_port":         10443,
		"routing_mark":        100,
		"rule":                "legacy-rule",
		"proxy":               "legacy-proxy",
		"keep_alive_interval": 1000,
		"max_open_streams":    32,
		"sni":                 "not-an-inbound-field",
		"quic_version_probe":  true,
		"unknown":             "must-not-survive",
		"jls_upstream": map[string]interface{}{
			"addr":               "www.example.com:443",
			"proxy":              "upstream-group",
			"quic_version_probe": true,
		},
	}
	listener := buildMihomoListener(
		model.MihomoInbound{Type: "shadowquic", Tag: "sq-in"},
		payload,
		mihomoInboundRouteRef{RuleName: "should-not-apply", ProxyTarget: "should-not-apply"},
	)

	for _, key := range []string{
		"routing_mark", "routing-mark", "rule", "proxy", "detour", "tls",
		"keep_alive_interval", "max_open_streams", "sni", "quic_version_probe", "quic-version-probe", "unknown",
	} {
		if _, exists := listener[key]; exists {
			t.Fatalf("unexpected top-level %s in listener: %#v", key, listener)
		}
	}
	upstream, ok := listener["jls-upstream"].(map[string]interface{})
	if !ok || upstream["proxy"] != "upstream-group" {
		t.Fatalf("expected nested jls-upstream.proxy to remain, got %#v", listener["jls-upstream"])
	}
	if _, exists := upstream["quic-version-probe"]; exists {
		t.Fatalf("quic-version-probe must not reach listener yaml: %#v", upstream)
	}
}

func TestShadowQUICUsersContainOnlyUsernameAndPassword(t *testing.T) {
	users, err := normalizeMihomoUsersForList("shadowquic", []string{
		`{"name":"alice","password":"secret","uuid":"legacy","unexpected":"drop"}`,
	}, map[string]interface{}{})
	if err != nil {
		t.Fatalf("normalizeMihomoUsersForList failed: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("expected one user, got %#v", users)
	}
	var user map[string]interface{}
	if err := json.Unmarshal(users[0], &user); err != nil {
		t.Fatalf("decode normalized user failed: %v", err)
	}
	if len(user) != 2 || user["username"] != "alice" || user["password"] != "secret" {
		t.Fatalf("unexpected ShadowQUIC users output: %#v", user)
	}
}

func TestShadowQUICSyncClashOptionsStripTLS(t *testing.T) {
	raw, err := buildMihomoClashOptions(map[string]interface{}{
		"type":                  "shadowquic",
		"tag":                   "sq-in",
		"server":                "panel.example.com",
		"server_port":           10443,
		"username":              "alice",
		"password":              "secret",
		"quic_versions":         []string{"v1"},
		"zero_rtt":              false,
		"disable_mtu_discovery": false,
		"tls":                   map[string]interface{}{"enabled": true},
		"detour":                "unexpected",
	}, "m_sq-in_alice")
	if err != nil {
		t.Fatalf("buildMihomoClashOptions failed: %v", err)
	}

	var proxy map[string]interface{}
	if err := json.Unmarshal(raw, &proxy); err != nil {
		t.Fatalf("decode ClashOptions failed: %v", err)
	}
	if proxy["name"] != "m_sq-in_alice" || proxy["type"] != "shadowquic" {
		t.Fatalf("unexpected synced proxy: %#v", proxy)
	}
	for _, key := range []string{"tls", "detour", "routing-mark", "rule", "proxy"} {
		if _, exists := proxy[key]; exists {
			t.Fatalf("unexpected field %s in synced ClashOptions: %#v", key, proxy)
		}
	}
	for _, key := range []string{"zero-rtt", "disable-mtu-discovery"} {
		if _, exists := proxy[key]; !exists {
			t.Fatalf("expected explicit false field %s in synced ClashOptions: %#v", key, proxy)
		}
	}
}

func TestShadowQUICSyncClientToSubManagerPersistsRawAndClashOptions(t *testing.T) {
	db := setupMihomoSyncTestDB(t, "shadowquic-client-sync.db")
	inbound := &model.MihomoInbound{
		Type:  "shadowquic",
		Tag:   "sq-in",
		Addrs: json.RawMessage(`[]`),
		OutJson: json.RawMessage(`{
          "sni":"cdn.example.com",
          "zero_rtt":false,
		  "username":"inbound-template-user",
		  "password":"inbound-template-password",
          "tls":{"enabled":true}
        }`),
		Options: json.RawMessage(`{
          "listen":"0.0.0.0",
          "listen_port":10443,
          "jls_upstream":{"addr":"www.example.com:443"}
        }`),
	}
	if err := fillMihomoOutJson(inbound, "panel.example.com"); err != nil {
		t.Fatalf("fillMihomoOutJson failed: %v", err)
	}
	var template map[string]interface{}
	if err := json.Unmarshal(inbound.OutJson, &template); err != nil {
		t.Fatalf("decode generated ShadowQUIC out_json failed: %v", err)
	}
	for _, key := range []string{"username", "password"} {
		if _, exists := template[key]; exists {
			t.Fatalf("generated ShadowQUIC out_json must not retain %s: %#v", key, template)
		}
	}
	if err := db.Create(inbound).Error; err != nil {
		t.Fatalf("create ShadowQUIC inbound failed: %v", err)
	}

	client := &model.MihomoClient{
		Enable:   true,
		Name:     "alice",
		Config:   json.RawMessage(`{"shadowquic":{"username":"alice","password":"secret"}}`),
		Inbounds: mustJSONRaw(t, []uint{inbound.Id}),
	}
	if err := db.Create(client).Error; err != nil {
		t.Fatalf("create Mihomo client failed: %v", err)
	}

	if _, err := (&MihomoSyncService{}).SyncClientToSubManager(client.Name, "panel.example.com"); err != nil {
		t.Fatalf("SyncClientToSubManager failed: %v", err)
	}

	var synced model.SubOutbound
	if err := db.Where("source_type = ? AND source_client_id = ? AND source_inbound_id = ?", subOutboundSourceMihomoClient, client.Id, inbound.Id).First(&synced).Error; err != nil {
		t.Fatalf("load synced suboutbound failed: %v", err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(synced.RawOutbound, &raw); err != nil {
		t.Fatalf("decode synced RawOutbound failed: %v", err)
	}
	if raw["type"] != "shadowquic" || raw["username"] != "alice" || raw["password"] != "secret" {
		t.Fatalf("unexpected synced RawOutbound: %#v", raw)
	}
	if _, exists := raw["tls"]; exists {
		t.Fatalf("synced RawOutbound must not carry TLS: %#v", raw)
	}

	var clash map[string]interface{}
	if err := json.Unmarshal(synced.ClashOptions, &clash); err != nil {
		t.Fatalf("decode synced ClashOptions failed: %v", err)
	}
	if clash["name"] != synced.Tag || clash["type"] != "shadowquic" {
		t.Fatalf("unexpected synced ClashOptions: %#v", clash)
	}
	if _, exists := clash["tls"]; exists {
		t.Fatalf("synced ClashOptions must not carry TLS: %#v", clash)
	}
}

func TestShadowQUICOutSyncsServerFieldsAndKeepsClientOwnedFields(t *testing.T) {
	inbound := &model.MihomoInbound{
		Type: "shadowquic",
		Tag:  "sq-sync",
		OutJson: json.RawMessage(`{
          "udp_over_stream": true,
          "keep_alive_interval": 25000,
          "max_open_streams": 2048,
          "up": "20 Mbps",
          "down": "30 Mbps",
          "mihomo_common": {
            "udp": true,
            "ip_version": "ipv6",
            "routing_mark": 88,
            "tcp_fast_open": true
          }
		  }`),
		Options: json.RawMessage(`{
          "listen":"0.0.0.0",
          "listen_port":10443,
          "jls_upstream":{
            "addr":"www.example.com:443",
            "sni":"cdn.example.com",
            "quic_version_probe":true
          },
          "alpn":["h3","h2"],
          "quic_versions":["v2","v1"],
          "zero_rtt":true,
          "congestion_controller":"bbr",
          "up":"100 Mbps",
          "down":"100 Mbps",
          "cwnd":32,
          "bbr_profile":"standard",
          "max_datagram_frame_size":1400,
          "recv_window_conn":0,
          "recv_window":0,
          "disable_mtu_discovery":false
		  }`),
	}

	if err := fillMihomoOutJson(inbound, "panel.example.com"); err != nil {
		t.Fatalf("fillMihomoOutJson failed: %v", err)
	}
	template := map[string]interface{}{}
	if err := json.Unmarshal(inbound.OutJson, &template); err != nil {
		t.Fatalf("decode synced out_json failed: %v", err)
	}
	if template["sni"] != "cdn.example.com" || template["zero_rtt"] != true || template["congestion_controller"] != "bbr" {
		t.Fatalf("expected server fields in template, got %#v", template)
	}
	if fmt.Sprint(template["alpn"]) != "[h3 h2]" || fmt.Sprint(template["quic_versions"]) != "[v2 v1]" {
		t.Fatalf("unexpected normalized protocol lists: %#v", template)
	}
	for key, expected := range map[string]float64{
		"cwnd":                    32,
		"max_datagram_frame_size": 1400,
		"recv_window_conn":        0,
		"recv_window":             0,
		"keep_alive_interval":     25000,
		"max_open_streams":        2048,
	} {
		if template[key] != expected {
			t.Fatalf("unexpected %s: got %#v want %#v", key, template[key], expected)
		}
	}
	if template["udp_over_stream"] != true {
		t.Fatalf("client-only udp-over-stream must be preserved: %#v", template)
	}
	if template["up"] != "20 Mbps" || template["down"] != "30 Mbps" {
		t.Fatalf("client bandwidth must not be overwritten by listener bandwidth: %#v", template)
	}
	common, ok := template["mihomo_common"].(map[string]interface{})
	if !ok || common["udp"] != true || common["ip_version"] != "ipv6" || common["routing_mark"] != float64(88) {
		t.Fatalf("expected supported common fields to remain, got %#v", template["mihomo_common"])
	}
	if _, exists := common["tcp_fast_open"]; exists {
		t.Fatalf("unsupported common fields must be removed, got %#v", common)
	}

	inbound.Options = json.RawMessage(`{
      "listen":"0.0.0.0",
      "listen_port":10443,
      "jls_upstream":{"addr":"www.example.com:443"}
	    }`)
	if err := fillMihomoOutJson(inbound, "panel.example.com"); err != nil {
		t.Fatalf("fillMihomoOutJson after clearing server options failed: %v", err)
	}
	cleared := map[string]interface{}{}
	if err := json.Unmarshal(inbound.OutJson, &cleared); err != nil {
		t.Fatalf("decode cleared out_json failed: %v", err)
	}
	for _, key := range []string{
		"sni", "alpn", "quic_versions", "zero_rtt", "congestion_controller",
		"cwnd", "bbr_profile", "max_datagram_frame_size",
		"recv_window_conn", "recv_window", "disable_mtu_discovery",
	} {
		if _, exists := cleared[key]; exists {
			t.Fatalf("cleared server field %s must not remain in client template: %#v", key, cleared)
		}
	}
	if cleared["udp_over_stream"] != true || cleared["keep_alive_interval"] != float64(25000) || cleared["max_open_streams"] != float64(2048) {
		t.Fatalf("client-only fields must survive server sync: %#v", cleared)
	}
	if cleared["up"] != "20 Mbps" || cleared["down"] != "30 Mbps" {
		t.Fatalf("client bandwidth must survive server option changes: %#v", cleared)
	}
}

func TestShadowQUICOutInitializesClientOnlyDefaults(t *testing.T) {
	inbound := &model.MihomoInbound{
		Type:    "shadowquic",
		Tag:     "sq-client-defaults",
		OutJson: json.RawMessage(`{}`),
		Options: json.RawMessage(`{
          "listen":"0.0.0.0",
          "listen_port":10443,
          "jls_upstream":{"addr":"www.example.com:443"}
        }`),
	}

	if err := fillMihomoOutJson(inbound, "panel.example.com"); err != nil {
		t.Fatalf("fillMihomoOutJson failed: %v", err)
	}
	template := map[string]interface{}{}
	if err := json.Unmarshal(inbound.OutJson, &template); err != nil {
		t.Fatalf("decode default ShadowQUIC out_json failed: %v", err)
	}
	if template["udp_over_stream"] != false || template["keep_alive_interval"] != float64(10000) || template["max_open_streams"] != float64(1024) {
		t.Fatalf("unexpected ShadowQUIC client-only defaults: %#v", template)
	}
}

func TestShadowQUICClientSyncRefreshesServerOwnedFields(t *testing.T) {
	db := setupMihomoSyncTestDB(t, "shadowquic-sync-refreshes-server-fields.db")
	inbound := &model.MihomoInbound{
		Type: "shadowquic",
		Tag:  "sq-sync-refresh",
		OutJson: json.RawMessage(`{
          "sni":"stale.example.com",
          "alpn":["h2"],
          "zero_rtt":false,
          "max_datagram_frame_size":1200,
          "udp_over_stream":true,
          "keep_alive_interval":25000,
          "max_open_streams":2048,
          "up":"20 Mbps",
          "down":"30 Mbps",
          "mihomo_common":{
            "udp":true,
            "ip_version":"ipv6",
            "routing_mark":88,
            "tcp_fast_open":true
          }
        }`),
		Options: json.RawMessage(`{
          "listen":"0.0.0.0",
          "listen_port":10443,
          "jls_upstream":{
            "addr":"www.example.com:443",
            "sni":"cdn.example.com",
            "quic_version_probe":true
          },
          "alpn":["h3","h2"],
          "quic_versions":["v2","v1"],
          "zero_rtt":true,
          "congestion_controller":"bbr",
          "up":"100 Mbps",
          "down":"100 Mbps",
          "max_datagram_frame_size":1400
        }`),
	}
	if err := db.Create(inbound).Error; err != nil {
		t.Fatalf("create ShadowQUIC inbound failed: %v", err)
	}

	clientConfig := map[string]interface{}{
		"shadowquic": map[string]interface{}{
			"username": "alice",
			"password": "secret",
		},
	}
	syncService := &MihomoSyncService{}
	outbound, clashSource, err := syncService.buildSyncedOutbound(db, inbound, clientConfig, "alice", "panel.example.com", false)
	if err != nil {
		t.Fatalf("buildSyncedOutbound failed: %v", err)
	}
	if outbound["sni"] != "cdn.example.com" || outbound["zero_rtt"] != true || outbound["max_datagram_frame_size"] != 1400 {
		t.Fatalf("expected current server fields in synced outbound, got %#v", outbound)
	}
	if fmt.Sprint(outbound["alpn"]) != "[h3 h2]" || fmt.Sprint(outbound["quic_versions"]) != "[v2 v1]" {
		t.Fatalf("unexpected synced protocol lists: %#v", outbound)
	}
	if outbound["udp_over_stream"] != true || outbound["keep_alive_interval"] != 25000 || outbound["max_open_streams"] != 2048 {
		t.Fatalf("client-only fields must survive subscription sync: %#v", outbound)
	}
	if outbound["up"] != "20 Mbps" || outbound["down"] != "30 Mbps" {
		t.Fatalf("client bandwidth must survive subscription sync: %#v", outbound)
	}
	clashData, err := buildMihomoClashOptions(clashSource, inbound.Tag)
	if err != nil {
		t.Fatalf("buildMihomoClashOptions failed: %v", err)
	}
	clash := map[string]interface{}{}
	if err := json.Unmarshal(clashData, &clash); err != nil {
		t.Fatalf("decode ShadowQUIC Clash options failed: %v", err)
	}
	if clash["udp"] != true || clash["ip-version"] != "ipv6" || clash["routing-mark"] != float64(88) {
		t.Fatalf("expected supported common fields in Clash proxy, got %#v", clash)
	}
	if got := clash["quic-versions"]; fmt.Sprint(got) != "[v2 v1]" {
		t.Fatalf("expected Clash proxy quic versions to remain ordered, got %#v", got)
	}
	if clash["up"] != "20 Mbps" || clash["down"] != "30 Mbps" {
		t.Fatalf("expected client bandwidth in Clash proxy, got %#v", clash)
	}
	for _, key := range []string{"tls", "proxy", "quic-version-probe"} {
		if _, exists := clash[key]; exists {
			t.Fatalf("unsupported ShadowQUIC Clash field %s must not be generated: %#v", key, clash)
		}
	}

	inbound.Options = json.RawMessage(`{
      "listen":"0.0.0.0",
      "listen_port":10443,
      "jls_upstream":{"addr":"www.example.com:443"}
    }`)
	outbound, clashSource, err = syncService.buildSyncedOutbound(db, inbound, clientConfig, "alice", "panel.example.com", false)
	if err != nil {
		t.Fatalf("buildSyncedOutbound after clearing server fields failed: %v", err)
	}
	for _, key := range []string{
		"sni", "alpn", "quic_versions", "zero_rtt", "congestion_controller", "max_datagram_frame_size",
	} {
		if _, exists := outbound[key]; exists {
			t.Fatalf("cleared server field %s must not remain in synced outbound: %#v", key, outbound)
		}
	}
	if outbound["udp_over_stream"] != true || outbound["keep_alive_interval"] != 25000 || outbound["max_open_streams"] != 2048 {
		t.Fatalf("client-only fields must remain after clearing server options: %#v", outbound)
	}
	if outbound["up"] != "20 Mbps" || outbound["down"] != "30 Mbps" {
		t.Fatalf("client bandwidth must remain after clearing server options: %#v", outbound)
	}
	clashData, err = buildMihomoClashOptions(clashSource, inbound.Tag)
	if err != nil {
		t.Fatalf("buildMihomoClashOptions after clearing server fields failed: %v", err)
	}
	clash = map[string]interface{}{}
	if err := json.Unmarshal(clashData, &clash); err != nil {
		t.Fatalf("decode cleared ShadowQUIC Clash options failed: %v", err)
	}
	for _, key := range []string{"udp", "ip-version", "routing-mark"} {
		if _, exists := clash[key]; !exists {
			t.Fatalf("supported common field %s must remain in Clash proxy: %#v", key, clash)
		}
	}
}

func TestShadowQUICInboundAddProvisionsLegacyClientCredentials(t *testing.T) {
	db := setupMihomoSyncTestDB(t, "shadowquic-inbound-add-credentials.db")
	client := &model.MihomoClient{
		Enable:   true,
		Name:     "legacy-alice",
		Config:   json.RawMessage(`{}`),
		Inbounds: json.RawMessage(`[]`),
		Links:    json.RawMessage(`[]`),
	}
	if err := db.Create(client).Error; err != nil {
		t.Fatalf("create legacy Mihomo client failed: %v", err)
	}
	inbound := &model.MihomoInbound{
		Type:    "shadowquic",
		Tag:     "sq-inbound-add",
		Addrs:   json.RawMessage(`[]`),
		OutJson: json.RawMessage(`{}`),
		Options: json.RawMessage(`{"listen":"0.0.0.0","listen_port":10443,"jls_upstream":{"addr":"www.example.com:443"}}`),
	}
	if err := db.Create(inbound).Error; err != nil {
		t.Fatalf("create ShadowQUIC inbound failed: %v", err)
	}

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction failed: %v", tx.Error)
	}
	if err := (&MihomoClientService{}).UpdateClientsOnInboundAdd(tx, fmt.Sprintf("%d", client.Id), inbound.Id, "panel.example.com"); err != nil {
		tx.Rollback()
		t.Fatalf("UpdateClientsOnInboundAdd failed: %v", err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit transaction failed: %v", err)
	}

	var stored model.MihomoClient
	if err := db.First(&stored, client.Id).Error; err != nil {
		t.Fatalf("reload repaired Mihomo client failed: %v", err)
	}
	assertShadowQUICClientCredentials(t, stored.Config, "legacy-alice")
}

func TestShadowQUICManagerRepairsLegacyBoundClientCredentials(t *testing.T) {
	db := setupMihomoSyncTestDB(t, "shadowquic-manager-repair-credentials.db")
	inbound := &model.MihomoInbound{
		Type:    "shadowquic",
		Tag:     "sq-manager-repair",
		Addrs:   json.RawMessage(`[]`),
		OutJson: json.RawMessage(`{}`),
		Options: json.RawMessage(`{"listen":"0.0.0.0","listen_port":10443,"jls_upstream":{"addr":"www.example.com:443"}}`),
	}
	if err := db.Create(inbound).Error; err != nil {
		t.Fatalf("create ShadowQUIC inbound failed: %v", err)
	}
	client := &model.MihomoClient{
		Enable:   true,
		Name:     "legacy-bob",
		Config:   json.RawMessage(`{}`),
		Inbounds: mustJSONRaw(t, []uint{inbound.Id}),
		Links:    json.RawMessage(`[]`),
	}
	if err := db.Create(client).Error; err != nil {
		t.Fatalf("create legacy Mihomo client failed: %v", err)
	}

	document, err := NewMihomoManagerService().GenerateServerDocument()
	if err != nil {
		t.Fatalf("GenerateServerDocument failed: %v", err)
	}

	var stored model.MihomoClient
	if err := db.First(&stored, client.Id).Error; err != nil {
		t.Fatalf("reload repaired Mihomo client failed: %v", err)
	}
	password := assertShadowQUICClientCredentials(t, stored.Config, "legacy-bob")

	listener := findMihomoListenerByTag(document, inbound.Tag)
	if listener == nil {
		t.Fatalf("expected ShadowQUIC listener %q in generated document", inbound.Tag)
	}
	users, ok := listener["users"].([]interface{})
	if !ok || len(users) != 1 {
		t.Fatalf("expected one repaired ShadowQUIC user, got %#v", listener["users"])
	}
	user, ok := users[0].(map[string]interface{})
	if !ok || user["username"] != "legacy-bob" || user["password"] != password {
		t.Fatalf("expected generated listener to use repaired credentials, got %#v", users[0])
	}
}

func TestShadowQUICManagerDropsLegacyInboundOptionFields(t *testing.T) {
	db := setupMihomoSyncTestDB(t, "shadowquic-manager-legacy-options.db")
	inbound := &model.MihomoInbound{
		Type:    "shadowquic",
		Tag:     "sq-legacy-options",
		Addrs:   json.RawMessage(`[]`),
		OutJson: json.RawMessage(`{}`),
		Options: json.RawMessage(`{
          "listen":"0.0.0.0",
          "listen_port":10443,
          "jls_upstream":{"addr":"www.example.com:443"},
          "users":[{"username":"rogue","password":"rogue-secret"}],
          "keep_alive_interval":1000,
          "max_open_streams":16,
          "sni":"not-an-inbound-field",
          "tls":{"enabled":true},
          "unknown":"must-not-survive"
        }`),
	}
	if err := db.Create(inbound).Error; err != nil {
		t.Fatalf("create legacy ShadowQUIC inbound failed: %v", err)
	}

	document, err := NewMihomoManagerService().GenerateServerDocument()
	if err != nil {
		t.Fatalf("GenerateServerDocument failed: %v", err)
	}
	listener := findMihomoListenerByTag(document, inbound.Tag)
	if listener == nil {
		t.Fatalf("expected ShadowQUIC listener %q in generated document", inbound.Tag)
	}
	for _, key := range []string{"users", "keep_alive_interval", "max_open_streams", "sni", "tls", "unknown"} {
		if _, exists := listener[key]; exists {
			t.Fatalf("legacy ShadowQUIC listener field %s must not survive: %#v", key, listener)
		}
	}
}

func TestShadowQUICImportedRawProxyCannotRestoreUnsupportedFields(t *testing.T) {
	result := convertMihomoOutboundsToClash([]map[string]interface{}{
		{
			"type":        "shadowquic",
			"tag":         "sq-imported",
			"server":      "edge.example.com",
			"server_port": 10443,
			"username":    "alice",
			"password":    "secret",
			mihomoImportedClashProxyKey: map[string]interface{}{
				"name":         "sq-imported",
				"type":         "shadowquic",
				"server":       "edge.example.com",
				"port":         10443,
				"username":     "alice",
				"password":     "secret",
				"unexpected":   "must-not-survive",
				"tls":          true,
				"routing-mark": 100,
			},
		},
	})
	if result == nil || len(result.Proxies) != 1 {
		t.Fatalf("expected one ShadowQUIC proxy, got %#v", result)
	}
	proxy := result.Proxies[0]
	for _, key := range []string{"unexpected", "tls", "routing-mark", mihomoImportedClashProxyKey} {
		if _, exists := proxy[key]; exists {
			t.Fatalf("raw imported field %s must not survive ShadowQUIC conversion: %#v", key, proxy)
		}
	}
}

func assertShadowQUICClientCredentials(t *testing.T, raw json.RawMessage, expectedUsername string) string {
	t.Helper()
	configs := map[string]interface{}{}
	if err := json.Unmarshal(raw, &configs); err != nil {
		t.Fatalf("decode Mihomo client config failed: %v", err)
	}
	credentials, ok := configs["shadowquic"].(map[string]interface{})
	if !ok || credentials == nil {
		t.Fatalf("expected ShadowQUIC credential config, got %#v", configs)
	}
	if got, _ := credentials["username"].(string); got != expectedUsername {
		t.Fatalf("expected ShadowQUIC username %q, got %#v", expectedUsername, credentials["username"])
	}
	password, _ := credentials["password"].(string)
	if password == "" {
		t.Fatalf("expected generated ShadowQUIC password, got %#v", credentials["password"])
	}
	if len(credentials) != 2 {
		t.Fatalf("expected only ShadowQUIC username/password, got %#v", credentials)
	}
	return password
}

func findMihomoListenerByTag(document map[string]interface{}, tag string) map[string]interface{} {
	listeners, _ := document["listeners"].([]interface{})
	for _, raw := range listeners {
		listener, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if listener["name"] == tag {
			return listener
		}
	}
	return nil
}

func TestShadowQUICClashImportMapsOfficialFields(t *testing.T) {
	outbound, ok := convertClashProxyToSubOutbound(map[string]interface{}{
		"name":                    "import-sq",
		"type":                    "shadowquic",
		"server":                  "edge.example.com",
		"port":                    443,
		"username":                "alice",
		"password":                "secret",
		"sni":                     "cdn.example.com",
		"alpn":                    []interface{}{"h3"},
		"quic-versions":           []interface{}{"v1"},
		"udp-over-stream":         false,
		"zero-rtt":                false,
		"keep-alive-interval":     0,
		"congestion-controller":   "cubic",
		"up":                      "100 Mbps",
		"down":                    "200 Mbps",
		"cwnd":                    0,
		"bbr-profile":             "standard",
		"max-datagram-frame-size": 1400,
		"max-open-streams":        0,
		"recv-window-conn":        0,
		"recv-window":             0,
		"disable-mtu-discovery":   false,
		"tls":                     true,
	})
	if !ok {
		t.Fatal("expected ShadowQUIC Clash proxy import to succeed")
	}
	if outbound["type"] != "shadowquic" || outbound["tag"] != "import-sq" || outbound["server_port"] != 443 {
		t.Fatalf("unexpected imported base fields: %#v", outbound)
	}
	for _, key := range []string{
		"udp_over_stream", "zero_rtt", "keep_alive_interval", "cwnd", "max_open_streams",
		"recv_window_conn", "recv_window", "disable_mtu_discovery",
	} {
		if _, exists := outbound[key]; !exists {
			t.Fatalf("expected explicit imported option %s, got %#v", key, outbound)
		}
	}
	if _, exists := outbound["tls"]; exists {
		t.Fatalf("ShadowQUIC import must not create TLS data: %#v", outbound)
	}
}
