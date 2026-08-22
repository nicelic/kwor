package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestValidateSingboxInboundPayloadRejectsLossyIdentifiersAndPorts(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		wantErr string
	}{
		{
			name:    "missing tag",
			payload: `{"type":"socks","tag":"","listen_port":443}`,
			wantErr: "tag is required",
		},
		{
			name:    "missing listen port",
			payload: `{"type":"socks","tag":"socks-1"}`,
			wantErr: "listen_port is required",
		},
		{
			name:    "fractional listen port",
			payload: `{"type":"socks","tag":"socks-1","listen_port":443.5}`,
			wantErr: "complete decimal integer",
		},
		{
			name:    "fractional override port",
			payload: `{"type":"direct","tag":"direct-1","listen_port":8443,"override_port":53.5}`,
			wantErr: "override_port",
		},
		{
			name:    "fractional record id",
			payload: `{"id":1.5,"type":"socks","tag":"socks-1","listen_port":443}`,
			wantErr: "id: must be a complete decimal integer",
		},
		{
			name:    "required TLS",
			payload: `{"type":"tuic","tag":"tuic-1","listen_port":443,"tls_id":0}`,
			wantErr: "requires a TLS configuration",
		},
		{
			name:    "TUN MTU below minimum",
			payload: `{"type":"tun","tag":"tun-1","mtu":575}`,
			wantErr: "mtu: must be an integer from 576 to 65535",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload := json.RawMessage(test.payload)
			var inbound model.Inbound
			if err := inbound.UnmarshalJSON(payload); err != nil {
				t.Fatalf("unmarshal payload: %v", err)
			}
			err := validateSingboxInboundPayload(payload, &inbound, "new")
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("validate error = %v, want substring %q", err, test.wantErr)
			}
		})
	}
}

func TestValidateSingboxInboundPayloadAllowsTunAndCanonicalizesNumericStrings(t *testing.T) {
	tunPayload := json.RawMessage(`{"type":"tun","tag":"tun-1","interface_name":"tun0"}`)
	var tun model.Inbound
	if err := tun.UnmarshalJSON(tunPayload); err != nil {
		t.Fatalf("unmarshal tun payload: %v", err)
	}
	if err := validateSingboxInboundPayload(tunPayload, &tun, "new"); err != nil {
		t.Fatalf("tun without listen_port should be valid: %v", err)
	}

	payload := json.RawMessage(`{"type":"direct","tag":" direct-1 ","listen_port":"8443","override_port":"53"}`)
	var inbound model.Inbound
	if err := inbound.UnmarshalJSON(payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if err := validateSingboxInboundPayload(payload, &inbound, "new"); err != nil {
		t.Fatalf("validate normalized payload: %v", err)
	}
	if inbound.Tag != "direct-1" {
		t.Fatalf("tag was not trimmed: %q", inbound.Tag)
	}
	var options map[string]json.RawMessage
	if err := json.Unmarshal(inbound.Options, &options); err != nil {
		t.Fatalf("decode normalized options: %v", err)
	}
	for key, want := range map[string]int{"listen_port": 8443, "override_port": 53} {
		var got int
		if err := json.Unmarshal(options[key], &got); err != nil || got != want {
			t.Fatalf("normalized %s = %v (err=%v), want %d", key, got, err, want)
		}
	}
}

func TestValidateAndNormalizeSingboxEndpointPayloadProtectsWireguardNumericFields(t *testing.T) {
	payload := json.RawMessage(`{
		"type":" wireguard ",
		"tag":" wg-endpoint ",
		"listen_port":"51820",
		"workers":"2",
		"mtu":"1420",
		"peers":[{
			"port":"51821",
			"persistent_keepalive_interval":"25",
			"reserved":["1",2,"3"]
		}],
		"ext":{}
	}`)

	normalized, identity, err := validateAndNormalizeSingboxEndpointPayload(payload, "new")
	if err != nil {
		t.Fatalf("normalize endpoint payload: %v", err)
	}
	if identity.Type != "wireguard" || identity.Tag != "wg-endpoint" {
		t.Fatalf("normalized identity = (%q, %q)", identity.Type, identity.Tag)
	}

	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(normalized, &fields); err != nil {
		t.Fatalf("decode normalized endpoint: %v", err)
	}
	for key, want := range map[string]int{"listen_port": 51820, "workers": 2, "mtu": 1420} {
		var got int
		if err := json.Unmarshal(fields[key], &got); err != nil || got != want {
			t.Fatalf("normalized %s = %v (err=%v), want %d", key, got, err, want)
		}
	}

	peers := []map[string]json.RawMessage{}
	if err := json.Unmarshal(fields["peers"], &peers); err != nil || len(peers) != 1 {
		t.Fatalf("decode normalized peers: peers=%v err=%v", peers, err)
	}
	for key, want := range map[string]int{"port": 51821, "persistent_keepalive_interval": 25} {
		var got int
		if err := json.Unmarshal(peers[0][key], &got); err != nil || got != want {
			t.Fatalf("normalized peer %s = %v (err=%v), want %d", key, got, err, want)
		}
	}
	var reserved []int
	if err := json.Unmarshal(peers[0]["reserved"], &reserved); err != nil || len(reserved) != 3 || reserved[0] != 1 || reserved[1] != 2 || reserved[2] != 3 {
		t.Fatalf("normalized peer reserved = %v (err=%v)", reserved, err)
	}

	for _, test := range []struct {
		name    string
		payload string
		wantErr string
	}{
		{
			name:    "fractional worker",
			payload: `{"type":"wireguard","tag":"wg","workers":1.5}`,
			wantErr: "workers",
		},
		{
			name:    "fractional peer port",
			payload: `{"type":"wireguard","tag":"wg","peers":[{"port":51820.5}]}`,
			wantErr: "peers[0]: port",
		},
		{
			name:    "invalid reserved byte count",
			payload: `{"type":"wireguard","tag":"wg","peers":[{"reserved":[1,2]}]}`,
			wantErr: "exactly three bytes",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := validateAndNormalizeSingboxEndpointPayload(json.RawMessage(test.payload), "new")
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("normalize error = %v, want substring %q", err, test.wantErr)
			}
		})
	}
}

func TestValidateAndNormalizeSingboxOutboundPayloadProtectsNestedNumericFields(t *testing.T) {
	payload := json.RawMessage(`{
		"type":"shadowtls",
		"tag":"shadowtls-1",
		"server":"example.com",
		"server_port":"443",
		"handshake":{"server":"addons.mozilla.org","server_port":"8443"},
		"transport":{"max_connections":"8","min_streams":"0"}
	}`)
	normalized, _, err := validateAndNormalizeSingboxOutboundPayload(payload, "new")
	if err != nil {
		t.Fatalf("normalize outbound payload: %v", err)
	}
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(normalized, &fields); err != nil {
		t.Fatalf("decode normalized outbound: %v", err)
	}
	var serverPort int
	if err := json.Unmarshal(fields["server_port"], &serverPort); err != nil || serverPort != 443 {
		t.Fatalf("normalized server_port = %d (err=%v)", serverPort, err)
	}
	handshake := map[string]json.RawMessage{}
	if err := json.Unmarshal(fields["handshake"], &handshake); err != nil {
		t.Fatalf("decode normalized handshake: %v", err)
	}
	var handshakePort int
	if err := json.Unmarshal(handshake["server_port"], &handshakePort); err != nil || handshakePort != 8443 {
		t.Fatalf("normalized handshake server_port = %d (err=%v)", handshakePort, err)
	}
	transport := map[string]json.RawMessage{}
	if err := json.Unmarshal(fields["transport"], &transport); err != nil {
		t.Fatalf("decode normalized transport: %v", err)
	}
	for key, want := range map[string]int{"max_connections": 8, "min_streams": 0} {
		var got int
		if err := json.Unmarshal(transport[key], &got); err != nil || got != want {
			t.Fatalf("normalized transport %s = %v (err=%v), want %d", key, got, err, want)
		}
	}

	_, _, err = validateAndNormalizeSingboxOutboundPayload(json.RawMessage(`{"type":"socks","tag":"socks-1","server_port":443.5}`), "new")
	if err == nil || !strings.Contains(err.Error(), "server_port") {
		t.Fatalf("fractional outbound port error = %v", err)
	}
	_, _, err = validateAndNormalizeSingboxOutboundPayload(json.RawMessage(`{"type":"shadowtls","tag":"shadowtls-1","handshake":{"server_port":443.5}}`), "new")
	if err == nil || !strings.Contains(err.Error(), "handshake.server_port") {
		t.Fatalf("fractional nested outbound port error = %v", err)
	}
}

func TestSingboxInboundAndServicePayloadsNormalizeSharedVisibleIntegers(t *testing.T) {
	inboundPayload := json.RawMessage(`{"type":"tun","tag":"tun-integer","mtu":"1500","udp_timeout":"5m"}`)
	var inbound model.Inbound
	if err := inbound.UnmarshalJSON(inboundPayload); err != nil {
		t.Fatalf("unmarshal inbound payload: %v", err)
	}
	if err := validateSingboxInboundPayload(inboundPayload, &inbound, "new"); err != nil {
		t.Fatalf("normalize inbound payload: %v", err)
	}
	inboundOptions := map[string]json.RawMessage{}
	if err := json.Unmarshal(inbound.Options, &inboundOptions); err != nil {
		t.Fatalf("decode normalized inbound options: %v", err)
	}
	var mtu int
	if err := json.Unmarshal(inboundOptions["mtu"], &mtu); err != nil || mtu != 1500 {
		t.Fatalf("normalized inbound mtu = %d (err=%v)", mtu, err)
	}

	fractionalInbound := json.RawMessage(`{"type":"tun","tag":"tun-integer","mtu":1500.5}`)
	if err := inbound.UnmarshalJSON(fractionalInbound); err != nil {
		t.Fatalf("unmarshal fractional inbound payload: %v", err)
	}
	if err := validateSingboxInboundPayload(fractionalInbound, &inbound, "new"); err == nil || !strings.Contains(err.Error(), "mtu") {
		t.Fatalf("fractional inbound mtu error = %v", err)
	}

	servicePayload := json.RawMessage(`{
		"type":"derp",
		"tag":"derp-integer",
		"listen_port":"8443",
		"mesh_with":[{"server":"mesh.example.com","server_port":"443"}]
	}`)
	normalizedService, _, err := validateAndNormalizeSingboxServicePayload(servicePayload, "new")
	if err != nil {
		t.Fatalf("normalize service payload: %v", err)
	}
	serviceFields := map[string]json.RawMessage{}
	if err := json.Unmarshal(normalizedService, &serviceFields); err != nil {
		t.Fatalf("decode normalized service: %v", err)
	}
	var listenPort int
	if err := json.Unmarshal(serviceFields["listen_port"], &listenPort); err != nil || listenPort != 8443 {
		t.Fatalf("normalized service listen_port = %d (err=%v)", listenPort, err)
	}
	mesh := []map[string]json.RawMessage{}
	if err := json.Unmarshal(serviceFields["mesh_with"], &mesh); err != nil || len(mesh) != 1 {
		t.Fatalf("decode normalized service mesh: mesh=%v err=%v", mesh, err)
	}
	var meshPort int
	if err := json.Unmarshal(mesh[0]["server_port"], &meshPort); err != nil || meshPort != 443 {
		t.Fatalf("normalized service mesh port = %d (err=%v)", meshPort, err)
	}

	for _, test := range []struct {
		name    string
		payload string
		wantErr string
	}{
		{
			name:    "missing listener",
			payload: `{"type":"derp","tag":"derp-integer"}`,
			wantErr: "listen_port is required",
		},
		{
			name:    "fractional listener",
			payload: `{"type":"derp","tag":"derp-integer","listen_port":8443.5}`,
			wantErr: "listen_port",
		},
		{
			name:    "fractional nested mesh port",
			payload: `{"type":"derp","tag":"derp-integer","listen_port":8443,"mesh_with":[{"server_port":443.5}]}`,
			wantErr: "mesh_with[0].server_port",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := validateAndNormalizeSingboxServicePayload(json.RawMessage(test.payload), "new")
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("service normalization error = %v, want substring %q", err, test.wantErr)
			}
		})
	}
}

func TestSingboxVisibleIntegerOptionsCoverRemainingDefaultEditors(t *testing.T) {
	payload := json.RawMessage(`{
		"type":"urltest",
		"tag":"numeric-editors",
		"tolerance":"50",
		"insecure_concurrency":"2",
		"max_early_data":"1024",
		"ping_interval":"30",
		"sc_max_each_post_bytes":"4096"
	}`)
	normalized, _, err := validateAndNormalizeSingboxOutboundPayload(payload, "new")
	if err != nil {
		t.Fatalf("normalize visible integer payload: %v", err)
	}
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(normalized, &fields); err != nil {
		t.Fatalf("decode normalized visible integer payload: %v", err)
	}
	for key, want := range map[string]int{
		"tolerance":              50,
		"insecure_concurrency":   2,
		"max_early_data":         1024,
		"ping_interval":          30,
		"sc_max_each_post_bytes": 4096,
	} {
		var got int
		if err := json.Unmarshal(fields[key], &got); err != nil || got != want {
			t.Fatalf("normalized %s = %v (err=%v), want %d", key, got, err, want)
		}
	}

	inboundPayload := json.RawMessage(`{
		"type":"hysteria2",
		"tag":"hysteria-bandwidth",
		"listen_port":8443,
		"tls_id":1,
		"server_up_mbps":"500",
		"server_down_mbps":"600",
		"handshake_timeout":"5"
	}`)
	var inbound model.Inbound
	if err := inbound.UnmarshalJSON(inboundPayload); err != nil {
		t.Fatalf("unmarshal bandwidth inbound: %v", err)
	}
	if err := validateSingboxInboundPayload(inboundPayload, &inbound, "new"); err != nil {
		t.Fatalf("normalize bandwidth inbound: %v", err)
	}
	inboundOptions := map[string]json.RawMessage{}
	if err := json.Unmarshal(inbound.Options, &inboundOptions); err != nil {
		t.Fatalf("decode normalized inbound options: %v", err)
	}
	for key, want := range map[string]int{
		"server_up_mbps":    500,
		"server_down_mbps":  600,
		"handshake_timeout": 5,
	} {
		var got int
		if err := json.Unmarshal(inboundOptions[key], &got); err != nil || got != want {
			t.Fatalf("normalized inbound %s = %v (err=%v), want %d", key, got, err, want)
		}
	}

	for _, test := range []struct {
		name    string
		payload string
		wantErr string
	}{
		{name: "fractional tolerance", payload: `{"type":"urltest","tag":"numeric","tolerance":50.5}`, wantErr: "tolerance"},
		{name: "fractional Naive concurrency", payload: `{"type":"naive","tag":"numeric","insecure_concurrency":2.5}`, wantErr: "insecure_concurrency"},
		{name: "fractional WebSocket early data", payload: `{"type":"vmess","tag":"numeric","transport":{"max_early_data":1024.5}}`, wantErr: "transport.max_early_data"},
		{name: "fractional XHTTP post bytes", payload: `{"type":"vmess","tag":"numeric","transport":{"sc_max_each_post_bytes":4096.5}}`, wantErr: "transport.sc_max_each_post_bytes"},
		{name: "fractional Hysteria bandwidth", payload: `{"type":"hysteria2","tag":"numeric","listen_port":8443,"server_up_mbps":500.5}`, wantErr: "server_up_mbps"},
		{name: "fractional Sudoku timeout", payload: `{"type":"sudoku","tag":"numeric","listen_port":8443,"handshake_timeout":5.5}`, wantErr: "handshake_timeout"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if strings.Contains(test.payload, `"listen_port"`) {
				var candidate model.Inbound
				data := json.RawMessage(test.payload)
				if err := candidate.UnmarshalJSON(data); err != nil {
					t.Fatalf("unmarshal inbound payload: %v", err)
				}
				err := validateSingboxInboundPayload(data, &candidate, "new")
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("inbound validation error = %v, want substring %q", err, test.wantErr)
				}
				return
			}
			_, _, err := validateAndNormalizeSingboxOutboundPayload(json.RawMessage(test.payload), "new")
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("outbound normalization error = %v, want substring %q", err, test.wantErr)
			}
		})
	}
}

func TestSingboxEndpointAndOutboundSaveCanonicalizeVisibleIntegerFields(t *testing.T) {
	db := initOutboundEditMergeTestDB(t, "singbox-visible-integer-fields.db")

	endpointPayload := mustMarshalJSON(t, map[string]interface{}{
		"type":        "wireguard",
		"tag":         "wg-normalized",
		"listen_port": "51820",
		"workers":     "2",
		"mtu":         "1420",
		"peers": []map[string]interface{}{{
			"port":                          "51821",
			"persistent_keepalive_interval": "25",
			"reserved":                      []string{"1", "2", "3"},
		}},
		"ext": map[string]interface{}{},
	})
	if err := (&EndpointService{}).Save(db, "new", endpointPayload); err != nil {
		t.Fatalf("save normalized endpoint: %v", err)
	}
	var endpoint model.Endpoint
	if err := db.Where("tag = ?", "wg-normalized").First(&endpoint).Error; err != nil {
		t.Fatalf("load endpoint: %v", err)
	}
	endpointOptions := map[string]json.RawMessage{}
	if err := json.Unmarshal(endpoint.Options, &endpointOptions); err != nil {
		t.Fatalf("decode endpoint options: %v", err)
	}
	for key, want := range map[string]int{"listen_port": 51820, "workers": 2, "mtu": 1420} {
		var got int
		if err := json.Unmarshal(endpointOptions[key], &got); err != nil || got != want {
			t.Fatalf("stored endpoint %s = %v (err=%v), want %d", key, got, err, want)
		}
	}

	outboundPayload := mustMarshalJSON(t, map[string]interface{}{
		"type":        "socks",
		"tag":         "socks-normalized",
		"server":      "127.0.0.1",
		"server_port": "1080",
	})
	if err := (&OutboundService{}).Save(db, "new", outboundPayload); err != nil {
		t.Fatalf("save normalized outbound: %v", err)
	}
	var outbound model.Outbound
	if err := db.Where("tag = ?", "socks-normalized").First(&outbound).Error; err != nil {
		t.Fatalf("load outbound: %v", err)
	}
	rawOutbound := map[string]json.RawMessage{}
	if err := json.Unmarshal(outbound.RawOutbound, &rawOutbound); err != nil {
		t.Fatalf("decode stored outbound: %v", err)
	}
	var gotPort int
	if err := json.Unmarshal(rawOutbound["server_port"], &gotPort); err != nil || gotPort != 1080 {
		t.Fatalf("stored outbound server_port = %d (err=%v)", gotPort, err)
	}
}

func TestSingboxEditRejectsMissingRecordsWithoutCreatingThem(t *testing.T) {
	db := initOutboundEditMergeTestDB(t, "singbox-edit-missing-records.db")

	tests := []struct {
		name  string
		save  func() error
		count func() (int64, error)
	}{
		{
			name: "inbound",
			save: func() error {
				_, err := (&InboundService{}).Save(db, "edit", mustMarshalJSON(t, map[string]interface{}{
					"id":          9101,
					"type":        "socks",
					"tag":         "missing-inbound",
					"listen":      "::",
					"listen_port": 19101,
				}), "", "panel.example.com")
				return err
			},
			count: func() (int64, error) {
				var count int64
				err := db.Model(&model.Inbound{}).Where("id = ?", 9101).Count(&count).Error
				return count, err
			},
		},
		{
			name: "outbound",
			save: func() error {
				return (&OutboundService{}).Save(db, "edit", mustMarshalJSON(t, map[string]interface{}{
					"id":   9102,
					"type": "direct",
					"tag":  "missing-outbound",
				}))
			},
			count: func() (int64, error) {
				var count int64
				err := db.Model(&model.Outbound{}).Where("id = ?", 9102).Count(&count).Error
				return count, err
			},
		},
		{
			name: "endpoint",
			save: func() error {
				return (&EndpointService{}).Save(db, "edit", mustMarshalJSON(t, map[string]interface{}{
					"id":   9103,
					"type": "wireguard",
					"tag":  "missing-endpoint",
					"ext":  map[string]interface{}{},
				}))
			},
			count: func() (int64, error) {
				var count int64
				err := db.Model(&model.Endpoint{}).Where("id = ?", 9103).Count(&count).Error
				return count, err
			},
		},
		{
			name: "service",
			save: func() error {
				return (&ServicesService{}).Save(db, "edit", mustMarshalJSON(t, map[string]interface{}{
					"id":          9104,
					"type":        "resolved",
					"tag":         "missing-service",
					"listen":      "::",
					"listen_port": 53,
				}))
			},
			count: func() (int64, error) {
				var count int64
				err := db.Model(&model.Service{}).Where("id = ?", 9104).Count(&count).Error
				return count, err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.save(); err == nil {
				t.Fatal("expected edit of a missing record to fail")
			}
			count, err := test.count()
			if err != nil {
				t.Fatalf("count persisted records: %v", err)
			}
			if count != 0 {
				t.Fatalf("missing record was unexpectedly created, count=%d", count)
			}
		})
	}
}

func TestSingboxSaveRejectsMissingTLSRecord(t *testing.T) {
	db := initOutboundEditMergeTestDB(t, "singbox-missing-tls-record.db")
	inboundPayload := mustMarshalJSON(t, map[string]interface{}{
		"type":        "direct",
		"tag":         "direct-missing-tls",
		"listen":      "::",
		"listen_port": 18080,
		"tls_id":      999,
	})
	if _, err := (&InboundService{}).Save(db, "new", inboundPayload, "", "panel.example.com"); err == nil {
		t.Fatal("inbound save accepted a missing TLS record")
	}

	servicePayload := mustMarshalJSON(t, map[string]interface{}{
		"type":        "resolved",
		"tag":         "resolved-missing-tls",
		"listen":      "::",
		"listen_port": 5353,
		"tls_id":      999,
	})
	if err := (&ServicesService{}).Save(db, "new", servicePayload); err == nil {
		t.Fatal("service save accepted a missing TLS record")
	}
}
