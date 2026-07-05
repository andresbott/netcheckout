package rsync

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func stubRunner(res runResult, err error) runner {
	return func(_ context.Context, _ string, _ []string, tee io.Writer) (runResult, error) {
		if tee != nil {
			_, _ = io.WriteString(tee, res.stdout)
		}
		return res, err
	}
}

func localJob(dir Direction) Job {
	return Job{Local: Endpoint{Path: "/l"}, Remote: Endpoint{Path: "/r"}, Direction: dir}
}

func TestDiffParsesRunnerOutput(t *testing.T) {
	s := &Syncer{run: stubRunner(runResult{stdout: ">f+++++++++ a.txt\n"}, nil)}
	d, err := s.Diff(context.Background(), localJob(Pull))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Changes) != 1 || d.Changes[0] != (Change{Path: "a.txt", Type: Created}) {
		t.Errorf("Changes = %#v", d.Changes)
	}
}

func TestSyncReturnsChangesAndRaw(t *testing.T) {
	raw := "<f+++++++++ a.txt\n"
	s := &Syncer{run: stubRunner(runResult{stdout: raw}, nil)}
	r, err := s.Sync(context.Background(), localJob(Push))
	if err != nil {
		t.Fatal(err)
	}
	if r.Raw != raw || len(r.Changes) != 1 {
		t.Errorf("Result = %#v", r)
	}
}

func TestDiffWrapsRunnerError(t *testing.T) {
	base := errors.New("exit status 23")
	s := &Syncer{run: stubRunner(runResult{stderr: "boom", exitCode: 23}, base)}
	_, err := s.Diff(context.Background(), localJob(Pull))
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("want *Error, got %T", err)
	}
	if e.Op != "diff" || e.ExitCode != 23 || e.Stderr != "boom" || !errors.Is(err, base) {
		t.Errorf("Error = %#v", e)
	}
}

func TestValidationErrorIsNotWrapped(t *testing.T) {
	s := &Syncer{run: stubRunner(runResult{}, nil)}
	_, err := s.Diff(context.Background(), Job{Direction: Pull}) // empty paths
	if err == nil {
		t.Fatal("want validation error")
	}
	var e *Error
	if errors.As(err, &e) {
		t.Errorf("validation error should not be *Error, got %#v", e)
	}
}

func TestOutputTeeReceivesRunnerOutput(t *testing.T) {
	var buf bytes.Buffer
	s := &Syncer{Output: &buf, run: stubRunner(runResult{stdout: "hello"}, nil)}
	if _, err := s.Diff(context.Background(), localJob(Pull)); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "hello" {
		t.Errorf("tee = %q, want hello", buf.String())
	}
}

func TestBinDefaultsToRsync(t *testing.T) {
	if (&Syncer{}).bin() != "rsync" {
		t.Error("empty Bin should default to rsync")
	}
	if (&Syncer{Bin: "/opt/rsync"}).bin() != "/opt/rsync" {
		t.Error("explicit Bin should win")
	}
}

func TestNewIsUsable(t *testing.T) {
	s := New()
	if s.Bin != "rsync" || s.run == nil {
		t.Errorf("New() = %#v", s)
	}
}

func TestExecRunCapturesStdout(t *testing.T) {
	res, err := execRun(context.Background(), "sh", []string{"-c", "printf hello"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.stdout != "hello" {
		t.Errorf("stdout = %q", res.stdout)
	}
}

func TestExecRunCapturesStderrAndExitCode(t *testing.T) {
	res, err := execRun(context.Background(), "sh", []string{"-c", "printf boom >&2; exit 23"}, nil)
	if err == nil {
		t.Fatal("want error")
	}
	if res.exitCode != 23 || strings.TrimSpace(res.stderr) != "boom" {
		t.Errorf("res = %#v", res)
	}
}

func TestExecRunTeesOutput(t *testing.T) {
	var buf bytes.Buffer
	if _, err := execRun(context.Background(), "sh", []string{"-c", "printf hi"}, &buf); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "hi" {
		t.Errorf("tee = %q", buf.String())
	}
}

func TestExecRunTeeIsConcurrencySafe(t *testing.T) {
	var buf bytes.Buffer
	// sh writes to stdout and stderr; os/exec copies each on its own goroutine, so
	// both tee into buf concurrently. Without the synchronized tee this races under
	// `go test -race`.
	if _, err := execRun(context.Background(), "sh", []string{"-c", "printf out; printf err >&2"}, &buf); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if len(got) != 6 || !strings.Contains(got, "out") || !strings.Contains(got, "err") {
		t.Errorf("tee = %q, want both \"out\" and \"err\" (6 bytes)", got)
	}
}
