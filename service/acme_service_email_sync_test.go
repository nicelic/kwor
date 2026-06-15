package service

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestEnsureAcmeAccountEmailForServerUpdateSuccess(t *testing.T) {
	svc := &AcmeService{}
	calls := make([][]string, 0, 2)

	runner := func(timeout time.Duration, command string, args []string, envPairs []string, logSession *acmeLogSession) (string, error) {
		copied := append([]string{}, args...)
		calls = append(calls, copied)
		return "", nil
	}

	err := svc.ensureAcmeAccountEmailForServerWithRunner("/tmp/acme.sh", "/tmp/acme-home", "domain@example.com", "letsencrypt", nil, runner)
	if err != nil {
		t.Fatalf("ensureAcmeAccountEmailForServerWithRunner failed: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("expected 1 command call, got %d", len(calls))
	}
	if !containsToken(calls[0], "--update-account") || !containsToken(calls[0], "-m") || !containsToken(calls[0], "domain@example.com") {
		t.Fatalf("expected update-account command with -m email, got %#v", calls[0])
	}
	if !containsToken(calls[0], "--server") || !containsToken(calls[0], "letsencrypt") {
		t.Fatalf("expected update-account command with --server letsencrypt, got %#v", calls[0])
	}
	if containsToken(calls[0], "--register-account") {
		t.Fatalf("did not expect register-account call in success path, got %#v", calls[0])
	}
}

func TestEnsureAcmeAccountEmailForServerRegisterThenRetryUpdate(t *testing.T) {
	svc := &AcmeService{}
	verbs := make([]string, 0, 4)
	registered := false

	runner := func(timeout time.Duration, command string, args []string, envPairs []string, logSession *acmeLogSession) (string, error) {
		if containsToken(args, "--update-account") {
			verbs = append(verbs, "update")
			if !registered {
				return "", errors.New(`Please add '--register-account' first`)
			}
			return "", nil
		}
		if containsToken(args, "--register-account") {
			verbs = append(verbs, "register")
			registered = true
			return "", nil
		}
		return "", nil
	}

	err := svc.ensureAcmeAccountEmailForServerWithRunner("/tmp/acme.sh", "/tmp/acme-home", "domain@example.com", "zerossl", nil, runner)
	if err != nil {
		t.Fatalf("ensureAcmeAccountEmailForServerWithRunner failed: %v", err)
	}

	want := []string{"update", "register", "update"}
	if !reflect.DeepEqual(verbs, want) {
		t.Fatalf("unexpected command flow: got=%#v want=%#v", verbs, want)
	}
}

func TestEnsureAcmeAccountEmailForServerStopsOnInvalidContact(t *testing.T) {
	svc := &AcmeService{}
	verbs := make([]string, 0, 2)

	runner := func(timeout time.Duration, command string, args []string, envPairs []string, logSession *acmeLogSession) (string, error) {
		if containsToken(args, "--update-account") {
			verbs = append(verbs, "update")
			return "", errors.New("urn:ietf:params:acme:error:invalidContact: unable to parse email")
		}
		if containsToken(args, "--register-account") {
			verbs = append(verbs, "register")
		}
		return "", nil
	}

	err := svc.ensureAcmeAccountEmailForServerWithRunner("/tmp/acme.sh", "/tmp/acme-home", "domain@example.com", "letsencrypt", nil, runner)
	if err == nil {
		t.Fatal("expected invalid contact error, got nil")
	}
	if !isAcmeInvalidContactError(err) {
		t.Fatalf("expected invalid contact error, got: %v", err)
	}
	if len(verbs) != 1 || verbs[0] != "update" {
		t.Fatalf("invalid-contact should stop before register: got %#v", verbs)
	}
}

func TestEnsureAcmeAccountEmailForServerFallsBackToLegacyEmailFlag(t *testing.T) {
	svc := &AcmeService{}
	calls := make([][]string, 0, 3)

	runner := func(timeout time.Duration, command string, args []string, envPairs []string, logSession *acmeLogSession) (string, error) {
		copied := append([]string{}, args...)
		calls = append(calls, copied)
		if containsToken(args, "--update-account") && containsToken(args, "-m") {
			return "", errors.New("unknown option: -m")
		}
		return "", nil
	}

	err := svc.ensureAcmeAccountEmailForServerWithRunner("/tmp/acme.sh", "/tmp/acme-home", "domain@example.com", "letsencrypt", nil, runner)
	if err != nil {
		t.Fatalf("ensureAcmeAccountEmailForServerWithRunner failed: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("expected 2 update-account attempts, got %d", len(calls))
	}
	if !containsToken(calls[0], "-m") {
		t.Fatalf("first attempt should use -m, got %#v", calls[0])
	}
	if !containsToken(calls[1], "--accountemail") {
		t.Fatalf("fallback attempt should use --accountemail, got %#v", calls[1])
	}
	if containsToken(calls[0], "--register-account") || containsToken(calls[1], "--register-account") {
		t.Fatalf("did not expect register-account in fallback flow, got %#v", calls)
	}
}

func containsToken(args []string, token string) bool {
	for _, item := range args {
		if item == token {
			return true
		}
	}
	return false
}
