package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestValidateMihomoInboundPayloadStrictPortAndTLSRules(t *testing.T) {
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
			name:    "missing port",
			payload: `{"type":"socks","tag":"socks-1"}`,
			wantErr: "listen_port is required",
		},
		{
			name:    "fractional number",
			payload: `{"type":"socks","tag":"socks-1","listen_port":443.5}`,
			wantErr: "complete decimal integer",
		},
		{
			name:    "fractional string",
			payload: `{"type":"socks","tag":"socks-1","listen_port":"443.5"}`,
			wantErr: "complete decimal integer",
		},
		{
			name:    "out of range",
			payload: `{"type":"socks","tag":"socks-1","listen_port":65536}`,
			wantErr: "between 1 and 65535",
		},
		{
			name:    "required TLS",
			payload: `{"type":"tuic","tag":"tuic-1","listen_port":443,"tls_id":0}`,
			wantErr: "requires a TLS configuration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var inbound model.MihomoInbound
			payload := json.RawMessage(tt.payload)
			if err := inbound.UnmarshalJSON(payload); err != nil {
				t.Fatalf("unmarshal payload: %v", err)
			}
			err := validateMihomoInboundPayload(payload, &inbound)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validate error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateMihomoInboundPayloadAllowsTunWithoutListenPort(t *testing.T) {
	payload := json.RawMessage(`{"type":"tun","tag":"tun-1","interface_name":"tun0"}`)
	var inbound model.MihomoInbound
	if err := inbound.UnmarshalJSON(payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if err := validateMihomoInboundPayload(payload, &inbound); err != nil {
		t.Fatalf("tun without listen_port should be valid: %v", err)
	}
}

func TestMihomoInboundRequiresTLSList(t *testing.T) {
	for _, inboundType := range []string{"anytls", "hysteria2", "trusttunnel", "tuic"} {
		if !mihomoInboundRequiresTLS(inboundType) {
			t.Fatalf("%s should require TLS", inboundType)
		}
	}
	for _, inboundType := range []string{"mixed", "socks", "http", "shadowsocks", "vmess", "vless", "trojan", "snell", "shadowquic", "mieru", "sudoku", "tun", "redirect", "tproxy"} {
		if mihomoInboundRequiresTLS(inboundType) {
			t.Fatalf("%s should not be unconditionally TLS-required", inboundType)
		}
	}
}
