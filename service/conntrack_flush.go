package service

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func flushConntrackTable() error {
	conntrackPath, err := exec.LookPath("conntrack")
	if err != nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, conntrackPath, "-F")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		stderrText := strings.TrimSpace(stderr.String())
		if stderrText == "" {
			return fmt.Errorf("conntrack -F failed: %w", err)
		}
		return fmt.Errorf("conntrack -F failed: %w: %s", err, stderrText)
	}
	return nil
}
