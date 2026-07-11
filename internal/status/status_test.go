package status

import (
	"context"
	"errors"
	"os"
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

func TestComputeMultipleTargets(t *testing.T) {
	localRoot := t.TempDir()
	remoteRoot := t.TempDir()
	localA := filepath.Join(localRoot, "a")
	if err := os.MkdirAll(localA, 0o700); err != nil {
		t.Fatal(err)
	}
	localB := filepath.Join(localRoot, "b") // left uncreated: never checked out

	// Mark the profile checked out (aggregate: one target's marker is enough).
	if err := os.MkdirAll(filepath.Join(remoteRoot, "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(remoteRoot, "a", ".netcheckout.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	d := &fakeDiffer{diffs: map[string]map[rsync.Direction]rsync.Diff{
		localA: {rsync.Pull: inSyncDiff(), rsync.Push: changesDiff("x.txt")},
		localB: {rsync.Pull: changesDiff("y.txt")},
	}}
	p := config.Profile{LocalRoot: localRoot, RemoteRoot: remoteRoot, Subpaths: []string{"a", "b"}}

	st, err := Compute(context.Background(), d, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Targets) != 2 {
		t.Fatalf("Targets = %#v", st.Targets)
	}
	if st.InSync() {
		t.Error("want not in sync overall")
	}
	if st.Targets[0].Subpath != "a" || st.Targets[0].InSync() {
		t.Errorf("Targets[0] = %#v", st.Targets[0])
	}
	if st.Targets[1].Subpath != "b" || !st.Targets[1].LocalMissing {
		t.Errorf("Targets[1] = %#v", st.Targets[1])
	}
}

func TestComputeRemoteRootNotMounted(t *testing.T) {
	localRoot := t.TempDir()
	missingRemote := filepath.Join(t.TempDir(), "not-mounted")
	p := config.Profile{LocalRoot: localRoot, RemoteRoot: missingRemote}
	d := &fakeDiffer{}
	_, err := Compute(context.Background(), d, p)
	if err == nil {
		t.Fatal("want error")
	}
	if got, want := err.Error(), "remote root "+missingRemote+" is not mounted"; got != want {
		t.Errorf("err = %q, want %q", got, want)
	}
	if len(d.calls) != 0 {
		t.Errorf("calls = %#v, want none (should fail before diffing)", d.calls)
	}
}

func TestComputeInvalidSubpath(t *testing.T) {
	localRoot := t.TempDir()
	remoteRoot := t.TempDir()
	p := config.Profile{LocalRoot: localRoot, RemoteRoot: remoteRoot, Subpaths: []string{"../escape"}}
	_, err := Compute(context.Background(), &fakeDiffer{}, p)
	if err == nil {
		t.Fatal("want error for subpath escaping root")
	}
}

func TestProfileStatusInSyncAggregate(t *testing.T) {
	build := func(secondPush rsync.Diff) ProfileStatus {
		return ProfileStatus{Targets: []TargetStatus{
			{Pull: inSyncDiff(), Push: inSyncDiff()},
			{Subpath: "a", Pull: inSyncDiff(), Push: secondPush},
		}}
	}
	if !build(inSyncDiff()).InSync() {
		t.Error("want in sync when every target is in sync")
	}
	if build(changesDiff("z.txt")).InSync() {
		t.Error("want not in sync when any target differs")
	}
}

func TestComputeStopsEarlyWhenNotCheckedOut(t *testing.T) {
	remoteRoot := t.TempDir()
	p := config.Profile{LocalRoot: t.TempDir(), RemoteRoot: remoteRoot}
	d := &fakeDiffer{}
	st, err := Compute(context.Background(), d, p)
	if err != nil {
		t.Fatal(err)
	}
	if st.CheckedOut {
		t.Error("want CheckedOut false when no marker is present")
	}
	if len(st.Targets) != 0 {
		t.Errorf("want no targets, got %#v", st.Targets)
	}
	if len(d.calls) != 0 {
		t.Errorf("want zero differ calls, got %#v", d.calls)
	}
}

func TestComputeRunsWhenCheckedOut(t *testing.T) {
	localRoot := t.TempDir()
	remoteRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(remoteRoot, ".netcheckout.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	d := &fakeDiffer{diffs: map[string]map[rsync.Direction]rsync.Diff{
		localRoot: {rsync.Pull: inSyncDiff(), rsync.Push: inSyncDiff()},
	}}
	p := config.Profile{LocalRoot: localRoot, RemoteRoot: remoteRoot}
	st, err := Compute(context.Background(), d, p)
	if err != nil {
		t.Fatal(err)
	}
	if !st.CheckedOut {
		t.Error("want CheckedOut true when a marker is present")
	}
	if len(st.Targets) != 1 {
		t.Fatalf("want 1 target, got %#v", st.Targets)
	}
	if len(d.calls) != 2 {
		t.Errorf("want 2 differ calls (pull+push), got %d", len(d.calls))
	}
}
