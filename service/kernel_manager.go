package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/config"
	"github.com/alireza0/s-ui/database"
)

var (
	xanmodSourceForgeBaseURL = "https://sourceforge.net/projects/xanmod/files"
	kernelSupportedLines     = map[string]struct{}{
		"lts":  {},
		"main": {},
		"rt":   {},
		"edge": {},
	}
	kernelSupportedArches = map[string]struct{}{
		"x64v1": {},
		"x64v2": {},
		"x64v3": {},
	}
	kernelPackagePattern    = regexp.MustCompile(`^.*linux-(image|headers)-.*_(amd64|arm64)\.deb$`)
	kernelArchPattern       = regexp.MustCompile(`(^|-)x64v[123](-|$)`)
	kernelVersionDigits     = regexp.MustCompile(`\d+`)
	kernelAptNamePattern    = regexp.MustCompile(`^[A-Za-z0-9.+:_-]+$`)
	kernelDownloadIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{8,128}$`)
)

type KernelManagerService struct{}

const (
	kernelProviderXanMod                   = "xanmod"
	kernelProviderBBRPlus                  = "bbrplus"
	kernelFailedCleanupDirName             = ".failed-cleanup"
	kernelDownloadProgressTTL              = 30 * time.Minute
	kernelUnknownPackageSizeEstimate int64 = 50 * 1024 * 1024
)

var kernelFailedDownloadCleanupDelay = 5 * time.Second

var kernelDownloadProgressStore = newKernelDownloadProgressStore()
var kernelDownloadCleanupStore = newKernelDownloadCleanupScheduler()

var bbrplusVersionDisplayPriority = map[string]int{
	"6.1.81-bbrplus": 0,
}

type bbrplusReleaseAsset struct {
	Arch string
	Name string
	Type string
}

type bbrplusReleaseEntry struct {
	Version string
	Assets  []bbrplusReleaseAsset
}

var bbrplusReleaseCatalog = []bbrplusReleaseEntry{
	{
		Version: "6.7.9-bbrplus",
		Assets: []bbrplusReleaseAsset{
			{Type: "headers", Arch: "amd64", Name: "Debian-Ubuntu_Optional_linux-headers-6.7.9-bbrplus_6.7.9-bbrplus-1_amd64.deb"},
			{Type: "headers", Arch: "arm64", Name: "Debian-Ubuntu_Optional_linux-headers-6.7.9-bbrplus_6.7.9-bbrplus-1_arm64.deb"},
			{Type: "image", Arch: "amd64", Name: "Debian-Ubuntu_Required_linux-image-6.7.9-bbrplus_6.7.9-bbrplus-1_amd64.deb"},
			{Type: "image", Arch: "arm64", Name: "Debian-Ubuntu_Required_linux-image-6.7.9-bbrplus_6.7.9-bbrplus-1_arm64.deb"},
		},
	},
	{
		Version: "6.6.21-bbrplus",
		Assets: []bbrplusReleaseAsset{
			{Type: "headers", Arch: "amd64", Name: "Debian-Ubuntu_Optional_linux-headers-6.6.21-bbrplus_6.6.21-bbrplus-1_amd64.deb"},
			{Type: "headers", Arch: "arm64", Name: "Debian-Ubuntu_Optional_linux-headers-6.6.21-bbrplus_6.6.21-bbrplus-1_arm64.deb"},
			{Type: "image", Arch: "amd64", Name: "Debian-Ubuntu_Required_linux-image-6.6.21-bbrplus_6.6.21-bbrplus-1_amd64.deb"},
			{Type: "image", Arch: "arm64", Name: "Debian-Ubuntu_Required_linux-image-6.6.21-bbrplus_6.6.21-bbrplus-1_arm64.deb"},
		},
	},
	{
		Version: "6.5.13-bbrplus",
		Assets: []bbrplusReleaseAsset{
			{Type: "headers", Arch: "amd64", Name: "Debian-Ubuntu_Optional_linux-headers-6.5.13-bbrplus_6.5.13-bbrplus-1_amd64.deb"},
			{Type: "headers", Arch: "arm64", Name: "Debian-Ubuntu_Optional_linux-headers-6.5.13-bbrplus_6.5.13-bbrplus-1_arm64.deb"},
			{Type: "image", Arch: "amd64", Name: "Debian-Ubuntu_Required_linux-image-6.5.13-bbrplus_6.5.13-bbrplus-1_amd64.deb"},
			{Type: "image", Arch: "arm64", Name: "Debian-Ubuntu_Required_linux-image-6.5.13-bbrplus_6.5.13-bbrplus-1_arm64.deb"},
		},
	},
	{
		Version: "6.4.16-bbrplus",
		Assets: []bbrplusReleaseAsset{
			{Type: "headers", Arch: "amd64", Name: "Debian-Ubuntu_Optional_linux-headers-6.4.16-bbrplus_6.4.16-1_amd64.deb"},
			{Type: "headers", Arch: "arm64", Name: "Debian-Ubuntu_Optional_linux-headers-6.4.16-bbrplus_6.4.16-1_arm64.deb"},
			{Type: "image", Arch: "amd64", Name: "Debian-Ubuntu_Required_linux-image-6.4.16-bbrplus_6.4.16-1_amd64.deb"},
			{Type: "image", Arch: "arm64", Name: "Debian-Ubuntu_Required_linux-image-6.4.16-bbrplus_6.4.16-1_arm64.deb"},
		},
	},
	{
		Version: "6.3.13-bbrplus",
		Assets: []bbrplusReleaseAsset{
			{Type: "headers", Arch: "amd64", Name: "Debian-Ubuntu_Optional_linux-headers-6.3.13-bbrplus_6.3.13-1_amd64.deb"},
			{Type: "headers", Arch: "arm64", Name: "Debian-Ubuntu_Optional_linux-headers-6.3.13-bbrplus_6.3.13-1_arm64.deb"},
			{Type: "image", Arch: "amd64", Name: "Debian-Ubuntu_Required_linux-image-6.3.13-bbrplus_6.3.13-1_amd64.deb"},
			{Type: "image", Arch: "arm64", Name: "Debian-Ubuntu_Required_linux-image-6.3.13-bbrplus_6.3.13-1_arm64.deb"},
		},
	},
	{
		Version: "6.2.16-bbrplus",
		Assets: []bbrplusReleaseAsset{
			{Type: "headers", Arch: "amd64", Name: "Debian-Ubuntu_Optional_linux-headers-6.2.16-bbrplus_6.2.16-bbrplus-1_amd64.deb"},
			{Type: "headers", Arch: "arm64", Name: "Debian-Ubuntu_Optional_linux-headers-6.2.16-bbrplus_6.2.16-bbrplus-1_arm64.deb"},
			{Type: "image", Arch: "amd64", Name: "Debian-Ubuntu_Required_linux-image-6.2.16-bbrplus_6.2.16-bbrplus-1_amd64.deb"},
			{Type: "image", Arch: "arm64", Name: "Debian-Ubuntu_Required_linux-image-6.2.16-bbrplus_6.2.16-bbrplus-1_arm64.deb"},
		},
	},
	{
		Version: "6.1.81-bbrplus",
		Assets: []bbrplusReleaseAsset{
			{Type: "headers", Arch: "amd64", Name: "Debian-Ubuntu_Optional_linux-headers-6.1.81-bbrplus_6.1.81-bbrplus-1_amd64.deb"},
			{Type: "headers", Arch: "arm64", Name: "Debian-Ubuntu_Optional_linux-headers-6.1.81-bbrplus_6.1.81-bbrplus-1_arm64.deb"},
			{Type: "image", Arch: "amd64", Name: "Debian-Ubuntu_Required_linux-image-6.1.81-bbrplus_6.1.81-bbrplus-1_amd64.deb"},
			{Type: "image", Arch: "arm64", Name: "Debian-Ubuntu_Required_linux-image-6.1.81-bbrplus_6.1.81-bbrplus-1_arm64.deb"},
		},
	},
	{
		Version: "6.0.19-bbrplus",
		Assets: []bbrplusReleaseAsset{
			{Type: "headers", Arch: "amd64", Name: "Debian-Ubuntu_Optional_linux-headers-6.0.19-bbrplus_6.0.19-bbrplus-1_amd64.deb"},
			{Type: "headers", Arch: "arm64", Name: "Debian-Ubuntu_Optional_linux-headers-6.0.19-bbrplus_6.0.19-bbrplus-1_arm64.deb"},
			{Type: "image", Arch: "amd64", Name: "Debian-Ubuntu_Required_linux-image-6.0.19-bbrplus_6.0.19-bbrplus-1_amd64.deb"},
			{Type: "image", Arch: "arm64", Name: "Debian-Ubuntu_Required_linux-image-6.0.19-bbrplus_6.0.19-bbrplus-1_arm64.deb"},
		},
	},
	{
		Version: "5.15.151-bbrplus",
		Assets: []bbrplusReleaseAsset{
			{Type: "headers", Arch: "amd64", Name: "Debian-Ubuntu_Optional_linux-headers-5.15.151-bbrplus_5.15.151-bbrplus-1_amd64.deb"},
			{Type: "headers", Arch: "arm64", Name: "Debian-Ubuntu_Optional_linux-headers-5.15.151-bbrplus_5.15.151-bbrplus-1_arm64.deb"},
			{Type: "image", Arch: "amd64", Name: "Debian-Ubuntu_Required_linux-image-5.15.151-bbrplus_5.15.151-bbrplus-1_amd64.deb"},
			{Type: "image", Arch: "arm64", Name: "Debian-Ubuntu_Required_linux-image-5.15.151-bbrplus_5.15.151-bbrplus-1_arm64.deb"},
		},
	},
	{
		Version: "5.10.212-bbrplus",
		Assets: []bbrplusReleaseAsset{
			{Type: "headers", Arch: "amd64", Name: "Debian-Ubuntu_Optional_linux-headers-5.10.212-bbrplus_5.10.212-bbrplus-1_amd64.deb"},
			{Type: "headers", Arch: "arm64", Name: "Debian-Ubuntu_Optional_linux-headers-5.10.212-bbrplus_5.10.212-bbrplus-1_arm64.deb"},
			{Type: "image", Arch: "amd64", Name: "Debian-Ubuntu_Required_linux-image-5.10.212-bbrplus_5.10.212-bbrplus-1_amd64.deb"},
			{Type: "image", Arch: "arm64", Name: "Debian-Ubuntu_Required_linux-image-5.10.212-bbrplus_5.10.212-bbrplus-1_arm64.deb"},
		},
	},
}

type KernelOverview struct {
	Supported           bool   `json:"supported"`
	Reason              string `json:"reason"`
	SystemFamily        string `json:"systemFamily,omitempty"`
	SystemID            string `json:"systemId,omitempty"`
	SystemIDLike        string `json:"systemIdLike,omitempty"`
	CurrentKernel       string `json:"currentKernel,omitempty"`
	DownloadRoot        string `json:"downloadRoot"`
	RebootHint          bool   `json:"rebootHint"`
	DownloadedKernel    string `json:"downloadedKernel,omitempty"`
	DownloadedDirectory string `json:"downloadedDirectory,omitempty"`
}

type KernelVersionItem struct {
	Name string `json:"name"`
}

type KernelVersionListResponse struct {
	Line     string              `json:"line"`
	Versions []KernelVersionItem `json:"versions"`
}

type KernelArchItem struct {
	Arch    string `json:"arch"`
	DirName string `json:"dirName"`
}

type KernelArchListResponse struct {
	Line    string           `json:"line"`
	Version string           `json:"version"`
	Arches  []KernelArchItem `json:"arches"`
}

type KernelPackageItem struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	DownloadURL string `json:"downloadUrl"`
	FullPath    string `json:"fullPath"`
}

type KernelPackageListResponse struct {
	Line      string              `json:"line"`
	Version   string              `json:"version"`
	Arch      string              `json:"arch"`
	ArchDir   string              `json:"archDir"`
	Packages  []KernelPackageItem `json:"packages"`
	Directory string              `json:"directory"`
}

type KernelDownloadedPackage struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	DownloadURL string `json:"downloadUrl"`
	LocalPath   string `json:"localPath"`
}

type KernelDownloadResult struct {
	Line       string                    `json:"line"`
	Version    string                    `json:"version"`
	Arch       string                    `json:"arch"`
	Directory  string                    `json:"directory"`
	Downloaded []KernelDownloadedPackage `json:"downloaded"`
	SessionID  string                    `json:"sessionId,omitempty"`
}

type KernelDownloadProgress struct {
	ID              string  `json:"id"`
	Status          string  `json:"status"`
	Percent         float64 `json:"percent"`
	Approximate     bool    `json:"approximate"`
	DownloadedBytes int64   `json:"downloadedBytes"`
	TotalBytes      int64   `json:"totalBytes"`
	CurrentPackage  string  `json:"currentPackage"`
	DownloadedCount int     `json:"downloadedCount"`
	TotalCount      int     `json:"totalCount"`
	Error           string  `json:"error,omitempty"`
	StartedAt       int64   `json:"startedAt"`
	UpdatedAt       int64   `json:"updatedAt"`
	FinishedAt      int64   `json:"finishedAt,omitempty"`
}

type kernelDownloadProgressSessionStore struct {
	mu       sync.Mutex
	sessions map[string]*kernelDownloadProgressSession
}

type kernelDownloadProgressSession struct {
	id              string
	status          string
	percent         float64
	approximate     bool
	downloadedBytes int64
	totalBytes      int64
	currentPackage  string
	downloadedCount int
	totalCount      int
	errText         string
	startedAt       int64
	updatedAt       int64
	finishedAt      int64
}

type kernelDownloadCleanupScheduler struct {
	mu     sync.Mutex
	states map[string]*kernelDownloadCleanupState
}

type kernelDownloadCleanupState struct {
	key   string
	phase string
	timer *time.Timer
}

type KernelInstallResult struct {
	Installed             bool     `json:"installed"`
	NeedsReboot           bool     `json:"needsReboot"`
	Command               string   `json:"command"`
	InstalledPackage      []string `json:"installedPackage"`
	CleanupDone           bool     `json:"cleanupDone"`
	CleanupWarning        string   `json:"cleanupWarning,omitempty"`
	PinnedKernel          string   `json:"pinnedKernel,omitempty"`
	PinnedUpdated         bool     `json:"pinnedUpdated"`
	SystemCleanupDone     bool     `json:"systemCleanupDone"`
	SystemCleanupWarnings []string `json:"systemCleanupWarnings,omitempty"`
	SystemCleanupSummary  string   `json:"systemCleanupSummary,omitempty"`
}

type KernelCleanupPackageItem struct {
	Name            string `json:"name"`
	Status          string `json:"status"`
	IsImage         bool   `json:"isImage"`
	IsHeaders       bool   `json:"isHeaders"`
	IsPinnedKernel  bool   `json:"isPinnedKernel"`
	IsCurrentKernel bool   `json:"isCurrentKernel"`
	Risk            string `json:"risk"`
}

type KernelCleanupScanResponse struct {
	CurrentKernel string                     `json:"currentKernel"`
	PinnedKernel  string                     `json:"pinnedKernel"`
	Packages      []KernelCleanupPackageItem `json:"packages"`
}

type KernelCleanupPurgeResult struct {
	Requested             []string `json:"requested"`
	Command               string   `json:"command"`
	NeedsReboot           bool     `json:"needsReboot"`
	Succeeded             []string `json:"succeeded"`
	Failed                []string `json:"failed"`
	Message               string   `json:"message"`
	SystemCleanupDone     bool     `json:"systemCleanupDone"`
	SystemCleanupWarnings []string `json:"systemCleanupWarnings,omitempty"`
	SystemCleanupSummary  string   `json:"systemCleanupSummary,omitempty"`
}

type kernelDownloadedMarker struct {
	Provider   string `json:"provider"`
	Line       string `json:"line,omitempty"`
	Version    string `json:"version"`
	Arch       string `json:"arch,omitempty"`
	Directory  string `json:"directory"`
	Downloaded int64  `json:"downloadedAt,omitempty"`
}

type KernelDownloadedStatus struct {
	Exists    bool   `json:"exists"`
	Provider  string `json:"provider,omitempty"`
	Line      string `json:"line,omitempty"`
	Version   string `json:"version,omitempty"`
	Arch      string `json:"arch,omitempty"`
	Directory string `json:"directory,omitempty"`
	Display   string `json:"display,omitempty"`
}

type KernelDownloadedClearResult struct {
	Cleared   bool   `json:"cleared"`
	Directory string `json:"directory"`
}

type kernelSelectionEntry struct {
	Name   string
	Status string
}

type sourceForgeFileEntry struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	DownloadURL string `json:"download_url"`
	URL         string `json:"url"`
	FullPath    string `json:"full_path"`
	Type        string `json:"type"`
}

const kernelCleanupPinnedKernelSettingKey = "kernelCleanupPinnedKernel"
const kernelDownloadedMarkerSettingKey = "kernelDownloadedMarker"

var kernelResolveAptCommand = resolveKernelAptCommand
var kernelRunSystemCleanup = runKernelStrictSystemCleanup
var kernelRunPrivilegedCommand = runKernelPrivilegedCommand
var kernelEnsureRuntimeSupported = ensureKernelRuntimeSupported
var kernelLookPath = exec.LookPath

type kernelSystemCleanupReport struct {
	Done     bool
	Warnings []string
	Summary  string
}

func runKernelStrictSystemCleanup() *kernelSystemCleanupReport {
	report := &kernelSystemCleanupReport{Done: true}
	runStep := func(label string, timeout time.Duration, command string, args ...string) {
		if err := kernelRunPrivilegedCommand(timeout, command, args...); err != nil {
			report.Done = false
			report.Warnings = append(report.Warnings, fmt.Sprintf("%s: %s", label, strings.TrimSpace(err.Error())))
		}
	}

	if aptCommand, err := kernelResolveAptCommand(); err != nil {
		report.Done = false
		report.Warnings = append(report.Warnings, "apt cleanup skipped: "+strings.TrimSpace(err.Error()))
	} else {
		runStep("apt autoremove", 20*time.Minute, aptCommand, "autoremove", "-y")
		runStep("apt autoclean", 10*time.Minute, aptCommand, "autoclean")
		runStep("apt clean", 10*time.Minute, aptCommand, "clean")
	}

	if journalctlPath, err := kernelLookPath("journalctl"); err != nil {
		report.Done = false
		report.Warnings = append(report.Warnings, "journal cleanup skipped: "+strings.TrimSpace(err.Error()))
	} else {
		runStep("journalctl rotate", 5*time.Minute, journalctlPath, "--rotate")
		runStep("journalctl vacuum", 5*time.Minute, journalctlPath, "--vacuum-time=1s")
	}

	runStep("clear /var/log", 20*time.Minute, "sh", "-lc", `find /var/log -xdev -type f -exec truncate -s 0 -- {} +`)
	runStep("clear /tmp and /var/tmp", 20*time.Minute, "sh", "-lc", `find /tmp /var/tmp -mindepth 1 -xdev -exec rm -rf -- {} +`)

	report.Summary = buildKernelSystemCleanupSummary(report.Warnings)
	return report
}

func buildKernelSystemCleanupSummary(warnings []string) string {
	trimmed := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		text := strings.TrimSpace(warning)
		if text != "" {
			trimmed = append(trimmed, text)
		}
	}
	if len(trimmed) == 0 {
		return "system cleanup completed"
	}
	return "system cleanup completed with warnings: " + strings.Join(trimmed, "; ")
}

func applyKernelSystemCleanupResultInstall(result *KernelInstallResult, report *kernelSystemCleanupReport) {
	if result == nil || report == nil {
		return
	}
	result.SystemCleanupDone = report.Done
	result.SystemCleanupSummary = report.Summary
	if len(report.Warnings) > 0 {
		result.SystemCleanupWarnings = append([]string(nil), report.Warnings...)
	}
}

func applyKernelSystemCleanupResultPurge(result *KernelCleanupPurgeResult, report *kernelSystemCleanupReport) {
	if result == nil || report == nil {
		return
	}
	result.SystemCleanupDone = report.Done
	result.SystemCleanupSummary = report.Summary
	if len(report.Warnings) > 0 {
		result.SystemCleanupWarnings = append([]string(nil), report.Warnings...)
	}
}

func newKernelDownloadProgressStore() *kernelDownloadProgressSessionStore {
	return &kernelDownloadProgressSessionStore{
		sessions: make(map[string]*kernelDownloadProgressSession),
	}
}

func newKernelDownloadCleanupScheduler() *kernelDownloadCleanupScheduler {
	return &kernelDownloadCleanupScheduler{
		states: make(map[string]*kernelDownloadCleanupState),
	}
}

func (s *kernelDownloadCleanupScheduler) markRunning(key string) {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	state := s.states[trimmed]
	if state == nil {
		s.states[trimmed] = &kernelDownloadCleanupState{
			key:   trimmed,
			phase: "running",
		}
		return
	}
	if state.timer != nil {
		state.timer.Stop()
		state.timer = nil
	}
	state.phase = "running"
}

func (s *kernelDownloadCleanupScheduler) markCompleted(key string) {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	state := s.states[trimmed]
	if state != nil && state.timer != nil {
		state.timer.Stop()
	}
	delete(s.states, trimmed)
}

func (s *kernelDownloadCleanupScheduler) release(key string) {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	state := s.states[trimmed]
	if state != nil && state.timer != nil {
		state.timer.Stop()
	}
	delete(s.states, trimmed)
}

func (s *kernelDownloadCleanupScheduler) isProtected(key string) bool {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	state := s.states[trimmed]
	if state == nil {
		return false
	}
	return state.phase == "running" || state.phase == "failed" || state.phase == "cleaning"
}

func (s *kernelDownloadCleanupScheduler) scheduleFailedCleanup(key string, delay time.Duration, fn func() error) {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" || fn == nil {
		return
	}
	if delay < 0 {
		delay = 0
	}

	s.mu.Lock()
	state := s.states[trimmed]
	if state == nil {
		state = &kernelDownloadCleanupState{key: trimmed}
		s.states[trimmed] = state
	}
	if state.timer != nil {
		state.timer.Stop()
		state.timer = nil
	}
	state.phase = "failed"
	state.timer = time.AfterFunc(delay, func() {
		s.mu.Lock()
		current := s.states[trimmed]
		if current == nil || current.phase != "failed" {
			s.mu.Unlock()
			return
		}
		current.phase = "cleaning"
		current.timer = nil
		s.mu.Unlock()

		_ = fn()

		s.mu.Lock()
		defer s.mu.Unlock()
		current = s.states[trimmed]
		if current == nil {
			return
		}
		if current.phase == "running" {
			return
		}
		delete(s.states, trimmed)
	})
	s.mu.Unlock()
}

func (s *kernelDownloadProgressSessionStore) start(id string, totalCount int) *kernelDownloadProgressSession {
	now := time.Now().Unix()
	sessionID := normalizeKernelDownloadSessionID(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(now)

	if totalCount < 0 {
		totalCount = 0
	}
	session := &kernelDownloadProgressSession{
		id:         sessionID,
		status:     "running",
		totalCount: totalCount,
		startedAt:  now,
		updatedAt:  now,
	}
	s.sessions[sessionID] = session
	return session
}

func (s *kernelDownloadProgressSessionStore) get(id string) *KernelDownloadProgress {
	now := time.Now().Unix()
	trimmed := strings.TrimSpace(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(now)

	session := s.sessions[trimmed]
	if session == nil {
		return &KernelDownloadProgress{
			ID:        trimmed,
			Status:    "missing",
			Percent:   0,
			StartedAt: now,
			UpdatedAt: now,
		}
	}
	return session.snapshotLocked()
}

func (s *kernelDownloadProgressSessionStore) setTotals(id string, totalBytes int64, approximate bool) {
	now := time.Now().Unix()
	s.mu.Lock()
	defer s.mu.Unlock()
	session := s.sessions[id]
	if session == nil {
		return
	}
	if totalBytes < 0 {
		totalBytes = 0
	}
	session.totalBytes = totalBytes
	session.approximate = approximate
	session.updatedAt = now
	session.recalculatePercentLocked()
}

func (s *kernelDownloadProgressSessionStore) setCurrentPackage(id string, packageName string) {
	now := time.Now().Unix()
	s.mu.Lock()
	defer s.mu.Unlock()
	session := s.sessions[id]
	if session == nil {
		return
	}
	session.currentPackage = strings.TrimSpace(packageName)
	session.updatedAt = now
}

func (s *kernelDownloadProgressSessionStore) addDownloadedBytes(id string, delta int64) {
	if delta <= 0 {
		return
	}
	now := time.Now().Unix()
	s.mu.Lock()
	defer s.mu.Unlock()
	session := s.sessions[id]
	if session == nil {
		return
	}
	session.downloadedBytes += delta
	if session.downloadedBytes < 0 {
		session.downloadedBytes = 0
	}
	session.updatedAt = now
	session.recalculatePercentLocked()
}

func (s *kernelDownloadProgressSessionStore) setEstimatedTotalAtLeast(id string, minTotal int64) {
	if minTotal <= 0 {
		return
	}
	now := time.Now().Unix()
	s.mu.Lock()
	defer s.mu.Unlock()
	session := s.sessions[id]
	if session == nil {
		return
	}
	if session.totalBytes < minTotal {
		session.totalBytes = minTotal
		session.approximate = true
		session.recalculatePercentLocked()
	}
	session.updatedAt = now
}

func (s *kernelDownloadProgressSessionStore) incrementDownloadedCount(id string) {
	now := time.Now().Unix()
	s.mu.Lock()
	defer s.mu.Unlock()
	session := s.sessions[id]
	if session == nil {
		return
	}
	session.downloadedCount++
	if session.downloadedCount > session.totalCount {
		session.totalCount = session.downloadedCount
	}
	session.updatedAt = now
	session.recalculatePercentLocked()
}

func (s *kernelDownloadProgressSessionStore) finishSuccess(id string) {
	now := time.Now().Unix()
	s.mu.Lock()
	defer s.mu.Unlock()
	session := s.sessions[id]
	if session == nil {
		return
	}
	session.status = "success"
	session.currentPackage = ""
	session.downloadedCount = session.totalCount
	if session.totalBytes <= 0 || session.totalBytes < session.downloadedBytes {
		session.totalBytes = session.downloadedBytes
	}
	session.percent = 100
	session.updatedAt = now
	session.finishedAt = now
}

func (s *kernelDownloadProgressSessionStore) finishError(id string, message string) {
	now := time.Now().Unix()
	s.mu.Lock()
	defer s.mu.Unlock()
	session := s.sessions[id]
	if session == nil {
		return
	}
	session.status = "error"
	session.currentPackage = ""
	session.errText = strings.TrimSpace(message)
	session.updatedAt = now
	session.finishedAt = now
	session.recalculatePercentLocked()
}

func (s *kernelDownloadProgressSessionStore) pruneLocked(now int64) {
	ttlSeconds := int64(kernelDownloadProgressTTL / time.Second)
	for id, session := range s.sessions {
		if now-session.updatedAt > ttlSeconds {
			delete(s.sessions, id)
		}
	}
}

func (s *kernelDownloadProgressSession) recalculatePercentLocked() {
	total := s.totalBytes
	downloaded := s.downloadedBytes
	if total > 0 {
		percent := float64(downloaded) * 100 / float64(total)
		if percent < 0 {
			percent = 0
		}
		if s.status == "running" && s.approximate && percent >= 100 {
			percent = 99
		}
		if percent > 100 {
			percent = 100
		}
		s.percent = percent
		return
	}
	if s.status == "success" {
		s.percent = 100
		return
	}
	s.percent = 0
}

func (s *kernelDownloadProgressSession) snapshotLocked() *KernelDownloadProgress {
	return &KernelDownloadProgress{
		ID:              s.id,
		Status:          s.status,
		Percent:         s.percent,
		Approximate:     s.approximate,
		DownloadedBytes: s.downloadedBytes,
		TotalBytes:      s.totalBytes,
		CurrentPackage:  s.currentPackage,
		DownloadedCount: s.downloadedCount,
		TotalCount:      s.totalCount,
		Error:           s.errText,
		StartedAt:       s.startedAt,
		UpdatedAt:       s.updatedAt,
		FinishedAt:      s.finishedAt,
	}
}

func normalizeKernelDownloadSessionID(id string) string {
	trimmed := strings.TrimSpace(id)
	if kernelDownloadIDPattern.MatchString(trimmed) {
		return trimmed
	}

	var buf [12]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("kernel-%d", time.Now().UnixNano())
	}
	return "kernel-" + hex.EncodeToString(buf[:])
}

func (s *KernelManagerService) GetOverview(provider string) (*KernelOverview, error) {
	normalizedProvider, err := normalizeKernelProviderRequired(provider)
	if err != nil {
		return nil, err
	}

	fields := readKernelOSReleaseFields()
	systemFamily := detectKernelLinuxSystemFamily(fields)
	supported := runtime.GOOS == "linux" && systemFamily == "debian"

	reason := ""
	if runtime.GOOS != "linux" {
		reason = "kernel management only supports linux hosts"
	} else if systemFamily != "debian" {
		reason = "only Debian/Ubuntu and derivatives are supported"
	}

	currentKernel := ""
	if output, err := runKernelCommandOutput(8*time.Second, "uname", "-r"); err == nil {
		currentKernel = strings.TrimSpace(output)
	}

	overview := &KernelOverview{
		Supported:     supported,
		Reason:        reason,
		SystemFamily:  systemFamily,
		SystemID:      strings.ToLower(strings.TrimSpace(fields["ID"])),
		SystemIDLike:  strings.ToLower(strings.TrimSpace(fields["ID_LIKE"])),
		CurrentKernel: currentKernel,
		DownloadRoot:  s.getKernelDownloadRoot(normalizedProvider),
		RebootHint:    kernelRebootHintRequired(),
	}
	if downloadedStatus, statusErr := s.GetDownloadedKernelStatus(); statusErr == nil && downloadedStatus != nil && downloadedStatus.Exists {
		overview.DownloadedKernel = downloadedStatus.Display
		overview.DownloadedDirectory = downloadedStatus.Directory
	}

	return overview, nil
}

func (s *KernelManagerService) GetVersions(provider, line string) (*KernelVersionListResponse, error) {
	normalizedProvider, err := normalizeKernelProviderRequired(provider)
	if err != nil {
		return nil, err
	}
	if normalizedProvider == kernelProviderBBRPlus {
		versions := make([]KernelVersionItem, 0, len(bbrplusReleaseCatalog))
		for _, item := range bbrplusReleaseCatalog {
			versions = append(versions, KernelVersionItem{Name: item.Version})
		}
		sort.SliceStable(versions, func(i, j int) bool {
			return bbrplusVersionSortPriority(versions[i].Name) < bbrplusVersionSortPriority(versions[j].Name)
		})
		return &KernelVersionListResponse{
			Line:     normalizedProvider,
			Versions: versions,
		}, nil
	}

	normalizedLine, err := normalizeKernelLine(line)
	if err != nil {
		return nil, err
	}

	entries, err := fetchSourceForgeFileEntries(pathJoinSlash("releases", normalizedLine))
	if err != nil {
		return nil, err
	}

	versions := make([]KernelVersionItem, 0)
	for _, entry := range entries {
		if entry.Type != "d" || strings.TrimSpace(entry.Name) == "" {
			continue
		}
		versions = append(versions, KernelVersionItem{Name: entry.Name})
	}

	sort.Slice(versions, func(i, j int) bool {
		return compareKernelVersionNameDesc(versions[i].Name, versions[j].Name) < 0
	})

	return &KernelVersionListResponse{
		Line:     normalizedLine,
		Versions: versions,
	}, nil
}

func (s *KernelManagerService) GetArches(provider, line, version string) (*KernelArchListResponse, error) {
	normalizedProvider, err := normalizeKernelProviderRequired(provider)
	if err != nil {
		return nil, err
	}
	if normalizedProvider == kernelProviderBBRPlus {
		normalizedVersion := strings.TrimSpace(version)
		if normalizedVersion == "" {
			return nil, fmt.Errorf("version is required")
		}
		entry, ok := findBBRPlusReleaseEntry(normalizedVersion)
		if !ok {
			return nil, fmt.Errorf("unsupported bbrplus version: %s", version)
		}
		hostArch, err := normalizeKernelProviderArch(normalizedProvider, "")
		if err != nil {
			return nil, err
		}
		items := make([]KernelArchItem, 0, 1)
		for _, asset := range entry.Assets {
			if asset.Arch != hostArch {
				continue
			}
			items = append(items, KernelArchItem{
				Arch:    hostArch,
				DirName: hostArch,
			})
			break
		}
		return &KernelArchListResponse{
			Line:    normalizedProvider,
			Version: normalizedVersion,
			Arches:  items,
		}, nil
	}

	normalizedLine, normalizedVersion, err := normalizeKernelLineVersion(line, version)
	if err != nil {
		return nil, err
	}

	entries, err := fetchSourceForgeFileEntries(pathJoinSlash("releases", normalizedLine, normalizedVersion))
	if err != nil {
		return nil, err
	}

	items := make([]KernelArchItem, 0, 3)
	seen := map[string]struct{}{}
	for _, entry := range entries {
		if entry.Type != "d" {
			continue
		}
		arch := extractKernelArchLevel(entry.Name)
		if arch == "" {
			continue
		}
		if _, ok := seen[arch]; ok {
			continue
		}
		seen[arch] = struct{}{}
		items = append(items, KernelArchItem{
			Arch:    arch,
			DirName: entry.Name,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Arch > items[j].Arch
	})

	return &KernelArchListResponse{
		Line:    normalizedLine,
		Version: normalizedVersion,
		Arches:  items,
	}, nil
}

func (s *KernelManagerService) GetPackages(provider, line, version, arch string) (*KernelPackageListResponse, error) {
	normalizedProvider, err := normalizeKernelProviderRequired(provider)
	if err != nil {
		return nil, err
	}
	if normalizedProvider == kernelProviderBBRPlus {
		normalizedVersion := strings.TrimSpace(version)
		if normalizedVersion == "" {
			return nil, fmt.Errorf("version is required")
		}
		normalizedArch, err := normalizeKernelProviderArch(normalizedProvider, arch)
		if err != nil {
			return nil, err
		}
		entry, ok := findBBRPlusReleaseEntry(normalizedVersion)
		if !ok {
			return nil, fmt.Errorf("unsupported bbrplus version: %s", version)
		}

		packages := make([]KernelPackageItem, 0, 2)
		for _, asset := range entry.Assets {
			if asset.Arch != normalizedArch {
				continue
			}
			packages = append(packages, KernelPackageItem{
				Name:        asset.Name,
				Type:        asset.Type,
				DownloadURL: buildBBRPlusReleaseURL(entry.Version, asset.Name),
				FullPath:    asset.Name,
			})
		}
		sort.Slice(packages, func(i, j int) bool {
			if packages[i].Type != packages[j].Type {
				return packages[i].Type < packages[j].Type
			}
			return packages[i].Name < packages[j].Name
		})

		return &KernelPackageListResponse{
			Line:      normalizedProvider,
			Version:   normalizedVersion,
			Arch:      normalizedArch,
			ArchDir:   normalizedArch,
			Packages:  packages,
			Directory: filepath.Join(s.getKernelDownloadRoot(normalizedProvider), normalizedVersion, normalizedArch),
		}, nil
	}

	normalizedLine, normalizedVersion, normalizedArch, err := normalizeKernelLineVersionArch(line, version, arch)
	if err != nil {
		return nil, err
	}

	archDir, err := s.resolveKernelArchDirectory(normalizedProvider, normalizedLine, normalizedVersion, normalizedArch)
	if err != nil {
		return nil, err
	}

	entries, err := fetchSourceForgeFileEntries(pathJoinSlash("releases", normalizedLine, normalizedVersion, archDir))
	if err != nil {
		return nil, err
	}

	packages := make([]KernelPackageItem, 0, 2)
	for _, entry := range entries {
		if entry.Type != "f" {
			continue
		}
		name := strings.TrimSpace(entry.Name)
		if !kernelPackagePattern.MatchString(name) {
			continue
		}
		pkgType := kernelPackageType(name)
		packages = append(packages, KernelPackageItem{
			Name:        name,
			Type:        pkgType,
			DownloadURL: strings.TrimSpace(entry.DownloadURL),
			FullPath:    strings.TrimSpace(entry.FullPath),
		})
	}

	sort.Slice(packages, func(i, j int) bool {
		if packages[i].Type != packages[j].Type {
			return packages[i].Type < packages[j].Type
		}
		return packages[i].Name < packages[j].Name
	})

	return &KernelPackageListResponse{
		Line:      normalizedLine,
		Version:   normalizedVersion,
		Arch:      normalizedArch,
		ArchDir:   archDir,
		Packages:  packages,
		Directory: filepath.Join(s.getKernelDownloadRoot(normalizedProvider), normalizedLine, normalizedVersion, normalizedArch),
	}, nil
}

func (s *KernelManagerService) DownloadPackages(provider, line, version, arch string, downloadSessionID string) (*KernelDownloadResult, error) {
	if err := kernelEnsureRuntimeSupported(); err != nil {
		return nil, err
	}

	pkgs, err := s.GetPackages(provider, line, version, arch)
	if err != nil {
		return nil, err
	}
	if len(pkgs.Packages) == 0 {
		return nil, fmt.Errorf("no downloadable kernel deb packages found")
	}

	if err := ensureKernelPackagePair(pkgs.Packages); err != nil {
		return nil, err
	}

	if err := s.removeKernelDownloadPath(s.getKernelDownloadRoot("")); err != nil {
		return nil, err
	}

	downloadDir := pkgs.Directory
	if err := os.MkdirAll(downloadDir, 0o755); err != nil {
		return nil, fmt.Errorf("create kernel download directory failed: %w", err)
	}

	session := kernelDownloadProgressStore.start(downloadSessionID, len(pkgs.Packages))
	sessionID := session.id
	kernelDownloadCleanupStore.markRunning(downloadDir)
	kernelDownloadProgressStore.setTotals(
		sessionID,
		int64(len(pkgs.Packages))*kernelUnknownPackageSizeEstimate,
		true,
	)

	packageSizes := make(map[string]int64, len(pkgs.Packages))
	var knownTotal int64
	knownCount := 0
	for _, pkg := range pkgs.Packages {
		size, ok := probeRemoteContentLength(pkg.DownloadURL)
		if !ok || size <= 0 {
			continue
		}
		packageSizes[pkg.Name] = size
		knownTotal += size
		knownCount++
	}
	unknownCount := len(pkgs.Packages) - knownCount
	defaultEstimate := kernelUnknownPackageSizeEstimate
	if knownCount > 0 {
		average := knownTotal / int64(knownCount)
		if average > 0 {
			defaultEstimate = average
		}
	}
	estimatedTotal := knownTotal + int64(unknownCount)*defaultEstimate
	if estimatedTotal <= 0 {
		estimatedTotal = int64(len(pkgs.Packages)) * kernelUnknownPackageSizeEstimate
	}
	kernelDownloadProgressStore.setTotals(sessionID, estimatedTotal, unknownCount > 0)

	result := &KernelDownloadResult{
		Line:       pkgs.Line,
		Version:    pkgs.Version,
		Arch:       pkgs.Arch,
		Directory:  downloadDir,
		Downloaded: make([]KernelDownloadedPackage, 0, len(pkgs.Packages)),
		SessionID:  sessionID,
	}

	completedUnknownCount := 0
	var completedUnknownBytes int64
	for _, pkg := range pkgs.Packages {
		if strings.TrimSpace(pkg.DownloadURL) == "" {
			kernelDownloadProgressStore.finishError(sessionID, fmt.Sprintf("package %s does not provide download url", pkg.Name))
			s.scheduleKernelFailedDownloadCleanup(downloadDir)
			return nil, fmt.Errorf("package %s does not provide download url", pkg.Name)
		}
		localPath := filepath.Join(downloadDir, pkg.Name)
		kernelDownloadProgressStore.setCurrentPackage(sessionID, pkg.Name)
		if err := downloadFileToPath(
			pkg.DownloadURL,
			localPath,
			func(delta int64) {
				kernelDownloadProgressStore.addDownloadedBytes(sessionID, delta)
			},
		); err != nil {
			kernelDownloadProgressStore.finishError(sessionID, fmt.Sprintf("download %s failed: %v", pkg.Name, err))
			s.scheduleKernelFailedDownloadCleanup(downloadDir)
			return nil, fmt.Errorf("download %s failed: %w", pkg.Name, err)
		}
		kernelDownloadProgressStore.incrementDownloadedCount(sessionID)

		if _, ok := packageSizes[pkg.Name]; !ok {
			info, statErr := os.Stat(localPath)
			if statErr == nil && info != nil {
				completedUnknownCount++
				completedUnknownBytes += info.Size()
				remainingUnknown := unknownCount - completedUnknownCount
				if remainingUnknown < 0 {
					remainingUnknown = 0
				}
				nextEstimate := defaultEstimate
				if completedUnknownCount > 0 {
					nextEstimate = completedUnknownBytes / int64(completedUnknownCount)
					if nextEstimate <= 0 {
						nextEstimate = defaultEstimate
					}
				}
				nextTotal := knownTotal + completedUnknownBytes + int64(remainingUnknown)*nextEstimate
				if nextTotal > 0 {
					kernelDownloadProgressStore.setEstimatedTotalAtLeast(sessionID, nextTotal)
				}
			}
		}

		result.Downloaded = append(result.Downloaded, KernelDownloadedPackage{
			Name:        pkg.Name,
			Type:        pkg.Type,
			DownloadURL: pkg.DownloadURL,
			LocalPath:   localPath,
		})
	}

	if err := validateKernelDownloadedPair(downloadDir); err != nil {
		kernelDownloadProgressStore.finishError(sessionID, err.Error())
		s.scheduleKernelFailedDownloadCleanup(downloadDir)
		return nil, err
	}

	kernelDownloadProgressStore.finishSuccess(sessionID)
	kernelDownloadCleanupStore.markCompleted(downloadDir)
	if markerErr := s.saveDownloadedMarker(kernelDownloadedMarker{
		Provider:   strings.TrimSpace(provider),
		Line:       pkgs.Line,
		Version:    pkgs.Version,
		Arch:       pkgs.Arch,
		Directory:  downloadDir,
		Downloaded: time.Now().Unix(),
	}); markerErr != nil {
		return result, fmt.Errorf("download completed but save downloaded marker failed: %w", markerErr)
	}
	return result, nil
}

func (s *KernelManagerService) GetDownloadProgress(id string) *KernelDownloadProgress {
	return kernelDownloadProgressStore.get(id)
}

func ensureKernelPackagePair(packages []KernelPackageItem) error {
	hasHeaders := false
	hasImage := false
	for _, pkg := range packages {
		switch strings.ToLower(strings.TrimSpace(pkg.Type)) {
		case "headers":
			hasHeaders = true
		case "image":
			hasImage = true
		}
	}
	if !hasHeaders || !hasImage {
		return fmt.Errorf("kernel package list is incomplete: headers and image packages are both required")
	}
	return nil
}

func validateKernelDownloadedPair(dir string) error {
	imageDeb, headersDeb, err := findKernelInstallDebPair(dir)
	if err != nil {
		return fmt.Errorf("kernel package pair validation failed: %w", err)
	}
	if strings.TrimSpace(imageDeb) == "" || strings.TrimSpace(headersDeb) == "" {
		return fmt.Errorf("kernel package pair validation failed: headers and image packages must both exist")
	}
	return nil
}

func (s *KernelManagerService) scheduleKernelFailedDownloadCleanup(downloadDir string) {
	trimmedDir := strings.TrimSpace(downloadDir)
	if trimmedDir == "" {
		return
	}

	cleanupTarget := s.prepareKernelFailedDownloadCleanup(trimmedDir)
	if cleanupTarget == "" {
		return
	}
	kernelDownloadCleanupStore.scheduleFailedCleanup(cleanupTarget, kernelFailedDownloadCleanupDelay, func() error {
		return s.cleanupKernelFailedDownloadArtifacts(cleanupTarget)
	})
}

func (s *KernelManagerService) cleanupKernelFailedDownloadArtifacts(downloadDir string) error {
	if err := s.removeKernelDownloadPath(downloadDir); err != nil {
		return err
	}
	if err := s.clearDownloadedMarkerIfWithinRoot(downloadDir); err != nil {
		return err
	}
	return nil
}

func (s *KernelManagerService) prepareKernelFailedDownloadCleanup(downloadDir string) string {
	trimmedDir := strings.TrimSpace(downloadDir)
	if trimmedDir == "" {
		return ""
	}

	kernelDownloadCleanupStore.release(trimmedDir)
	_ = s.clearDownloadedMarkerIfWithinRoot(trimmedDir)

	info, err := os.Stat(trimmedDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ""
		}
		return trimmedDir
	}
	if info == nil || !info.IsDir() {
		return trimmedDir
	}

	cleanupRoot := s.getKernelFailedCleanupRoot()
	if err := os.MkdirAll(cleanupRoot, 0o755); err != nil {
		return trimmedDir
	}

	cleanupTarget := filepath.Join(cleanupRoot, buildKernelFailedCleanupDirName(trimmedDir))
	if err := os.Rename(trimmedDir, cleanupTarget); err != nil {
		return trimmedDir
	}
	return cleanupTarget
}

func (s *KernelManagerService) saveDownloadedMarker(marker kernelDownloadedMarker) error {
	if database.GetDB() == nil {
		return nil
	}
	normalizedProvider, err := normalizeKernelProvider(marker.Provider)
	if err != nil {
		return err
	}
	marker.Provider = normalizedProvider
	marker.Line = strings.TrimSpace(marker.Line)
	marker.Version = strings.TrimSpace(marker.Version)
	marker.Arch = strings.TrimSpace(marker.Arch)
	marker.Directory = strings.TrimSpace(marker.Directory)
	if marker.Version == "" {
		return fmt.Errorf("downloaded marker version is empty")
	}
	if marker.Directory == "" {
		return fmt.Errorf("downloaded marker directory is empty")
	}
	if marker.Downloaded <= 0 {
		marker.Downloaded = time.Now().Unix()
	}
	raw, err := json.Marshal(marker)
	if err != nil {
		return err
	}
	settingSvc := &SettingService{}
	return settingSvc.saveSetting(kernelDownloadedMarkerSettingKey, string(raw))
}

func (s *KernelManagerService) loadDownloadedMarker() (*kernelDownloadedMarker, error) {
	if database.GetDB() == nil {
		return nil, nil
	}
	settingSvc := &SettingService{}
	setting, err := settingSvc.getSetting(kernelDownloadedMarkerSettingKey)
	if database.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	raw := strings.TrimSpace(setting.Value)
	if raw == "" {
		return nil, nil
	}
	marker := kernelDownloadedMarker{}
	if err := json.Unmarshal([]byte(raw), &marker); err != nil {
		return nil, err
	}
	marker.Provider = strings.TrimSpace(marker.Provider)
	marker.Line = strings.TrimSpace(marker.Line)
	marker.Version = strings.TrimSpace(marker.Version)
	marker.Arch = strings.TrimSpace(marker.Arch)
	marker.Directory = strings.TrimSpace(marker.Directory)
	return &marker, nil
}

func (s *KernelManagerService) clearDownloadedMarker() error {
	if database.GetDB() == nil {
		return nil
	}
	settingSvc := &SettingService{}
	return settingSvc.saveSetting(kernelDownloadedMarkerSettingKey, "")
}

func (s *KernelManagerService) GetDownloadedKernelStatus() (*KernelDownloadedStatus, error) {
	marker, err := s.loadDownloadedMarker()
	if err != nil {
		return nil, err
	}
	if marker == nil || marker.Version == "" || marker.Directory == "" {
		return s.findLegacyDownloadedKernelStatus()
	}

	info, statErr := os.Stat(marker.Directory)
	if statErr != nil || info == nil || !info.IsDir() {
		_ = s.clearDownloadedMarker()
		return s.findLegacyDownloadedKernelStatus()
	}

	if err := validateKernelDownloadedPair(marker.Directory); err != nil {
		_ = s.cleanupKernelFailedDownloadArtifacts(marker.Directory)
		return s.findLegacyDownloadedKernelStatus()
	}
	return buildKernelDownloadedStatus(marker), nil
}

func (s *KernelManagerService) ClearDownloadedKernel() (*KernelDownloadedClearResult, error) {
	root, err := s.cleanupAllKernelDownloadArtifacts()
	if err != nil {
		return nil, err
	}
	return &KernelDownloadedClearResult{
		Cleared:   true,
		Directory: root,
	}, nil
}

func (s *KernelManagerService) GetPinnedKernel() (string, error) {
	settingSvc := &SettingService{}
	value, err := settingSvc.getString(kernelCleanupPinnedKernelSettingKey)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

func (s *KernelManagerService) SetPinnedKernel(kernel string) error {
	trimmed := strings.TrimSpace(kernel)
	if trimmed != "" && !kernelAptNamePattern.MatchString(trimmed) {
		return fmt.Errorf("invalid kernel marker: %s", kernel)
	}
	settingSvc := &SettingService{}
	return settingSvc.setString(kernelCleanupPinnedKernelSettingKey, trimmed)
}

func (s *KernelManagerService) ScanCleanupPackages() (*KernelCleanupScanResponse, error) {
	if err := kernelEnsureRuntimeSupported(); err != nil {
		return nil, err
	}

	rawOutput, err := runKernelCommandOutput(20*time.Second, "dpkg", "--get-selections")
	if err != nil {
		return nil, err
	}

	currentKernel := ""
	if currentOutput, currentErr := runKernelCommandOutput(8*time.Second, "uname", "-r"); currentErr == nil {
		currentKernel = strings.TrimSpace(currentOutput)
	}

	pinnedKernel, err := s.GetPinnedKernel()
	if err != nil {
		return nil, err
	}

	entries := parseKernelSelections(rawOutput)
	items := buildKernelCleanupPackageItems(entries, currentKernel, pinnedKernel)
	return &KernelCleanupScanResponse{
		CurrentKernel: currentKernel,
		PinnedKernel:  pinnedKernel,
		Packages:      items,
	}, nil
}

func (s *KernelManagerService) PurgePackages(packages []string) (result *KernelCleanupPurgeResult, err error) {
	if err := kernelEnsureRuntimeSupported(); err != nil {
		return nil, err
	}

	result = &KernelCleanupPurgeResult{
		NeedsReboot: kernelRebootHintRequired(),
		Succeeded:   []string{},
		Failed:      []string{},
		Message:     "purge completed",
	}
	defer func() {
		systemCleanup := kernelRunSystemCleanup()
		applyKernelSystemCleanupResultPurge(result, systemCleanup)
		if err != nil {
			summary := buildKernelSystemCleanupSummary(nil)
			if systemCleanup != nil {
				summary = systemCleanup.Summary
			}
			err = fmt.Errorf("%w; %s", err, summary)
		}
	}()

	targets, normalizeErr := normalizeKernelPurgeTargets(packages)
	if normalizeErr != nil {
		err = normalizeErr
		result.Message = "purge failed"
		return result, err
	}
	result.Requested = append([]string(nil), targets...)

	aptCommand, resolveErr := kernelResolveAptCommand()
	if resolveErr != nil {
		err = resolveErr
		result.Message = "purge failed"
		result.Failed = append(result.Failed, targets...)
		return result, err
	}

	commandArgs := buildKernelPurgeCommand(aptCommand, targets)
	result.Command = strings.Join(commandArgs, " ")
	if len(commandArgs) == 0 {
		err = fmt.Errorf("failed to build purge command")
		result.Message = "purge failed"
		result.Failed = append(result.Failed, targets...)
		return result, err
	}

	if err = kernelRunPrivilegedCommand(40*time.Minute, commandArgs[0], commandArgs[1:]...); err != nil {
		err = fmt.Errorf("kernel purge command failed: %w", err)
		result.Message = "purge failed"
		result.Failed = append(result.Failed, targets...)
		return result, err
	}

	result.Succeeded = append(result.Succeeded, targets...)
	return result, nil
}

func (s *KernelManagerService) AutoCleanupPackages() (result *KernelCleanupPurgeResult, err error) {
	if err := kernelEnsureRuntimeSupported(); err != nil {
		return nil, err
	}

	scanResult, err := s.ScanCleanupPackages()
	if err != nil {
		systemCleanup := kernelRunSystemCleanup()
		summary := buildKernelSystemCleanupSummary(nil)
		if systemCleanup != nil {
			summary = systemCleanup.Summary
		}
		err = fmt.Errorf("%w; %s", err, summary)
		return nil, err
	}
	return s.autoCleanupPackagesFromScanResult(scanResult)
}

func (s *KernelManagerService) autoCleanupPackagesFromScanResult(scanResult *KernelCleanupScanResponse) (result *KernelCleanupPurgeResult, err error) {
	if scanResult == nil {
		scanResult = &KernelCleanupScanResponse{}
	}
	targets := buildAutoCleanupPurgeTargets(scanResult.Packages, scanResult.CurrentKernel)
	if len(targets) == 0 {
		result = &KernelCleanupPurgeResult{
			Requested:   []string{},
			Command:     "",
			NeedsReboot: kernelRebootHintRequired(),
			Succeeded:   []string{},
			Failed:      []string{},
			Message:     "no packages matched auto cleanup policy",
		}
		applyKernelSystemCleanupResultPurge(result, kernelRunSystemCleanup())
		return result, nil
	}
	return s.PurgePackages(targets)
}

func (s *KernelManagerService) InstallDownloadedPackages(provider, line, version, arch string) (result *KernelInstallResult, err error) {
	if err := kernelEnsureRuntimeSupported(); err != nil {
		return nil, err
	}

	result = &KernelInstallResult{
		NeedsReboot:      kernelRebootHintRequired(),
		InstalledPackage: []string{},
	}
	defer func() {
		systemCleanup := kernelRunSystemCleanup()
		if result != nil {
			applyKernelSystemCleanupResultInstall(result, systemCleanup)
		}
		if err != nil {
			summary := buildKernelSystemCleanupSummary(nil)
			if systemCleanup != nil {
				summary = systemCleanup.Summary
			}
			err = fmt.Errorf("%w; %s", err, summary)
		}
	}()

	pkgs, getErr := s.GetPackages(provider, line, version, arch)
	if getErr != nil {
		err = getErr
		return result, err
	}
	installDir := strings.TrimSpace(pkgs.Directory)
	imageDeb, headersDeb, findErr := findKernelInstallDebPair(installDir)
	if findErr != nil {
		err = findErr
		return result, err
	}

	dpkgPath, lookErr := exec.LookPath("dpkg")
	if lookErr != nil {
		err = fmt.Errorf("dpkg command not found: %w", lookErr)
		return result, err
	}

	sudoPath, _ := exec.LookPath("sudo")
	commandArgs := buildKernelInstallCommand(dpkgPath, sudoPath, os.Geteuid(), imageDeb, headersDeb)
	if len(commandArgs) == 0 {
		err = fmt.Errorf("failed to build install command")
		return result, err
	}

	imagePackage := kernelDebPackageName(imageDeb)
	headersPackage := kernelDebPackageName(headersDeb)
	result.Command = strings.Join(commandArgs, " ")
	result.InstalledPackage = []string{imagePackage, headersPackage}

	if err = runCommandWithTimeout(40*time.Minute, commandArgs[0], commandArgs[1:]...); err != nil {
		err = fmt.Errorf("kernel package install failed: %w", err)
		return result, err
	}

	installedImage := isKernelPackageInstalled(imagePackage)
	installedHeaders := isKernelPackageInstalled(headersPackage)

	installed := installedImage && installedHeaders
	result.Installed = installed
	result.NeedsReboot = installed || kernelRebootHintRequired()

	if installed {
		pinnedKernel := extractKernelIDFromPackage(imagePackage)
		if pinnedKernel != "" {
			if markerErr := s.SetPinnedKernel(pinnedKernel); markerErr == nil {
				result.PinnedKernel = pinnedKernel
				result.PinnedUpdated = true
			} else {
				result.CleanupWarning = joinKernelWarnings(result.CleanupWarning, "save pinned kernel marker failed: "+markerErr.Error())
			}
		}
		if cleanupErr := s.autoCleanupAfterInstall(installDir); cleanupErr != nil {
			result.CleanupDone = false
			result.CleanupWarning = joinKernelWarnings(result.CleanupWarning, cleanupErr.Error())
		} else {
			result.CleanupDone = true
			if markerErr := s.clearDownloadedMarker(); markerErr != nil {
				result.CleanupWarning = joinKernelWarnings(result.CleanupWarning, "clear downloaded marker failed: "+markerErr.Error())
			}
		}
	} else {
		if pinnedKernel, markerErr := s.GetPinnedKernel(); markerErr == nil {
			result.PinnedKernel = pinnedKernel
		}
	}

	return result, nil
}

func (s *KernelManagerService) autoCleanupAfterInstall(downloadDir string) error {
	if err := s.cleanupKernelFailedDownloadArtifacts(downloadDir); err != nil {
		return err
	}
	return nil
}

func (s *KernelManagerService) cleanupAllKernelDownloadArtifacts() (string, error) {
	root := s.getKernelDownloadRoot("")
	if err := s.removeKernelDownloadPath(root); err != nil {
		return root, err
	}
	if err := s.clearDownloadedMarker(); err != nil {
		return root, err
	}
	return root, nil
}

func (s *KernelManagerService) clearDownloadedMarkerIfWithinRoot(targetPath string) error {
	trimmed := strings.TrimSpace(targetPath)
	if trimmed == "" {
		return nil
	}
	if database.GetDB() == nil {
		return nil
	}
	marker, err := s.loadDownloadedMarker()
	if err != nil {
		return err
	}
	if marker == nil || strings.TrimSpace(marker.Directory) == "" {
		return nil
	}
	markerDir, err := filepath.Abs(marker.Directory)
	if err != nil {
		return err
	}
	absTarget, err := filepath.Abs(trimmed)
	if err != nil {
		return err
	}
	if filepath.Clean(markerDir) != filepath.Clean(absTarget) {
		return nil
	}
	return s.clearDownloadedMarker()
}

func runKernelAutoRemoveAndClean() error {
	aptCommand, err := kernelResolveAptCommand()
	if err != nil {
		return err
	}
	if err = kernelRunPrivilegedCommand(20*time.Minute, aptCommand, "autoremove", "-y"); err != nil {
		return fmt.Errorf("auto cleanup command failed (autoremove): %w", err)
	}
	if err = kernelRunPrivilegedCommand(10*time.Minute, aptCommand, "clean"); err != nil {
		return fmt.Errorf("auto cleanup command failed (clean): %w", err)
	}
	return nil
}

func resolveKernelAptCommand() (string, error) {
	if aptGetPath, err := exec.LookPath("apt-get"); err == nil {
		return aptGetPath, nil
	}
	if aptPath, err := exec.LookPath("apt"); err == nil {
		return aptPath, nil
	}
	return "", fmt.Errorf("apt or apt-get command not found")
}

func runKernelPrivilegedCommand(timeout time.Duration, command string, args ...string) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return fmt.Errorf("command is empty")
	}
	if os.Geteuid() != 0 {
		if sudoPath, err := exec.LookPath("sudo"); err == nil && strings.TrimSpace(sudoPath) != "" {
			sudoArgs := make([]string, 0, len(args)+2)
			sudoArgs = append(sudoArgs, "-n", command)
			sudoArgs = append(sudoArgs, args...)
			return runCommandWithTimeout(timeout, sudoPath, sudoArgs...)
		}
	}
	return runCommandWithTimeout(timeout, command, args...)
}

func (s *KernelManagerService) removeKernelDownloadPath(targetPath string) error {
	rootPath := strings.TrimSpace(targetPath)
	if rootPath == "" {
		return fmt.Errorf("kernel download path is empty")
	}

	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		return fmt.Errorf("resolve kernel download path failed: %w", err)
	}
	absDataDir, err := filepath.Abs(config.GetDataDir())
	if err != nil {
		return fmt.Errorf("resolve data dir failed: %w", err)
	}

	relToDataDir, err := filepath.Rel(absDataDir, absRoot)
	if err != nil {
		return fmt.Errorf("resolve cleanup relative path failed: %w", err)
	}
	relToDataDir = filepath.Clean(relToDataDir)
	if relToDataDir == "." || relToDataDir == "" {
		return fmt.Errorf("refuse to remove data root directly: %s", absDataDir)
	}
	if relToDataDir == ".." || strings.HasPrefix(relToDataDir, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("refuse to remove path outside data dir: %s", absRoot)
	}
	if relToDataDir != "kernel" && !strings.HasPrefix(relToDataDir, "kernel"+string(os.PathSeparator)) {
		return fmt.Errorf("refuse to remove non-kernel path: %s", absRoot)
	}

	if _, statErr := os.Stat(absRoot); statErr != nil {
		if errors.Is(statErr, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("stat kernel download path failed: %w", statErr)
	}
	if err = os.RemoveAll(absRoot); err != nil {
		return fmt.Errorf("remove kernel download path failed: %w", err)
	}
	return nil
}

func (s *KernelManagerService) RebootSystem() error {
	if err := kernelEnsureRuntimeSupported(); err != nil {
		return err
	}

	rebootPath, err := exec.LookPath("reboot")
	if err != nil {
		return fmt.Errorf("reboot command not found: %w", err)
	}

	sudoPath, _ := exec.LookPath("sudo")
	if os.Geteuid() != 0 && strings.TrimSpace(sudoPath) != "" {
		return runCommandWithTimeout(12*time.Second, sudoPath, "-n", rebootPath)
	}
	return runCommandWithTimeout(12*time.Second, rebootPath)
}

func (s *KernelManagerService) resolveKernelArchDirectory(provider, line, version, arch string) (string, error) {
	normalizedProvider, err := normalizeKernelProvider(provider)
	if err != nil {
		return "", err
	}
	if normalizedProvider == kernelProviderBBRPlus {
		normalizedArch, err := normalizeKernelProviderArch(normalizedProvider, arch)
		if err != nil {
			return "", err
		}
		return normalizedArch, nil
	}

	arches, err := s.GetArches(kernelProviderXanMod, line, version)
	if err != nil {
		return "", err
	}
	for _, item := range arches.Arches {
		if item.Arch == arch {
			return item.DirName, nil
		}
	}
	return "", fmt.Errorf("architecture %s not available for %s/%s", arch, line, version)
}

func (s *KernelManagerService) getKernelDownloadRoot(provider string) string {
	normalizedProvider, err := normalizeKernelProvider(provider)
	if err != nil {
		normalizedProvider = kernelProviderXanMod
	}
	if normalizedProvider == kernelProviderBBRPlus {
		return filepath.Join(config.GetDataDir(), "kernel", kernelProviderBBRPlus)
	}
	return filepath.Join(config.GetDataDir(), "kernel")
}

func (s *KernelManagerService) getKernelFailedCleanupRoot() string {
	return filepath.Join(s.getKernelDownloadRoot(""), kernelFailedCleanupDirName)
}

func (s *KernelManagerService) findLegacyDownloadedKernelStatus() (*KernelDownloadedStatus, error) {
	root := s.getKernelDownloadRoot("")
	candidates, err := s.scanKernelDownloadedDirectories(root)
	if err != nil {
		return &KernelDownloadedStatus{Exists: false}, nil
	}
	if len(candidates) == 0 {
		return &KernelDownloadedStatus{Exists: false}, nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		return len(strings.Split(filepath.Clean(candidates[i]), string(os.PathSeparator))) >
			len(strings.Split(filepath.Clean(candidates[j]), string(os.PathSeparator)))
	})
	targetDir := candidates[0]
	marker := buildKernelDownloadedMarkerFromPath(targetDir)
	if marker == nil {
		return &KernelDownloadedStatus{Exists: false}, nil
	}
	_ = s.saveDownloadedMarker(*marker)
	return buildKernelDownloadedStatus(marker), nil
}

func (s *KernelManagerService) scanKernelDownloadedDirectories(root string) ([]string, error) {
	absRoot, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if info == nil || !info.IsDir() {
		return nil, nil
	}

	cleanupRoot := filepath.Clean(filepath.Join(absRoot, kernelFailedCleanupDirName))
	matches := make([]string, 0, 4)
	err = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d == nil || !d.IsDir() {
			return nil
		}
		cleanPath := filepath.Clean(path)
		if cleanPath == absRoot {
			return nil
		}
		if isPathWithinRoot(cleanPath, cleanupRoot) {
			if cleanPath == cleanupRoot {
				return nil
			}
			if kernelDownloadCleanupStore.isProtected(cleanPath) {
				return filepath.SkipDir
			}
			_ = s.removeKernelDownloadPath(cleanPath)
			return filepath.SkipDir
		}
		if kernelDownloadCleanupStore.isProtected(cleanPath) {
			return filepath.SkipDir
		}
		if validateKernelDownloadedPair(cleanPath) == nil {
			matches = append(matches, cleanPath)
			return filepath.SkipDir
		}
		if hasAnyKernelArtifacts(cleanPath) {
			_ = s.removeKernelDownloadPath(cleanPath)
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return matches, nil
}

func isPathWithinRoot(path string, root string) bool {
	cleanPath := filepath.Clean(strings.TrimSpace(path))
	cleanRoot := filepath.Clean(strings.TrimSpace(root))
	if cleanPath == "" || cleanRoot == "" {
		return false
	}
	if cleanPath == cleanRoot {
		return true
	}
	rel, err := filepath.Rel(cleanRoot, cleanPath)
	if err != nil {
		return false
	}
	rel = filepath.Clean(rel)
	if rel == "." || rel == "" {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func hasAnyKernelArtifacts(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			continue
		}
		lower := strings.ToLower(strings.TrimSpace(name))
		if kernelPackagePattern.MatchString(name) || strings.HasSuffix(lower, ".deb.tmp") || strings.HasSuffix(lower, ".tmp") {
			return true
		}
	}
	return false
}

func buildKernelFailedCleanupDirName(downloadDir string) string {
	base := filepath.Base(filepath.Clean(strings.TrimSpace(downloadDir)))
	base = sanitizeKernelCleanupPathSegment(base)
	if base == "" {
		base = "kernel"
	}
	token := strings.TrimPrefix(normalizeKernelDownloadSessionID(""), "kernel-")
	return "failed-" + base + "-" + token
}

func sanitizeKernelCleanupPathSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var builder strings.Builder
	builder.Grow(len(value))
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			builder.WriteRune(r)
		default:
			builder.WriteByte('_')
		}
	}
	return strings.Trim(builder.String(), "._-")
}

func buildKernelDownloadedMarkerFromPath(path string) *kernelDownloadedMarker {
	cleaned := filepath.Clean(strings.TrimSpace(path))
	if cleaned == "" {
		return nil
	}
	parts := strings.Split(cleaned, string(os.PathSeparator))
	if len(parts) < 2 {
		return nil
	}
	for i := range parts {
		if parts[i] != "kernel" {
			continue
		}
		rest := parts[i+1:]
		if len(rest) >= 3 {
			provider := kernelProviderXanMod
			line := rest[0]
			version := rest[1]
			arch := rest[2]
			if rest[0] == kernelProviderBBRPlus {
				provider = kernelProviderBBRPlus
				line = ""
				version = rest[1]
				arch = rest[2]
			}
			return &kernelDownloadedMarker{
				Provider:   provider,
				Line:       line,
				Version:    version,
				Arch:       arch,
				Directory:  cleaned,
				Downloaded: time.Now().Unix(),
			}
		}
	}
	return nil
}

func buildKernelDownloadedStatus(marker *kernelDownloadedMarker) *KernelDownloadedStatus {
	if marker == nil {
		return &KernelDownloadedStatus{Exists: false}
	}
	display := marker.Version
	if marker.Provider == kernelProviderXanMod {
		line := strings.TrimSpace(marker.Line)
		if line != "" {
			display = line + "/" + marker.Version
		}
		if strings.TrimSpace(marker.Arch) != "" {
			display = display + "/" + marker.Arch
		}
	} else if marker.Provider == kernelProviderBBRPlus {
		if strings.TrimSpace(marker.Arch) != "" {
			display = marker.Version + "/" + marker.Arch
		}
	}
	return &KernelDownloadedStatus{
		Exists:    true,
		Provider:  marker.Provider,
		Line:      marker.Line,
		Version:   marker.Version,
		Arch:      marker.Arch,
		Directory: marker.Directory,
		Display:   display,
	}
}

func kernelPackageType(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	if strings.Contains(lower, "linux-image-") {
		return "image"
	}
	if strings.Contains(lower, "linux-headers-") {
		return "headers"
	}
	return "unknown"
}

func kernelDebPackageName(path string) string {
	base := filepath.Base(strings.TrimSpace(path))
	lower := strings.ToLower(base)
	start := strings.Index(lower, "linux-image-")
	if start < 0 {
		start = strings.Index(lower, "linux-headers-")
	}
	if start >= 0 {
		trimmed := base[start:]
		if idx := strings.Index(trimmed, "_"); idx > 0 {
			return strings.TrimSpace(trimmed[:idx])
		}
		return strings.TrimSpace(trimmed)
	}
	parts := strings.SplitN(base, "_", 2)
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}

func kernelDebKernelID(path string) string {
	pkgName := kernelDebPackageName(path)
	if pkgName == "" {
		return ""
	}
	return extractKernelIDFromPackage(pkgName)
}

func isKernelPackageInstalled(pkg string) bool {
	pkg = strings.TrimSpace(pkg)
	if pkg == "" {
		return false
	}
	output, err := runKernelCommandOutput(10*time.Second, "dpkg-query", "-W", "-f=${Status}", pkg)
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(output), "install ok installed")
}

func kernelRebootHintRequired() bool {
	_, err := os.Stat("/var/run/reboot-required")
	return err == nil
}

func findKernelInstallDebPair(dir string) (string, string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", "", fmt.Errorf("kernel download directory not found: %s", dir)
		}
		return "", "", err
	}

	imagesByID := make(map[string][]string, 2)
	headersByID := make(map[string][]string, 2)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !kernelPackagePattern.MatchString(name) {
			continue
		}
		fullPath := filepath.Join(dir, name)
		kernelID := kernelDebKernelID(fullPath)
		if kernelID == "" {
			continue
		}
		switch kernelPackageType(name) {
		case "image":
			imagesByID[kernelID] = append(imagesByID[kernelID], fullPath)
		case "headers":
			headersByID[kernelID] = append(headersByID[kernelID], fullPath)
		}
	}

	commonIDs := make([]string, 0, 2)
	for kernelID := range imagesByID {
		if len(headersByID[kernelID]) > 0 {
			commonIDs = append(commonIDs, kernelID)
		}
	}
	if len(commonIDs) == 0 {
		return "", "", fmt.Errorf("linux-image and linux-headers deb packages are required in %s", dir)
	}
	if len(commonIDs) > 1 {
		sort.Strings(commonIDs)
		return "", "", fmt.Errorf("multiple kernel package pairs found in %s: %s", dir, strings.Join(commonIDs, ", "))
	}

	kernelID := commonIDs[0]
	images := imagesByID[kernelID]
	headers := headersByID[kernelID]
	sort.Strings(images)
	sort.Strings(headers)
	return images[len(images)-1], headers[len(headers)-1], nil
}

func buildKernelInstallCommand(dpkgPath, sudoPath string, euid int, imageDeb, headersDeb string) []string {
	dpkgPath = strings.TrimSpace(dpkgPath)
	imageDeb = strings.TrimSpace(imageDeb)
	headersDeb = strings.TrimSpace(headersDeb)
	if dpkgPath == "" || imageDeb == "" || headersDeb == "" {
		return nil
	}

	if euid != 0 && strings.TrimSpace(sudoPath) != "" {
		return []string{sudoPath, "-n", dpkgPath, "-i", imageDeb, headersDeb}
	}
	return []string{dpkgPath, "-i", imageDeb, headersDeb}
}

func buildKernelPurgeCommand(aptCommand string, packages []string) []string {
	aptCommand = strings.TrimSpace(aptCommand)
	if aptCommand == "" || len(packages) == 0 {
		return nil
	}
	args := make([]string, 0, len(packages)+3)
	args = append(args, aptCommand, "purge", "-y")
	args = append(args, packages...)
	return args
}

func normalizeKernelPurgeTargets(packages []string) ([]string, error) {
	targets := make([]string, 0, len(packages))
	seen := make(map[string]struct{}, len(packages))
	for _, raw := range packages {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if !kernelAptNamePattern.MatchString(name) {
			return nil, fmt.Errorf("invalid package name: %s", raw)
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		targets = append(targets, name)
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("at least one package is required")
	}
	return targets, nil
}

func parseKernelSelections(raw string) []kernelSelectionEntry {
	lines := strings.Split(raw, "\n")
	items := make([]kernelSelectionEntry, 0, len(lines))
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" || !strings.Contains(strings.ToLower(trimmedLine), "linux") {
			continue
		}
		fields := strings.Fields(trimmedLine)
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimSpace(fields[0])
		status := strings.TrimSpace(fields[len(fields)-1])
		if name == "" {
			continue
		}
		items = append(items, kernelSelectionEntry{
			Name:   name,
			Status: status,
		})
	}
	return items
}

func buildKernelCleanupPackageItems(entries []kernelSelectionEntry, currentKernel, pinnedKernel string) []KernelCleanupPackageItem {
	currentKernel = strings.TrimSpace(currentKernel)
	pinnedKernel = strings.TrimSpace(pinnedKernel)
	result := make([]KernelCleanupPackageItem, 0, len(entries))
	for _, entry := range entries {
		name := strings.TrimSpace(entry.Name)
		if name == "" {
			continue
		}
		kernelID := extractKernelIDFromPackage(name)
		isImage := strings.HasPrefix(strings.ToLower(name), "linux-image-")
		isHeaders := strings.HasPrefix(strings.ToLower(name), "linux-headers-")
		isPinnedKernel := kernelID != "" && kernelID == pinnedKernel
		isCurrentKernel := kernelID != "" && kernelID == currentKernel
		risk := "high"
		if isImage || isHeaders {
			risk = "normal"
		}
		result = append(result, KernelCleanupPackageItem{
			Name:            name,
			Status:          strings.TrimSpace(entry.Status),
			IsImage:         isImage,
			IsHeaders:       isHeaders,
			IsPinnedKernel:  isPinnedKernel,
			IsCurrentKernel: isCurrentKernel,
			Risk:            risk,
		})
	}
	return result
}

func buildAutoCleanupPurgeTargets(items []KernelCleanupPackageItem, currentKernel string) []string {
	currentKernel = strings.TrimSpace(currentKernel)
	targets := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		if !item.IsImage && !item.IsHeaders {
			continue
		}
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		kernelID := extractKernelIDFromPackage(name)
		if currentKernel != "" && kernelID == currentKernel {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		targets = append(targets, name)
	}
	return targets
}

func extractKernelIDFromPackage(pkg string) string {
	trimmed := strings.TrimSpace(pkg)
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "linux-image-") {
		return strings.TrimSpace(trimmed[len("linux-image-"):])
	}
	if strings.HasPrefix(lower, "linux-headers-") {
		return strings.TrimSpace(trimmed[len("linux-headers-"):])
	}
	return ""
}

func joinKernelWarnings(existing, next string) string {
	existing = strings.TrimSpace(existing)
	next = strings.TrimSpace(next)
	if existing == "" {
		return next
	}
	if next == "" {
		return existing
	}
	return existing + "; " + next
}

func normalizeKernelProvider(provider string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(provider))
	if normalized == "" {
		return kernelProviderXanMod, nil
	}
	switch normalized {
	case kernelProviderXanMod, kernelProviderBBRPlus:
		return normalized, nil
	default:
		return "", fmt.Errorf("unsupported kernel provider: %s", provider)
	}
}

func normalizeKernelProviderRequired(provider string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(provider))
	if normalized == "" {
		return "", fmt.Errorf("provider is required")
	}
	return normalizeKernelProvider(normalized)
}

func normalizeKernelLine(line string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(line))
	if _, ok := kernelSupportedLines[normalized]; !ok {
		return "", fmt.Errorf("unsupported kernel line: %s", line)
	}
	return normalized, nil
}

func normalizeKernelArch(arch string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(arch))
	if _, ok := kernelSupportedArches[normalized]; !ok {
		return "", fmt.Errorf("unsupported architecture: %s", arch)
	}
	return normalized, nil
}

func normalizeKernelLineVersion(line, version string) (string, string, error) {
	normalizedLine, err := normalizeKernelLine(line)
	if err != nil {
		return "", "", err
	}
	normalizedVersion := strings.TrimSpace(version)
	if normalizedVersion == "" {
		return "", "", fmt.Errorf("version is required")
	}
	return normalizedLine, normalizedVersion, nil
}

func normalizeKernelLineVersionArch(line, version, arch string) (string, string, string, error) {
	normalizedLine, normalizedVersion, err := normalizeKernelLineVersion(line, version)
	if err != nil {
		return "", "", "", err
	}
	normalizedArch, err := normalizeKernelArch(arch)
	if err != nil {
		return "", "", "", err
	}
	return normalizedLine, normalizedVersion, normalizedArch, nil
}

func normalizeKernelProviderArch(provider, arch string) (string, error) {
	normalizedProvider, err := normalizeKernelProvider(provider)
	if err != nil {
		return "", err
	}
	normalizedArch := strings.ToLower(strings.TrimSpace(arch))
	switch normalizedProvider {
	case kernelProviderBBRPlus:
		if normalizedArch == "" {
			normalizedArch = strings.ToLower(strings.TrimSpace(runtime.GOARCH))
		}
		switch normalizedArch {
		case "amd64", "x86_64", "x64":
			return "amd64", nil
		case "arm64", "aarch64":
			return "arm64", nil
		default:
			return "", fmt.Errorf("unsupported architecture: %s", arch)
		}
	default:
		return normalizeKernelArch(arch)
	}
}

func findBBRPlusReleaseEntry(version string) (bbrplusReleaseEntry, bool) {
	normalizedVersion := strings.TrimSpace(version)
	for _, item := range bbrplusReleaseCatalog {
		if item.Version == normalizedVersion {
			return item, true
		}
	}
	return bbrplusReleaseEntry{}, false
}

func buildBBRPlusReleaseURL(version, assetName string) string {
	return fmt.Sprintf("https://github.com/nicelic/bbrplus-6.x_stable/releases/download/%s/%s", strings.TrimSpace(version), strings.TrimSpace(assetName))
}

func parseKernelVersionDigits(name string) []int {
	parts := kernelVersionDigits.FindAllString(strings.TrimSpace(name), -1)
	if len(parts) == 0 {
		return nil
	}
	out := make([]int, 0, len(parts))
	for _, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil {
			continue
		}
		out = append(out, value)
	}
	return out
}

func bbrplusVersionSortPriority(version string) int {
	if priority, ok := bbrplusVersionDisplayPriority[strings.TrimSpace(version)]; ok {
		return priority
	}
	return 1
}

// compareKernelVersionNameDesc compares version-like names in descending order.
// return < 0 means left should be before right.
func compareKernelVersionNameDesc(left, right string) int {
	leftParts := parseKernelVersionDigits(left)
	rightParts := parseKernelVersionDigits(right)

	maxLen := len(leftParts)
	if len(rightParts) > maxLen {
		maxLen = len(rightParts)
	}
	for i := 0; i < maxLen; i++ {
		hasLeft := i < len(leftParts)
		hasRight := i < len(rightParts)
		if !hasLeft && !hasRight {
			break
		}
		if !hasLeft {
			return 1
		}
		if !hasRight {
			return -1
		}
		if leftParts[i] > rightParts[i] {
			return -1
		}
		if leftParts[i] < rightParts[i] {
			return 1
		}
	}

	leftTrimmed := strings.TrimSpace(left)
	rightTrimmed := strings.TrimSpace(right)
	if leftTrimmed == rightTrimmed {
		return 0
	}
	if leftTrimmed > rightTrimmed {
		return -1
	}
	return 1
}

func extractKernelArchLevel(rawName string) string {
	name := strings.ToLower(strings.TrimSpace(rawName))
	if name == "" || !kernelArchPattern.MatchString(name) {
		return ""
	}
	switch {
	case strings.Contains(name, "-x64v1-"):
		return "x64v1"
	case strings.Contains(name, "-x64v2-"):
		return "x64v2"
	case strings.Contains(name, "-x64v3-"):
		return "x64v3"
	default:
		return ""
	}
}

func ensureKernelRuntimeSupported() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("kernel management only supports linux hosts")
	}
	fields := readKernelOSReleaseFields()
	if detectKernelLinuxSystemFamily(fields) != "debian" {
		return fmt.Errorf("only Debian/Ubuntu and derivatives are supported")
	}
	return nil
}

func readKernelOSReleaseFields() map[string]string {
	paths := []string{"/etc/os-release", "/usr/lib/os-release"}
	for _, p := range paths {
		content, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		return parseOsReleaseFields(string(content))
	}
	return map[string]string{}
}

func detectKernelLinuxSystemFamily(fields map[string]string) string {
	idLike := strings.ToLower(strings.TrimSpace(fields["ID_LIKE"]))
	id := strings.ToLower(strings.TrimSpace(fields["ID"]))
	switch {
	case strings.Contains(idLike, "debian") || id == "debian" || id == "ubuntu":
		return "debian"
	case strings.Contains(idLike, "rhel") || strings.Contains(idLike, "fedora") || id == "fedora" || id == "rhel" || id == "centos" || id == "rocky" || id == "almalinux":
		return "rhel"
	case strings.Contains(idLike, "suse"):
		return "suse"
	case strings.Contains(idLike, "arch"):
		return "arch"
	default:
		return id
	}
}

func runKernelCommandOutput(timeout time.Duration, command string, args ...string) (string, error) {
	if strings.TrimSpace(command) == "" {
		return "", fmt.Errorf("command is empty")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, args...)
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("command timed out: %s %s", command, strings.Join(args, " "))
	}
	if err != nil {
		return "", fmt.Errorf("command failed (%s %s): %w: %s", command, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func fetchSourceForgeFileEntries(relativePath string) ([]sourceForgeFileEntry, error) {
	relativePath = strings.TrimSpace(relativePath)
	if relativePath == "" {
		return nil, fmt.Errorf("relative path is required")
	}
	relativePath = strings.Trim(relativePath, "/")
	url := fmt.Sprintf("%s/%s/", xanmodSourceForgeBaseURL, relativePath)

	client := &http.Client{Timeout: 45 * time.Second}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "kwor")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch sourceforge page failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sourceforge returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	jsonBlob, err := extractNetSFFilesJSON(string(body))
	if err != nil {
		return nil, err
	}

	raw := map[string]sourceForgeFileEntry{}
	if err = json.Unmarshal([]byte(jsonBlob), &raw); err != nil {
		return nil, fmt.Errorf("parse sourceforge files json failed: %w", err)
	}

	entries := make([]sourceForgeFileEntry, 0, len(raw))
	for _, entry := range raw {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name > entries[j].Name
	})
	return entries, nil
}

func extractNetSFFilesJSON(html string) (string, error) {
	const marker = "net.sf.files = "
	startMarker := strings.Index(html, marker)
	if startMarker < 0 {
		return "", fmt.Errorf("sourceforge files json marker not found")
	}
	jsonStart := strings.Index(html[startMarker:], "{")
	if jsonStart < 0 {
		return "", fmt.Errorf("sourceforge files json start not found")
	}
	jsonStart += startMarker

	depth := 0
	inString := false
	escaped := false
	jsonEnd := -1
	for i := jsonStart; i < len(html); i++ {
		ch := html[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}

		if ch == '"' {
			inString = true
			continue
		}
		if ch == '{' {
			depth++
			continue
		}
		if ch == '}' {
			depth--
			if depth == 0 {
				jsonEnd = i
				break
			}
		}
	}

	if jsonEnd < 0 || jsonEnd <= jsonStart {
		return "", fmt.Errorf("sourceforge files json end not found")
	}
	return html[jsonStart : jsonEnd+1], nil
}

func probeRemoteContentLength(url string) (int64, bool) {
	url = strings.TrimSpace(url)
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		return 0, false
	}

	client := &http.Client{Timeout: 20 * time.Second}
	req, err := http.NewRequest(http.MethodHead, url, nil)
	if err != nil {
		return 0, false
	}
	req.Header.Set("User-Agent", "kwor")

	resp, err := client.Do(req)
	if err != nil {
		return 0, false
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return 0, false
	}
	if resp.ContentLength <= 0 {
		return 0, false
	}
	return resp.ContentLength, true
}

func downloadFileToPath(url, targetPath string, onProgress func(delta int64)) error {
	url = strings.TrimSpace(url)
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		return fmt.Errorf("invalid download url: %s", url)
	}
	targetPath = strings.TrimSpace(targetPath)
	if targetPath == "" {
		return fmt.Errorf("target path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}

	client := &http.Client{Timeout: 10 * time.Minute}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "kwor")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed, HTTP %d", resp.StatusCode)
	}

	tmpPath := targetPath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	buf := make([]byte, 32*1024)
	for {
		readCount, readErr := resp.Body.Read(buf)
		if readCount > 0 {
			writtenCount, writeErr := out.Write(buf[:readCount])
			if writeErr != nil {
				_ = out.Close()
				_ = os.Remove(tmpPath)
				return writeErr
			}
			if writtenCount != readCount {
				_ = out.Close()
				_ = os.Remove(tmpPath)
				return io.ErrShortWrite
			}
			if onProgress != nil {
				onProgress(int64(writtenCount))
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			_ = out.Close()
			_ = os.Remove(tmpPath)
			return readErr
		}
	}
	if err = out.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	if err = os.Rename(tmpPath, targetPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func pathJoinSlash(parts ...string) string {
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.Trim(strings.TrimSpace(part), "/")
		if trimmed == "" {
			continue
		}
		items = append(items, trimmed)
	}
	return strings.Join(items, "/")
}
