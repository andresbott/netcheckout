//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"
)

// runCLI runs the built netcheckout binary with "--config configPath" plus args, under a
// 30-second timeout. A failure to start the process at all (for example a missing
// binary) is a harness bug, not a scenario outcome, so it calls t.Fatalf directly; a
// normal non-zero exit is returned as exitCode for the caller to assert on.
func runCLI(t *testing.T, configPath string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fullArgs := append([]string{"--config", configPath}, args...)
	cmd := exec.CommandContext(ctx, binPath, fullArgs...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	stdout = outBuf.String()
	stderr = errBuf.String()

	var exitErr *exec.ExitError
	switch {
	case err == nil:
		exitCode = 0
	case errors.As(err, &exitErr):
		exitCode = exitErr.ExitCode()
	default:
		t.Fatalf("run %s %v: %v (stderr: %s)", binPath, fullArgs, err, stderr)
	}
	return stdout, stderr, exitCode
}
