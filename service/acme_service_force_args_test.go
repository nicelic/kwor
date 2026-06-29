package service

import "testing"

func TestEnsureAcmeFreshIssueArgsAddsForceWhenMissing(t *testing.T) {
	args := []string{"--issue", "-d", "1.2.3.4", "--standalone", "--keylength", "ec-256"}

	got := ensureAcmeFreshIssueArgs(args)

	found := false
	for _, arg := range got {
		if arg == "--force" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected --force to be appended, got %#v", got)
	}
}

func TestEnsureAcmeFreshIssueArgsDoesNotDuplicateForce(t *testing.T) {
	args := []string{"--issue", "-d", "1.2.3.4", "--force", "--standalone"}

	got := ensureAcmeFreshIssueArgs(args)

	count := 0
	for _, arg := range got {
		if arg == "--force" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one --force, got count=%d args=%#v", count, got)
	}
}
