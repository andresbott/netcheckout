package rsync

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// runner executes an rsync command. It is injectable so tests need not shell out.
type runner func(ctx context.Context, bin string, args []string, tee io.Writer) (runResult, error)

type runResult struct {
	stdout   string
	stderr   string
	exitCode int
}

// Syncer runs rsync. The zero value is usable (Bin defaults to "rsync", the
// runner to execRun); New returns a ready one. When Output is set, a Syncer's
// methods are not safe to call concurrently: concurrent calls are not
// serialized against a shared Output.
type Syncer struct {
	// Bin is the rsync binary. Empty means "rsync".
	Bin string
	// Output, when non-nil, receives rsync's stdout and stderr live.
	Output io.Writer
	run    runner
}

// New returns a Syncer that shells out to rsync on PATH.
func New() *Syncer {
	return &Syncer{Bin: "rsync", run: execRun}
}

func (s *Syncer) bin() string {
	if s.Bin == "" {
		return "rsync"
	}
	return s.Bin
}

// withFilesFrom splices "--files-from=<path>" in front of the two trailing
// positional paths so rsync parses it as an option, not a source.
func withFilesFrom(args []string, listPath string) []string {
	n := len(args)
	out := make([]string, 0, n+1)
	out = append(out, args[:n-2]...)
	out = append(out, "--files-from="+listPath)
	out = append(out, args[n-2:]...)
	return out
}

// writeFileList writes one relative path per line to a temp file and returns its
// name; the caller removes it.
func writeFileList(paths []string) (string, error) {
	f, err := os.CreateTemp("", "netcheckout-files-*.txt")
	if err != nil {
		return "", err
	}
	name := f.Name()
	if _, err := f.WriteString(strings.Join(paths, "\n") + "\n"); err != nil {
		_ = f.Close()
		_ = os.Remove(name)
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	return name, nil
}

// runRsync builds args for the job, runs it, and returns raw stdout. A non-zero
// exit becomes an *Error; a validation failure is returned unwrapped.
func (s *Syncer) runRsync(ctx context.Context, j Job, dryRun bool, op string) (string, error) {
	args, err := buildArgs(j, dryRun)
	if err != nil {
		return "", err
	}
	if len(j.Files) > 0 {
		listPath, err := writeFileList(j.Files)
		if err != nil {
			return "", err
		}
		defer func() { _ = os.Remove(listPath) }()
		args = withFilesFrom(args, listPath)
	}
	r := s.run
	if r == nil {
		r = execRun
	}
	res, err := r(ctx, s.bin(), args, s.Output)
	if err != nil {
		return "", &Error{Op: op, Args: args, Stderr: res.stderr, ExitCode: res.exitCode, Err: err}
	}
	return res.stdout, nil
}

// Diff performs a dry run and returns the changes a Sync of the same job would make.
func (s *Syncer) Diff(ctx context.Context, j Job) (Diff, error) {
	out, err := s.runRsync(ctx, j, true, "diff")
	if err != nil {
		return Diff{}, err
	}
	return parseItemize(out), nil
}

// Sync performs a real transfer and returns the changes rsync reported making.
func (s *Syncer) Sync(ctx context.Context, j Job) (Result, error) {
	out, err := s.runRsync(ctx, j, false, "sync")
	if err != nil {
		return Result{}, err
	}
	return Result{Changes: parseItemize(out).Changes, Raw: out}, nil
}

// syncWriter serializes writes to an underlying writer. execRun copies stdout and
// stderr on separate goroutines; wrapping a shared tee in one keeps a
// non-concurrent-safe Output (for example a bytes.Buffer) safe.
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}

// execRun is the default runner: it runs the command to completion, capturing
// stdout and stderr and, when tee is non-nil, mirroring both to it live.
func execRun(ctx context.Context, bin string, args []string, tee io.Writer) (runResult, error) {
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // G204: bin and args are built by this package from typed Job fields, not untrusted external input.
	var out, errb bytes.Buffer
	if tee != nil {
		st := &syncWriter{w: tee}
		cmd.Stdout = io.MultiWriter(&out, st)
		cmd.Stderr = io.MultiWriter(&errb, st)
	} else {
		cmd.Stdout = &out
		cmd.Stderr = &errb
	}
	err := cmd.Run()
	res := runResult{stdout: out.String(), stderr: errb.String()}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		res.exitCode = exitErr.ExitCode()
	}
	return res, err
}
