package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestKernelAPIValidation(t *testing.T) {
	svc := &ApiService{}

	t.Run("versions line required", func(t *testing.T) {
		rec, msg := performKernelAPIGet(t, svc.GetKernelVersions, "/api/kernel-versions")
		if msg.Success {
			t.Fatalf("expected failure when line is missing")
		}
		if rec.Code != 200 || !strings.Contains(msg.Msg, "provider is required") {
			t.Fatalf("unexpected response: code=%d msg=%q", rec.Code, msg.Msg)
		}
	})

	t.Run("overview provider required", func(t *testing.T) {
		rec, msg := performKernelAPIGet(t, svc.GetKernelOverview, "/api/kernel-overview")
		if msg.Success {
			t.Fatalf("expected failure when provider is missing")
		}
		if rec.Code != 200 || !strings.Contains(msg.Msg, "provider is required") {
			t.Fatalf("unexpected response: code=%d msg=%q", rec.Code, msg.Msg)
		}
	})

	t.Run("arches version required", func(t *testing.T) {
		rec, msg := performKernelAPIGet(t, svc.GetKernelArches, "/api/kernel-arches?provider=xanmod&line=lts")
		if msg.Success {
			t.Fatalf("expected failure when version is missing")
		}
		if rec.Code != 200 || !strings.Contains(msg.Msg, "version is required") {
			t.Fatalf("unexpected response: code=%d msg=%q", rec.Code, msg.Msg)
		}
	})

	t.Run("packages arch required", func(t *testing.T) {
		rec, msg := performKernelAPIGet(t, svc.GetKernelPackages, "/api/kernel-packages?provider=xanmod&line=lts&version=6.18.27-xanmod1")
		if msg.Success {
			t.Fatalf("expected failure when arch is missing")
		}
		if rec.Code != 200 || !strings.Contains(msg.Msg, "arch is required") {
			t.Fatalf("unexpected response: code=%d msg=%q", rec.Code, msg.Msg)
		}
	})

	t.Run("bbrplus versions line optional", func(t *testing.T) {
		rec, msg := performKernelAPIGet(t, svc.GetKernelVersions, "/api/kernel-versions?provider=bbrplus")
		if !msg.Success {
			t.Fatalf("expected success for bbrplus versions without line, got code=%d msg=%q", rec.Code, msg.Msg)
		}
	})

	t.Run("bbrplus packages arch optional", func(t *testing.T) {
		rec, msg := performKernelAPIGet(t, svc.GetKernelPackages, "/api/kernel-packages?provider=bbrplus&version=6.7.9-bbrplus")
		if !msg.Success {
			t.Fatalf("expected success for bbrplus packages without arch, got code=%d msg=%q", rec.Code, msg.Msg)
		}
	})

	t.Run("download line required", func(t *testing.T) {
		rec, msg := performKernelAPIPostJSON(t, svc.DownloadKernelPackages, `{"provider":"xanmod","version":"6.18.27-xanmod1","arch":"x64v3"}`)
		if msg.Success {
			t.Fatalf("expected failure when line is missing")
		}
		if rec.Code != 200 || !strings.Contains(msg.Msg, "line is required") {
			t.Fatalf("unexpected response: code=%d msg=%q", rec.Code, msg.Msg)
		}
	})

	t.Run("install arch required", func(t *testing.T) {
		rec, msg := performKernelAPIPostJSON(t, svc.InstallKernelPackages, `{"provider":"xanmod","line":"lts","version":"6.18.27-xanmod1"}`)
		if msg.Success {
			t.Fatalf("expected failure when arch is missing")
		}
		if rec.Code != 200 || !strings.Contains(msg.Msg, "arch is required") {
			t.Fatalf("unexpected response: code=%d msg=%q", rec.Code, msg.Msg)
		}
	})

	t.Run("cleanup purge packages required", func(t *testing.T) {
		rec, msg := performKernelAPIPostJSON(t, svc.PurgeKernelCleanupPackages, `{"packages":[]}`)
		if msg.Success {
			t.Fatalf("expected failure when packages are missing")
		}
		if rec.Code != 200 || !strings.Contains(msg.Msg, "packages are required") {
			t.Fatalf("unexpected response: code=%d msg=%q", rec.Code, msg.Msg)
		}
	})

	t.Run("cleanup marker kernel required", func(t *testing.T) {
		rec, msg := performKernelAPIPostJSON(t, svc.SaveKernelCleanupMarker, `{"kernel":""}`)
		if msg.Success {
			t.Fatalf("expected failure when kernel marker is empty")
		}
		if rec.Code != 200 || !strings.Contains(msg.Msg, "kernel is required") {
			t.Fatalf("unexpected response: code=%d msg=%q", rec.Code, msg.Msg)
		}
	})

	t.Run("download progress id required", func(t *testing.T) {
		rec, msg := performKernelAPIGet(t, svc.GetKernelDownloadProgress, "/api/kernel-download-progress")
		if msg.Success {
			t.Fatalf("expected failure when progress id is missing")
		}
		if rec.Code != 200 || !strings.Contains(msg.Msg, "id is required") {
			t.Fatalf("unexpected response: code=%d msg=%q", rec.Code, msg.Msg)
		}
	})

	t.Run("download provider required", func(t *testing.T) {
		rec, msg := performKernelAPIPostJSON(t, svc.DownloadKernelPackages, `{"version":"6.18.27-xanmod1","line":"lts","arch":"x64v3"}`)
		if msg.Success {
			t.Fatalf("expected failure when provider is missing")
		}
		if rec.Code != 200 || !strings.Contains(msg.Msg, "provider is required") {
			t.Fatalf("unexpected response: code=%d msg=%q", rec.Code, msg.Msg)
		}
	})

	t.Run("install provider required", func(t *testing.T) {
		rec, msg := performKernelAPIPostJSON(t, svc.InstallKernelPackages, `{"line":"lts","version":"6.18.27-xanmod1","arch":"x64v3"}`)
		if msg.Success {
			t.Fatalf("expected failure when provider is missing")
		}
		if rec.Code != 200 || !strings.Contains(msg.Msg, "provider is required") {
			t.Fatalf("unexpected response: code=%d msg=%q", rec.Code, msg.Msg)
		}
	})
}

func performKernelAPIGet(t *testing.T, handler func(*gin.Context), url string) (*httptest.ResponseRecorder, Msg) {
	t.Helper()
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest("GET", url, nil)
	handler(ctx)
	return rec, decodeKernelAPIMessage(t, rec.Body.String())
}

func performKernelAPIPostJSON(t *testing.T, handler func(*gin.Context), body string) (*httptest.ResponseRecorder, Msg) {
	t.Helper()
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest("POST", "/api/kernel", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	handler(ctx)
	return rec, decodeKernelAPIMessage(t, rec.Body.String())
}

func decodeKernelAPIMessage(t *testing.T, raw string) Msg {
	t.Helper()
	msg := Msg{}
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatalf("decode api message failed: %v, body=%q", err, raw)
	}
	return msg
}
