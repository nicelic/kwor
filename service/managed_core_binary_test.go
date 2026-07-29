package service

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/alireza0/s-ui/database/model"
)

func withManagedCoreBinaryPlatform(t *testing.T, architecture string) {
	t.Helper()
	previous, previousErr := GetSystemPlatform()
	previousChmod := managedCoreBinaryChmod
	t.Cleanup(func() {
		managedCoreBinaryChmod = previousChmod
		if previousErr != nil || previous == nil {
			clearSystemPlatformSnapshot()
			return
		}
		setSystemPlatformSnapshot(previous)
	})
	setSystemPlatformSnapshot(&model.SystemPlatform{OS: "linux", Architecture: architecture})
}

func writeManagedCoreELF(t *testing.T, machine uint16, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "core")
	header := make([]byte, managedCoreELFHeaderSize)
	header[0] = 0x7f
	header[1] = 'E'
	header[2] = 'L'
	header[3] = 'F'
	header[4] = elfClass64
	header[5] = elfDataLittleEndian
	binary.LittleEndian.PutUint16(header[18:20], machine)
	if err := os.WriteFile(path, header, mode); err != nil {
		t.Fatalf("write ELF fixture: %v", err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("set ELF fixture mode: %v", err)
	}
	return path
}

func TestManagedCoreBinaryInspection(t *testing.T) {
	t.Run("amd64 and arm64 match their startup snapshots", func(t *testing.T) {
		for _, test := range []struct {
			name         string
			machine      uint16
			architecture string
		}{
			{name: "amd64", machine: elfMachineX8664, architecture: "amd64"},
			{name: "arm64", machine: elfMachineAArch64, architecture: "arm64"},
		} {
			t.Run(test.name, func(t *testing.T) {
				withManagedCoreBinaryPlatform(t, test.architecture)
				path := writeManagedCoreELF(t, test.machine, 0o755)
				probes := 0
				inspection := inspectManagedLinuxCoreBinary(path, "sing-box", func(os.FileInfo, bool) (string, string) {
					probes++
					return "1.12.0", "sing-box version 1.12.0"
				})
				if !inspection.Installed || !inspection.Compatible || inspection.Architecture != test.architecture {
					t.Fatalf("unexpected inspection: %+v", inspection)
				}
				if probes != 1 {
					t.Fatalf("version probe calls=%d, want 1", probes)
				}
			})
		}
	})

	t.Run("cross architecture is rejected before execution", func(t *testing.T) {
		withManagedCoreBinaryPlatform(t, "amd64")
		path := writeManagedCoreELF(t, elfMachineAArch64, 0o755)
		probes := 0
		inspection := inspectManagedLinuxCoreBinary(path, "sing-box", func(os.FileInfo, bool) (string, string) {
			probes++
			return "1.12.0", "sing-box version 1.12.0"
		})
		if !inspection.Installed || inspection.Compatible || inspection.Architecture != "arm64" {
			t.Fatalf("unexpected inspection: %+v", inspection)
		}
		if probes != 0 {
			t.Fatalf("cross-architecture binary was probed %d times", probes)
		}
	})

	t.Run("s390 and non ELF files are rejected before execution", func(t *testing.T) {
		withManagedCoreBinaryPlatform(t, "amd64")
		for _, test := range []struct {
			name string
			path func(t *testing.T) string
		}{
			{name: "s390", path: func(t *testing.T) string { return writeManagedCoreELF(t, 22, 0o755) }},
			{name: "not ELF", path: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "core")
				if err := os.WriteFile(path, []byte("not an ELF binary"), 0o755); err != nil {
					t.Fatalf("write non ELF fixture: %v", err)
				}
				return path
			}},
		} {
			t.Run(test.name, func(t *testing.T) {
				probes := 0
				inspection := inspectManagedLinuxCoreBinary(test.path(t), "mihomo", func(os.FileInfo, bool) (string, string) {
					probes++
					return "v1.19.0", "Mihomo Meta v1.19.0"
				})
				if !inspection.Installed || inspection.Compatible || inspection.Architecture != "" {
					t.Fatalf("unexpected inspection: %+v", inspection)
				}
				if probes != 0 {
					t.Fatalf("unsupported file was probed %d times", probes)
				}
			})
		}
	})

	t.Run("version and identity are both required", func(t *testing.T) {
		withManagedCoreBinaryPlatform(t, "amd64")
		path := writeManagedCoreELF(t, elfMachineX8664, 0o755)
		for _, test := range []struct {
			name        string
			version     string
			versionInfo string
		}{
			{name: "version missing", versionInfo: "sing-box version unknown"},
			{name: "wrong identity", version: "1.12.0", versionInfo: "another core version 1.12.0"},
		} {
			t.Run(test.name, func(t *testing.T) {
				inspection := inspectManagedLinuxCoreBinary(path, "sing-box", func(os.FileInfo, bool) (string, string) {
					return test.version, test.versionInfo
				})
				if !inspection.Installed || inspection.Compatible {
					t.Fatalf("unexpected inspection: %+v", inspection)
				}
			})
		}
	})

	t.Run("permission setup is required before probing", func(t *testing.T) {
		withManagedCoreBinaryPlatform(t, "amd64")
		path := writeManagedCoreELF(t, elfMachineX8664, 0o644)
		managedCoreBinaryChmod = func(string, os.FileMode) error { return errors.New("chmod denied") }
		probes := 0
		inspection := inspectManagedLinuxCoreBinary(path, "sing-box", func(os.FileInfo, bool) (string, string) {
			probes++
			return "1.12.0", "sing-box version 1.12.0"
		})
		if !inspection.Installed || inspection.Compatible || probes != 0 {
			t.Fatalf("unexpected inspection=%+v probes=%d", inspection, probes)
		}
	})

	t.Run("permission completion requests 0755", func(t *testing.T) {
		withManagedCoreBinaryPlatform(t, "amd64")
		path := writeManagedCoreELF(t, elfMachineX8664, 0o644)
		previousChmod := managedCoreBinaryChmod
		var requestedMode os.FileMode
		managedCoreBinaryChmod = func(path string, mode os.FileMode) error {
			requestedMode = mode
			return previousChmod(path, mode)
		}
		inspection := inspectManagedLinuxCoreBinary(path, "sing-box", func(_ os.FileInfo, forceRefresh bool) (string, string) {
			if !forceRefresh {
				t.Fatal("permission correction did not force a cache refresh")
			}
			return "1.12.0", "sing-box version 1.12.0"
		})
		if requestedMode != 0o755 {
			t.Fatalf("chmod mode=%#o, want 0755", requestedMode)
		}
		if runtime.GOOS != "windows" {
			if !inspection.Compatible {
				t.Fatalf("expected compatible inspection after chmod: %+v", inspection)
			}
			statInfo, err := os.Stat(path)
			if err != nil {
				t.Fatalf("stat after inspection: %v", err)
			}
			if statInfo.Mode().Perm() != 0o755 {
				t.Fatalf("mode after inspection=%v, want 0755", statInfo.Mode())
			}
		}
	})
}

func TestManagedCoreVersionCachesIncludePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "core")
	modTime := time.Now().Round(0)

	setCoreLocalVersionCache(path, modTime, 100, 0o755, "1.12.0", "sing-box version 1.12.0")
	defer clearCoreLocalVersionCache(path)
	if _, _, ok := getCoreLocalVersionCache(path, modTime, 100, 0o644); ok {
		t.Fatal("sing-box cache survived a permission change")
	}

	setMihomoLocalVersionCache(path, modTime, 100, 0o755, "v1.19.0", "Mihomo Meta v1.19.0")
	defer clearMihomoLocalVersionCache(path)
	if _, _, ok := getMihomoLocalVersionCache(path, modTime, 100, 0o644); ok {
		t.Fatal("Mihomo cache survived a permission change")
	}
}

func TestInferMihomoAMD64Level(t *testing.T) {
	if level := inferMihomoAMD64Level("v2", "Mihomo Meta goamd64=v3"); level != "v2" {
		t.Fatalf("GOAMD64 must win over version output, got %q", level)
	}
	if level := inferMihomoAMD64Level("", "Mihomo Meta goamd64=v3"); level != "v3" {
		t.Fatalf("version output fallback=%q, want v3", level)
	}
	if level := inferMihomoAMD64Level("", "Mihomo Meta v1.19.0"); level != "" {
		t.Fatalf("unrecognized AMD64 level=%q", level)
	}
}
