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
