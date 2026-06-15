package api

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"strconv"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/service"
	"github.com/gin-gonic/gin"
)

func TestApplyAcmeCertificateRouteAppendsAndReorders(t *testing.T) {
	initAcmeRouteTestDB(t)

	first := upsertAcmeRouteTestCertificateRecord(t, "route-apply-1")
	second := upsertAcmeRouteTestCertificateRecord(t, "route-apply-2")

	svc := &ApiService{}
	performAcmeRouteJSONPost(t, svc.ApplyAcmeCertificate, `{"id":`+uintJSON(first.Id)+`,"target":"panel"}`)
	performAcmeRouteJSONPost(t, svc.ApplyAcmeCertificate, `{"id":`+uintJSON(second.Id)+`,"target":"panel"}`)
	performAcmeRouteJSONPost(t, svc.ApplyAcmeCertificate, `{"id":`+uintJSON(first.Id)+`,"target":"panel"}`)

	ids, err := service.GetAssignedCertificateRecordIDs(&service.SettingService{}, service.PanelSelfSignedTargetPanel)
	if err != nil {
		t.Fatalf("read assigned ids failed: %v", err)
	}
	if len(ids) != 2 || ids[0] != first.Id || ids[1] != second.Id {
		t.Fatalf("route apply ordering mismatch: got=%v want=[%d %d]", ids, first.Id, second.Id)
	}
}

func TestUnapplyAcmeCertificateRouteRejectsLastCertificate(t *testing.T) {
	initAcmeRouteTestDB(t)

	only := upsertAcmeRouteTestCertificateRecord(t, "route-unapply-last")
	if err := service.SetAssignedCertificateRecordIDs(&service.SettingService{}, service.PanelSelfSignedTargetPanel, []uint{only.Id}); err != nil {
		t.Fatalf("seed assigned ids failed: %v", err)
	}

	svc := &ApiService{}
	rec, msg := performAcmeRouteJSONPost(t, svc.UnapplyAcmeCertificate, `{"id":`+uintJSON(only.Id)+`,"target":"panel"}`)
	if msg.Success {
		t.Fatalf("expected unapply last certificate to fail, got success response: %#v", msg)
	}
	if !strings.Contains(msg.Msg, "at least one certificate must remain for target") {
		t.Fatalf("unexpected unapply error: %q", msg.Msg)
	}
	if rec.Code != 200 {
		t.Fatalf("unexpected HTTP status code: %d", rec.Code)
	}

	ids, err := service.GetAssignedCertificateRecordIDs(&service.SettingService{}, service.PanelSelfSignedTargetPanel)
	if err != nil {
		t.Fatalf("read assigned ids failed: %v", err)
	}
	if len(ids) != 1 || ids[0] != only.Id {
		t.Fatalf("last-certificate rejection should not change bindings: got=%v want=[%d]", ids, only.Id)
	}
}

func performAcmeRouteJSONPost(t *testing.T, handler func(*gin.Context), body string) (*httptest.ResponseRecorder, Msg) {
	t.Helper()
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest("POST", "/api/acme", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	handler(ctx)
	return rec, decodeAPIMessage(t, rec.Body.String())
}

func initAcmeRouteTestDB(t *testing.T) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "acme-route-test.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	if sqlDB, err := database.GetDB().DB(); err == nil && sqlDB != nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}

	settingService := &service.SettingService{}
	if _, err := settingService.GetAllSetting(); err != nil {
		t.Fatalf("seed default settings failed: %v", err)
	}
}

func upsertAcmeRouteTestCertificateRecord(t *testing.T, sourceRef string) *model.CertificateRecord {
	t.Helper()
	row, err := (&service.CertificateInventoryService{}).Upsert(service.CertificateUpsertPayload{
		SourceType:   service.CertificateSourceImported,
		SourceRef:    "route:" + sourceRef,
		MainDomain:   sourceRef + ".example.com",
		Domains:      []string{sourceRef + ".example.com"},
		CertPEM:      []byte("test-cert-" + sourceRef),
		KeyPEM:       []byte("test-key-" + sourceRef),
		FullchainPEM: []byte("test-cert-" + sourceRef),
	})
	if err != nil {
		t.Fatalf("upsert certificate record failed: %v", err)
	}
	return row
}

func uintJSON(value uint) string {
	return strconv.FormatUint(uint64(value), 10)
}
