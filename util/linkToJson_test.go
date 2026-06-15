package util

import (
	"reflect"
	"testing"
)

func extractServerPorts(t *testing.T, outbound map[string]interface{}) []string {
	t.Helper()

	raw, ok := outbound["server_ports"]
	if !ok {
		t.Fatalf("expected server_ports to exist")
	}

	switch ports := raw.(type) {
	case []string:
		return ports
	case []interface{}:
		result := make([]string, 0, len(ports))
		for _, p := range ports {
			s, ok := p.(string)
			if !ok {
				t.Fatalf("server_ports contains non-string item: %T", p)
			}
			result = append(result, s)
		}
		return result
	default:
		t.Fatalf("unexpected server_ports type: %T", raw)
	}

	return nil
}

func TestParsePortHopRangeNormalizesInput(t *testing.T) {
	got := ParsePortHopRange("41000-45000,  46000:47000，55100")
	want := []string{"41000:45000", "46000:47000", "55100"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParsePortHopRange mismatch: got=%v want=%v", got, want)
	}
}

func TestGetOutboundParsesHysteriaMPort(t *testing.T) {
	outbound, _, err := GetOutbound("hysteria://example.com:443?mport=41000-45000,46000:47000#node", 0)
	if err != nil {
		t.Fatalf("GetOutbound failed: %v", err)
	}

	serverPorts := extractServerPorts(t, *outbound)
	wantPorts := []string{"41000:45000", "46000:47000"}
	if !reflect.DeepEqual(serverPorts, wantPorts) {
		t.Fatalf("server_ports mismatch: got=%v want=%v", serverPorts, wantPorts)
	}

	gotPort, ok := (*outbound)["server_port"].(int)
	if !ok {
		t.Fatalf("expected server_port int, got %T", (*outbound)["server_port"])
	}
	if gotPort != 41000 {
		t.Fatalf("expected server_port=41000, got %d", gotPort)
	}
}

func TestGetOutboundParsesHysteria2MPort(t *testing.T) {
	outbound, _, err := GetOutbound("hysteria2://pass@example.com:443?mport=50000%2C50100-50200#node", 0)
	if err != nil {
		t.Fatalf("GetOutbound failed: %v", err)
	}

	serverPorts := extractServerPorts(t, *outbound)
	wantPorts := []string{"50000", "50100:50200"}
	if !reflect.DeepEqual(serverPorts, wantPorts) {
		t.Fatalf("server_ports mismatch: got=%v want=%v", serverPorts, wantPorts)
	}

	gotPort, ok := (*outbound)["server_port"].(int)
	if !ok {
		t.Fatalf("expected server_port int, got %T", (*outbound)["server_port"])
	}
	if gotPort != 50000 {
		t.Fatalf("expected server_port=50000, got %d", gotPort)
	}
}

func TestGetOutboundParsesMieruSimpleLink(t *testing.T) {
	outbound, tag, err := GetOutbound("mierus://alice:secret@example.com?profile=home&port=2090-2099&protocol=TCP&multiplexing=MULTIPLEXING_HIGH&handshake-mode=HANDSHAKE_NO_WAIT#node", 0)
	if err != nil {
		t.Fatalf("GetOutbound failed: %v", err)
	}

	if tag != "node" {
		t.Fatalf("expected tag=node, got %q", tag)
	}
	if got, _ := (*outbound)["type"].(string); got != "mieru" {
		t.Fatalf("expected type=mieru, got %v", (*outbound)["type"])
	}
	if got, _ := (*outbound)["server"].(string); got != "example.com" {
		t.Fatalf("expected server=example.com, got %v", (*outbound)["server"])
	}
	if got, _ := (*outbound)["port_range"].(string); got != "2090-2099" {
		t.Fatalf("expected port_range=2090-2099, got %v", (*outbound)["port_range"])
	}
	if got, _ := (*outbound)["transport"].(string); got != "TCP" {
		t.Fatalf("expected transport=TCP, got %v", (*outbound)["transport"])
	}
	if got, _ := (*outbound)["multiplexing"].(string); got != "MULTIPLEXING_HIGH" {
		t.Fatalf("expected multiplexing=MULTIPLEXING_HIGH, got %v", (*outbound)["multiplexing"])
	}
	if got, _ := (*outbound)["handshake_mode"].(string); got != "HANDSHAKE_NO_WAIT" {
		t.Fatalf("expected handshake_mode=HANDSHAKE_NO_WAIT, got %v", (*outbound)["handshake_mode"])
	}
}

func TestGetOutboundRejectsMieruSimpleLinkWithMultipleBindings(t *testing.T) {
	_, _, err := GetOutbound("mierus://alice:secret@example.com?profile=home&port=2999&port=2090-2099&protocol=TCP&protocol=TCP", 0)
	if err == nil {
		t.Fatalf("expected multi-binding mieru link to be rejected")
	}
}

func TestGetOutboundParsesExtendedTUICLink(t *testing.T) {
	outbound, tag, err := GetOutbound("tuic://00000000-0000-0000-0000-000000000001:secret@example.com:443?congestion_control=bbr&udp_relay_mode=native&request_timeout=8000ms&heartbeat=10s&max_open_streams=20&max_udp_relay_packet_size=1400&cwnd=16&ip=1.1.1.1&zero_rtt_handshake=1&fast_open=0&udp_over_stream=1&udp_over_stream_version=2&disable_mtu_discovery=1&max_datagram_frame_size=1200&sni=edge.example.com&alpn=h3#node", 0)
	if err != nil {
		t.Fatalf("GetOutbound failed: %v", err)
	}
	if tag != "node" {
		t.Fatalf("expected tag=node, got %q", tag)
	}
	if got, _ := (*outbound)["type"].(string); got != "tuic" {
		t.Fatalf("expected type=tuic, got %v", (*outbound)["type"])
	}
	if got, _ := (*outbound)["request_timeout"].(string); got != "8000ms" {
		t.Fatalf("expected request_timeout=8000ms, got %v", (*outbound)["request_timeout"])
	}
	if got, _ := (*outbound)["heartbeat"].(string); got != "10s" {
		t.Fatalf("expected heartbeat=10s, got %v", (*outbound)["heartbeat"])
	}
	if got, _ := (*outbound)["max_open_streams"].(int); got != 20 {
		t.Fatalf("expected max_open_streams=20, got %v", (*outbound)["max_open_streams"])
	}
	if got, _ := (*outbound)["max_udp_relay_packet_size"].(int); got != 1400 {
		t.Fatalf("expected max_udp_relay_packet_size=1400, got %v", (*outbound)["max_udp_relay_packet_size"])
	}
	if got, _ := (*outbound)["cwnd"].(int); got != 16 {
		t.Fatalf("expected cwnd=16, got %v", (*outbound)["cwnd"])
	}
	if got, _ := (*outbound)["ip"].(string); got != "1.1.1.1" {
		t.Fatalf("expected ip=1.1.1.1, got %v", (*outbound)["ip"])
	}
	if got, _ := (*outbound)["zero_rtt_handshake"].(bool); !got {
		t.Fatalf("expected zero_rtt_handshake=true, got %v", (*outbound)["zero_rtt_handshake"])
	}
	if got, _ := (*outbound)["mihomo_fast_open"].(bool); got {
		t.Fatalf("expected mihomo_fast_open=false, got %v", (*outbound)["mihomo_fast_open"])
	}
	if got, _ := (*outbound)["udp_over_stream"].(bool); !got {
		t.Fatalf("expected udp_over_stream=true, got %v", (*outbound)["udp_over_stream"])
	}
	if got, _ := (*outbound)["udp_over_stream_version"].(int); got != 2 {
		t.Fatalf("expected udp_over_stream_version=2, got %v", (*outbound)["udp_over_stream_version"])
	}
	if got, _ := (*outbound)["disable_mtu_discovery"].(bool); !got {
		t.Fatalf("expected disable_mtu_discovery=true, got %v", (*outbound)["disable_mtu_discovery"])
	}
	if got, _ := (*outbound)["max_datagram_frame_size"].(int); got != 1200 {
		t.Fatalf("expected max_datagram_frame_size=1200, got %v", (*outbound)["max_datagram_frame_size"])
	}
	tls, ok := (*outbound)["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected tls map, got %T", (*outbound)["tls"])
	}
	if got, _ := tls["server_name"].(string); got != "edge.example.com" {
		t.Fatalf("expected tls.server_name=edge.example.com, got %v", tls["server_name"])
	}
}

func TestGetOutboundParsesSnellLink(t *testing.T) {
	outbound, tag, err := GetOutbound("snell://secret-pass@example.com:8443?version=4&reuse=1&obfs=http&host=cdn.example.com#snell-node", 0)
	if err != nil {
		t.Fatalf("GetOutbound failed: %v", err)
	}

	if tag != "snell-node" {
		t.Fatalf("expected tag=snell-node, got %q", tag)
	}
	if got, _ := (*outbound)["type"].(string); got != "snell" {
		t.Fatalf("expected type=snell, got %v", (*outbound)["type"])
	}
	if got, _ := (*outbound)["server"].(string); got != "example.com" {
		t.Fatalf("expected server=example.com, got %v", (*outbound)["server"])
	}
	if got, _ := (*outbound)["server_port"].(int); got != 8443 {
		t.Fatalf("expected server_port=8443, got %v", (*outbound)["server_port"])
	}
	if got, _ := (*outbound)["psk"].(string); got != "secret-pass" {
		t.Fatalf("expected psk=secret-pass, got %v", (*outbound)["psk"])
	}
	if got, _ := (*outbound)["version"].(int); got != 4 {
		t.Fatalf("expected version=4, got %v", (*outbound)["version"])
	}
	if got, _ := (*outbound)["reuse"].(bool); !got {
		t.Fatalf("expected reuse=true, got %v", (*outbound)["reuse"])
	}

	obfsOpts, ok := (*outbound)["obfs_opts"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected obfs_opts map, got %T", (*outbound)["obfs_opts"])
	}
	if got, _ := obfsOpts["mode"].(string); got != "http" {
		t.Fatalf("expected obfs_opts.mode=http, got %v", obfsOpts["mode"])
	}
	if got, _ := obfsOpts["host"].(string); got != "cdn.example.com" {
		t.Fatalf("expected obfs_opts.host=cdn.example.com, got %v", obfsOpts["host"])
	}
}
