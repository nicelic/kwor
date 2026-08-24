package service

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestIsAcmeDomainsNotChangedError(t *testing.T) {
	errText := "exit status 2: [Sunday, May 03, 2026 AM11:13:11 HKT] Domains not changed.\n[Sunday, May 03, 2026 AM11:13:11 HKT] Skipping. Next renewal time is: 2026-07-18T02:12:34Z\n[Sunday, May 03, 2026 AM11:13:11 HKT] Add '--force' to force renewal."
	if !isAcmeDomainsNotChangedError(assertErr(errText)) {
		t.Fatalf("expected domains-not-changed error to be recognized")
	}
}

func TestIsAcmeDomainsNotChangedErrorWithAnsiQuotes(t *testing.T) {
	errText := "exit status 2: Domains not changed.\nSkipping. Next renewal time is: \u001b[32m2026-07-18T02:12:34Z\u001b[0m\nAdd '\u001b[31m--force\u001b[0m' to force renewal."
	if !isAcmeDomainsNotChangedError(assertErr(errText)) {
		t.Fatalf("expected domains-not-changed error with ansi color codes to be recognized")
	}
}

func TestIsAcmeRenewSkippedError(t *testing.T) {
	errText := "exit status 2: [Sun] Skipping. Next renewal time is: 2026-07-18T02:12:34Z\n[Sun] Add '--force' to force renewal."
	if !isAcmeRenewSkippedError(assertErr(errText)) {
		t.Fatalf("expected renew skip error to be recognized")
	}
}

func TestIsAcmeRenewSkippedErrorFalse(t *testing.T) {
	errText := "exit status 1: [Sun] Some dns provider api returned auth failed"
	if isAcmeRenewSkippedError(assertErr(errText)) {
		t.Fatalf("did not expect generic renew failure to be recognized as skip")
	}
}

func TestNormalizeAcmeOutputForMatch(t *testing.T) {
	raw := " \u001b[32mDomains\u001b[0m   not changed.\r\nAdd '--force'\tto force renewal.\x00 "
	got := normalizeAcmeOutputForMatch(raw)
	expected := "domains not changed. add '--force' to force renewal."
	if got != expected {
		t.Fatalf("unexpected normalized output: got=%q want=%q", got, expected)
	}
}

func TestAcmeCommandContextErrorKeepsLastRedactedOutput(t *testing.T) {
	err := acmeCommandContextError(
		"timed out",
		"/usr/local/bin/acme.sh",
		"Let's finalize the order.\n502 Bad Gateway\nCF_Token=********",
		context.DeadlineExceeded,
	)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout cause was not preserved: %v", err)
	}
	text := err.Error()
	for _, want := range []string{"command timed out", "Let's finalize the order.", "502 Bad Gateway", "CF_Token=********"} {
		if !strings.Contains(text, want) {
			t.Fatalf("timeout error does not retain %q: %s", want, text)
		}
	}
}

func TestAnnotateAcmeOrderCommandErrorClassifiesObservedZeroSSLFailures(t *testing.T) {
	base := errors.New("exit status 1: Le_OrderFinalize not found\n<title>502 Bad Gateway</title>")
	annotated := annotateAcmeOrderCommandError(base)
	if !errors.Is(annotated, base) {
		t.Fatalf("original command error was not preserved: %v", annotated)
	}
	if !strings.Contains(annotated.Error(), "502 网关错误") || !strings.Contains(annotated.Error(), "Le_OrderFinalize") {
		t.Fatalf("ZeroSSL 502 failure was not accurately annotated: %v", annotated)
	}

	dnsBase := errors.New("curl error: 6 Could not resolve host: acme.zerossl.com")
	dnsAnnotated := annotateAcmeOrderCommandError(dnsBase)
	if !strings.Contains(dnsAnnotated.Error(), "主机名解析失败") {
		t.Fatalf("DNS resolution failure was not annotated: %v", dnsAnnotated)
	}
}

func assertErr(msg string) error {
	return simpleErr(msg)
}

type simpleErr string

func (e simpleErr) Error() string {
	return string(e)
}
