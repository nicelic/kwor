package service

import (
	"debug/buildinfo"
	"encoding/json"
	"strings"
)

const mihomoCoreDownloadPreferenceKey = "mihomoCoreDownloadPreference"

type MihomoCoreDownloadPreference struct {
	Target    MihomoCoreDownloadTarget `json:"target"`
	CustomURL string                   `json:"customUrl"`
}

func normalizeMihomoDownloadPreferenceTarget(target MihomoCoreDownloadTarget) MihomoCoreDownloadTarget {
	normalized := MihomoCoreDownloadTarget{
		OS:         strings.ToLower(strings.TrimSpace(target.OS)),
		Arch:       strings.ToLower(strings.TrimSpace(target.Arch)),
		Amd64Level: normalizeMihomoAMD64Level(target.Amd64Level),
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
	return normalized
}

func normalizeMihomoDownloadPreference(preference MihomoCoreDownloadPreference) MihomoCoreDownloadPreference {
	return MihomoCoreDownloadPreference{
		Target:    normalizeMihomoDownloadPreferenceTarget(preference.Target),
		CustomURL: strings.TrimSpace(preference.CustomURL),
	}
}

func getMihomoCoreDownloadPreference() (MihomoCoreDownloadPreference, error) {
	raw, err := (&SettingService{}).getString(mihomoCoreDownloadPreferenceKey)
	if err != nil {
		return MihomoCoreDownloadPreference{}, err
	}
	if strings.TrimSpace(raw) == "" {
		return MihomoCoreDownloadPreference{}, nil
	}

	var preference MihomoCoreDownloadPreference
	if err := json.Unmarshal([]byte(raw), &preference); err != nil {
		return MihomoCoreDownloadPreference{}, err
	}
	return normalizeMihomoDownloadPreference(preference), nil
}

func saveMihomoCoreDownloadPreference(preference MihomoCoreDownloadPreference) error {
	payload, err := json.Marshal(normalizeMihomoDownloadPreference(preference))
	if err != nil {
		return err
	}
	return (&SettingService{}).setString(mihomoCoreDownloadPreferenceKey, string(payload))
}

func updateMihomoCoreDownloadPreference(update func(*MihomoCoreDownloadPreference)) error {
	preference, err := getMihomoCoreDownloadPreference()
	if err != nil {
		return err
	}
	update(&preference)
	return saveMihomoCoreDownloadPreference(preference)
}

func inferMihomoTargetFromPlatform(platform string) MihomoCoreDownloadTarget {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(platform)), "/")
	if len(parts) != 2 {
		return MihomoCoreDownloadTarget{}
	}
	return normalizeMihomoDownloadPreferenceTarget(MihomoCoreDownloadTarget{
		OS:   parts[0],
		Arch: parts[1],
	})
}

func inferMihomoTargetFromGoBuildInfo(binPath string) MihomoCoreDownloadTarget {
	info, err := buildinfo.ReadFile(binPath)
	if err != nil || info == nil {
		return MihomoCoreDownloadTarget{}
	}
	target := MihomoCoreDownloadTarget{}
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
	return normalizeMihomoDownloadPreferenceTarget(target)
}

func mergeMihomoInstalledTargetWithPreference(installed MihomoCoreDownloadTarget, preferenceTarget MihomoCoreDownloadTarget) MihomoCoreDownloadTarget {
	installed = normalizeMihomoDownloadPreferenceTarget(installed)
	preferenceTarget = normalizeMihomoDownloadPreferenceTarget(preferenceTarget)
	if installed.OS == "" && installed.Arch == "" {
		return MihomoCoreDownloadTarget{}
	}
	if installed.Amd64Level == "" && installed.OS == preferenceTarget.OS && installed.Arch == "amd64" && preferenceTarget.Arch == "amd64" {
		installed.Amd64Level = preferenceTarget.Amd64Level
	}
	return installed
}

func (s *MihomoCoreManagerService) GetDownloadPreference() (MihomoCoreDownloadPreference, error) {
	return getMihomoCoreDownloadPreference()
}

func (s *MihomoCoreManagerService) SaveDownloadPreference(preference MihomoCoreDownloadPreference) error {
	return saveMihomoCoreDownloadPreference(preference)
}

func (s *MihomoCoreManagerService) SaveDownloadTarget(target MihomoCoreDownloadTarget) error {
	return updateMihomoCoreDownloadPreference(func(preference *MihomoCoreDownloadPreference) {
		preference.Target = target
	})
}

func (s *MihomoCoreManagerService) SaveCustomDownloadURL(downloadURL string) error {
	return updateMihomoCoreDownloadPreference(func(preference *MihomoCoreDownloadPreference) {
		preference.CustomURL = downloadURL
	})
}

func (s *MihomoCoreManagerService) normalizeStatusDownloadPreference(preference MihomoCoreDownloadPreference) MihomoCoreDownloadPreference {
	preference = normalizeMihomoDownloadPreference(preference)
	preference.Target = s.normalizeDownloadTarget(preference.Target)
	return preference
}
