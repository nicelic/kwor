package service

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestCollectInboundLimitRanges_Hysteria2IncludesListenAndHopRange(t *testing.T) {
	inbound := &model.Inbound{
		Type: "hysteria2",
		Options: json.RawMessage(`{
  "listen_port": 31100,
  "port_hop_range": "21000-25000"
}`),
	}

	ranges := collectInboundLimitRanges(inbound)
	want := []portRange{
		{start: 21000, end: 25000},
		{start: 31100, end: 31100},
	}
	if !reflect.DeepEqual(ranges, want) {
		t.Fatalf("unexpected ranges: got=%#v want=%#v", ranges, want)
	}
}

func TestCollectInboundLimitRanges_MieruIncludesListenPortRangeAndBindings(t *testing.T) {
	t.Run("port_range", func(t *testing.T) {
		inbound := &model.Inbound{
			Type:    "mieru",
			Options: json.RawMessage(`{"listen_port":31100,"port_range":"21000-25000"}`),
		}

		ranges := collectInboundLimitRanges(inbound)
		want := []portRange{
			{start: 21000, end: 25000},
			{start: 31100, end: 31100},
		}
		if !reflect.DeepEqual(ranges, want) {
			t.Fatalf("unexpected ranges: got=%#v want=%#v", ranges, want)
		}
	})

	t.Run("port_bindings", func(t *testing.T) {
		inbound := &model.Inbound{
			Type:    "mieru",
			Options: json.RawMessage(`{"listen_port":31100,"port_bindings":"4000,5000-5002"}`),
		}

		ranges := collectInboundLimitRanges(inbound)
		want := []portRange{
			{start: 4000, end: 4000},
			{start: 5000, end: 5002},
			{start: 31100, end: 31100},
		}
		if !reflect.DeepEqual(ranges, want) {
			t.Fatalf("unexpected ranges: got=%#v want=%#v", ranges, want)
		}
	})
}

func TestCollectMihomoInboundLimitRanges_MieruAndHysteria2(t *testing.T) {
	t.Run("mieru from options", func(t *testing.T) {
		inbound := &model.MihomoInbound{
			Type:    "mieru",
			Options: json.RawMessage(`{"listen_port":31100,"port_range":"21000-25000"}`),
		}

		ranges := collectMihomoInboundLimitRanges(inbound)
		want := []portRange{
			{start: 21000, end: 25000},
			{start: 31100, end: 31100},
		}
		if !reflect.DeepEqual(ranges, want) {
			t.Fatalf("unexpected ranges: got=%#v want=%#v", ranges, want)
		}
	})

	t.Run("mieru from outjson", func(t *testing.T) {
		inbound := &model.MihomoInbound{
			Type:    "mieru",
			Options: json.RawMessage(`{"listen_port":31100}`),
			OutJson: json.RawMessage(`{"port_range":"21000-25000"}`),
		}

		ranges := collectMihomoInboundLimitRanges(inbound)
		want := []portRange{
			{start: 21000, end: 25000},
			{start: 31100, end: 31100},
		}
		if !reflect.DeepEqual(ranges, want) {
			t.Fatalf("unexpected ranges: got=%#v want=%#v", ranges, want)
		}
	})

	t.Run("hysteria2", func(t *testing.T) {
		inbound := &model.MihomoInbound{
			Type:    "hysteria2",
			Options: json.RawMessage(`{"listen_port":31100,"port_hop_range":"21000-25000"}`),
		}

		ranges := collectMihomoInboundLimitRanges(inbound)
		want := []portRange{
			{start: 21000, end: 25000},
			{start: 31100, end: 31100},
		}
		if !reflect.DeepEqual(ranges, want) {
			t.Fatalf("unexpected ranges: got=%#v want=%#v", ranges, want)
		}
	})
}

func TestExpandPortRangesToPorts(t *testing.T) {
	ports := expandPortRangesToPorts([]portRange{
		{start: 5002, end: 5000},
		{start: 4000, end: 4000},
		{start: 5001, end: 5003},
	})

	want := []int{4000, 5000, 5001, 5002, 5003}
	if !reflect.DeepEqual(ports, want) {
		t.Fatalf("unexpected ports: got=%v want=%v", ports, want)
	}
}
