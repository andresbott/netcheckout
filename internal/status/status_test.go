package status

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/andresbott/netcheckout/internal/config"
	"github.com/andresbott/netcheckout/internal/rsync"
)

// fakeDiffer returns a canned Diff per (local path, direction), or a fixed
// error for every call if err is set. calls records every Job it was asked
// to diff, in order.
type fakeDiffer struct {
	diffs map[string]map[rsync.Direction]rsync.Diff
	err   error
	calls []rsync.Job
}

func (f *fakeDiffer) Diff(_ context.Context, j rsync.Job) (rsync.Diff, error) {
	f.calls = append(f.calls, j)
	if f.err != nil {
		return rsync.Diff{}, f.err
	}
	return f.diffs[j.Local.Path][j.Direction], nil
}

func inSyncDiff() rsync.Diff { return rsync.Diff{InSync: true} }

func changesDiff(paths ...string) rsync.Diff {
	changes := make([]rsync.Change, 0, len(paths))
	for _, p := range paths {
		changes = append(changes, rsync.Change{Path: p, Type: rsync.Modified})
	}
	return rsync.Diff{Changes: changes, InSync: false}
}

func TestComputeTargetInSync(t *testing.T) {
	local := t.TempDir()
	d := &fakeDiffer{diffs: map[string]map[rsync.Direction]rsync.Diff{
		local: {rsync.Pull: inSyncDiff(), rsync.Push: inSyncDiff()},
	}}
	ts, err := computeTarget(context.Background(), d, config.Target{Local: local, Remote: "/remote"})
	if err != nil {
		t.Fatal(err)
	}
	if !ts.InSync() || ts.LocalMissing {
		t.Errorf("TargetStatus = %#v, want in sync and not missing", ts)
	}
	if len(d.calls) != 2 {
		t.Errorf("calls = %d, want 2 (pull and push)", len(d.calls))
	}
}

func TestComputeTargetWithDifferences(t *testing.T) {
	local := t.TempDir()
	d := &fakeDiffer{diffs: map[string]map[rsync.Direction]rsync.Diff{
		local: {
			rsync.Pull: inSyncDiff(),
			rsync.Push: changesDiff("report.pdf"),
		},
	}}
	ts, err := computeTarget(context.Background(), d, config.Target{Local: local, Remote: "/remote"})
	if err != nil {
		t.Fatal(err)
	}
	if ts.InSync() {
		t.Error("want not in sync")
	}
	if len(ts.Push.Changes) != 1 || ts.Push.Changes[0].Path != "report.pdf" {
		t.Errorf("Push.Changes = %#v", ts.Push.Changes)
	}
}

func TestComputeTargetLocalMissingSkipsPush(t *testing.T) {
	local := filepath.Join(t.TempDir(), "never-checked-out")
	d := &fakeDiffer{diffs: map[string]map[rsync.Direction]rsync.Diff{
		local: {rsync.Pull: changesDiff("a.txt", "b.txt")},
	}}
	ts, err := computeTarget(context.Background(), d, config.Target{Local: local, Remote: "/remote"})
	if err != nil {
		t.Fatal(err)
	}
	if !ts.LocalMissing {
		t.Error("want LocalMissing true")
	}
	if len(ts.Pull.Changes) != 2 {
		t.Errorf("Pull.Changes = %#v", ts.Pull.Changes)
	}
	if len(d.calls) != 1 || d.calls[0].Direction != rsync.Pull {
		t.Errorf("calls = %#v, want exactly one Pull call", d.calls)
	}
}

func TestComputeTargetDifferErrorNamesSubpathAndDirection(t *testing.T) {
	local := t.TempDir()
	d := &fakeDiffer{err: errors.New("boom")}
	_, err := computeTarget(context.Background(), d, config.Target{Subpath: "notes/2024", Local: local, Remote: "/remote"})
	if err == nil || !errors.Is(err, d.err) {
		t.Fatalf("err = %v, want wrapped %v", err, d.err)
	}
	if got := err.Error(); got != "notes/2024: pull diff: boom" {
		t.Errorf("err = %q", got)
	}
}

func TestTargetStatusLabel(t *testing.T) {
	if got := (TargetStatus{}).Label(); got != "(root)" {
		t.Errorf("Label() = %q, want (root)", got)
	}
	if got := (TargetStatus{Subpath: "notes/2024"}).Label(); got != "notes/2024" {
		t.Errorf("Label() = %q, want notes/2024", got)
	}
}
