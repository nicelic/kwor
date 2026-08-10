package service

import (
	"debug/buildinfo"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	singboxCoreLocalTargetCacheTTL = 10 * time.Minute

	singboxReleaseChannelStable = "stable"
	singboxReleaseChannelAlpha  = "alpha"
)

type singboxCoreLocalTargetCacheEntry struct {
	expiresAt  time.Time
	binModTime time.Time
	binSize    int64
	binMode    os.FileMode
	installed  bool
	compatible bool
	arch       string
	libc       string
	channel    string
}

var singboxCoreLocalTargetCache = struct {
	sync.Mutex
	items map[string]singboxCoreLocalTargetCacheEntry
}{
	items: make(map[string]singboxCoreLocalTargetCacheEntry),
}

func getSingboxCoreLocalTargetCache(binPath string, binModTime time.Time, binSize int64, binMode os.FileMode) (singboxCoreLocalTargetCacheEntry, bool) {
	now := time.Now()
	singboxCoreLocalTargetCache.Lock()
	defer singboxCoreLocalTargetCache.Unlock()

	entry, ok := singboxCoreLocalTargetCache.items[binPath]
	if !ok {
		return singboxCoreLocalTargetCacheEntry{}, false
	}
	if now.After(entry.expiresAt) {
		delete(singboxCoreLocalTargetCache.items, binPath)
		return singboxCoreLocalTargetCacheEntry{}, false
	}
	if !entry.binModTime.Equal(binModTime) || entry.binSize != binSize || entry.binMode != binMode {
		delete(singboxCoreLocalTargetCache.items, binPath)
		return singboxCoreLocalTargetCacheEntry{}, false
	}
	return entry, true
}

func setSingboxCoreLocalTargetCache(binPath string, binModTime time.Time, binSize int64, binMode os.FileMode, inspection managedCoreBinaryInspection) {
	channel := strings.TrimSpace(inspection.Channel)
	if channel == "" {
		channel = detectSingboxInstalledChannel(inspection.Version, inspection.VersionInfo)
	}
	singboxCoreLocalTargetCache.Lock()
	defer singboxCoreLocalTargetCache.Unlock()

	singboxCoreLocalTargetCache.items[binPath] = singboxCoreLocalTargetCacheEntry{
		expiresAt:  time.Now().Add(singboxCoreLocalTargetCacheTTL),
		binModTime: binModTime,
		binSize:    binSize,
		binMode:    binMode,
		installed:  inspection.Installed,
		compatible: inspection.Compatible,
		arch:       inspection.Architecture,
		libc:       inspection.Libc,
		channel:    channel,
	}
}

func clearSingboxCoreLocalTargetCache(binPath string) {
	singboxCoreLocalTargetCache.Lock()
	defer singboxCoreLocalTargetCache.Unlock()
	delete(singboxCoreLocalTargetCache.items, binPath)
}

func singboxManagedIdentityMatches(versionInfo string) bool {
	for _, field := range strings.Fields(strings.ToLower(versionInfo)) {
		field = strings.Trim(field, "\"'()[]{}:,;")
		if field == "sing-box" {
			return true
		}
	}
	return false
}

func normalizeSingboxVersionTag(value string) string {
	return normalizeAcmeVersionTag(value)
}

func isLikelySingboxVersionTag(value string) bool {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return false
	}
	if normalized[0] == 'v' || normalized[0] == 'V' {
		normalized = strings.TrimSpace(normalized[1:])
	}
	if normalized == "" || normalized[0] < '0' || normalized[0] > '9' {
		return false
	}
	return isLikelySemverTag(value)
}

// extractSingboxVersionFromOutput only accepts the version attached to a
// sing-box version line. This avoids accidentally treating later build
// metadata, such as a Go toolchain version, as the Core version.
func extractSingboxVersionFromOutput(output string) string {
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToLower(line), "sing-box ") {
			continue
		}
		fields := strings.Fields(line)
		for index, field := range fields {
			if !strings.EqualFold(strings.Trim(field, "\"'()[]{}"), "version") || index+1 >= len(fields) {
				continue
			}
			candidate := strings.Trim(fields[index+1], "\"'(),;[]{}")
			if !isLikelySingboxVersionTag(candidate) {
				continue
			}
			return candidate
		}
	}
	return ""
}

func detectSingboxInstalledChannelFromVersion(version string) string {
	normalized := strings.ToLower(normalizeSingboxVersionTag(strings.TrimSpace(version)))
	if !isLikelySingboxVersionTag(normalized) {
		return ""
	}
	switch {
	case strings.Contains(normalized, "alpha"),
		strings.Contains(normalized, "beta"),
		strings.Contains(normalized, "rc"),
		isSingboxPrereleaseVersion(normalized):
		return singboxReleaseChannelAlpha
	default:
		return singboxReleaseChannelStable
	}
}

func isSingboxPrereleaseVersion(version string) bool {
	version = strings.TrimSpace(strings.SplitN(version, "+", 2)[0])
	if !isLikelySemverTag(version) {
		return false
	}
	return strings.Contains(version, "-")
}

func detectSingboxInstalledChannel(version string, versionInfo string) string {
	version = strings.TrimSpace(version)
	versionInfo = strings.TrimSpace(versionInfo)
	if version == "" || versionInfo == "" || !singboxManagedIdentityMatches(versionInfo) {
		return ""
	}
	return detectSingboxInstalledChannelFromVersion(version)
}

func inferSingboxInstalledChannelFromBuildInfo(binPath string) string {
	info, err := buildinfo.ReadFile(binPath)
	if err != nil || info == nil {
		return ""
	}
	identityParts := []string{
		strings.ToLower(strings.TrimSpace(info.Path)),
		strings.ToLower(strings.TrimSpace(info.Main.Path)),
	}
	for _, part := range identityParts {
		if strings.Contains(part, "sing-box") {
			version := strings.TrimSpace(info.Main.Version)
			if version == "" || version == "(devel)" {
				return ""
			}
			return detectSingboxInstalledChannelFromVersion(version)
		}
	}
	return ""
}

func (s *CoreManagerService) inspectManagedLocalSingboxBinary(binPath string) managedCoreBinaryInspection {
	statInfo, err := os.Stat(binPath)
	if err != nil {
		return managedCoreBinaryInspection{}
	}
	binMode := statInfo.Mode().Perm()
	if cached, ok := getSingboxCoreLocalTargetCache(binPath, statInfo.ModTime(), statInfo.Size(), binMode); ok {
		inspection := managedCoreBinaryInspection{
			Installed:    cached.installed,
			Compatible:   cached.compatible,
			Architecture: cached.arch,
			Libc:         cached.libc,
			Channel:      cached.channel,
		}
		if cached.compatible {
			inspection.Version, inspection.VersionInfo = s.getCachedLocalVersion(binPath, statInfo, false)
			inspection.Compatible = inspection.Version != "" && singboxManagedIdentityMatches(inspection.VersionInfo)
			if channel := detectSingboxInstalledChannel(inspection.Version, inspection.VersionInfo); channel != "" {
				inspection.Channel = channel
			}
			if inspection.Channel == "" {
				inspection.Channel = inferSingboxInstalledChannelFromBuildInfo(binPath)
			}
		}
		return inspection
	}

	inspection := inspectManagedLinuxCoreBinary(binPath, "sing-box", func(statInfo os.FileInfo, forceRefresh bool) (string, string) {
		return s.getCachedLocalVersion(binPath, statInfo, forceRefresh)
	})
	inspection.Compatible = inspection.Compatible && singboxManagedIdentityMatches(inspection.VersionInfo)
	inspection.Channel = detectSingboxInstalledChannel(inspection.Version, inspection.VersionInfo)
	if inspection.Channel == "" {
		inspection.Channel = inferSingboxInstalledChannelFromBuildInfo(binPath)
	}
	finalStat, finalErr := os.Stat(binPath)
	if finalErr == nil {
		setSingboxCoreLocalTargetCache(binPath, finalStat.ModTime(), finalStat.Size(), finalStat.Mode().Perm(), inspection)
		s.refreshManagedCoreOwnershipIfPresent(binPath)
	}
	return inspection
}
