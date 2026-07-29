package service

import (
	"os"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func withPortForwardKernelStateFiles(t *testing.T, values map[string]string, host string) {
	t.Helper()
	previousHost := portForwardKernelHostFingerprintFn
	previousRead := portForwardKernelReadFileFn
	previousWrite := portForwardKernelWriteFileFn
	portForwardKernelHostFingerprintFn = func() (string, error) { return host, nil }
	portForwardKernelReadFileFn = func(path string) ([]byte, error) { return []byte(values[path]), nil }
	portForwardKernelWriteFileFn = func(path string, value []byte, _ os.FileMode) error {
		values[path] = string(value)
		return nil
	}
	t.Cleanup(func() {
		portForwardKernelHostFingerprintFn = previousHost
		portForwardKernelReadFileFn = previousRead
		portForwardKernelWriteFileFn = previousWrite
	})
}

func TestPortForwardKernelForwardStateRestoresOnlyModuleBaseline(t *testing.T) {
	openPortForwardMultiTestDB(t)
	values := map[string]string{
		portForwardIPv4ForwardPath: "0\n",
		portForwardIPv6ForwardPath: "1\n",
	}
	withPortForwardKernelStateFiles(t, values, "host-a")

	rows := []model.PortForwardRule{{Enabled: true, Family: portForwardFamilyIPv4, TargetIP: "198.51.100.8"}}
	if err := ensureKernelForwardingForRows(rows); err != nil {
		t.Fatalf("enable kernel forwarding: %v", err)
	}
	if values[portForwardIPv4ForwardPath] != "1\n" {
		t.Fatalf("ipv4 forwarding = %q", values[portForwardIPv4ForwardPath])
	}
	if values[portForwardIPv6ForwardPath] != "1\n" {
		t.Fatalf("ipv6 forwarding changed unexpectedly: %q", values[portForwardIPv6ForwardPath])
	}

	var state model.PortForwardKernelForwardState
	if err := database.GetDB().First(&state, 1).Error; err != nil {
		t.Fatalf("load saved baseline: %v", err)
	}
	if !state.IPv4Modified || state.IPv4Original != "0\n" || state.IPv6Modified {
		t.Fatalf("unexpected baseline state: %#v", state)
	}

	if err := restorePortForwardKernelForwarding(); err != nil {
		t.Fatalf("restore kernel forwarding: %v", err)
	}
	if values[portForwardIPv4ForwardPath] != "0\n" {
		t.Fatalf("ipv4 baseline was not restored: %q", values[portForwardIPv4ForwardPath])
	}
	var count int64
	if err := database.GetDB().Model(&model.PortForwardKernelForwardState{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("baseline state should be removed, count=%d err=%v", count, err)
	}
}

func TestPortForwardKernelForwardStateDropsForeignHostWithoutWritingSysctl(t *testing.T) {
	openPortForwardMultiTestDB(t)
	state := model.PortForwardKernelForwardState{
		Id:              1,
		HostFingerprint: "old-host",
		IPv4Modified:    true,
		IPv4Original:    "0\n",
	}
	if err := database.GetDB().Create(&state).Error; err != nil {
		t.Fatalf("create old host state: %v", err)
	}
	values := map[string]string{portForwardIPv4ForwardPath: "1\n", portForwardIPv6ForwardPath: "1\n"}
	withPortForwardKernelStateFiles(t, values, "new-host")

	if err := restorePortForwardKernelForwarding(); err != nil {
		t.Fatalf("restore foreign host state: %v", err)
	}
	if !strings.HasPrefix(values[portForwardIPv4ForwardPath], "1") {
		t.Fatalf("foreign state must not alter sysctl: %q", values[portForwardIPv4ForwardPath])
	}
	var count int64
	if err := database.GetDB().Model(&model.PortForwardKernelForwardState{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("foreign baseline should be removed, count=%d err=%v", count, err)
	}
}

func TestPortForwardKernelForwardStateDoesNotRequireFingerprintWithoutBaseline(t *testing.T) {
	openPortForwardMultiTestDB(t)
	previousHost := portForwardKernelHostFingerprintFn
	portForwardKernelHostFingerprintFn = func() (string, error) { return "", os.ErrNotExist }
	t.Cleanup(func() { portForwardKernelHostFingerprintFn = previousHost })

	if err := restorePortForwardKernelForwarding(); err != nil {
		t.Fatalf("restore without a module baseline should be a no-op: %v", err)
	}
}
