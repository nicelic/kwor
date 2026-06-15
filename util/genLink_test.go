package util

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestLinkGenerator_TUICIncludesExtendedOutJSONFields(t *testing.T) {
	clientConfig := json.RawMessage(`{
  "tuic": {
    "uuid": "00000000-0000-0000-0000-000000000001",
    "password": "secret"
  }
}`)
	inbound := &model.Inbound{
		Type: "tuic",
		Tag:  "tuic-in",
		Addrs: json.RawMessage(`[
  {
    "server": "example.com",
    "server_port": 443,
    "remark": " node"
  }
]`),
		OutJson: json.RawMessage(`{
  "congestion_control": "bbr",
  "udp_relay_mode": "native",
  "request_timeout": "8000ms",
  "heartbeat": "10s",
  "max_open_streams": 20,
  "max_udp_relay_packet_size": 1400,
  "cwnd": 16,
  "ip": "1.1.1.1",
  "zero_rtt_handshake": true,
  "mihomo_fast_open": true,
  "udp_over_stream": true,
  "udp_over_stream_version": 2,
  "disable_mtu_discovery": true,
  "max_datagram_frame_size": 1200
}`),
	}

	links := LinkGenerator(clientConfig, inbound, "ignored.example.com")
	if len(links) != 1 {
		t.Fatalf("expected 1 TUIC link, got %d", len(links))
	}

	parsed, err := url.Parse(links[0])
	if err != nil {
		t.Fatalf("failed to parse generated TUIC link: %v", err)
	}
	query := parsed.Query()
	if got := query.Get("request_timeout"); got != "8000ms" {
		t.Fatalf("expected request_timeout=8000ms, got %q", got)
	}
	if got := query.Get("heartbeat"); got != "10s" {
		t.Fatalf("expected heartbeat=10s, got %q", got)
	}
	if got := query.Get("max_open_streams"); got != "20" {
		t.Fatalf("expected max_open_streams=20, got %q", got)
	}
	if got := query.Get("max_udp_relay_packet_size"); got != "1400" {
		t.Fatalf("expected max_udp_relay_packet_size=1400, got %q", got)
	}
	if got := query.Get("cwnd"); got != "16" {
		t.Fatalf("expected cwnd=16, got %q", got)
	}
	if got := query.Get("ip"); got != "1.1.1.1" {
		t.Fatalf("expected ip=1.1.1.1, got %q", got)
	}
	if got := query.Get("zero_rtt_handshake"); got != "1" {
		t.Fatalf("expected zero_rtt_handshake=1, got %q", got)
	}
	if got := query.Get("fast_open"); got != "1" {
		t.Fatalf("expected fast_open=1, got %q", got)
	}
	if got := query.Get("udp_over_stream"); got != "1" {
		t.Fatalf("expected udp_over_stream=1, got %q", got)
	}
	if got := query.Get("udp_over_stream_version"); got != "2" {
		t.Fatalf("expected udp_over_stream_version=2, got %q", got)
	}
	if got := query.Get("disable_mtu_discovery"); got != "1" {
		t.Fatalf("expected disable_mtu_discovery=1, got %q", got)
	}
	if got := query.Get("max_datagram_frame_size"); got != "1200" {
		t.Fatalf("expected max_datagram_frame_size=1200, got %q", got)
	}
}

func TestLinkGenerator_VLESSIncludesMihomoEncryptionQuery(t *testing.T) {
	clientConfig := json.RawMessage(`{
  "vless": {
    "uuid": "00000000-0000-0000-0000-000000000001"
  }
}`)
	inbound := &model.Inbound{
		Type: "vless",
		Tag:  "vless-in",
		Addrs: json.RawMessage(`[
  {
    "server": "example.com",
    "server_port": 443,
    "remark": " node"
  }
]`),
		Options: json.RawMessage(`{
  "listen": "::",
  "listen_port": 443,
  "transport": {
    "type": "ws",
    "path": "/ws"
  }
}`),
		OutJson: json.RawMessage(`{
  "encryption": "mlkem768x25519plus.random.0rtt.100-111-1111.75-0-111.50-0-3333.pass.client"
}`),
	}

	links := LinkGenerator(clientConfig, inbound, "ignored.example.com")
	if len(links) != 1 {
		t.Fatalf("expected 1 vless link, got %d", len(links))
	}

	parsed, err := url.Parse(links[0])
	if err != nil {
		t.Fatalf("failed to parse generated VLESS link: %v", err)
	}
	if got := parsed.Query().Get("encryption"); got != "mlkem768x25519plus.random.0rtt.100-111-1111.75-0-111.50-0-3333.pass.client" {
		t.Fatalf("expected encryption query to be preserved, got %q", got)
	}
}

func TestLinkGenerator_MieruSupportsSinglePortRangeOption(t *testing.T) {
	clientConfig := json.RawMessage(`{
  "mieru": {
    "username": "alice",
    "password": "secret"
  }
}`)
	inbound := &model.Inbound{
		Type: "mieru",
		Tag:  "mieru-in",
		Addrs: json.RawMessage(`[
  {
    "server": "panel.example.com",
    "server_port": 2999,
    "remark": " node"
  }
]`),
		Options: json.RawMessage(`{
  "listen": "::",
  "listen_port": 2999,
  "transport": "tcp",
  "port_range": "2090：2099"
}`),
		OutJson: json.RawMessage(`{
  "multiplexing": "MULTIPLEXING_LOW",
  "handshake_mode": "HANDSHAKE_STANDARD"
}`),
	}

	links := LinkGenerator(clientConfig, inbound, "ignored.example.com")
	if len(links) != 1 {
		t.Fatalf("expected 1 mieru link, got %d", len(links))
	}

	parsed, err := url.Parse(links[0])
	if err != nil {
		t.Fatalf("failed to parse generated mieru link: %v", err)
	}
	if got := parsed.Scheme; got != "mierus" {
		t.Fatalf("expected mierus scheme, got %q", got)
	}
	query := parsed.Query()
	if got := query.Get("port"); got != "2090-2099" {
		t.Fatalf("expected port=2090-2099, got %q", got)
	}
	if got := query.Get("protocol"); got != "TCP" {
		t.Fatalf("expected protocol=TCP, got %q", got)
	}
}

func TestLinkGenerator_HysteriaOmitsZeroBandwidthParams(t *testing.T) {
	clientConfig := json.RawMessage(`{
  "hysteria": {
    "auth_str": "secret"
  }
}`)
	inbound := &model.Inbound{
		Type: "hysteria",
		Tag:  "hy1-zero-bandwidth",
		Addrs: json.RawMessage(`[
  {
    "server": "example.com",
    "server_port": 443,
    "remark": " node"
  }
]`),
		OutJson: json.RawMessage(`{
  "up_mbps": 0,
  "down_mbps": 0
}`),
		Options: json.RawMessage(`{
  "listen": "::",
  "listen_port": 443,
  "server_up_mbps": 2000,
  "server_down_mbps": 2000
}`),
	}

	links := LinkGenerator(clientConfig, inbound, "ignored.example.com")
	if len(links) != 1 {
		t.Fatalf("expected 1 hysteria link, got %d", len(links))
	}
	if strings.Contains(links[0], "upmbps=") || strings.Contains(links[0], "downmbps=") {
		t.Fatalf("expected zero bandwidth params to be omitted, got %q", links[0])
	}
}

func TestLinkGenerator_Hysteria2OmitsZeroBandwidthParams(t *testing.T) {
	clientConfig := json.RawMessage(`{
  "hysteria2": {
    "password": "secret"
  }
}`)
	inbound := &model.Inbound{
		Type: "hysteria2",
		Tag:  "hy2-zero-bandwidth",
		Addrs: json.RawMessage(`[
  {
    "server": "example.com",
    "server_port": 443,
    "remark": " node"
  }
]`),
		OutJson: json.RawMessage(`{
  "up_mbps": 0,
  "down_mbps": 0
}`),
		Options: json.RawMessage(`{
  "listen": "::",
  "listen_port": 443,
  "server_up_mbps": 2000,
  "server_down_mbps": 2000
}`),
	}

	links := LinkGenerator(clientConfig, inbound, "ignored.example.com")
	if len(links) != 1 {
		t.Fatalf("expected 1 hysteria2 link, got %d", len(links))
	}
	if strings.Contains(links[0], "upmbps=") || strings.Contains(links[0], "downmbps=") {
		t.Fatalf("expected zero bandwidth params to be omitted, got %q", links[0])
	}
}

func TestLinkGenerator_SnellIncludesPSKVersionReuseAndObfs(t *testing.T) {
	clientConfig := json.RawMessage(`{
  "snell": {
    "psk": "secret-pass"
  }
}`)
	inbound := &model.Inbound{
		Type: "snell",
		Tag:  "snell-in",
		Addrs: json.RawMessage(`[
  {
    "server": "example.com",
    "server_port": 8443,
    "remark": " snell-node"
  }
]`),
		Options: json.RawMessage(`{
  "version": 5,
  "obfs_opts": {
    "mode": "tls",
    "host": "cdn.example.com"
  }
}`),
		OutJson: json.RawMessage(`{
  "version": 4,
  "reuse": true,
  "obfs_opts": {
    "mode": "tls",
    "host": "cdn.example.com"
  }
}`),
	}

	links := LinkGenerator(clientConfig, inbound, "ignored.example.com")
	if len(links) != 1 {
		t.Fatalf("expected 1 snell link, got %d", len(links))
	}

	parsed, err := url.Parse(links[0])
	if err != nil {
		t.Fatalf("failed to parse generated snell link: %v", err)
	}
	if got := parsed.Scheme; got != "snell" {
		t.Fatalf("expected snell scheme, got %q", got)
	}
	if parsed.User == nil {
		t.Fatalf("expected snell user info to contain psk")
	}
	if got := parsed.User.Username(); got != "secret-pass" {
		t.Fatalf("expected psk in user info, got %q", got)
	}
	query := parsed.Query()
	if got := query.Get("version"); got != "4" {
		t.Fatalf("expected version=4, got %q", got)
	}
	if got := query.Get("reuse"); got != "1" {
		t.Fatalf("expected reuse=1, got %q", got)
	}
	if got := query.Get("obfs"); got != "tls" {
		t.Fatalf("expected obfs=tls, got %q", got)
	}
	if got := query.Get("host"); got != "cdn.example.com" {
		t.Fatalf("expected host=cdn.example.com, got %q", got)
	}
}

func TestLinkGenerator_SnellKeepsClientVersionWhenServerAlsoUses5(t *testing.T) {
	clientConfig := json.RawMessage(`{
  "snell": {
    "psk": "secret-pass"
  }
}`)
	inbound := &model.Inbound{
		Type: "snell",
		Tag:  "snell-in",
		Addrs: json.RawMessage(`[
  {
    "server": "example.com",
    "server_port": 8443,
    "remark": "snell-node"
  }
]`),
		Options: json.RawMessage(`{
  "version": 4
}`),
		OutJson: json.RawMessage(`{
  "version": 5,
  "reuse": false
}`),
	}

	links := LinkGenerator(clientConfig, inbound, "ignored.example.com")
	if len(links) != 1 {
		t.Fatalf("expected 1 snell link, got %d", len(links))
	}

	parsed, err := url.Parse(links[0])
	if err != nil {
		t.Fatalf("failed to parse generated snell link: %v", err)
	}

	if got := parsed.Query().Get("version"); got != "5" {
		t.Fatalf("expected client version=5 to be preserved, got %q", got)
	}
}

func TestLinkGenerator_SnellEncodesPSKAsUserInfo(t *testing.T) {
	clientConfig := json.RawMessage(`{
  "snell": {
    "psk": "a b+c@d"
  }
}`)
	inbound := &model.Inbound{
		Type: "snell",
		Tag:  "snell-escaped-psk",
		Addrs: json.RawMessage(`[
  {
    "server": "example.com",
    "server_port": 443,
    "remark": "snell-node"
  }
]`),
	}

	links := LinkGenerator(clientConfig, inbound, "ignored.example.com")
	if len(links) != 1 {
		t.Fatalf("expected 1 snell link, got %d", len(links))
	}

	parsed, err := url.Parse(links[0])
	if err != nil {
		t.Fatalf("failed to parse generated snell link: %v", err)
	}
	if parsed.User == nil {
		t.Fatalf("expected snell user info to contain psk")
	}
	if got := parsed.User.Username(); got != "a b+c@d" {
		t.Fatalf("expected psk to round-trip through user info, got %q", got)
	}
}

func TestLinkGenerator_HysteriaUsesClientOutJsonBandwidthParams(t *testing.T) {
	clientConfig := json.RawMessage(`{
  "hysteria": {
    "auth_str": "secret"
  }
}`)
	inbound := &model.Inbound{
		Type: "hysteria",
		Tag:  "hy1-client-bandwidth",
		Addrs: json.RawMessage(`[
  {
    "server": "example.com",
    "server_port": 443,
    "remark": " node"
  }
]`),
		OutJson: json.RawMessage(`{
  "up_mbps": 111,
  "down_mbps": 222
}`),
		Options: json.RawMessage(`{
  "listen": "::",
  "listen_port": 443,
  "server_up_mbps": 2000,
  "server_down_mbps": 2000
}`),
	}

	links := LinkGenerator(clientConfig, inbound, "ignored.example.com")
	if len(links) != 1 {
		t.Fatalf("expected 1 hysteria link, got %d", len(links))
	}
	if !strings.Contains(links[0], "upmbps=111") || !strings.Contains(links[0], "downmbps=222") {
		t.Fatalf("expected client bandwidth params from out_json, got %q", links[0])
	}
}
