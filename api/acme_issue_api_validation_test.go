package api

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestIssueAcmeCertificateRequiresAccountForDomain(t *testing.T) {
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest("POST", "/api/acme-issue", strings.NewReader(`{"domains":"example.com","certificateType":"domain","challenge":"dns"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	(&ApiService{}).IssueAcmeCertificate(ctx)

	msg := decodeAPIMessage(t, rec.Body.String())
	if msg.Success {
		t.Fatalf("expected request without acmeAccountId to fail: %#v", msg)
	}
	if !strings.Contains(msg.Msg, "acmeAccountId is required for domain certificate") {
		t.Fatalf("unexpected error message: %q", msg.Msg)
	}
}

func TestIssueAcmeCertificateTaskRequiresAccountForDomain(t *testing.T) {
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest("POST", "/api/acme-issue-task", strings.NewReader(`{"domains":"example.com","certificateType":"domain","challenge":"dns"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	(&ApiService{}).IssueAcmeCertificateTask(ctx)

	msg := decodeAPIMessage(t, rec.Body.String())
	if msg.Success {
		t.Fatalf("expected async request without acmeAccountId to fail: %#v", msg)
	}
	if !strings.Contains(msg.Msg, "acmeAccountId is required for domain certificate") {
		t.Fatalf("unexpected async issue error message: %q", msg.Msg)
	}
}

func TestRenewAcmeCertificateTaskRequiresCertificateID(t *testing.T) {
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest("POST", "/api/acme-renew-task", strings.NewReader(`{"force":false}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	(&ApiService{}).RenewAcmeCertificateTask(ctx)

	msg := decodeAPIMessage(t, rec.Body.String())
	if msg.Success {
		t.Fatalf("expected async renew request without id to fail: %#v", msg)
	}
	if !strings.Contains(msg.Msg, "id is required") {
		t.Fatalf("unexpected async renew error message: %q", msg.Msg)
	}
}

func TestNormalizeAcmeIssueCertificateType(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "", want: "domain"},
		{input: "domain", want: "domain"},
		{input: "ip", want: "ip"},
		{input: "ipcert", want: "ip"},
		{input: "ip_certificate", want: "ip"},
	}
	for _, tt := range tests {
		if got := normalizeAcmeIssueCertificateType(tt.input); got != tt.want {
			t.Fatalf("normalizeAcmeIssueCertificateType(%q)=%q, want %q", tt.input, got, tt.want)
		}
	}
}
