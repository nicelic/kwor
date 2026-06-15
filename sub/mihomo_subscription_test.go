package sub

import (
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util"
	"gopkg.in/yaml.v3"
)

func TestMihomoClientSubscriptions(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mihomo-subscriptions.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	db := database.GetDB()
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB failed: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
	certificateLines, _ := buildLeafCertificateForFingerprint(t)

	tlsConfig := model.MihomoTls{
		Name: "mihomo-tls",
		Server: mustRawJSON(t, map[string]interface{}{
			"enabled":     true,
			"server_name": "edge.example.com",
			"certificate": certificateLines,
		}),
		Client: mustRawJSON(t, map[string]interface{}{
			"insecure":                   true,
			"include_server_certificate": true,
		}),
	}
	if err := db.Create(&tlsConfig).Error; err != nil {
		t.Fatalf("create mihomo tls failed: %v", err)
	}

	inbound := model.MihomoInbound{
		Type:    "trojan",
		Tag:     "trojan-443",
		TlsId:   tlsConfig.Id,
		Tls:     &tlsConfig,
		Addrs:   mustRawJSON(t, []interface{}{}),
		OutJson: mustRawJSON(t, map[string]interface{}{}),
		Options: mustRawJSON(t, map[string]interface{}{
			"listen_port": 443,
		}),
	}

	baseInbound := inbound.ToBase()
	if err := util.FillOutJson(&baseInbound, "panel.example.com"); err != nil {
		t.Fatalf("FillOutJson failed: %v", err)
	}
	inbound.OutJson = baseInbound.OutJson

	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create mihomo inbound failed: %v", err)
	}

	client := model.MihomoClient{
		Enable: true,
		Name:   "mihomo-user",
		Config: mustRawJSON(t, map[string]interface{}{
			"trojan": map[string]interface{}{
				"password": "secret-pass",
			},
		}),
		Inbounds: mustRawJSON(t, []uint{inbound.Id}),
		Links: mustRawJSON(t, []map[string]string{
			{
				"remark": inbound.Tag,
				"type":   "local",
				"uri":    "trojan://secret-pass@panel.example.com:443#trojan-443",
			},
		}),
		Volume: 1024,
		Up:     12,
		Down:   34,
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create mihomo client failed: %v", err)
	}

	plainSub, plainHeaders, err := (&SubService{}).GetMihomoSubs(client.Name)
	if err != nil {
		t.Fatalf("GetMihomoSubs failed: %v", err)
	}
	if len(plainHeaders) != 3 || plainHeaders[2] != client.Name {
		t.Fatalf("unexpected mihomo headers: %#v", plainHeaders)
	}
	decodedPlain, err := base64.StdEncoding.DecodeString(*plainSub)
	if err != nil {
		t.Fatalf("decode plain mihomo subscription failed: %v", err)
	}
	if !strings.Contains(string(decodedPlain), "trojan://secret-pass@panel.example.com:443#trojan-443") {
		t.Fatalf("unexpected plain mihomo subscription: %s", string(decodedPlain))
	}

	jsonSub, jsonHeaders, err := (&JsonService{}).GetMihomoJson(client.Name, "json")
	if err != nil {
		t.Fatalf("GetMihomoJson failed: %v", err)
	}
	if len(jsonHeaders) != 3 || jsonHeaders[2] != client.Name {
		t.Fatalf("unexpected mihomo json headers: %#v", jsonHeaders)
	}

	var jsonDoc map[string]interface{}
	if err := json.Unmarshal([]byte(*jsonSub), &jsonDoc); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	jsonOutbound := findTaggedOutbound(t, jsonDoc["outbounds"], inbound.Tag)
	if got, _ := jsonOutbound["type"].(string); got != "trojan" {
		t.Fatalf("expected mihomo json outbound type trojan, got %v", jsonOutbound["type"])
	}
	if got, _ := jsonOutbound["password"].(string); got != "secret-pass" {
		t.Fatalf("expected mihomo json password to be merged from client config, got %v", jsonOutbound["password"])
	}
	tlsMap := asMap(t, jsonOutbound["tls"])
	if got, _ := tlsMap["server_name"].(string); got != "edge.example.com" {
		t.Fatalf("expected mihomo json tls server_name edge.example.com, got %v", tlsMap["server_name"])
	}
	if _, exists := tlsMap["certificate"]; !exists {
		t.Fatalf("expected mihomo json tls certificate to be carried from inbound tls: %#v", tlsMap)
	}

	clashSub, clashHeaders, err := (&ClashService{}).GetMihomoClash(client.Name)
	if err != nil {
		t.Fatalf("GetMihomoClash failed: %v", err)
	}
	if len(clashHeaders) != 3 || clashHeaders[2] != client.Name {
		t.Fatalf("unexpected mihomo clash headers: %#v", clashHeaders)
	}

	var clashDoc map[string]interface{}
	if err := yaml.Unmarshal([]byte(*clashSub), &clashDoc); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}
	clashProxy := findNamedProxy(t, clashDoc["proxies"], inbound.Tag)
	if got, _ := clashProxy["type"].(string); got != "trojan" {
		t.Fatalf("expected mihomo clash proxy type trojan, got %v", clashProxy["type"])
	}
	if got, _ := clashProxy["sni"].(string); got != "edge.example.com" {
		t.Fatalf("expected mihomo clash proxy sni edge.example.com, got %v", clashProxy["sni"])
	}
	if got, _ := clashProxy["password"].(string); got != "secret-pass" {
		t.Fatalf("expected mihomo clash password secret-pass, got %v", clashProxy["password"])
	}
}

func TestMihomoClashSubscriptions_Snell(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mihomo-snell-subscriptions.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	db := database.GetDB()
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB failed: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	inbound := model.MihomoInbound{
		Type:  "snell",
		Tag:   "snell-8443",
		Addrs: mustRawJSON(t, []interface{}{}),
		OutJson: mustRawJSON(t, map[string]interface{}{
			"type":    "snell",
			"version": 4,
			"udp":     false,
			"reuse":   true,
			"obfs_opts": map[string]interface{}{
				"mode": "http",
				"host": "cdn.example.com",
			},
		}),
		Options: mustRawJSON(t, map[string]interface{}{
			"listen_port": 8443,
			"version":     5,
			"obfs_opts": map[string]interface{}{
				"mode": "http",
				"host": "cdn.example.com",
			},
		}),
	}

	baseInbound := inbound.ToBase()
	if err := util.FillOutJson(&baseInbound, "panel.example.com"); err != nil {
		t.Fatalf("FillOutJson failed: %v", err)
	}
	inbound.OutJson = baseInbound.OutJson

	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create mihomo snell inbound failed: %v", err)
	}

	client := model.MihomoClient{
		Enable: true,
		Name:   "mihomo-snell-user",
		Config: mustRawJSON(t, map[string]interface{}{
			"snell": map[string]interface{}{
				"name": "mihomo-snell-user",
				"psk":  "secret-pass",
			},
		}),
		Inbounds: mustRawJSON(t, []uint{inbound.Id}),
		Links:    mustRawJSON(t, []map[string]string{}),
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create mihomo snell client failed: %v", err)
	}

	clashSub, _, err := (&ClashService{}).GetMihomoClash(client.Name)
	if err != nil {
		t.Fatalf("GetMihomoClash failed: %v", err)
	}

	var clashDoc map[string]interface{}
	if err := yaml.Unmarshal([]byte(*clashSub), &clashDoc); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}
	clashProxy := findNamedProxy(t, clashDoc["proxies"], inbound.Tag)
	if got, _ := clashProxy["type"].(string); got != "snell" {
		t.Fatalf("expected mihomo clash proxy type snell, got %v", clashProxy["type"])
	}
	if got, _ := clashProxy["psk"].(string); got != "secret-pass" {
		t.Fatalf("expected mihomo clash psk secret-pass, got %v", clashProxy["psk"])
	}
	if got, ok := clashProxy["udp"].(bool); !ok || got {
		t.Fatalf("expected mihomo clash udp=false, got %#v", clashProxy["udp"])
	}
	if got, _ := clashProxy["reuse"].(bool); !got {
		t.Fatalf("expected mihomo clash reuse=true, got %v", clashProxy["reuse"])
	}
	obfsOpts := asMap(t, clashProxy["obfs-opts"])
	if got, _ := obfsOpts["mode"].(string); got != "http" {
		t.Fatalf("expected mihomo clash obfs-opts.mode=http, got %v", obfsOpts["mode"])
	}
	if got, _ := obfsOpts["host"].(string); got != "cdn.example.com" {
		t.Fatalf("expected mihomo clash obfs-opts.host=cdn.example.com, got %v", obfsOpts["host"])
	}
}

func TestNormalizeMihomoSubscriptionOutJSON_TUICStripsListenerOnlyFields(t *testing.T) {
	inbound := model.Inbound{
		Type: "tuic",
		OutJson: mustRawJSON(t, map[string]interface{}{
			"request_timeout":    "8s",
			"zero_rtt_handshake": true,
			"heartbeat":          "10s",
			"auth_timeout":       "5s",
			"max_idle_time":      "30s",
			"network":            "udp",
			"fast_open":          true,
			"mihomo_fast_open":   true,
		}),
		Options: mustRawJSON(t, map[string]interface{}{
			"listen_port":               443,
			"max_udp_relay_packet_size": 1400,
			"cwnd":                      16,
			"authentication-timeout":    1000,
			"max-idle-time":             15000,
		}),
	}

	if err := normalizeMihomoSubscriptionOutJSON(&inbound); err != nil {
		t.Fatalf("normalizeMihomoSubscriptionOutJSON failed: %v", err)
	}

	var outbound map[string]interface{}
	if err := json.Unmarshal(inbound.OutJson, &outbound); err != nil {
		t.Fatalf("unmarshal normalized out_json failed: %v", err)
	}
	if got, _ := outbound["request_timeout"].(string); got != "8s" {
		t.Fatalf("expected request_timeout 8s, got %#v", outbound["request_timeout"])
	}
	for _, key := range []string{"zero_rtt_handshake", "heartbeat", "auth_timeout", "max_idle_time", "network", "fast_open"} {
		if _, exists := outbound[key]; exists {
			t.Fatalf("expected %s to be removed, got %#v", key, outbound[key])
		}
	}
	if got, ok := outbound["mihomo_fast_open"].(bool); !ok || !got {
		t.Fatalf("expected mihomo_fast_open=true to be preserved for clash subscription, got %#v", outbound["mihomo_fast_open"])
	}
	if got, _ := outbound["max_udp_relay_packet_size"].(float64); got != 1400 {
		t.Fatalf("expected max_udp_relay_packet_size 1400, got %#v", outbound["max_udp_relay_packet_size"])
	}
	if got, _ := outbound["cwnd"].(float64); got != 16 {
		t.Fatalf("expected cwnd 16, got %#v", outbound["cwnd"])
	}
}

func TestNormalizeMihomoSubscriptionOutJSON_MigratesLegacyCommonFields(t *testing.T) {
	inbound := model.Inbound{
		Type: "vless",
		OutJson: mustRawJSON(t, map[string]interface{}{
			"udp":            false,
			"ip_version":     "ipv6-prefer",
			"routing_mark":   99,
			"tcp_fast_open":  true,
			"tcp_multi_path": true,
			"multiplex": map[string]interface{}{
				"enabled":   true,
				"statistic": true,
				"only_tcp":  true,
			},
		}),
	}

	if err := normalizeMihomoSubscriptionOutJSON(&inbound); err != nil {
		t.Fatalf("normalizeMihomoSubscriptionOutJSON failed: %v", err)
	}

	var outbound map[string]interface{}
	if err := json.Unmarshal(inbound.OutJson, &outbound); err != nil {
		t.Fatalf("unmarshal normalized out_json failed: %v", err)
	}
	for _, key := range []string{"udp", "ip_version", "routing_mark", "tcp_fast_open", "tcp_multi_path", "multiplex"} {
		if _, exists := outbound[key]; exists {
			t.Fatalf("expected %s to be migrated off root outbound, got %#v", key, outbound[key])
		}
	}
	common := asMap(t, outbound["mihomo_common"])
	if got, ok := common["udp"].(bool); !ok || got {
		t.Fatalf("unexpected common udp: %#v", common["udp"])
	}
	smux := asMap(t, common["smux"])
	if got, ok := smux["statistic"].(bool); !ok || !got {
		t.Fatalf("unexpected smux statistic: %#v", smux["statistic"])
	}
	if got, ok := smux["only_tcp"].(bool); !ok || !got {
		t.Fatalf("unexpected smux only_tcp: %#v", smux["only_tcp"])
	}
}

func TestNormalizeMihomoSubscriptionOutJSON_MigratesLegacyBBRProfileToCommonFields(t *testing.T) {
	inbound := model.Inbound{
		Type: "hysteria2",
		OutJson: mustRawJSON(t, map[string]interface{}{
			"bbr_profile": "aggressive",
		}),
	}

	if err := normalizeMihomoSubscriptionOutJSON(&inbound); err != nil {
		t.Fatalf("normalizeMihomoSubscriptionOutJSON failed: %v", err)
	}

	var outbound map[string]interface{}
	if err := json.Unmarshal(inbound.OutJson, &outbound); err != nil {
		t.Fatalf("unmarshal normalized out_json failed: %v", err)
	}
	if _, exists := outbound["bbr_profile"]; exists {
		t.Fatalf("expected bbr_profile to be migrated off root outbound, got %#v", outbound["bbr_profile"])
	}
	common := asMap(t, outbound["mihomo_common"])
	if got, _ := common["bbr_profile"].(string); got != "aggressive" {
		t.Fatalf("unexpected common bbr_profile: %#v", common["bbr_profile"])
	}
}

func TestNormalizeMihomoSubscriptionOutJSON_ShadowTLSDropsLegacySSNetwork(t *testing.T) {
	inbound := model.Inbound{
		Type: "shadowtls",
		OutJson: mustRawJSON(t, map[string]interface{}{
			"ss_config": map[string]interface{}{
				"method":   "2022-blake3-aes-128-gcm",
				"password": "ss-pass",
				"network":  "udp",
				"mihomo_common": map[string]interface{}{
					"udp": false,
				},
			},
		}),
	}

	if err := normalizeMihomoSubscriptionOutJSON(&inbound); err != nil {
		t.Fatalf("normalizeMihomoSubscriptionOutJSON failed: %v", err)
	}

	var outbound map[string]interface{}
	if err := json.Unmarshal(inbound.OutJson, &outbound); err != nil {
		t.Fatalf("unmarshal normalized out_json failed: %v", err)
	}

	ssConfig := asMap(t, outbound["ss_config"])
	if _, exists := ssConfig["network"]; exists {
		t.Fatalf("expected ss_config.network to be removed for mihomo shadowtls subscriptions, got %#v", ssConfig["network"])
	}
	common := asMap(t, ssConfig["mihomo_common"])
	if got, ok := common["udp"].(bool); !ok || got {
		t.Fatalf("unexpected mihomo_common.udp: %#v", common["udp"])
	}
}

func TestMihomoSubscriptions_SeparateJSONAndClashCommonFields(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mihomo-common-fields.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	db := database.GetDB()
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB failed: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	inbound := model.MihomoInbound{
		Type:  "vless",
		Tag:   "vless-common-443",
		Addrs: mustRawJSON(t, []interface{}{}),
		OutJson: mustRawJSON(t, map[string]interface{}{
			"uuid": "00000000-0000-0000-0000-000000000222",
			"mihomo_common": map[string]interface{}{
				"udp":            false,
				"ip_version":     "ipv4-prefer",
				"routing_mark":   77,
				"tcp_fast_open":  true,
				"tcp_multi_path": true,
				"smux": map[string]interface{}{
					"enabled":         true,
					"protocol":        "smux",
					"max_connections": 5,
					"statistic":       true,
					"only_tcp":        true,
				},
			},
		}),
		Options: mustRawJSON(t, map[string]interface{}{
			"listen_port": 443,
		}),
	}

	baseInbound := inbound.ToBase()
	if err := util.FillOutJson(&baseInbound, "panel.example.com"); err != nil {
		t.Fatalf("FillOutJson failed: %v", err)
	}
	inbound.OutJson = baseInbound.OutJson

	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create mihomo inbound failed: %v", err)
	}

	client := model.MihomoClient{
		Enable: true,
		Name:   "mihomo-common-user",
		Config: mustRawJSON(t, map[string]interface{}{}),
		Inbounds: mustRawJSON(t, []uint{
			inbound.Id,
		}),
		Links: mustRawJSON(t, []map[string]string{}),
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create mihomo client failed: %v", err)
	}

	jsonSub, _, err := (&JsonService{}).GetMihomoJson(client.Name, "json")
	if err != nil {
		t.Fatalf("GetMihomoJson failed: %v", err)
	}

	var jsonDoc map[string]interface{}
	if err := json.Unmarshal([]byte(*jsonSub), &jsonDoc); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	jsonOutbound := findTaggedOutbound(t, jsonDoc["outbounds"], inbound.Tag)
	for _, key := range []string{"mihomo_common", "udp", "ip_version", "routing_mark", "tcp_fast_open", "tcp_multi_path", "multiplex"} {
		if _, exists := jsonOutbound[key]; exists {
			t.Fatalf("expected mihomo-only key %s to be absent from json outbound, got %#v", key, jsonOutbound[key])
		}
	}

	clashSub, _, err := (&ClashService{}).GetMihomoClash(client.Name)
	if err != nil {
		t.Fatalf("GetMihomoClash failed: %v", err)
	}

	var clashDoc map[string]interface{}
	if err := yaml.Unmarshal([]byte(*clashSub), &clashDoc); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}
	clashProxy := findNamedProxy(t, clashDoc["proxies"], inbound.Tag)
	if got, ok := clashProxy["udp"].(bool); !ok || got {
		t.Fatalf("expected clash udp=false, got %#v", clashProxy["udp"])
	}
	if got, _ := clashProxy["ip-version"].(string); got != "ipv4-prefer" {
		t.Fatalf("unexpected clash ip-version: %#v", clashProxy["ip-version"])
	}
	smux := asMap(t, clashProxy["smux"])
	if got, ok := smux["statistic"].(bool); !ok || !got {
		t.Fatalf("unexpected clash smux statistic: %#v", smux["statistic"])
	}
	if got, ok := smux["only-tcp"].(bool); !ok || !got {
		t.Fatalf("unexpected clash smux only-tcp: %#v", smux["only-tcp"])
	}
}

func TestMihomoSubscriptions_BBRProfileIsClashOnly(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mihomo-bbr-profile.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	db := database.GetDB()
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB failed: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	inbound := model.MihomoInbound{
		Type:  "hysteria2",
		Tag:   "hy2-bbr-profile",
		Addrs: mustRawJSON(t, []interface{}{}),
		OutJson: mustRawJSON(t, map[string]interface{}{
			"mihomo_common": map[string]interface{}{
				"bbr_profile": "aggressive",
			},
		}),
		Options: mustRawJSON(t, map[string]interface{}{
			"listen_port": 443,
		}),
	}

	baseInbound := inbound.ToBase()
	if err := util.FillOutJson(&baseInbound, "panel.example.com"); err != nil {
		t.Fatalf("FillOutJson failed: %v", err)
	}
	var outJSON map[string]interface{}
	if err := json.Unmarshal(baseInbound.OutJson, &outJSON); err != nil {
		t.Fatalf("json.Unmarshal base out_json failed: %v", err)
	}
	outJSON["mihomo_common"] = map[string]interface{}{
		"bbr_profile": "aggressive",
	}
	inbound.OutJson = mustRawJSON(t, outJSON)

	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create mihomo inbound failed: %v", err)
	}

	client := model.MihomoClient{
		Enable: true,
		Name:   "mihomo-bbr-user",
		Config: mustRawJSON(t, map[string]interface{}{
			"hysteria2": map[string]interface{}{
				"password": "hy2-secret",
			},
		}),
		Inbounds: mustRawJSON(t, []uint{inbound.Id}),
		Links:    mustRawJSON(t, []map[string]string{}),
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create mihomo client failed: %v", err)
	}

	jsonSub, _, err := (&JsonService{}).GetMihomoJson(client.Name, "json")
	if err != nil {
		t.Fatalf("GetMihomoJson failed: %v", err)
	}

	var jsonDoc map[string]interface{}
	if err := json.Unmarshal([]byte(*jsonSub), &jsonDoc); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	jsonOutbound := findTaggedOutbound(t, jsonDoc["outbounds"], inbound.Tag)
	if _, exists := jsonOutbound["bbr_profile"]; exists {
		t.Fatalf("expected json outbound to omit bbr_profile, got %#v", jsonOutbound["bbr_profile"])
	}
	if _, exists := jsonOutbound["mihomo_common"]; exists {
		t.Fatalf("expected json outbound to omit mihomo_common, got %#v", jsonOutbound["mihomo_common"])
	}

	clashSub, _, err := (&ClashService{}).GetMihomoClash(client.Name)
	if err != nil {
		t.Fatalf("GetMihomoClash failed: %v", err)
	}

	var clashDoc map[string]interface{}
	if err := yaml.Unmarshal([]byte(*clashSub), &clashDoc); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}
	clashProxy := findNamedProxy(t, clashDoc["proxies"], inbound.Tag)
	if got, _ := clashProxy["bbr-profile"].(string); got != "aggressive" {
		t.Fatalf("unexpected clash bbr-profile: %#v", clashProxy["bbr-profile"])
	}
}

func TestMihomoTUICClashSubscriptionOmitsFastOpen(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mihomo-tuic.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	db := database.GetDB()
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB failed: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	inbound := model.MihomoInbound{
		Type:  "tuic",
		Tag:   "tuic-443",
		Addrs: mustRawJSON(t, []interface{}{}),
		OutJson: mustRawJSON(t, map[string]interface{}{
			"request_timeout": "8s",
		}),
		Options: mustRawJSON(t, map[string]interface{}{
			"listen_port":               443,
			"congestion_control":        "bbr",
			"max_udp_relay_packet_size": 1400,
			"cwnd":                      16,
		}),
	}

	baseInbound := inbound.ToBase()
	if err := util.FillOutJson(&baseInbound, "panel.example.com"); err != nil {
		t.Fatalf("FillOutJson failed: %v", err)
	}
	inbound.OutJson = baseInbound.OutJson

	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create mihomo tuic inbound failed: %v", err)
	}

	client := model.MihomoClient{
		Enable: true,
		Name:   "mihomo-tuic-user",
		Config: mustRawJSON(t, map[string]interface{}{
			"tuic": map[string]interface{}{
				"uuid":     "00000000-0000-0000-0000-000000000001",
				"password": "secret",
			},
		}),
		Inbounds: mustRawJSON(t, []uint{inbound.Id}),
		Links:    mustRawJSON(t, []map[string]string{}),
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create mihomo tuic client failed: %v", err)
	}

	clashSub, _, err := (&ClashService{}).GetMihomoClash(client.Name)
	if err != nil {
		t.Fatalf("GetMihomoClash failed: %v", err)
	}

	var clashDoc map[string]interface{}
	if err := yaml.Unmarshal([]byte(*clashSub), &clashDoc); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}

	clashProxy := findNamedProxy(t, clashDoc["proxies"], inbound.Tag)
	if got, _ := clashProxy["type"].(string); got != "tuic" {
		t.Fatalf("expected mihomo clash proxy type tuic, got %v", clashProxy["type"])
	}
	if _, exists := clashProxy["fast-open"]; exists {
		t.Fatalf("expected mihomo tuic clash subscription to omit fast-open, got %#v", clashProxy["fast-open"])
	}
}

func TestMihomoShadowTLSSubscriptions(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mihomo-shadowtls.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	db := database.GetDB()
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB failed: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	inbound := model.MihomoInbound{
		Type:  "shadowtls",
		Tag:   "stls-443",
		Addrs: mustRawJSON(t, []interface{}{}),
		OutJson: mustRawJSON(t, map[string]interface{}{
			"tls": map[string]interface{}{
				"enabled":  true,
				"insecure": true,
				"alpn":     []interface{}{"h2", "http/1.1"},
				"utls": map[string]interface{}{
					"enabled":     true,
					"fingerprint": "safari",
				},
			},
		}),
		Options: mustRawJSON(t, map[string]interface{}{
			"listen_port": 443,
			"version":     3,
			"strict_mode": true,
			"handshake": map[string]interface{}{
				"server":      "addons.mozilla.org",
				"server_port": 443,
			},
			"ss_config": map[string]interface{}{
				"method":   "2022-blake3-aes-128-gcm",
				"password": "ss-pass",
				"network":  "udp",
				"udp_over_tcp": map[string]interface{}{
					"enabled": true,
					"version": 2,
				},
				"multiplex": map[string]interface{}{
					"enabled": true,
					"padding": true,
				},
			},
		}),
	}

	baseInbound := inbound.ToBase()
	if err := util.FillOutJson(&baseInbound, "panel.example.com"); err != nil {
		t.Fatalf("FillOutJson failed: %v", err)
	}
	inbound.OutJson = baseInbound.OutJson

	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create mihomo shadowtls inbound failed: %v", err)
	}

	client := model.MihomoClient{
		Enable: true,
		Name:   "shadowtls-user",
		Config: mustRawJSON(t, map[string]interface{}{
			"shadowtls": map[string]interface{}{
				"name":     "alice",
				"password": "shadow-pass",
			},
		}),
		Inbounds: mustRawJSON(t, []uint{inbound.Id}),
		Links:    mustRawJSON(t, []map[string]string{}),
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create mihomo shadowtls client failed: %v", err)
	}

	jsonSub, _, err := (&JsonService{}).GetMihomoJson(client.Name, "json")
	if err != nil {
		t.Fatalf("GetMihomoJson failed: %v", err)
	}

	var jsonDoc map[string]interface{}
	if err := json.Unmarshal([]byte(*jsonSub), &jsonDoc); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	ssOutbound := findTaggedOutbound(t, jsonDoc["outbounds"], inbound.Tag)
	if got, _ := ssOutbound["type"].(string); got != "shadowsocks" {
		t.Fatalf("expected shadowsocks outbound, got %v", ssOutbound["type"])
	}
	if got, _ := ssOutbound["password"].(string); got != "ss-pass" {
		t.Fatalf("expected shadowsocks password ss-pass, got %v", ssOutbound["password"])
	}
	if got, _ := ssOutbound["detour"].(string); got != inbound.Tag+"-out" {
		t.Fatalf("expected shadowsocks detour %q, got %v", inbound.Tag+"-out", ssOutbound["detour"])
	}
	if _, exists := ssOutbound["network"]; exists {
		t.Fatalf("expected mihomo shadowtls json shadowsocks outbound to omit legacy network, got %#v", ssOutbound["network"])
	}

	stlsOutbound := findTaggedOutbound(t, jsonDoc["outbounds"], inbound.Tag+"-out")
	if got, _ := stlsOutbound["type"].(string); got != "shadowtls" {
		t.Fatalf("expected shadowtls outbound, got %v", stlsOutbound["type"])
	}
	if got, _ := stlsOutbound["password"].(string); got != "shadow-pass" {
		t.Fatalf("expected shadowtls password shadow-pass, got %v", stlsOutbound["password"])
	}
	tlsMap := asMap(t, stlsOutbound["tls"])
	if got, _ := tlsMap["server_name"].(string); got != "addons.mozilla.org" {
		t.Fatalf("expected shadowtls tls server_name addons.mozilla.org, got %v", tlsMap["server_name"])
	}

	clashSub, _, err := (&ClashService{}).GetMihomoClash(client.Name)
	if err != nil {
		t.Fatalf("GetMihomoClash failed: %v", err)
	}

	var clashDoc map[string]interface{}
	if err := yaml.Unmarshal([]byte(*clashSub), &clashDoc); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}

	clashProxy := findNamedProxy(t, clashDoc["proxies"], inbound.Tag)
	if got, _ := clashProxy["type"].(string); got != "ss" {
		t.Fatalf("expected clash shadowtls proxy type ss, got %v", clashProxy["type"])
	}
	if got, _ := clashProxy["plugin"].(string); got != "shadow-tls" {
		t.Fatalf("expected clash plugin shadow-tls, got %v", clashProxy["plugin"])
	}
	if got, _ := clashProxy["password"].(string); got != "ss-pass" {
		t.Fatalf("expected clash shadowsocks password ss-pass, got %v", clashProxy["password"])
	}
	if got, _ := clashProxy["client-fingerprint"].(string); got != "safari" {
		t.Fatalf("expected clash client-fingerprint safari, got %v", clashProxy["client-fingerprint"])
	}
	if _, exists := clashProxy["udp"]; exists {
		t.Fatalf("expected mihomo shadowtls clash proxy not to derive udp from legacy network, got %#v", clashProxy["udp"])
	}
	if got, _ := clashProxy["udp-over-tcp"].(bool); !got {
		t.Fatalf("expected clash udp-over-tcp=true, got %v", clashProxy["udp-over-tcp"])
	}
	pluginOpts := asMap(t, clashProxy["plugin-opts"])
	if got, _ := pluginOpts["host"].(string); got != "addons.mozilla.org" {
		t.Fatalf("expected shadow-tls host addons.mozilla.org, got %v", pluginOpts["host"])
	}
	if got, _ := pluginOpts["password"].(string); got != "shadow-pass" {
		t.Fatalf("expected shadow-tls password shadow-pass, got %v", pluginOpts["password"])
	}
	if got := asIntValue(t, pluginOpts["version"]); got != 3 {
		t.Fatalf("expected shadow-tls version 3, got %v", pluginOpts["version"])
	}
	if _, exists := pluginOpts["skip-cert-verify"]; exists {
		t.Fatalf("expected mihomo shadow-tls plugin-opts.skip-cert-verify to be omitted, got %v", pluginOpts["skip-cert-verify"])
	}
	if _, exists := pluginOpts["alpn"]; exists {
		t.Fatalf("expected mihomo shadow-tls plugin-opts.alpn to be omitted, got %v", pluginOpts["alpn"])
	}
	if _, exists := pluginOpts["fingerprint"]; exists {
		t.Fatalf("expected mihomo shadow-tls plugin-opts.fingerprint to be omitted, got %v", pluginOpts["fingerprint"])
	}
}

func TestMihomoMieruSubscriptions(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mihomo-mieru.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	db := database.GetDB()
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB failed: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	inbound := model.MihomoInbound{
		Type:  "mieru",
		Tag:   "mieru-2999",
		Addrs: mustRawJSON(t, []interface{}{}),
		OutJson: mustRawJSON(t, map[string]interface{}{
			"udp":            true,
			"multiplexing":   "MULTIPLEXING_HIGH",
			"handshake_mode": "HANDSHAKE_NO_WAIT",
		}),
		Options: mustRawJSON(t, map[string]interface{}{
			"listen":        "0.0.0.0",
			"listen_port":   2999,
			"port_bindings": "2999,2090-2099",
			"transport":     "TCP",
		}),
	}

	baseInbound := inbound.ToBase()
	if err := util.FillOutJson(&baseInbound, "panel.example.com"); err != nil {
		t.Fatalf("FillOutJson failed: %v", err)
	}
	inbound.OutJson = baseInbound.OutJson

	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create mihomo mieru inbound failed: %v", err)
	}

	clientConfig := mustRawJSON(t, map[string]interface{}{
		"mieru": map[string]interface{}{
			"username": "alice",
			"password": "secret",
		},
	})
	links := util.LinkGenerator(clientConfig, &baseInbound, "panel.example.com")
	if len(links) != 2 {
		t.Fatalf("expected 2 mieru links, got %d", len(links))
	}

	linkDocs := make([]map[string]string, 0, len(links))
	for _, link := range links {
		linkDocs = append(linkDocs, map[string]string{
			"remark": inbound.Tag,
			"type":   "local",
			"uri":    link,
		})
	}

	client := model.MihomoClient{
		Enable:   true,
		Name:     "mieru-user",
		Config:   clientConfig,
		Inbounds: mustRawJSON(t, []uint{inbound.Id}),
		Links:    mustRawJSON(t, linkDocs),
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create mihomo mieru client failed: %v", err)
	}

	plainSub, _, err := (&SubService{}).GetMihomoSubs(client.Name)
	if err != nil {
		t.Fatalf("GetMihomoSubs failed: %v", err)
	}
	decodedPlain, err := base64.StdEncoding.DecodeString(*plainSub)
	if err != nil {
		t.Fatalf("decode plain mihomo subscription failed: %v", err)
	}
	plainText := string(decodedPlain)
	if !strings.Contains(plainText, "mierus://alice:secret@panel.example.com?") {
		t.Fatalf("unexpected mieru plain subscription: %s", plainText)
	}
	if !strings.Contains(plainText, "port=2999") || !strings.Contains(plainText, "port=2090-2099") {
		t.Fatalf("expected mieru ports in plain subscription, got %s", plainText)
	}

	jsonSub, _, err := (&JsonService{}).GetMihomoJson(client.Name, "json")
	if err != nil {
		t.Fatalf("GetMihomoJson failed: %v", err)
	}

	var jsonDoc map[string]interface{}
	if err := json.Unmarshal([]byte(*jsonSub), &jsonDoc); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if hasTaggedOutbound(jsonDoc["outbounds"], "1."+inbound.Tag) || hasTaggedOutbound(jsonDoc["outbounds"], "2."+inbound.Tag) {
		t.Fatalf("expected sing-box json subscription to skip unsupported mieru outbounds, got %#v", jsonDoc["outbounds"])
	}

	clashSub, _, err := (&ClashService{}).GetMihomoClash(client.Name)
	if err != nil {
		t.Fatalf("GetMihomoClash failed: %v", err)
	}

	var clashDoc map[string]interface{}
	if err := yaml.Unmarshal([]byte(*clashSub), &clashDoc); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}

	clashProxyA := findNamedProxy(t, clashDoc["proxies"], "1."+inbound.Tag)
	if got, _ := clashProxyA["transport"].(string); got != "TCP" {
		t.Fatalf("expected first mieru transport=TCP, got %v", clashProxyA["transport"])
	}
	if got, _ := clashProxyA["username"].(string); got != "alice" {
		t.Fatalf("expected first mieru username=alice, got %v", clashProxyA["username"])
	}
	clashProxyB := findNamedProxy(t, clashDoc["proxies"], "2."+inbound.Tag)
	if got, _ := clashProxyB["port-range"].(string); got != "2090-2099" {
		t.Fatalf("expected second mieru port-range=2090-2099, got %v", clashProxyB["port-range"])
	}
	if got, _ := clashProxyB["handshake-mode"].(string); got != "HANDSHAKE_NO_WAIT" {
		t.Fatalf("expected second mieru handshake-mode=HANDSHAKE_NO_WAIT, got %v", clashProxyB["handshake-mode"])
	}
	if got, _ := clashProxyB["username"].(string); got != "alice" {
		t.Fatalf("expected second mieru username=alice, got %v", clashProxyB["username"])
	}
}

func TestMihomoMieruSubscriptionSinglePortRangeOption(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mihomo-mieru-single-range.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	db := database.GetDB()
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB failed: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	inbound := model.MihomoInbound{
		Type:  "mieru",
		Tag:   "mieru-single-range",
		Addrs: mustRawJSON(t, []interface{}{}),
		OutJson: mustRawJSON(t, map[string]interface{}{
			"udp":            true,
			"multiplexing":   "MULTIPLEXING_LOW",
			"handshake_mode": "HANDSHAKE_STANDARD",
			"port_range":     "2090：2099",
		}),
		Options: mustRawJSON(t, map[string]interface{}{
			"listen":      "0.0.0.0",
			"listen_port": 16939,
			"transport":   "TCP",
			"port_range":  "2090：2099",
		}),
	}

	baseInbound := inbound.ToBase()
	if err := util.FillOutJson(&baseInbound, "panel.example.com"); err != nil {
		t.Fatalf("FillOutJson failed: %v", err)
	}
	inbound.OutJson = baseInbound.OutJson

	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create mihomo mieru inbound failed: %v", err)
	}

	client := model.MihomoClient{
		Enable:   true,
		Name:     "mieru-single-range-user",
		Config:   mustRawJSON(t, map[string]interface{}{"mieru": map[string]interface{}{"username": "alice", "password": "secret"}}),
		Inbounds: mustRawJSON(t, []uint{inbound.Id}),
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create mihomo mieru client failed: %v", err)
	}

	clashSub, _, err := (&ClashService{}).GetMihomoClash(client.Name)
	if err != nil {
		t.Fatalf("GetMihomoClash failed: %v", err)
	}

	var clashDoc map[string]interface{}
	if err := yaml.Unmarshal([]byte(*clashSub), &clashDoc); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}

	clashProxy := findNamedProxy(t, clashDoc["proxies"], inbound.Tag)
	if got, _ := clashProxy["port-range"].(string); got != "2090-2099" {
		t.Fatalf("expected mieru port-range=2090-2099, got %v", clashProxy["port-range"])
	}
	if _, exists := clashProxy["port"]; exists {
		t.Fatalf("expected clash mieru port to be omitted when port-range is set, got %#v", clashProxy["port"])
	}
	if got, _ := clashProxy["username"].(string); got != "alice" {
		t.Fatalf("expected mieru username=alice, got %v", clashProxy["username"])
	}
}

func TestMihomoMieruSubscriptionLegacyNameFallback(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mihomo-mieru-legacy-name.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	db := database.GetDB()
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB failed: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	inbound := model.MihomoInbound{
		Type:  "mieru",
		Tag:   "mieru-legacy-name",
		Addrs: mustRawJSON(t, []interface{}{}),
		OutJson: mustRawJSON(t, map[string]interface{}{
			"udp":            true,
			"multiplexing":   "MULTIPLEXING_LOW",
			"handshake_mode": "HANDSHAKE_STANDARD",
		}),
		Options: mustRawJSON(t, map[string]interface{}{
			"listen":      "0.0.0.0",
			"listen_port": 16940,
			"transport":   "TCP",
		}),
	}

	baseInbound := inbound.ToBase()
	if err := util.FillOutJson(&baseInbound, "panel.example.com"); err != nil {
		t.Fatalf("FillOutJson failed: %v", err)
	}
	inbound.OutJson = baseInbound.OutJson

	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create mihomo mieru inbound failed: %v", err)
	}

	client := model.MihomoClient{
		Enable: true,
		Name:   "mieru-legacy-user",
		Config: mustRawJSON(t, map[string]interface{}{
			"mieru": map[string]interface{}{
				"name":     "legacy-alice",
				"password": "secret",
			},
		}),
		Inbounds: mustRawJSON(t, []uint{inbound.Id}),
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create mihomo mieru client failed: %v", err)
	}

	clashSub, _, err := (&ClashService{}).GetMihomoClash(client.Name)
	if err != nil {
		t.Fatalf("GetMihomoClash failed: %v", err)
	}

	var clashDoc map[string]interface{}
	if err := yaml.Unmarshal([]byte(*clashSub), &clashDoc); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}

	clashProxy := findNamedProxy(t, clashDoc["proxies"], inbound.Tag)
	if got, _ := clashProxy["username"].(string); got != "legacy-alice" {
		t.Fatalf("expected mieru legacy name to fallback as username, got %v", clashProxy["username"])
	}
	if got, _ := clashProxy["password"].(string); got != "secret" {
		t.Fatalf("expected mieru password=secret, got %v", clashProxy["password"])
	}
}

func TestMihomoSudokuSubscriptions(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mihomo-sudoku.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	db := database.GetDB()
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB failed: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	inbound := model.MihomoInbound{
		Type:  "sudoku",
		Tag:   "sudoku-38571",
		Addrs: mustRawJSON(t, []interface{}{}),
		OutJson: mustRawJSON(t, map[string]interface{}{
			"httpmask": map[string]interface{}{
				"mode": "legacy",
			},
		}),
		Options: mustRawJSON(t, map[string]interface{}{
			"listen":       "0.0.0.0",
			"listen_port":  38571,
			"key":          "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
			"table_type":   "prefer_entropy",
			"custom_table": "ppvxvxvv",
			"custom_tables": []interface{}{
				"ppvxvxvv",
				"vxvpvpvx",
			},
		}),
	}

	baseInbound := inbound.ToBase()
	if err := util.FillOutJson(&baseInbound, "panel.example.com"); err != nil {
		t.Fatalf("FillOutJson failed: %v", err)
	}
	inbound.OutJson = baseInbound.OutJson

	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create mihomo sudoku inbound failed: %v", err)
	}

	t.Run("client uuid maps to clash key", func(t *testing.T) {
		client := model.MihomoClient{
			Enable: true,
			Name:   "sudoku-user",
			Config: mustRawJSON(t, map[string]interface{}{
				"sudoku": map[string]interface{}{
					"uuid": " 11111111-2222-3333-4444-555555555555 \n",
				},
			}),
			Inbounds: mustRawJSON(t, []uint{inbound.Id}),
		}
		if err := db.Create(&client).Error; err != nil {
			t.Fatalf("create mihomo sudoku client failed: %v", err)
		}

		clashSub, _, err := (&ClashService{}).GetMihomoClash(client.Name)
		if err != nil {
			t.Fatalf("GetMihomoClash failed: %v", err)
		}

		var clashDoc map[string]interface{}
		if err := yaml.Unmarshal([]byte(*clashSub), &clashDoc); err != nil {
			t.Fatalf("yaml.Unmarshal failed: %v", err)
		}

		clashProxy := findNamedProxy(t, clashDoc["proxies"], inbound.Tag)
		if got, _ := clashProxy["type"].(string); got != "sudoku" {
			t.Fatalf("expected sudoku clash proxy type, got %v", clashProxy["type"])
		}
		if got, _ := clashProxy["key"].(string); got != "11111111-2222-3333-4444-555555555555" {
			t.Fatalf("expected sudoku key from client uuid, got %#v", clashProxy["key"])
		}
		if _, exists := clashProxy["uuid"]; exists {
			t.Fatalf("expected clash sudoku proxy to omit uuid, got %#v", clashProxy["uuid"])
		}
		if !strings.Contains(*clashSub, `custom-tables: ["ppvxvxvv","vxvpvpvx"]`) {
			t.Fatalf("expected clash yaml custom-tables flow style, got:\n%s", *clashSub)
		}
	})

	t.Run("fallback to inbound options key when client uuid is empty", func(t *testing.T) {
		client := model.MihomoClient{
			Enable: true,
			Name:   "sudoku-user-fallback",
			Config: mustRawJSON(t, map[string]interface{}{
				"sudoku": map[string]interface{}{
					"uuid": "",
				},
			}),
			Inbounds: mustRawJSON(t, []uint{inbound.Id}),
		}
		if err := db.Create(&client).Error; err != nil {
			t.Fatalf("create fallback mihomo sudoku client failed: %v", err)
		}

		clashSub, _, err := (&ClashService{}).GetMihomoClash(client.Name)
		if err != nil {
			t.Fatalf("GetMihomoClash failed: %v", err)
		}

		var clashDoc map[string]interface{}
		if err := yaml.Unmarshal([]byte(*clashSub), &clashDoc); err != nil {
			t.Fatalf("yaml.Unmarshal failed: %v", err)
		}

		clashProxy := findNamedProxy(t, clashDoc["proxies"], inbound.Tag)
		if got, _ := clashProxy["key"].(string); got != "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee" {
			t.Fatalf("expected sudoku key fallback from inbound options, got %#v", clashProxy["key"])
		}
	})

	t.Run("drops invalid custom tables in clash output", func(t *testing.T) {
		invalidInbound := model.MihomoInbound{
			Type:    "sudoku",
			Tag:     "sudoku-invalid-custom-tables",
			Addrs:   mustRawJSON(t, []interface{}{}),
			OutJson: mustRawJSON(t, map[string]interface{}{}),
			Options: mustRawJSON(t, map[string]interface{}{
				"listen":       "0.0.0.0",
				"listen_port":  53322,
				"key":          "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
				"aead_method":  "chacha20-poly1305",
				"padding_min":  1,
				"padding_max":  15,
				"table_type":   "prefer_ascii",
				"custom_table": "vvxxpvxp",
				"custom_tables": []interface{}{
					"vvxxpvxp",
					"xvvvxxpp",
				},
				"httpmask": map[string]interface{}{
					"mode": "legacy",
				},
			}),
		}

		invalidBaseInbound := invalidInbound.ToBase()
		if err := util.FillOutJson(&invalidBaseInbound, "panel.example.com"); err != nil {
			t.Fatalf("FillOutJson for invalid sudoku inbound failed: %v", err)
		}
		invalidInbound.OutJson = invalidBaseInbound.OutJson

		if err := db.Create(&invalidInbound).Error; err != nil {
			t.Fatalf("create invalid sudoku inbound failed: %v", err)
		}

		client := model.MihomoClient{
			Enable: true,
			Name:   "sudoku-user-invalid-custom",
			Config: mustRawJSON(t, map[string]interface{}{
				"sudoku": map[string]interface{}{
					"uuid": "11111111-2222-3333-4444-555555555555",
				},
			}),
			Inbounds: mustRawJSON(t, []uint{invalidInbound.Id}),
		}
		if err := db.Create(&client).Error; err != nil {
			t.Fatalf("create invalid sudoku client failed: %v", err)
		}

		clashSub, _, err := (&ClashService{}).GetMihomoClash(client.Name)
		if err != nil {
			t.Fatalf("GetMihomoClash failed: %v", err)
		}

		var clashDoc map[string]interface{}
		if err := yaml.Unmarshal([]byte(*clashSub), &clashDoc); err != nil {
			t.Fatalf("yaml.Unmarshal failed: %v", err)
		}

		clashProxy := findNamedProxy(t, clashDoc["proxies"], invalidInbound.Tag)
		if _, exists := clashProxy["custom-table"]; exists {
			t.Fatalf("expected invalid custom-table to be removed, got %#v", clashProxy["custom-table"])
		}
		if _, exists := clashProxy["custom-tables"]; exists {
			t.Fatalf("expected invalid custom-tables to be removed, got %#v", clashProxy["custom-tables"])
		}
		if got, _ := clashProxy["table-type"].(string); got != "prefer_ascii" {
			t.Fatalf("expected table-type to stay prefer_ascii when no valid custom tables, got %#v", clashProxy["table-type"])
		}
	})

	t.Run("mihomo json subscription skips sudoku outbound", func(t *testing.T) {
		client := model.MihomoClient{
			Enable: true,
			Name:   "sudoku-user-json",
			Config: mustRawJSON(t, map[string]interface{}{
				"sudoku": map[string]interface{}{
					"uuid": "11111111-2222-3333-4444-555555555555",
				},
			}),
			Inbounds: mustRawJSON(t, []uint{inbound.Id}),
		}
		if err := db.Create(&client).Error; err != nil {
			t.Fatalf("create mihomo sudoku json client failed: %v", err)
		}

		jsonSub, _, err := (&JsonService{}).GetMihomoJson(client.Name, "json")
		if err != nil {
			t.Fatalf("GetMihomoJson failed: %v", err)
		}

		var jsonDoc map[string]interface{}
		if err := json.Unmarshal([]byte(*jsonSub), &jsonDoc); err != nil {
			t.Fatalf("json.Unmarshal failed: %v", err)
		}

		outbounds, ok := jsonDoc["outbounds"].([]interface{})
		if !ok {
			t.Fatalf("expected outbounds array, got %#v", jsonDoc["outbounds"])
		}
		for _, item := range outbounds {
			obMap, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if got, _ := obMap["type"].(string); got == "sudoku" {
				t.Fatalf("expected mihomo json subscription to skip sudoku outbounds, got %#v", obMap)
			}
		}
	})
}

func TestMihomoTrustTunnelSubscriptions(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mihomo-trusttunnel.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	db := database.GetDB()
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB failed: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
	certificateLines, expectedFingerprint := buildLeafCertificateForFingerprint(t)

	tlsConfig := model.MihomoTls{
		Name: "trusttunnel-tls",
		Server: mustRawJSON(t, map[string]interface{}{
			"enabled":     true,
			"server_name": "edge.example.com",
			"alpn":        []interface{}{"h2"},
			"certificate": certificateLines,
		}),
		Client: mustRawJSON(t, map[string]interface{}{
			"insecure":    true,
			"disable_sni": true,
			"utls": map[string]interface{}{
				"enabled":     true,
				"fingerprint": "chrome",
			},
		}),
	}
	if err := db.Create(&tlsConfig).Error; err != nil {
		t.Fatalf("create mihomo tls failed: %v", err)
	}

	inbound := model.MihomoInbound{
		Type:  "trusttunnel",
		Tag:   "trusttunnel-443",
		TlsId: tlsConfig.Id,
		Tls:   &tlsConfig,
		Addrs: mustRawJSON(t, []interface{}{}),
		OutJson: mustRawJSON(t, map[string]interface{}{
			"network":               []interface{}{"tcp", "udp"},
			"congestion_controller": "bbr",
			"quic":                  true,
			"max_connections":       1,
			"min_streams":           0,
			"max_streams":           0,
		}),
		Options: mustRawJSON(t, map[string]interface{}{
			"listen":                "0.0.0.0",
			"listen_port":           443,
			"network":               []interface{}{"tcp", "udp"},
			"congestion_controller": "bbr",
		}),
	}

	baseInbound := inbound.ToBase()
	if err := util.FillOutJson(&baseInbound, "panel.example.com"); err != nil {
		t.Fatalf("FillOutJson failed: %v", err)
	}
	inbound.OutJson = baseInbound.OutJson

	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create mihomo trusttunnel inbound failed: %v", err)
	}

	client := model.MihomoClient{
		Enable: true,
		Name:   "trusttunnel-user",
		Config: mustRawJSON(t, map[string]interface{}{
			"trusttunnel": map[string]interface{}{
				"uuid": "tt-secret",
			},
		}),
		Inbounds: mustRawJSON(t, []uint{inbound.Id}),
		Links:    mustRawJSON(t, []map[string]string{}),
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create mihomo trusttunnel client failed: %v", err)
	}

	jsonSub, _, err := (&JsonService{}).GetMihomoJson(client.Name, "json")
	if err != nil {
		t.Fatalf("GetMihomoJson failed: %v", err)
	}

	var jsonDoc map[string]interface{}
	if err := json.Unmarshal([]byte(*jsonSub), &jsonDoc); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if hasTaggedOutbound(jsonDoc["outbounds"], inbound.Tag) {
		t.Fatalf("expected sing-box json subscription to filter trusttunnel outbounds, got %#v", jsonDoc["outbounds"])
	}

	clashSub, _, err := (&ClashService{}).GetMihomoClash(client.Name)
	if err != nil {
		t.Fatalf("GetMihomoClash failed: %v", err)
	}

	var clashDoc map[string]interface{}
	if err := yaml.Unmarshal([]byte(*clashSub), &clashDoc); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}

	clashProxy := findNamedProxy(t, clashDoc["proxies"], inbound.Tag)
	if got, _ := clashProxy["type"].(string); got != "trusttunnel" {
		t.Fatalf("expected trusttunnel clash proxy type, got %v", clashProxy["type"])
	}
	if got, _ := clashProxy["username"].(string); got != client.Name {
		t.Fatalf("expected trusttunnel username to use client name %q, got %v", client.Name, clashProxy["username"])
	}
	if got, _ := clashProxy["password"].(string); got != "tt-secret" {
		t.Fatalf("expected trusttunnel password fallback from uuid, got %v", clashProxy["password"])
	}
	if got, _ := clashProxy["sni"].(string); got != "edge.example.com" {
		t.Fatalf("expected trusttunnel sni edge.example.com, got %v", clashProxy["sni"])
	}
	alpn := asStringSliceValue(t, clashProxy["alpn"])
	if len(alpn) != 1 || alpn[0] != "h2" {
		t.Fatalf("expected trusttunnel alpn [h2], got %#v", clashProxy["alpn"])
	}
	if got, _ := clashProxy["fingerprint"].(string); got != expectedFingerprint {
		t.Fatalf("expected trusttunnel fingerprint %q, got %v", expectedFingerprint, clashProxy["fingerprint"])
	}
	if got, _ := clashProxy["client-fingerprint"].(string); got != "chrome" {
		t.Fatalf("expected trusttunnel client-fingerprint chrome, got %v", clashProxy["client-fingerprint"])
	}
	if got, _ := clashProxy["disable-sni"].(bool); !got {
		t.Fatalf("expected trusttunnel disable-sni=true, got %v", clashProxy["disable-sni"])
	}
	if got, _ := clashProxy["congestion-controller"].(string); got != "bbr" {
		t.Fatalf("expected trusttunnel congestion-controller bbr, got %v", clashProxy["congestion-controller"])
	}
	if got, _ := clashProxy["max-connections"].(int); got != 1 {
		t.Fatalf("expected trusttunnel max-connections=1, got %v", clashProxy["max-connections"])
	}
	if got, _ := clashProxy["min-streams"].(int); got != 0 {
		t.Fatalf("expected trusttunnel min-streams=0, got %v", clashProxy["min-streams"])
	}
	if got, _ := clashProxy["max-streams"].(int); got != 0 {
		t.Fatalf("expected trusttunnel max-streams=0, got %v", clashProxy["max-streams"])
	}
	if got, _ := clashProxy["quic"].(bool); !got {
		t.Fatalf("expected trusttunnel quic=true, got %v", clashProxy["quic"])
	}
	if got, _ := clashProxy["udp"].(bool); !got {
		t.Fatalf("expected trusttunnel subscription udp=true from listener network, got %v", clashProxy["udp"])
	}
}

func mustRawJSON(t *testing.T, value interface{}) json.RawMessage {
	t.Helper()

	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	return json.RawMessage(raw)
}

func findTaggedOutbound(t *testing.T, raw interface{}, tag string) map[string]interface{} {
	t.Helper()

	outbounds, ok := raw.([]interface{})
	if !ok {
		t.Fatalf("expected outbounds slice, got %T", raw)
	}

	for _, outboundRaw := range outbounds {
		outbound := asMap(t, outboundRaw)
		currentTag, _ := outbound["tag"].(string)
		if currentTag == tag {
			return outbound
		}
	}

	t.Fatalf("outbound with tag %q not found", tag)
	return nil
}

func findNamedProxy(t *testing.T, raw interface{}, name string) map[string]interface{} {
	t.Helper()

	proxies, ok := raw.([]interface{})
	if !ok {
		t.Fatalf("expected proxies slice, got %T", raw)
	}

	for _, proxyRaw := range proxies {
		proxy := asMap(t, proxyRaw)
		currentName, _ := proxy["name"].(string)
		if currentName == name {
			return proxy
		}
	}

	t.Fatalf("proxy with name %q not found", name)
	return nil
}

func hasTaggedOutbound(raw interface{}, tag string) bool {
	outbounds, ok := raw.([]interface{})
	if !ok {
		return false
	}

	for _, outboundRaw := range outbounds {
		outbound, ok := outboundRaw.(map[string]interface{})
		if !ok {
			continue
		}
		currentTag, _ := outbound["tag"].(string)
		if currentTag == tag {
			return true
		}
	}

	return false
}
