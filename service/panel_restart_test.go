package service

import (
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestPanelRestartSupportRejectsWindowsSnapshot(t *testing.T) {
	previous, _ := GetSystemPlatform()
	t.Cleanup(func() {
		setSystemPlatformSnapshot(previous)
	})
	setSystemPlatformSnapshot(&model.SystemPlatform{OS: "windows", Architecture: "amd64"})
	supported, hint := PanelRestartSupport()
	if supported || !strings.Contains(hint, "Windows") {
		t.Fatalf("windows restart support = (%t, %q), want disabled with hint", supported, hint)
	}
}
