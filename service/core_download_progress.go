package service

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"sync"
	"time"
)

const (
	coreDownloadProgressTTL = 30 * time.Minute

	coreDownloadStatusRunning = "running"
	coreDownloadStatusSuccess = "success"
	coreDownloadStatusError   = "error"
	coreDownloadStatusMissing = "missing"

	coreDownloadStageStopping    = "stopping"
	coreDownloadStageDownloading = "downloading"
	coreDownloadStageReplacing   = "replacing"
	coreDownloadStageValidating  = "validating"
	coreDownloadStageStarting    = "starting"
	coreDownloadStageStarted     = "started"
	coreDownloadStageCompleted   = "completed"
)

type CoreDownloadProgress struct {
	ID              string  `json:"id"`
	Core            string  `json:"core"`
	Status          string  `json:"status"`
	Stage           string  `json:"stage"`
	RunningBefore   bool    `json:"runningBefore"`
	Percent         float64 `json:"percent"`
	Approximate     bool    `json:"approximate"`
	DownloadedBytes int64   `json:"downloadedBytes"`
	TotalBytes      int64   `json:"totalBytes"`
	Error           string  `json:"error,omitempty"`
	StartedAt       int64   `json:"startedAt"`
	UpdatedAt       int64   `json:"updatedAt"`
	FinishedAt      int64   `json:"finishedAt,omitempty"`
}

type coreDownloadProgressStore struct {
	mu       sync.Mutex
	sessions map[string]*coreDownloadProgressSession
}

type coreDownloadProgressSession struct {
	id              string
	core            string
	status          string
	stage           string
	runningBefore   bool
	percent         float64
	approximate     bool
	downloadedBytes int64
	totalBytes      int64
	errText         string
	startedAt       int64
	updatedAt       int64
	finishedAt      int64
}

var sharedCoreDownloadProgressStore = newCoreDownloadProgressStore()

func newCoreDownloadProgressStore() *coreDownloadProgressStore {
	return &coreDownloadProgressStore{
		sessions: make(map[string]*coreDownloadProgressSession),
	}
}

func (s *coreDownloadProgressStore) start(coreName string, requestedID string, runningBefore bool) string {
	now := time.Now().Unix()
	sessionID := normalizeCoreDownloadProgressSessionID(requestedID)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(now)

	s.sessions[sessionID] = &coreDownloadProgressSession{
		id:            sessionID,
		core:          strings.TrimSpace(coreName),
		status:        coreDownloadStatusRunning,
		runningBefore: runningBefore,
		startedAt:     now,
		updatedAt:     now,
	}
	return sessionID
}

func (s *coreDownloadProgressStore) get(id string) *CoreDownloadProgress {
	now := time.Now().Unix()
	trimmed := strings.TrimSpace(id)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(now)

	session := s.sessions[trimmed]
	if session == nil {
		return &CoreDownloadProgress{
			ID:        trimmed,
			Status:    coreDownloadStatusMissing,
			StartedAt: now,
			UpdatedAt: now,
		}
	}
	return session.snapshotLocked()
}

func (s *coreDownloadProgressStore) setStage(id string, stage string) {
	now := time.Now().Unix()

	s.mu.Lock()
	defer s.mu.Unlock()

	session := s.sessions[id]
	if session == nil {
		return
	}
	session.stage = strings.ToLower(strings.TrimSpace(stage))
	session.updatedAt = now
}

func (s *coreDownloadProgressStore) setTotals(id string, totalBytes int64, approximate bool) {
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

func (s *coreDownloadProgressStore) addDownloadedBytes(id string, delta int64) {
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

func (s *coreDownloadProgressStore) finishSuccess(id string, finalStage string) {
	now := time.Now().Unix()

	s.mu.Lock()
	defer s.mu.Unlock()

	session := s.sessions[id]
	if session == nil {
		return
	}
	session.status = coreDownloadStatusSuccess
	session.stage = strings.ToLower(strings.TrimSpace(finalStage))
	if session.totalBytes <= 0 || session.totalBytes < session.downloadedBytes {
		session.totalBytes = session.downloadedBytes
	}
	session.percent = 100
	session.updatedAt = now
	session.finishedAt = now
}

func (s *coreDownloadProgressStore) finishError(id string, stage string, message string) {
	now := time.Now().Unix()

	s.mu.Lock()
	defer s.mu.Unlock()

	session := s.sessions[id]
	if session == nil {
		return
	}
	session.status = coreDownloadStatusError
	if trimmedStage := strings.ToLower(strings.TrimSpace(stage)); trimmedStage != "" {
		session.stage = trimmedStage
	}
	session.errText = strings.TrimSpace(message)
	session.updatedAt = now
	session.finishedAt = now
	session.recalculatePercentLocked()
}

func (s *coreDownloadProgressStore) pruneLocked(now int64) {
	ttlSeconds := int64(coreDownloadProgressTTL / time.Second)
	for id, session := range s.sessions {
		if now-session.updatedAt > ttlSeconds {
			delete(s.sessions, id)
		}
	}
}

func (s *coreDownloadProgressSession) recalculatePercentLocked() {
	if s.totalBytes > 0 {
		percent := float64(s.downloadedBytes) * 100 / float64(s.totalBytes)
		if percent < 0 {
			percent = 0
		}
		if s.status == coreDownloadStatusRunning && s.approximate && percent >= 100 {
			percent = 99
		}
		if percent > 100 {
			percent = 100
		}
		s.percent = percent
		return
	}
	if s.status == coreDownloadStatusSuccess {
		s.percent = 100
		return
	}
	s.percent = 0
}

func (s *coreDownloadProgressSession) snapshotLocked() *CoreDownloadProgress {
	return &CoreDownloadProgress{
		ID:              s.id,
		Core:            s.core,
		Status:          s.status,
		Stage:           s.stage,
		RunningBefore:   s.runningBefore,
		Percent:         s.percent,
		Approximate:     s.approximate,
		DownloadedBytes: s.downloadedBytes,
		TotalBytes:      s.totalBytes,
		Error:           s.errText,
		StartedAt:       s.startedAt,
		UpdatedAt:       s.updatedAt,
		FinishedAt:      s.finishedAt,
	}
}

func normalizeCoreDownloadProgressSessionID(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed != "" {
		return trimmed
	}

	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err == nil {
		return "core-download-" + hex.EncodeToString(buf)
	}
	return "core-download-" + strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "")
}

func StartCoreDownloadProgressSession(coreName string, requestedID string, runningBefore bool) string {
	return sharedCoreDownloadProgressStore.start(coreName, requestedID, runningBefore)
}

func GetCoreDownloadProgress(id string) *CoreDownloadProgress {
	return sharedCoreDownloadProgressStore.get(id)
}

func SetCoreDownloadProgressStage(id string, stage string) {
	sharedCoreDownloadProgressStore.setStage(id, stage)
}

func SetCoreDownloadProgressTotals(id string, totalBytes int64, approximate bool) {
	sharedCoreDownloadProgressStore.setTotals(id, totalBytes, approximate)
}

func AddCoreDownloadProgressBytes(id string, delta int64) {
	sharedCoreDownloadProgressStore.addDownloadedBytes(id, delta)
}

func FinishCoreDownloadProgressSuccess(id string, finalStage string) {
	sharedCoreDownloadProgressStore.finishSuccess(id, finalStage)
}

func FinishCoreDownloadProgressError(id string, stage string, message string) {
	sharedCoreDownloadProgressStore.finishError(id, stage, message)
}

type coreDownloadProgressWriter struct {
	sessionID string
}

func (w *coreDownloadProgressWriter) Write(p []byte) (int, error) {
	AddCoreDownloadProgressBytes(w.sessionID, int64(len(p)))
	return len(p), nil
}
