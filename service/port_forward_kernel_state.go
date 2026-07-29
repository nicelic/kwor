package service

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"gorm.io/gorm"
)

const (
	portForwardIPv4ForwardPath = "/proc/sys/net/ipv4/ip_forward"
	portForwardIPv6ForwardPath = "/proc/sys/net/ipv6/conf/all/forwarding"
)

var (
	portForwardKernelHostFingerprintFn = readVnstatHostFingerprint
	portForwardKernelReadFileFn        = os.ReadFile
	portForwardKernelWriteFileFn       = os.WriteFile
	portForwardKernelOwnershipRecordFn = recordPortForwardKernelOwnership
)

// ensureKernelForwardingForRows enables only the families required by active
// remote forwarding rules. Before changing a disabled sysctl it persists the
// exact old value and host fingerprint, so a later deletion can restore only
// values that this module actually changed.
func ensureKernelForwardingForRows(rows []model.PortForwardRule) error {
	needIPv4 := false
	needIPv6 := false
	for _, row := range rows {
		if !row.Enabled || portForwardTargetIsLocal(row.TargetIP) {
			continue
		}
		flags := portForwardFamilyFlagsFor(row.Family)
		needIPv4 = needIPv4 || flags.ipv4
		needIPv6 = needIPv6 || flags.ipv6
	}
	if !needIPv4 && !needIPv6 {
		return nil
	}
	return ensurePortForwardKernelForwarding(needIPv4, needIPv6)
}

func ensurePortForwardKernelForwarding(needIPv4 bool, needIPv6 bool) error {
	if !needIPv4 && !needIPv6 {
		return nil
	}
	db := database.GetDB()
	if db == nil {
		return errors.New("database is not ready for forwarding sysctl state")
	}

	hostFingerprint, err := portForwardKernelHostFingerprintFn()
	if err != nil || strings.TrimSpace(hostFingerprint) == "" {
		if err == nil {
			err = errors.New("empty host fingerprint")
		}
		return fmt.Errorf("read forwarding host fingerprint: %w", err)
	}

	state, found, err := loadPortForwardKernelForwardState(db, hostFingerprint)
	if err != nil {
		return err
	}
	if !found {
		state = model.PortForwardKernelForwardState{
			Id:              1,
			HostFingerprint: strings.TrimSpace(hostFingerprint),
		}
	}

	if err := ensureOnePortForwardKernelForwarding(db, &state, needIPv4, false); err != nil {
		return err
	}
	if err := ensureOnePortForwardKernelForwarding(db, &state, needIPv6, true); err != nil {
		return err
	}
	return nil
}

func ensureOnePortForwardKernelForwarding(db *gorm.DB, state *model.PortForwardKernelForwardState, required bool, ipv6 bool) error {
	if !required || state == nil {
		return nil
	}
	path := portForwardIPv4ForwardPath
	modified := state.IPv4Modified
	if ipv6 {
		path = portForwardIPv6ForwardPath
		modified = state.IPv6Modified
	}

	current, err := portForwardKernelReadFileFn(path)
	if err != nil {
		return fmt.Errorf("read forwarding sysctl %s: %w", path, err)
	}
	if strings.TrimSpace(string(current)) == "1" {
		return nil
	}

	// Persist the baseline before changing the kernel. If the write fails, the
	// state is rolled back below, avoiding false ownership records.
	if !modified {
		if ipv6 {
			state.IPv6Modified = true
			state.IPv6Original = string(current)
		} else {
			state.IPv4Modified = true
			state.IPv4Original = string(current)
		}
		if err := savePortForwardKernelForwardState(db, *state); err != nil {
			return fmt.Errorf("save forwarding sysctl baseline: %w", err)
		}
	}
	if err := portForwardKernelOwnershipRecordFn(*state, false); err != nil {
		return fmt.Errorf("record forwarding sysctl ownership: %w", err)
	}

	if err := portForwardKernelWriteFileFn(path, []byte("1\n"), 0o644); err != nil {
		if !modified {
			if ipv6 {
				state.IPv6Modified = false
				state.IPv6Original = ""
			} else {
				state.IPv4Modified = false
				state.IPv4Original = ""
			}
			_ = savePortForwardKernelForwardState(db, *state)
		}
		_ = portForwardKernelOwnershipRecordFn(*state, true)
		return fmt.Errorf("enable forwarding sysctl %s: %w", path, err)
	}
	if err := portForwardKernelOwnershipRecordFn(*state, true); err != nil {
		return fmt.Errorf("activate forwarding sysctl ownership: %w", err)
	}
	return nil
}

// restorePortForwardKernelForwarding restores only sysctls recorded as changed
// by this module. A foreign host fingerprint causes the stale state to be
// discarded instead of changing the current host.
func restorePortForwardKernelForwarding() error {
	db := database.GetDB()
	if db == nil {
		return nil
	}
	// Most local-only forwarding layouts never create a baseline record. Check
	// that first so they do not depend on a host fingerprint being readable.
	state := model.PortForwardKernelForwardState{}
	err := db.Where("id = ?", 1).First(&state).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	hostFingerprint, err := portForwardKernelHostFingerprintFn()
	if err != nil || strings.TrimSpace(hostFingerprint) == "" {
		if err == nil {
			err = errors.New("empty host fingerprint")
		}
		return fmt.Errorf("read forwarding host fingerprint: %w", err)
	}

	if strings.TrimSpace(state.HostFingerprint) != strings.TrimSpace(hostFingerprint) {
		// A stale local record must never be applied to another host. The
		// lightweight exporter also excludes this table, but this is retained
		// as a defensive boundary for imports and manual database copies.
		if err := db.Delete(&state).Error; err != nil {
			return err
		}
		return portForwardKernelOwnershipRecordFn(model.PortForwardKernelForwardState{}, true)
	}

	if state.IPv4Modified {
		if err := restorePortForwardKernelValueIfUnchanged(portForwardIPv4ForwardPath, state.IPv4Original); err != nil {
			return fmt.Errorf("restore forwarding sysctl %s: %w", portForwardIPv4ForwardPath, err)
		}
	}
	if state.IPv6Modified {
		if err := restorePortForwardKernelValueIfUnchanged(portForwardIPv6ForwardPath, state.IPv6Original); err != nil {
			return fmt.Errorf("restore forwarding sysctl %s: %w", portForwardIPv6ForwardPath, err)
		}
	}
	if err := db.Delete(&state).Error; err != nil {
		return fmt.Errorf("clear forwarding sysctl baseline: %w", err)
	}
	if err := portForwardKernelOwnershipRecordFn(model.PortForwardKernelForwardState{}, true); err != nil {
		return fmt.Errorf("clear forwarding sysctl ownership: %w", err)
	}
	return nil
}

func restorePortForwardKernelValueIfUnchanged(path string, original string) error {
	current, err := portForwardKernelReadFileFn(path)
	if err != nil {
		return err
	}
	currentValue := strings.TrimSpace(string(current))
	originalValue := strings.TrimSpace(original)
	if currentValue == originalValue {
		return nil
	}
	// kwor only ever writes 1. Any other current value was changed after the
	// baseline was captured and must not be overwritten during cleanup.
	if currentValue != "1" {
		return fmt.Errorf("current value %q no longer matches kwor applied value 1", currentValue)
	}
	if err := portForwardKernelWriteFileFn(path, []byte(original), 0o644); err != nil {
		return err
	}
	actual, err := portForwardKernelReadFileFn(path)
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(actual)) != originalValue {
		return fmt.Errorf("value remains %q after restore", strings.TrimSpace(string(actual)))
	}
	return nil
}

func recordPortForwardKernelOwnership(state model.PortForwardKernelForwardState, activate bool) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	if !state.IPv4Modified && !state.IPv6Modified {
		return RemoveHostResource("kernel-forwarding")
	}
	resource, err := BeginHostResource(HostResource{
		ID:            "kernel-forwarding",
		Kind:          HostResourceKernelForward,
		CleanupPolicy: HostCleanupRestoreValue,
		Paths:         []string{portForwardIPv4ForwardPath, portForwardIPv6ForwardPath},
		Metadata: map[string]string{
			"hostFingerprint": state.HostFingerprint,
			"ipv4Modified":    strconv.FormatBool(state.IPv4Modified),
			"ipv4Original":    state.IPv4Original,
			"ipv6Modified":    strconv.FormatBool(state.IPv6Modified),
			"ipv6Original":    state.IPv6Original,
		},
	})
	if err != nil {
		return err
	}
	if !activate {
		return nil
	}
	return ActivateHostResource(resource.ID)
}

func loadPortForwardKernelForwardState(db *gorm.DB, hostFingerprint string) (model.PortForwardKernelForwardState, bool, error) {
	state := model.PortForwardKernelForwardState{}
	err := db.Where("id = ?", 1).First(&state).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return state, false, nil
	}
	if err != nil {
		return state, false, err
	}
	if strings.TrimSpace(state.HostFingerprint) != strings.TrimSpace(hostFingerprint) {
		// This is a local-only runtime record. Never apply its values on a
		// different host, including an imported full database.
		if err := db.Delete(&state).Error; err != nil {
			return model.PortForwardKernelForwardState{}, false, err
		}
		return model.PortForwardKernelForwardState{}, false, nil
	}
	return state, true, nil
}

func savePortForwardKernelForwardState(db *gorm.DB, state model.PortForwardKernelForwardState) error {
	state.Id = 1
	if state.IPv4Modified || state.IPv6Modified {
		return db.Save(&state).Error
	}
	var existing model.PortForwardKernelForwardState
	if err := db.Where("id = ?", 1).First(&existing).Error; err == nil {
		return db.Delete(&existing).Error
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return nil
}
