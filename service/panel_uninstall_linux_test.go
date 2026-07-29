//go:build linux

package service

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestNewPanelUninstallDirectCommandCreatesNewSession(t *testing.T) {
	args := panelUninstallCommandArgs()
	cmd := newPanelUninstallDirectCommand("/opt/kwor/kwor_amd64", args)
	if cmd.Dir != filepath.Dir("/opt/kwor/kwor_amd64") {
		t.Fatalf("command directory = %q", cmd.Dir)
	}
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setsid {
		t.Fatal("direct uninstall worker must create a new Linux session")
	}
	if !reflect.DeepEqual(cmd.Args, append([]string{"/opt/kwor/kwor_amd64"}, args...)) {
		t.Fatalf("command args = %#v", cmd.Args)
	}
}
