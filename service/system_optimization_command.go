package service

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const systemOptimizationCommandOutputMaxBytes = 128 * 1024

type boundedCommandBuffer struct {
	buffer   bytes.Buffer
	maxBytes int
	exceeded bool
}

func (b *boundedCommandBuffer) Write(p []byte) (int, error) {
	if b.maxBytes <= 0 {
		b.maxBytes = systemOptimizationCommandOutputMaxBytes
	}
	remaining := b.maxBytes - b.buffer.Len()
	if remaining > 0 {
		if len(p) > remaining {
			_, _ = b.buffer.Write(p[:remaining])
			b.exceeded = true
		} else {
			_, _ = b.buffer.Write(p)
		}
	} else if len(p) > 0 {
		b.exceeded = true
	}
	// Consume all output so a noisy child cannot block on a full pipe.
	return len(p), nil
}

func optimizationCommandContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, timeout)
}

func runOptimizationCommandWithTimeout(parent context.Context, timeout time.Duration, command string, args ...string) error {
	ctx, cancel := optimizationCommandContext(parent, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, args...)
	stdout := &boundedCommandBuffer{maxBytes: systemOptimizationCommandOutputMaxBytes}
	stderr := &boundedCommandBuffer{maxBytes: systemOptimizationCommandOutputMaxBytes}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if stdout.exceeded || stderr.exceeded {
		return fmt.Errorf("命令输出超过 %d KiB 上限", systemOptimizationCommandOutputMaxBytes/1024)
	}
	if err != nil {
		message := strings.TrimSpace(stderr.buffer.String())
		if message == "" {
			message = strings.TrimSpace(stdout.buffer.String())
		}
		if message == "" {
			return err
		}
		return fmt.Errorf("%w: %s", err, message)
	}
	return nil
}

func runOptimizationCommandOutputWithTimeout(parent context.Context, timeout time.Duration, command string, args ...string) (string, error) {
	ctx, cancel := optimizationCommandContext(parent, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, args...)
	stdout := &boundedCommandBuffer{maxBytes: systemOptimizationCommandOutputMaxBytes}
	stderr := &boundedCommandBuffer{maxBytes: systemOptimizationCommandOutputMaxBytes}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if stdout.exceeded || stderr.exceeded {
		return "", fmt.Errorf("命令输出超过 %d KiB 上限", systemOptimizationCommandOutputMaxBytes/1024)
	}
	if err != nil {
		message := strings.TrimSpace(stderr.buffer.String())
		if message == "" {
			message = strings.TrimSpace(stdout.buffer.String())
		}
		if message == "" {
			return "", err
		}
		return "", fmt.Errorf("%w: %s", err, message)
	}
	return stdout.buffer.String(), nil
}
