package api

import (
	"reflect"
	"testing"

	"github.com/alireza0/s-ui/service"
)

func TestParsePortCheckRequestBodyJSON(t *testing.T) {
	raw := []byte(`{"single_ports":[4458,4458,70000],"udp_ranges":[{"id":" 1 ","tag":" hy1 ","range":" 31100:32200 "}]} `)

	req, err := parsePortCheckRequestBody(raw)
	if err != nil {
		t.Fatalf("parse json request failed: %v", err)
	}

	if !reflect.DeepEqual(req.SinglePorts, []int{4458}) {
		t.Fatalf("unexpected single ports: %#v", req.SinglePorts)
	}
	if len(req.UDPRanges) != 1 {
		t.Fatalf("unexpected udp ranges count: %d", len(req.UDPRanges))
	}
	expected := service.PortRangeCheckItem{
		ID:    "1",
		Tag:   "hy1",
		Range: "31100:32200",
	}
	if !reflect.DeepEqual(req.UDPRanges[0], expected) {
		t.Fatalf("unexpected udp range: %#v", req.UDPRanges[0])
	}
}

func TestParsePortCheckRequestBodyFormIndexed(t *testing.T) {
	raw := []byte("single_ports%5B%5D=4458&udp_ranges%5B0%5D%5Bid%5D=1&udp_ranges%5B0%5D%5Btag%5D=hy1&udp_ranges%5B0%5D%5Brange%5D=31100%3A32200")

	req, err := parsePortCheckRequestBody(raw)
	if err != nil {
		t.Fatalf("parse form request failed: %v", err)
	}

	if !reflect.DeepEqual(req.SinglePorts, []int{4458}) {
		t.Fatalf("unexpected single ports: %#v", req.SinglePorts)
	}
	if len(req.UDPRanges) != 1 {
		t.Fatalf("unexpected udp ranges count: %d", len(req.UDPRanges))
	}
	expected := service.PortRangeCheckItem{
		ID:    "1",
		Tag:   "hy1",
		Range: "31100:32200",
	}
	if !reflect.DeepEqual(req.UDPRanges[0], expected) {
		t.Fatalf("unexpected udp range: %#v", req.UDPRanges[0])
	}
}

func TestParsePortCheckRequestBodyFormNoIndex(t *testing.T) {
	raw := []byte("single_ports%5B0%5D=4458&udp_ranges%5B%5D%5Bid%5D=1&udp_ranges%5B%5D%5Btag%5D=hy1&udp_ranges%5B%5D%5Brange%5D=31100%3A32200")

	req, err := parsePortCheckRequestBody(raw)
	if err != nil {
		t.Fatalf("parse no-index form request failed: %v", err)
	}

	if !reflect.DeepEqual(req.SinglePorts, []int{4458}) {
		t.Fatalf("unexpected single ports: %#v", req.SinglePorts)
	}
	if len(req.UDPRanges) != 1 {
		t.Fatalf("unexpected udp ranges count: %d", len(req.UDPRanges))
	}
	expected := service.PortRangeCheckItem{
		ID:    "1",
		Tag:   "hy1",
		Range: "31100:32200",
	}
	if !reflect.DeepEqual(req.UDPRanges[0], expected) {
		t.Fatalf("unexpected udp range: %#v", req.UDPRanges[0])
	}
}

func TestParsePortCheckRequestBodyInvalid(t *testing.T) {
	_, err := parsePortCheckRequestBody([]byte("not_a_valid_payload"))
	if err == nil {
		t.Fatal("expected parse error for invalid payload")
	}
}
