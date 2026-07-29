package service

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

const (
	shortSystemCommandTimeout = 3 * time.Second
	systemCommandTimeout      = 10 * time.Second
	coreVersionCommandTimeout = 5 * time.Second
)

func runCommandOutputInDirWithTimeout(timeout time.Duration, dir string, name string, args ...string) ([]byte, error) {
	if timeout <= 0 {
		timeout = systemCommandTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return output, fmt.Errorf("%s timed out after %s", name, timeout)
	}
	return output, err
}

func runSystemctlCommand(args ...string) error {
	return runCommandWithTimeout(systemCommandTimeout, "systemctl", args...)
}

func runSystemctlOutput(args ...string) ([]byte, error) {
	output, err := runCommandOutputWithTimeout(systemCommandTimeout, "systemctl", args...)
	return []byte(output), err
}
