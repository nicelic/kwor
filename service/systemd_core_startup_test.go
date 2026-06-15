package service

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestWaitForSystemdUnitActive_ActivatingThenActive(t *testing.T) {
	states := []string{"activating", "activating", "active"}
	index := 0
	reader := func(unit string) (string, error) {
		if index >= len(states) {
			return "active", nil
		}
		state := states[index]
		index++
		return state, nil
	}

	result := waitForSystemdUnitActiveWithReader(reader, "kwor-singbox", time.Second, time.Nanosecond)
	if result.State != "active" {
		t.Fatalf("expected active state, got %q", result.State)
	}
	if result.TimedOut {
		t.Fatal("expected non-timeout result")
	}
}

func TestWaitForSystemdUnitActive_FailedState(t *testing.T) {
	reader := func(unit string) (string, error) {
		return "failed", errors.New("exit status 3")
	}

	result := waitForSystemdUnitActiveWithReader(reader, "kwor-singbox", time.Second, time.Nanosecond)
	if result.State != "failed" {
		t.Fatalf("expected failed state, got %q", result.State)
	}
	if result.TimedOut {
		t.Fatal("expected immediate failed result, got timeout")
	}
}

func TestWaitForSystemdUnitActive_Timeout(t *testing.T) {
	reader := func(unit string) (string, error) {
		return "activating", nil
	}

	result := waitForSystemdUnitActiveWithReader(reader, "kwor-singbox", time.Millisecond, time.Nanosecond)
	if !result.TimedOut {
		t.Fatal("expected timeout result")
	}
	if result.State != "activating" {
		t.Fatalf("expected last state activating, got %q", result.State)
	}
}

func TestBuildSystemdActivationErrorMessage_ContainsDiagnostics(t *testing.T) {
	diag := "systemctl show:\nActiveState=failed\nSubState=dead\nResult=exit-code\njournalctl -u kwor-singbox -n 40:\nlast error"
	msg := buildSystemdActivationErrorMessage(
		"kwor-singbox",
		systemdUnitActivationResult{State: "failed"},
		"some start output",
		diag,
	)

	required := []string{
		"kwor-singbox",
		"state=failed",
		"ActiveState=failed",
		"SubState=dead",
		"journalctl -u kwor-singbox",
	}
	for _, token := range required {
		if !strings.Contains(msg, token) {
			t.Fatalf("expected message to contain %q, got:\n%s", token, msg)
		}
	}
}

func TestIsSingboxCheckCommandUnsupported(t *testing.T) {
	cases := []string{
		`Error: unknown command "check" for "sing-box"`,
		`unknown subcommand: check`,
		`No help topic for 'check'`,
	}
	for _, text := range cases {
		if !isSingboxCheckCommandUnsupported(text) {
			t.Fatalf("expected check command unsupported for %q", text)
		}
	}
	if isSingboxCheckCommandUnsupported("FATAL[0000] decode config: unknown field") {
		t.Fatal("config decode failures must not be treated as unsupported check command")
	}
}

func TestWaitForSystemdUnitRemainActive_DropsToInactive(t *testing.T) {
	states := []string{"active", "active", "inactive"}
	index := 0
	reader := func(unit string) (string, error) {
		if index >= len(states) {
			return "inactive", nil
		}
		state := states[index]
		index++
		return state, nil
	}

	result := waitForSystemdUnitRemainActiveWithReader(reader, "kwor-singbox", 2*time.Millisecond, time.Nanosecond)
	if result.State != "inactive" {
		t.Fatalf("expected inactive state, got %q", result.State)
	}
}

func TestWaitForSystemdUnitRemainActive_Stable(t *testing.T) {
	reader := func(unit string) (string, error) {
		return "active", nil
	}

	result := waitForSystemdUnitRemainActiveWithReader(reader, "kwor-singbox", time.Millisecond, time.Nanosecond)
	if result.State != "active" {
		t.Fatalf("expected active state, got %q", result.State)
	}
}
