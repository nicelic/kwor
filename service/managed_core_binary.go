package service

import (
	"debug/elf"
	"encoding/binary"
	"io"
	"os"
	"strings"
)

const (
	managedCoreELFHeaderSize = 20
	elfClass64               = 2
	elfDataLittleEndian      = 1
	elfDataBigEndian         = 2
	elfMachineX8664          = 62
	elfMachineAArch64        = 183
)

type managedCoreBinaryInspection struct {
	Installed    bool
	Compatible   bool
	Architecture string
	Libc         string
	Amd64Level   string
	Channel      string
	Version      string
	VersionInfo  string
}

type managedCoreBinaryVersionProbe func(os.FileInfo, bool) (string, string)

var managedCoreBinaryChmod = os.Chmod

// inspectManagedLinuxCoreBinary only considers the exact binary path supplied
// by the caller. It never expands archives or searches neighboring filenames.
func inspectManagedLinuxCoreBinary(binPath string, expectedIdentity string, probe managedCoreBinaryVersionProbe) managedCoreBinaryInspection {
	statInfo, err := os.Stat(binPath)
	if err != nil {
		return managedCoreBinaryInspection{}
	}

	inspection := managedCoreBinaryInspection{Installed: true}
	if !statInfo.Mode().IsRegular() {
		return inspection
	}

	architecture, ok := readManagedLinuxELFArchitecture(binPath)
	if !ok {
		return inspection
	}
	inspection.Architecture = architecture
	inspection.Libc = detectManagedLinuxELFLibc(binPath)
	if architecture != GetSystemPlatformArchitecture() {
		return inspection
	}

	permissionsChanged := statInfo.Mode().Perm() != 0o755
	if permissionsChanged {
		if err := managedCoreBinaryChmod(binPath, 0o755); err != nil {
			return inspection
		}
		statInfo, err = os.Stat(binPath)
		if err != nil || !statInfo.Mode().IsRegular() {
			return inspection
		}
	}

	if probe == nil {
		return inspection
	}
	inspection.Version, inspection.VersionInfo = probe(statInfo, permissionsChanged)
	inspection.Compatible = inspection.Version != "" && managedCoreVersionOutputMatches(inspection.VersionInfo, expectedIdentity)
	return inspection
}

func managedCoreVersionOutputMatches(versionInfo string, expectedIdentity string) bool {
	identity := strings.ToLower(strings.TrimSpace(expectedIdentity))
	return identity != "" && strings.Contains(strings.ToLower(versionInfo), identity)
}

func readManagedLinuxELFArchitecture(binPath string) (string, bool) {
	file, err := os.Open(binPath)
	if err != nil {
		return "", false
	}
	defer file.Close()

	var header [managedCoreELFHeaderSize]byte
	if _, err := io.ReadFull(file, header[:]); err != nil {
		return "", false
	}
	if header[0] != 0x7f || header[1] != 'E' || header[2] != 'L' || header[3] != 'F' || header[4] != elfClass64 {
		return "", false
	}

	var machine uint16
	switch header[5] {
	case elfDataLittleEndian:
		machine = binary.LittleEndian.Uint16(header[18:20])
	case elfDataBigEndian:
		machine = binary.BigEndian.Uint16(header[18:20])
	default:
		return "", false
	}

	switch machine {
	case elfMachineX8664:
		return "amd64", true
	case elfMachineAArch64:
		return "arm64", true
	default:
		return "", false
	}
}

func detectManagedLinuxELFLibc(binPath string) string {
	file, err := elf.Open(binPath)
	if err != nil {
		return ""
	}
	defer file.Close()

	for _, prog := range file.Progs {
		if prog == nil || prog.Type != elf.PT_INTERP || prog.Filesz == 0 {
			continue
		}
		data := make([]byte, prog.Filesz)
		if _, err := prog.ReadAt(data, 0); err != nil {
			return ""
		}
		interpreter := strings.TrimRight(string(data), "\x00")
		if interpreter == "" {
			return ""
		}
		lower := strings.ToLower(interpreter)
		switch {
		case strings.Contains(lower, "musl"):
			return "musl"
		case strings.Contains(lower, "ld-linux"), strings.Contains(lower, "glibc"), strings.Contains(lower, "gnu"):
			return "glibc"
		default:
			return ""
		}
	}

	return "universal"
}
