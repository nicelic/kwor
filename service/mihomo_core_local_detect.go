package service

import (
	"debug/buildinfo"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	mihomoCoreLocalTargetCacheTTL = 10 * time.Minute

	mihomoReleaseChannelStable = "stable"
	mihomoReleaseChannelAlpha  = "alpha"
)

type mihomoCoreLocalTargetCacheEntry struct {
	expiresAt  time.Time
	binModTime time.Time
	binSize    int64
	binMode    os.FileMode
	installed  bool
	compatible bool
	arch       string
	amd64Level string
	channel    string
}

var mihomoCoreLocalTargetCache = struct {
	sync.Mutex
	items map[string]mihomoCoreLocalTargetCacheEntry
}{
	items: make(map[string]mihomoCoreLocalTargetCacheEntry),
}

func getMihomoCoreLocalTargetCache(binPath string, binModTime time.Time, binSize int64, binMode os.FileMode) (mihomoCoreLocalTargetCacheEntry, bool) {
	now := time.Now()
	mihomoCoreLocalTargetCache.Lock()
	defer mihomoCoreLocalTargetCache.Unlock()

	entry, ok := mihomoCoreLocalTargetCache.items[binPath]
	if !ok {
		return mihomoCoreLocalTargetCacheEntry{}, false
	}
	if now.After(entry.expiresAt) {
		delete(mihomoCoreLocalTargetCache.items, binPath)
		return mihomoCoreLocalTargetCacheEntry{}, false
	}
	if !entry.binModTime.Equal(binModTime) || entry.binSize != binSize || entry.binMode != binMode {
		delete(mihomoCoreLocalTargetCache.items, binPath)
		return mihomoCoreLocalTargetCacheEntry{}, false
	}
	return entry, true
}

func setMihomoCoreLocalTargetCache(binPath string, binModTime time.Time, binSize int64, binMode os.FileMode, inspection managedCoreBinaryInspection) {
	channel := strings.TrimSpace(inspection.Channel)
	if channel == "" {
		channel = detectMihomoInstalledChannel(inspection.Version, inspection.VersionInfo)
	}
	mihomoCoreLocalTargetCache.Lock()
	defer mihomoCoreLocalTargetCache.Unlock()

	mihomoCoreLocalTargetCache.items[binPath] = mihomoCoreLocalTargetCacheEntry{
		expiresAt:  time.Now().Add(mihomoCoreLocalTargetCacheTTL),
		binModTime: binModTime,
		binSize:    binSize,
		binMode:    binMode,
		installed:  inspection.Installed,
		compatible: inspection.Compatible,
		arch:       inspection.Architecture,
		amd64Level: inspection.Amd64Level,
		channel:    channel,
	}
}

func clearMihomoCoreLocalTargetCache(binPath string) {
	mihomoCoreLocalTargetCache.Lock()
	defer mihomoCoreLocalTargetCache.Unlock()
	delete(mihomoCoreLocalTargetCache.items, binPath)
}

func normalizeMihomoVersionTag(value string) string {
	return normalizeAcmeVersionTag(value)
}

func detectMihomoInstalledChannelFromVersion(version string) string {
	normalized := strings.ToLower(normalizeMihomoVersionTag(strings.TrimSpace(version)))
	if normalized == "" {
		return ""
	}
	switch {
	case strings.Contains(normalized, "alpha"),
		strings.Contains(normalized, "beta"),
		strings.Contains(normalized, "rc"):
		return mihomoReleaseChannelAlpha
	default:
		return mihomoReleaseChannelStable
	}
}

func detectMihomoInstalledChannel(version string, versionInfo string) string {
	version = strings.TrimSpace(version)
	versionInfo = strings.TrimSpace(versionInfo)
	if version == "" || versionInfo == "" || !managedCoreVersionOutputMatches(versionInfo, "mihomo") {
		return ""
	}
	return detectMihomoInstalledChannelFromVersion(version)
}

func inferMihomoInstalledChannelFromBuildInfo(binPath string) string {
	info, err := buildinfo.ReadFile(binPath)
	if err != nil || info == nil {
		return ""
	}
	identityParts := []string{
		strings.ToLower(strings.TrimSpace(info.Path)),
		strings.ToLower(strings.TrimSpace(info.Main.Path)),
	}
	for _, part := range identityParts {
		if strings.Contains(part, "mihomo") {
			version := strings.TrimSpace(info.Main.Version)
			if version == "" || version == "(devel)" {
				return ""
			}
			return detectMihomoInstalledChannelFromVersion(version)
		}
	}
	return ""
}

func (s *MihomoCoreManagerService) inspectManagedLocalMihomoBinary(binPath string) managedCoreBinaryInspection {
	statInfo, err := os.Stat(binPath)
	if err != nil {
		return managedCoreBinaryInspection{}
	}
	binMode := statInfo.Mode().Perm()
	if cached, ok := getMihomoCoreLocalTargetCache(binPath, statInfo.ModTime(), statInfo.Size(), binMode); ok {
		inspection := managedCoreBinaryInspection{
			Installed:    cached.installed,
			Compatible:   cached.compatible,
			Architecture: cached.arch,
			Amd64Level:   cached.amd64Level,
			Channel:      cached.channel,
		}
		if cached.compatible {
			inspection.Version, inspection.VersionInfo = s.getCachedLocalVersion(binPath, statInfo, false)
			inspection.Compatible = inspection.Version != "" && managedCoreVersionOutputMatches(inspection.VersionInfo, "mihomo")
			inspection.Channel = detectMihomoInstalledChannel(inspection.Version, inspection.VersionInfo)
			if inspection.Channel == "" {
				inspection.Channel = inferMihomoInstalledChannelFromBuildInfo(binPath)
			}
			if inspection.Architecture == "amd64" && inspection.Amd64Level == "" {
				inspection.Amd64Level = inferMihomoInstalledAMD64Level(binPath, inspection.VersionInfo)
			}
		}
		return inspection
	}

	inspection := inspectManagedLinuxCoreBinary(binPath, "mihomo", func(statInfo os.FileInfo, forceRefresh bool) (string, string) {
		return s.getCachedLocalVersion(binPath, statInfo, forceRefresh)
	})
	if inspection.Architecture == "amd64" {
		inspection.Amd64Level = inferMihomoInstalledAMD64Level(binPath, inspection.VersionInfo)
	}
	inspection.Channel = detectMihomoInstalledChannel(inspection.Version, inspection.VersionInfo)
	if inspection.Channel == "" {
		inspection.Channel = inferMihomoInstalledChannelFromBuildInfo(binPath)
	}
	finalStat, finalErr := os.Stat(binPath)
	if finalErr == nil {
		setMihomoCoreLocalTargetCache(binPath, finalStat.ModTime(), finalStat.Size(), finalStat.Mode().Perm(), inspection)
	}
	return inspection
}
