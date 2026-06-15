package service

import (
	"strings"
	"testing"
	"time"
)

func TestKernelDownloadProgressLifecycle(t *testing.T) {
	store := newKernelDownloadProgressStore()
	session := store.start("kernel-download-1234abcd", 2)
	if session.id != "kernel-download-1234abcd" {
		t.Fatalf("unexpected session id: %q", session.id)
	}

	store.setTotals(session.id, 200, false)
	store.setCurrentPackage(session.id, "linux-image.deb")
	store.addDownloadedBytes(session.id, 80)
	store.incrementDownloadedCount(session.id)

	progress := store.get(session.id)
	if progress.Status != "running" {
		t.Fatalf("expected running status, got %q", progress.Status)
	}
	if progress.Percent <= 39.9 || progress.Percent >= 40.1 {
		t.Fatalf("expected ~40%% progress, got %f", progress.Percent)
	}
	if progress.DownloadedCount != 1 || progress.TotalCount != 2 {
		t.Fatalf("unexpected counts: downloaded=%d total=%d", progress.DownloadedCount, progress.TotalCount)
	}
	if progress.CurrentPackage != "linux-image.deb" {
		t.Fatalf("unexpected current package: %q", progress.CurrentPackage)
	}

	store.addDownloadedBytes(session.id, 120)
	store.incrementDownloadedCount(session.id)
	store.finishSuccess(session.id)
	progress = store.get(session.id)
	if progress.Status != "success" {
		t.Fatalf("expected success status, got %q", progress.Status)
	}
	if progress.Percent != 100 {
		t.Fatalf("expected 100%% progress after finish, got %f", progress.Percent)
	}
	if progress.CurrentPackage != "" {
		t.Fatalf("expected empty current package after finish, got %q", progress.CurrentPackage)
	}
}

func TestKernelDownloadProgressApproximate(t *testing.T) {
	store := newKernelDownloadProgressStore()
	session := store.start("kernel-download-approx", 1)
	store.setTotals(session.id, 100, true)
	store.addDownloadedBytes(session.id, 40)

	progress := store.get(session.id)
	if !progress.Approximate {
		t.Fatalf("expected approximate flag to be true")
	}
	if progress.Percent <= 39.9 || progress.Percent >= 40.1 {
		t.Fatalf("expected ~40%% progress, got %f", progress.Percent)
	}

	store.setEstimatedTotalAtLeast(session.id, 200)
	progress = store.get(session.id)
	if progress.Percent <= 19.9 || progress.Percent >= 20.1 {
		t.Fatalf("expected ~20%% progress after estimate update, got %f", progress.Percent)
	}

	store.addDownloadedBytes(session.id, 200)
	progress = store.get(session.id)
	if progress.Percent != 99 {
		t.Fatalf("expected running approximate progress to clamp at 99%%, got %f", progress.Percent)
	}

	store.finishSuccess(session.id)
	progress = store.get(session.id)
	if progress.Percent != 100 {
		t.Fatalf("expected 100%% after finish success, got %f", progress.Percent)
	}
}

func TestKernelDownloadProgressPruneAndNormalizeID(t *testing.T) {
	store := newKernelDownloadProgressStore()
	session := store.start("bad id", 1)
	if !strings.HasPrefix(session.id, "kernel-") {
		t.Fatalf("expected normalized id with kernel- prefix, got %q", session.id)
	}

	store.finishError(session.id, "boom")
	progress := store.get(session.id)
	if progress.Status != "error" {
		t.Fatalf("expected error status, got %q", progress.Status)
	}
	if progress.Error != "boom" {
		t.Fatalf("expected error text, got %q", progress.Error)
	}

	store.mu.Lock()
	store.sessions[session.id].updatedAt = time.Now().Unix() - int64(kernelDownloadProgressTTL/time.Second) - 10
	store.mu.Unlock()

	missing := store.get(session.id)
	if missing.Status != "missing" {
		t.Fatalf("expected missing status after prune, got %q", missing.Status)
	}
}
