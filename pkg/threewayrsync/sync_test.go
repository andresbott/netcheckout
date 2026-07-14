package threewayrsync

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

// listRouter returns a runner that answers each list call with canned output chosen by the
// source path (args[len-2] is "<src>/", args[len-1] is the empty dest).
func listRouter(t *testing.T, bySrc map[string]string) runner {
	t.Helper()
	return func(_ context.Context, _ string, args []string, _ io.Writer) (runResult, error) {
		src := args[len(args)-2]
		for prefix, out := range bySrc {
			if strings.HasPrefix(src, prefix) {
				return runResult{stdout: out}, nil
			}
		}
		t.Fatalf("unexpected list source %q", src)
		return runResult{}, nil
	}
}

func TestDiffClassifiesFromListings(t *testing.T) {
	// base empty => a.txt is a local-only add (Push); b.txt is a remote-only add (Pull).
	run := listRouter(t, map[string]string{
		"/l/": ">f+++++++++ 1 2026/07/14-09:15:00 a.txt\n",
		"/r/": ">f+++++++++ 2 2026/07/14-09:15:00 b.txt\n",
	})
	s := &Syncer{Store: &memStore{}, run: run}
	plan, err := s.Diff(context.Background(), Endpoint{Path: "/l"}, Endpoint{Path: "/r"}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Push) != 1 || plan.Push[0] != "a.txt" {
		t.Errorf("Push = %v", plan.Push)
	}
	if len(plan.Pull) != 1 || plan.Pull[0] != "b.txt" {
		t.Errorf("Pull = %v", plan.Pull)
	}
}

func TestDiffValidatesEndpoints(t *testing.T) {
	s := &Syncer{Store: &memStore{}, run: listRouter(t, nil)}
	_, err := s.Diff(context.Background(), Endpoint{Path: ""}, Endpoint{Path: "/r"}, Options{})
	if err == nil {
		t.Fatal("want validation error for empty local path")
	}
}

func TestListWrapsRunnerError(t *testing.T) {
	boom := func(_ context.Context, _ string, _ []string, _ io.Writer) (runResult, error) {
		return runResult{stderr: "nope", exitCode: 12}, io.ErrUnexpectedEOF
	}
	s := &Syncer{Store: &memStore{}, run: boom}
	_, err := s.Diff(context.Background(), Endpoint{Path: "/l"}, Endpoint{Path: "/r"}, Options{})
	var e *Error
	if !errors.As(err, &e) || e.Op != "list" {
		t.Fatalf("want *Error op=list, got %v", err)
	}
}
