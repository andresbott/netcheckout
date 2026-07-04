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
