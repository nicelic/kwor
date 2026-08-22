package api

import (
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/service"
	"github.com/gin-gonic/gin"
)

func TestGetCertificateListRouteReturnsCertificates(t *testing.T) {
	initCertificateListRouteTestDB(t)

	row, err := (&service.CertificateInventoryService{}).Upsert(service.CertificateUpsertPayload{
		SourceType:   service.CertificateSourceImported,
		SourceRef:    "route-test",
		MainDomain:   "route-test.example.com",
		Domains:      []string{"route-test.example.com"},
		CertPEM:      []byte("test-cert"),
		KeyPEM:       []byte("test-key"),
		FullchainPEM: []byte("test-cert"),
	})
	if err != nil {
		t.Fatalf("upsert certificate failed: %v", err)
	}

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest("GET", "/api/certificate-list", nil)
	ctx.Params = gin.Params{{Key: "getAction", Value: "certificate-list"}}

	handler := &APIHandler{}
	handler.getHandler(ctx)

	msg := decodeAPIMessage(t, rec.Body.String())
	if !msg.Success {
		t.Fatalf("expected success response, got: %s", msg.Msg)
	}

	rows, ok := msg.Obj.([]interface{})
	if !ok {
		t.Fatalf("unexpected certificate list payload: %#v", msg.Obj)
	}

	found := false
	for _, raw := range rows {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		id, ok := item["id"].(float64)
		if !ok {
			continue
		}
		if uint(id) != row.Id {
			continue
		}
		found = true
		if item["mainDomain"] != row.MainDomain {
			t.Fatalf("mainDomain mismatch: got %#v want %q", item["mainDomain"], row.MainDomain)
		}
		break
	}
	if !found {
		t.Fatalf("expected certificate id %d in list payload: %#v", row.Id, rows)
	}
}

func TestGetCertificateListRoutePaginatesAndOmitsHistoricalOutput(t *testing.T) {
	initCertificateListRouteTestDB(t)
	inventory := &service.CertificateInventoryService{}
	for index := 0; index < 3; index++ {
		if _, err := inventory.Upsert(service.CertificateUpsertPayload{
			SourceType:   service.CertificateSourceImported,
			SourceRef:    fmt.Sprintf("page-test-%d", index),
			MainDomain:   fmt.Sprintf("page-%d.example.com", index),
			Domains:      []string{fmt.Sprintf("page-%d.example.com", index)},
			CertPEM:      []byte("test-cert"),
			KeyPEM:       []byte("test-key"),
			FullchainPEM: []byte("test-cert"),
			LastOutput:   "this historical output must not be in the list payload",
		}); err != nil {
			t.Fatalf("upsert certificate %d failed: %v", index, err)
		}
	}

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest("GET", "/api/certificate-list?page=1&per_page=2&search=page-", nil)
	ctx.Params = gin.Params{{Key: "getAction", Value: "certificate-list"}}
	(&APIHandler{}).getHandler(ctx)

	msg := decodeAPIMessage(t, rec.Body.String())
	if !msg.Success {
		t.Fatalf("expected paginated success response, got: %s", msg.Msg)
	}
	payload, ok := msg.Obj.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected paginated payload: %#v", msg.Obj)
	}
	items, ok := payload["items"].([]interface{})
	if !ok || len(items) != 2 {
		t.Fatalf("unexpected paginated items: %#v", payload["items"])
	}
	if total, ok := payload["total"].(float64); !ok || total != 3 {
		t.Fatalf("unexpected paginated total: %#v", payload["total"])
	}
	for _, raw := range items {
		item, ok := raw.(map[string]interface{})
		if !ok {
			t.Fatalf("unexpected paginated item: %#v", raw)
		}
		if _, exists := item["lastOutput"]; exists {
			t.Fatalf("historical output must not be included in paginated list: %#v", item)
		}
	}
}

func TestGetTLSCertificateOptionsRouteUsesCompactPayload(t *testing.T) {
	initCertificateListRouteTestDB(t)
	row, err := (&service.CertificateInventoryService{}).Upsert(service.CertificateUpsertPayload{
		SourceType:   service.CertificateSourceImported,
		SourceRef:    "option-route-test",
		MainDomain:   "option-route.example.com",
		Domains:      []string{"option-route.example.com", "alt.option-route.example.com"},
		CertPEM:      []byte("test-cert"),
		KeyPEM:       []byte("test-key"),
		FullchainPEM: []byte("test-cert"),
	})
	if err != nil {
		t.Fatalf("upsert certificate failed: %v", err)
	}

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest("GET", "/api/certificate-options", nil)
	ctx.Params = gin.Params{{Key: "getAction", Value: "certificate-options"}}
	(&APIHandler{}).getHandler(ctx)

	msg := decodeAPIMessage(t, rec.Body.String())
	if !msg.Success {
		t.Fatalf("expected options response, got: %s", msg.Msg)
	}
	items, ok := msg.Obj.([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("unexpected options payload: %#v", msg.Obj)
	}
	item, ok := items[0].(map[string]interface{})
	if !ok || uint(item["id"].(float64)) != row.Id {
		t.Fatalf("unexpected certificate option: %#v", item)
	}
	if _, exists := item["usageLabel"]; exists {
		t.Fatalf("TLS certificate options must not include usage aggregation: %#v", item)
	}
}

func TestGetAcmeCertificateLogRouteReturnsRequestedOutput(t *testing.T) {
	initCertificateListRouteTestDB(t)
	row, err := (&service.CertificateInventoryService{}).Upsert(service.CertificateUpsertPayload{
		SourceType:   service.CertificateSourceImported,
		SourceRef:    "log-route-test",
		MainDomain:   "log-route.example.com",
		Domains:      []string{"log-route.example.com"},
		CertPEM:      []byte("test-cert"),
		KeyPEM:       []byte("test-key"),
		FullchainPEM: []byte("test-cert"),
		LastOutput:   "requested historical output",
	})
	if err != nil {
		t.Fatalf("upsert certificate failed: %v", err)
	}

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest("GET", fmt.Sprintf("/api/acme-certificate-log?id=%d", row.Id), nil)
	ctx.Params = gin.Params{{Key: "getAction", Value: "acme-certificate-log"}}
	(&APIHandler{}).getHandler(ctx)

	msg := decodeAPIMessage(t, rec.Body.String())
	if !msg.Success {
		t.Fatalf("expected certificate log response, got: %s", msg.Msg)
	}
	payload, ok := msg.Obj.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected certificate log payload: %#v", msg.Obj)
	}
	if payload["lastOutput"] != "requested historical output" {
		t.Fatalf("unexpected historical output: %#v", payload["lastOutput"])
	}
}

func initCertificateListRouteTestDB(t *testing.T) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "certificate-list-route.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	if sqlDB, err := database.GetDB().DB(); err == nil && sqlDB != nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}
}
