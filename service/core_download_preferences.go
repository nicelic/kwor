package service

import (
	"debug/buildinfo"
	"encoding/json"
	"strings"
)

const (
	coreDownloadPreferenceKey       = "coreDownloadPreference"
	mihomoCoreDownloadPreferenceKey = "mihomoCoreDownloadPreference"
)

type CoreDownloadPreference struct {
	Target    CoreDownloadTarget `json:"target"`
	CustomURL string             `json:"customUrl"`
}

func normalizeDownloadPreferenceTarget(target CoreDownloadTarget) CoreDownloadTarget {
	normalized := CoreDownloadTarget{
		OS:         strings.ToLower(strings.TrimSpace(target.OS)),
		Arch:       strings.ToLower(strings.TrimSpace(target.Arch)),
		Libc:       strings.ToLower(strings.TrimSpace(target.Libc)),
		Amd64Level: normalizeAmd64Level(target.Amd64Level),
	}
	switch normalized.OS {
	case "", "linux", "windows", "darwin", "freebsd":
	default:
		normalized.OS = ""
	}
	switch normalized.Arch {
	case "", "amd64", "arm64", "386", "armv7", "arm":
	default:
		normalized.Arch = ""
	}
	if normalized.Arch != "amd64" {
		normalized.Amd64Level = ""
	}
	if normalized.OS != "linux" {
		normalized.Libc = ""
	} else {
		switch normalized.Libc {
		case "", "glibc", "musl", "universal":
		default:
			normalized.Libc = ""
		}
	}
	return normalized
}

func normalizeDownloadPreference(preference CoreDownloadPreference) CoreDownloadPreference {
	return CoreDownloadPreference{
		Target:    normalizeDownloadPreferenceTarget(preference.Target),
		CustomURL: strings.TrimSpace(preference.CustomURL),
	}
}

func getCoreDownloadPreference(key string) (CoreDownloadPreference, error) {
	settingSvc := &SettingService{}
	raw, err := settingSvc.getString(key)
	if err != nil {
		return CoreDownloadPreference{}, err
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return CoreDownloadPreference{}, nil
	}

	var preference CoreDownloadPreference
	if err := json.Unmarshal([]byte(raw), &preference); err != nil {
		return CoreDownloadPreference{}, err
	}
	return normalizeDownloadPreference(preference), nil
}

func saveCoreDownloadPreference(key string, preference CoreDownloadPreference) error {
	preference = normalizeDownloadPreference(preference)
	payload, err := json.Marshal(preference)
	if err != nil {
		return err
	}
	return (&SettingService{}).setString(key, string(payload))
}

func updateCoreDownloadPreference(key string, update func(*CoreDownloadPreference)) error {
	preference, err := getCoreDownloadPreference(key)
	if err != nil {
		return err
	}
	update(&preference)
	return saveCoreDownloadPreference(key, preference)
}

func inferTargetFromPlatform(platform string) CoreDownloadTarget {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(platform)), "/")
	if len(parts) != 2 {
		return CoreDownloadTarget{}
	}
	target := CoreDownloadTarget{
		OS:   parts[0],
		Arch: parts[1],
	}
	return normalizeDownloadPreferenceTarget(target)
}

func inferTargetFromGoBuildInfo(binPath string) CoreDownloadTarget {
	info, err := buildinfo.ReadFile(binPath)
	if err != nil || info == nil {
		return CoreDownloadTarget{}
	}
	target := CoreDownloadTarget{}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "GOOS":
			target.OS = setting.Value
		case "GOARCH":
			target.Arch = setting.Value
		case "GOAMD64":
			target.Amd64Level = setting.Value
		}
	}
	return normalizeDownloadPreferenceTarget(target)
}

func mergeInstalledTargetWithPreference(installed CoreDownloadTarget, preferenceTarget CoreDownloadTarget) CoreDownloadTarget {
	installed = normalizeDownloadPreferenceTarget(installed)
	preferenceTarget = normalizeDownloadPreferenceTarget(preferenceTarget)
	if installed.OS == "" && installed.Arch == "" {
		return CoreDownloadTarget{}
	}
	if installed.Amd64Level == "" && installed.OS == preferenceTarget.OS && installed.Arch == "amd64" && preferenceTarget.Arch == "amd64" {
		installed.Amd64Level = preferenceTarget.Amd64Level
	}
	if installed.Libc == "" && installed.OS == preferenceTarget.OS && installed.Arch == preferenceTarget.Arch {
		installed.Libc = preferenceTarget.Libc
	}
	return installed
}

func (s *CoreManagerService) GetDownloadPreference() (CoreDownloadPreference, error) {
	return getCoreDownloadPreference(coreDownloadPreferenceKey)
}

func (s *CoreManagerService) SaveDownloadPreference(preference CoreDownloadPreference) error {
	return saveCoreDownloadPreference(coreDownloadPreferenceKey, preference)
}

func (s *CoreManagerService) SaveDownloadTarget(target CoreDownloadTarget) error {
	return updateCoreDownloadPreference(coreDownloadPreferenceKey, func(preference *CoreDownloadPreference) {
		preference.Target = target
	})
}

func (s *CoreManagerService) SaveCustomDownloadURL(downloadURL string) error {
	return updateCoreDownloadPreference(coreDownloadPreferenceKey, func(preference *CoreDownloadPreference) {
		preference.CustomURL = downloadURL
	})
}

func (s *MihomoCoreManagerService) GetDownloadPreference() (CoreDownloadPreference, error) {
	return getCoreDownloadPreference(mihomoCoreDownloadPreferenceKey)
}

func (s *MihomoCoreManagerService) SaveDownloadPreference(preference CoreDownloadPreference) error {
	return saveCoreDownloadPreference(mihomoCoreDownloadPreferenceKey, preference)
}

func (s *MihomoCoreManagerService) SaveDownloadTarget(target CoreDownloadTarget) error {
	return updateCoreDownloadPreference(mihomoCoreDownloadPreferenceKey, func(preference *CoreDownloadPreference) {
		preference.Target = target
	})
}

func (s *MihomoCoreManagerService) SaveCustomDownloadURL(downloadURL string) error {
	return updateCoreDownloadPreference(mihomoCoreDownloadPreferenceKey, func(preference *CoreDownloadPreference) {
		preference.CustomURL = downloadURL
	})
}
