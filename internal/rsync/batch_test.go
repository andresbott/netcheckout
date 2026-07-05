package rsync

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

// scriptRunner yields a canned result/error per call, in order.
func scriptRunner(results []runResult, errs []error) runner {
	i := 0
	return func(_ context.Context, _ string, _ []string, _ io.Writer) (runResult, error) {
		r, e := results[i], errs[i]
		i++
		return r, e
	}
}

func TestDiffAllReturnsPerJobDiffs(t *testing.T) {
	s := &Syncer{run: scriptRunner(
		[]runResult{{stdout: ">f+++++++++ a\n"}, {stdout: ""}},
		[]error{nil, nil},
	)}
	jobs := []Job{localJob(Pull), localJob(Pull)}
	got, err := s.DiffAll(context.Background(), jobs)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || len(got[0].Changes) != 1 || !got[1].InSync {
		t.Errorf("DiffAll = %#v", got)
	}
}

func TestSyncAllFailsFastWithJobIndex(t *testing.T) {
	base := errors.New("exit status 23")
	s := &Syncer{run: scriptRunner(
		[]runResult{{stdout: ""}, {stderr: "boom", exitCode: 23}},
		[]error{nil, base},
	)}
	jobs := []Job{localJob(Push), localJob(Push)}
	got, err := s.SyncAll(context.Background(), jobs)
	if err == nil {
		t.Fatal("want error")
	}
	if len(got) != 1 {
		t.Errorf("want 1 partial result, got %d", len(got))
	}
	if !errors.Is(err, base) || !strings.Contains(err.Error(), "job 1") {
		t.Errorf("err = %v", err)
	}
}
