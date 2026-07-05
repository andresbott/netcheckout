package rsync

import (
	"errors"
	"strings"
	"testing"
)

func TestErrorMessageIncludesOpExitAndStderr(t *testing.T) {
	base := errors.New("exit status 23")
	e := &Error{Op: "sync", Stderr: "  rsync: link_stat failed\n", ExitCode: 23, Err: base}
	msg := e.Error()
	for _, want := range []string{"sync", "23", "link_stat failed"} {
		if !strings.Contains(msg, want) {
			t.Errorf("Error() = %q, missing %q", msg, want)
		}
	}
}

func TestErrorUnwrapReturnsWrapped(t *testing.T) {
	base := errors.New("boom")
	e := &Error{Op: "diff", Err: base}
	if !errors.Is(e, base) {
		t.Error("errors.Is(e, base) = false, want true")
	}
}

func TestErrorSurfacesCauseWhenNoExitCode(t *testing.T) {
	e := &Error{Op: "diff", Err: errors.New(`exec: "rsync": executable file not found in $PATH`)}
	msg := e.Error()
	if !strings.Contains(msg, "diff") || !strings.Contains(msg, "not found") {
		t.Errorf("Error() = %q, want the op and the wrapped cause", msg)
	}
	if strings.Contains(msg, "exit 0") {
		t.Errorf("Error() = %q, should not report \"exit 0\" for a start failure", msg)
	}
}
