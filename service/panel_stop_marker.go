package service

import (
	"os"
	"path/filepath"

	"github.com/alireza0/s-ui/config"
)

const panelStopOnlyMarkerFileName = "panel-stop-only.flag"

func panelStopOnlyMarkerPath() string {
	return filepath.Join(config.GetDataDir(), "runtime", panelStopOnlyMarkerFileName)
}

func MarkPanelStopOnly() error {
	markerPath := panelStopOnlyMarkerPath()
	if err := os.MkdirAll(filepath.Dir(markerPath), 0o740); err != nil {
		return err
	}
	return os.WriteFile(markerPath, []byte("1"), 0o640)
}

func ConsumePanelStopOnlyMarker() bool {
	markerPath := panelStopOnlyMarkerPath()
	if _, err := os.Stat(markerPath); err != nil {
		return false
	}
	_ = os.Remove(markerPath)
	return true
}

func ClearPanelStopOnlyMarker() {
	_ = os.Remove(panelStopOnlyMarkerPath())
}
