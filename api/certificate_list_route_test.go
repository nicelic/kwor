package api

import (
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
