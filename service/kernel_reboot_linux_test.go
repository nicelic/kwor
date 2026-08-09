//go:build linux

package service

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestNewKernelRebootDirectCommandCreatesNewSession(t *testing.T) {
	args := []string{kernelRebootWorkerSubcommand, KernelRebootConfirmedArgument}
	cmd := newKernelRebootDirectCommand("/opt/kwor/kwor_amd64", args)
	if cmd.Dir != filepath.Dir("/opt/kwor/kwor_amd64") {
		t.Fatalf("command directory = %q", cmd.Dir)
	}
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setsid {
		t.Fatal("direct kernel reboot worker must create a new Linux session")
	}
	if !reflect.DeepEqual(cmd.Args, append([]string{"/opt/kwor/kwor_amd64"}, args...)) {
		t.Fatalf("command args = %#v", cmd.Args)
	}

	foundInternalEnv := false
	for _, entry := range cmd.Env {
		if entry == InternalSystemdCommandEnv+"=1" {
			foundInternalEnv = true
			break
		}
	}
	if !foundInternalEnv {
		t.Fatalf("direct kernel reboot worker env = %#v, want %s=1", cmd.Env, InternalSystemdCommandEnv)
	}
}
