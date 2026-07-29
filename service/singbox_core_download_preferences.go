package service

import (
	"debug/buildinfo"
	"encoding/json"
	"strings"
)

const coreDownloadPreferenceKey = "coreDownloadPreference"

type SingboxCoreDownloadPreference struct {
	Target    SingboxCoreDownloadTarget `json:"target"`
	CustomURL string                    `json:"customUrl"`
}

func normalizeSingboxDownloadPreferenceTarget(target SingboxCoreDownloadTarget) SingboxCoreDownloadTarget {
	normalized := SingboxCoreDownloadTarget{
		OS:   strings.ToLower(strings.TrimSpace(target.OS)),
		Arch: strings.ToLower(strings.TrimSpace(target.Arch)),
		Libc: strings.ToLower(strings.TrimSpace(target.Libc)),
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

func normalizeSingboxDownloadPreference(preference SingboxCoreDownloadPreference) SingboxCoreDownloadPreference {
	return SingboxCoreDownloadPreference{
		Target:    normalizeSingboxDownloadPreferenceTarget(preference.Target),
		CustomURL: strings.TrimSpace(preference.CustomURL),
	}
}

func getSingboxCoreDownloadPreference() (SingboxCoreDownloadPreference, error) {
	raw, err := (&SettingService{}).getString(coreDownloadPreferenceKey)
	if err != nil {
		return SingboxCoreDownloadPreference{}, err
	}
	if strings.TrimSpace(raw) == "" {
		return SingboxCoreDownloadPreference{}, nil
	}

	var preference SingboxCoreDownloadPreference
	if err := json.Unmarshal([]byte(raw), &preference); err != nil {
		return SingboxCoreDownloadPreference{}, err
	}
	return normalizeSingboxDownloadPreference(preference), nil
}

func saveSingboxCoreDownloadPreference(preference SingboxCoreDownloadPreference) error {
	payload, err := json.Marshal(normalizeSingboxDownloadPreference(preference))
	if err != nil {
		return err
	}
	return (&SettingService{}).setString(coreDownloadPreferenceKey, string(payload))
}

func updateSingboxCoreDownloadPreference(update func(*SingboxCoreDownloadPreference)) error {
	preference, err := getSingboxCoreDownloadPreference()
	if err != nil {
		return err
	}
	update(&preference)
	return saveSingboxCoreDownloadPreference(preference)
}

func inferSingboxTargetFromPlatform(platform string) SingboxCoreDownloadTarget {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(platform)), "/")
	if len(parts) != 2 {
		return SingboxCoreDownloadTarget{}
	}
	return normalizeSingboxDownloadPreferenceTarget(SingboxCoreDownloadTarget{
		OS:   parts[0],
		Arch: parts[1],
	})
}

func inferSingboxTargetFromGoBuildInfo(binPath string) SingboxCoreDownloadTarget {
	info, err := buildinfo.ReadFile(binPath)
	if err != nil || info == nil {
		return SingboxCoreDownloadTarget{}
	}
	target := SingboxCoreDownloadTarget{}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "GOOS":
			target.OS = setting.Value
		case "GOARCH":
			target.Arch = setting.Value
		}
	}
	return normalizeSingboxDownloadPreferenceTarget(target)
}

func mergeSingboxInstalledTargetWithPreference(installed SingboxCoreDownloadTarget, preferenceTarget SingboxCoreDownloadTarget) SingboxCoreDownloadTarget {
	installed = normalizeSingboxDownloadPreferenceTarget(installed)
	preferenceTarget = normalizeSingboxDownloadPreferenceTarget(preferenceTarget)
	if installed.OS == "" && installed.Arch == "" {
		return SingboxCoreDownloadTarget{}
	}
	if installed.Libc == "" && installed.OS == preferenceTarget.OS && installed.Arch == preferenceTarget.Arch {
		installed.Libc = preferenceTarget.Libc
	}
	return installed
}

func migrateLegacySingboxCoreDownloadPreferenceJSON(raw string) (string, bool, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return raw, false, nil
	}

	var document map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &document); err != nil {
		return "", false, err
	}
	var target map[string]json.RawMessage
	if targetRaw, ok := document["target"]; ok {
		if err := json.Unmarshal(targetRaw, &target); err != nil {
			return "", false, err
		}
	}
	if _, exists := target["amd64Level"]; !exists {
		return raw, false, nil
	}

	var preference SingboxCoreDownloadPreference
	if err := json.Unmarshal([]byte(trimmed), &preference); err != nil {
		return "", false, err
	}
	payload, err := json.Marshal(normalizeSingboxDownloadPreference(preference))
	if err != nil {
		return "", false, err
	}
	return string(payload), true, nil
}

func MigrateLegacySingboxCoreDownloadPreference() (bool, error) {
	settingService := &SettingService{}
	raw, err := settingService.getString(coreDownloadPreferenceKey)
	if err != nil {
		return false, err
	}
	migrated, changed, err := migrateLegacySingboxCoreDownloadPreferenceJSON(raw)
	if err != nil || !changed {
		return false, err
	}
	if err := settingService.setString(coreDownloadPreferenceKey, migrated); err != nil {
		return false, err
	}
	return true, nil
}

func (s *CoreManagerService) GetDownloadPreference() (SingboxCoreDownloadPreference, error) {
	return getSingboxCoreDownloadPreference()
}

func (s *CoreManagerService) SaveDownloadPreference(preference SingboxCoreDownloadPreference) error {
	return saveSingboxCoreDownloadPreference(preference)
}

func (s *CoreManagerService) SaveDownloadTarget(target SingboxCoreDownloadTarget) error {
	return updateSingboxCoreDownloadPreference(func(preference *SingboxCoreDownloadPreference) {
		preference.Target = target
	})
}

func (s *CoreManagerService) SaveCustomDownloadURL(downloadURL string) error {
	return updateSingboxCoreDownloadPreference(func(preference *SingboxCoreDownloadPreference) {
		preference.CustomURL = downloadURL
	})
}
