package service

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	systemdCoreStartWaitTimeout  = 12 * time.Second
	systemdCoreStartPollInterval = 300 * time.Millisecond
	systemdCoreJournalTailLines  = 40
	systemdCorePostActiveHold    = 3 * time.Second
)

type systemdUnitActivationResult struct {
	State    string
	TimedOut bool
	LastErr  error
}

func waitForSystemdUnitActive(unit string, timeout time.Duration) systemdUnitActivationResult {
	return waitForSystemdUnitActiveWithReader(getSystemdUnitActiveState, unit, timeout, systemdCoreStartPollInterval)
}

func waitForSystemdUnitRemainActive(unit string, hold time.Duration) systemdUnitActivationResult {
	return waitForSystemdUnitRemainActiveWithReader(getSystemdUnitActiveState, unit, hold, systemdCoreStartPollInterval)
}

func waitForSystemdUnitActiveWithReader(
	readState func(unit string) (string, error),
	unit string,
	timeout time.Duration,
	interval time.Duration,
) systemdUnitActivationResult {
	if timeout <= 0 {
		timeout = systemdCoreStartWaitTimeout
	}
	if interval <= 0 {
		interval = systemdCoreStartPollInterval
	}
	if readState == nil {
		readState = getSystemdUnitActiveState
	}

	deadline := time.Now().Add(timeout)
	lastState := ""
	var lastErr error

	for {
		state, err := readState(unit)
		if state != "" {
			lastState = state
		}
		if err != nil {
			lastErr = err
		}

		switch strings.ToLower(strings.TrimSpace(state)) {
		case "active":
			return systemdUnitActivationResult{State: "active"}
		case "failed", "inactive", "deactivating":
			return systemdUnitActivationResult{
				State:   strings.ToLower(strings.TrimSpace(state)),
				LastErr: err,
			}
		}

		if time.Now().After(deadline) {
			if lastState == "" {
				lastState = "unknown"
			}
			return systemdUnitActivationResult{
				State:    strings.ToLower(strings.TrimSpace(lastState)),
				TimedOut: true,
				LastErr:  lastErr,
			}
		}
		time.Sleep(interval)
	}
}

func waitForSystemdUnitRemainActiveWithReader(
	readState func(unit string) (string, error),
	unit string,
	hold time.Duration,
	interval time.Duration,
) systemdUnitActivationResult {
	if hold <= 0 {
		hold = systemdCorePostActiveHold
	}
	if interval <= 0 {
		interval = systemdCoreStartPollInterval
	}
	if readState == nil {
		readState = getSystemdUnitActiveState
	}

	deadline := time.Now().Add(hold)
	var lastErr error
	for {
		state, err := readState(unit)
		normalized := strings.ToLower(strings.TrimSpace(state))
		if err != nil {
			lastErr = err
		}
		if normalized != "active" {
			if normalized == "" {
				normalized = "unknown"
			}
			return systemdUnitActivationResult{State: normalized, LastErr: err}
		}
		if time.Now().After(deadline) {
			return systemdUnitActivationResult{State: "active", LastErr: lastErr}
		}
		time.Sleep(interval)
	}
}

func getSystemdUnitActiveState(unit string) (string, error) {
	out, err := exec.Command("systemctl", "is-active", unit).CombinedOutput()
	state := strings.ToLower(strings.TrimSpace(string(out)))
	if state != "" {
		return state, nil
	}
	return state, err
}

func buildSystemdActivationErrorMessage(
	unit string,
	result systemdUnitActivationResult,
	startOutput string,
	diagnostics string,
) string {
	state := strings.TrimSpace(result.State)
	if state == "" {
		state = "unknown"
	}

	var builder strings.Builder
	if result.TimedOut {
		builder.WriteString(fmt.Sprintf(
			"systemd service %s did not become active within %s (last_state=%s)",
			unit,
			systemdCoreStartWaitTimeout,
			state,
		))
	} else {
		builder.WriteString(fmt.Sprintf("systemd service %s is not active after start (state=%s)", unit, state))
	}

	if strings.TrimSpace(startOutput) != "" {
		builder.WriteString("; start_output=")
		builder.WriteString(strings.TrimSpace(startOutput))
	}
	if result.LastErr != nil {
		builder.WriteString("; state_check_error=")
		builder.WriteString(strings.TrimSpace(result.LastErr.Error()))
	}
	if strings.TrimSpace(diagnostics) != "" {
		builder.WriteString("\n")
		builder.WriteString(strings.TrimSpace(diagnostics))
	}
	return builder.String()
}

func collectSystemdStartupDiagnostics(unit string, journalLines int) string {
	if journalLines <= 0 {
		journalLines = systemdCoreJournalTailLines
	}

	var parts []string
	showOut, showErr := exec.Command(
		"systemctl",
		"show",
		unit,
		"--property=ActiveState",
		"--property=SubState",
		"--property=Result",
		"--property=ExecMainCode",
		"--property=ExecMainStatus",
		"--property=ExecStartPre",
		"--property=ExecStart",
		"--property=ExecStopPost",
		"--property=WorkingDirectory",
	).CombinedOutput()
	if text := strings.TrimSpace(string(showOut)); text != "" {
		parts = append(parts, "systemctl show:\n"+text)
	} else if showErr != nil {
		parts = append(parts, "systemctl show error: "+showErr.Error())
	}

	journalOut, journalErr := exec.Command(
		"journalctl",
		"-u",
		unit,
		"-n",
		fmt.Sprintf("%d", journalLines),
		"--no-pager",
	).CombinedOutput()
	if text := strings.TrimSpace(string(journalOut)); text != "" {
		parts = append(parts, fmt.Sprintf("journalctl -u %s -n %d:\n%s", unit, journalLines, text))
	} else if journalErr != nil {
		parts = append(parts, "journalctl error: "+journalErr.Error())
	}

	return strings.TrimSpace(strings.Join(parts, "\n"))
}
