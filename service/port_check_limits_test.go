package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidatePortCheckRequestLimitsLargeRanges(t *testing.T) {
	err := ValidatePortCheckRequest(PortCheckRequest{
		UDPRanges: []PortRangeCheckItem{{Range: "20000-24100"}},
	})
	if err == nil || !strings.Contains(err.Error(), "too many ports") {
		t.Fatalf("expected range size rejection, got %v", err)
	}
}

func TestValidatePortCheckRequestKeepsInvalidRangeStructured(t *testing.T) {
	err := ValidatePortCheckRequest(PortCheckRequest{
		UDPRanges: []PortRangeCheckItem{{Range: "not-a-range"}},
	})
	if err != nil {
		t.Fatalf("invalid range should be returned as structured status, got %v", err)
	}
}

func TestReadProcPortsStreamsSocketTableLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tcp")
	content := strings.Join([]string{
		"  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode",
		"   0: 00000000:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000 0 0 1 1 0000000000000000 100 0 0 10 0",
		"   1: 00000000:2328 00000000:0000 01 00000000:00000000 00:00000000 00000000 0 0 2 1 0000000000000000 100 0 0 10 0",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write socket-table fixture failed: %v", err)
	}

	ports := make(map[int]struct{})
	if err := readProcPorts(path, ports, true); err != nil {
		t.Fatalf("read proc ports failed: %v", err)
	}
	if _, ok := ports[8080]; !ok {
		t.Fatalf("listening port 8080 was not collected: %#v", ports)
	}
	if _, ok := ports[9000]; ok {
		t.Fatalf("non-listening TCP port must not be collected: %#v", ports)
	}
}
