package service

import "testing"

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

func assertErr(msg string) error {
	return simpleErr(msg)
}

type simpleErr string

func (e simpleErr) Error() string {
	return string(e)
}
