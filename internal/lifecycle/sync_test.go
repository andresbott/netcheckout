package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/andresbott/netcheckout/internal/baseline"
	"github.com/andresbott/netcheckout/internal/config"
	"github.com/andresbott/netcheckout/internal/ident"
	"github.com/andresbott/netcheckout/internal/marker"
	"github.com/andresbott/netcheckout/internal/reconcile"
	"github.com/andresbott/netcheckout/internal/rsync"
)

// lcSyncer is a fake rsync.Syncer that copies the listed files between roots
// for Sync and returns an empty Diff (unused by lifecycle.Sync directly, but
// required to satisfy the Syncer interface).
type lcSyncer struct{}

func (lcSyncer) Sync(_ context.Context, j rsync.Job) (rsync.Result, error) {
	src, dst := j.Remote.Path, j.Local.Path
	if j.Direction == rsync.Push {
		src, dst = j.Local.Path, j.Remote.Path
	}
	for _, f := range j.Files {
		data, err := os.ReadFile(filepath.Join(src, f))
		if err != nil {
			return rsync.Result{}, err
		}
		_ = os.MkdirAll(filepath.Dir(filepath.Join(dst, f)), 0o755)
		ct := rsync.Created
		if _, err := os.Stat(filepath.Join(dst, f)); err == nil {
			ct = rsync.Modified
		}
		if err := os.WriteFile(filepath.Join(dst, f), data, 0o644); err != nil {
			return rsync.Result{}, err
		}
		if j.OnChange != nil {
			j.OnChange(rsync.Change{Path: f, Type: ct})
		}
	}
	return rsync.Result{}, nil
}
func (lcSyncer) Diff(context.Context, rsync.Job) (rsync.Diff, error) { return rsync.Diff{}, nil }

func heldFixture(t *testing.T) (name string, p config.Profile, id ident.Ident) {
	t.Helper()
	t.Setenv("NETCHECKOUT_STATE", t.TempDir())
	root := t.TempDir()
	local := filepath.Join(root, "local")
	remote := filepath.Join(root, "remote")
	_ = os.MkdirAll(local, 0o755)
	_ = os.MkdirAll(remote, 0o755)
	// One file checked out on both sides, recorded in the baseline.
	_ = os.WriteFile(filepath.Join(local, "keep.txt"), []byte("base"), 0o644)
	_ = os.WriteFile(filepath.Join(remote, "keep.txt"), []byte("base"), 0o644)
	id = ident.Ident{By: "me@host", Host: "host"}
	_ = marker.Write(remote, &marker.Marker{CheckedOutBy: id.By, Host: id.Host, Profile: "work", Relpaths: []string{"."}})
	files, _ := baseline.Snapshot(local, []string{"."})
	_ = baseline.Save(&baseline.Baseline{Profile: "work", Relpaths: []string{"."}, Files: files, LastSyncAt: time.Unix(0, 0)})
	return "work", config.Profile{LocalRoot: local, RemoteRoot: remote}, id
}

func TestSyncFailFastWithoutMarker(t *testing.T) {
	name, p, id := heldFixture(t)
	_ = marker.Remove(config.ExpandRoot(p.RemoteRoot))
	r := Runner{Syncer: lcSyncer{}, ToolVersion: "test"}
	if _, err := r.Sync(context.Background(), name, p, id, "", Options{}); err == nil {
		t.Fatal("sync must fail fast when no marker exists")
	}
}

func TestSyncPushesLocalEdit(t *testing.T) {
	name, p, id := heldFixture(t)
	local := config.ExpandRoot(p.LocalRoot)
	remote := config.ExpandRoot(p.RemoteRoot)
	// Edit locally after checkout.
	_ = os.WriteFile(filepath.Join(local, "keep.txt"), []byte("EDITED"), 0o644)
	r := Runner{Syncer: lcSyncer{}, ToolVersion: "test", Now: func() time.Time { return time.Unix(500, 0).UTC() }}
	rep, err := r.Sync(context.Background(), name, p, id, "", Options{})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(remote, "keep.txt")); string(got) != "EDITED" {
		t.Errorf("remote keep.txt = %q, want EDITED", got)
	}
	// Marker still ours; last_sync bumped; baseline re-snapshotted.
	m, _, _ := marker.Read(remote)
	if !m.OwnedBy(id.By, id.Host) {
		t.Error("marker ownership must be preserved")
	}
	if len(rep.Pushed) != 1 {
		t.Errorf("rep.Pushed = %v", rep.Pushed)
	}
}

func TestSyncForwardsApplyEvents(t *testing.T) {
	name, p, id := heldFixture(t)
	local := config.ExpandRoot(p.LocalRoot)
	_ = os.WriteFile(filepath.Join(local, "keep.txt"), []byte("EDITED"), 0o644)
	r := Runner{Syncer: lcSyncer{}, ToolVersion: "test"}

	var events []reconcile.Event
	_, err := r.Sync(context.Background(), name, p, id, "", Options{
		OnApply: func(e reconcile.Event) { events = append(events, e) },
	})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	want := reconcile.Event{Kind: reconcile.EventModify, Side: reconcile.SideRemote, Path: "keep.txt"}
	if len(events) != 1 || events[0] != want {
		t.Fatalf("events = %+v, want [%+v]", events, want)
	}
}

func TestSyncDryRunEmitsNoEvents(t *testing.T) {
	name, p, id := heldFixture(t)
	local := config.ExpandRoot(p.LocalRoot)
	_ = os.WriteFile(filepath.Join(local, "keep.txt"), []byte("EDITED"), 0o644)
	r := Runner{Syncer: lcSyncer{}, ToolVersion: "test"}

	var events []reconcile.Event
	if _, err := r.Sync(context.Background(), name, p, id, "", Options{
		DryRun:  true,
		OnApply: func(e reconcile.Event) { events = append(events, e) },
	}); err != nil {
		t.Fatalf("dry-run sync: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("dry run must apply nothing, got events %+v", events)
	}
}

func TestSyncConflictStops(t *testing.T) {
	name, p, id := heldFixture(t)
	local := config.ExpandRoot(p.LocalRoot)
	remote := config.ExpandRoot(p.RemoteRoot)
	_ = os.WriteFile(filepath.Join(local, "keep.txt"), []byte("LOCAL"), 0o644)
	_ = os.WriteFile(filepath.Join(remote, "keep.txt"), []byte("REMOTE"), 0o644)
	r := Runner{Syncer: lcSyncer{}, ToolVersion: "test"}
	rep, err := r.Sync(context.Background(), name, p, id, "", Options{})
	if err == nil {
		t.Fatal("want a conflict error")
	}
	if len(rep.Conflicts) == 0 {
		t.Error("report should list conflicts")
	}
	// Nothing written on either side.
	if got, _ := os.ReadFile(filepath.Join(remote, "keep.txt")); string(got) != "REMOTE" {
		t.Errorf("remote must be untouched on conflict, got %q", got)
	}
}

// TestSyncForceResolvesConflictAndReportsPush is the I1/M4 regression: a forced
// sync over a same-file conflict must not be reported as "conflicts — nothing
// written" (rep.Conflicts must be cleared, since Apply folds the conflict into
// a push, local-wins) and rep.Pushed must reflect the actual write.
func TestSyncForceResolvesConflictAndReportsPush(t *testing.T) {
	name, p, id := heldFixture(t)
	local := config.ExpandRoot(p.LocalRoot)
	remote := config.ExpandRoot(p.RemoteRoot)
	_ = os.WriteFile(filepath.Join(local, "keep.txt"), []byte("LOCAL"), 0o644)
	_ = os.WriteFile(filepath.Join(remote, "keep.txt"), []byte("REMOTE"), 0o644)

	r := Runner{Syncer: lcSyncer{}, ToolVersion: "test"}
	rep, err := r.Sync(context.Background(), name, p, id, "", Options{Force: true})
	if err != nil {
		t.Fatalf("force sync must not error: %v", err)
	}
	if len(rep.Conflicts) != 0 {
		t.Errorf("rep.Conflicts = %v, want empty on a force-resolved conflict", rep.Conflicts)
	}
	if len(rep.Pushed) == 0 {
		t.Error("rep.Pushed should be non-empty: local wins a forced conflict")
	}
	if got, _ := os.ReadFile(filepath.Join(remote, "keep.txt")); string(got) != "LOCAL" {
		t.Errorf("remote keep.txt = %q, want LOCAL (force resolves local-wins)", got)
	}
}

// TestSyncDryRunOverConflictDoesNotError is the M4 regression: --dry-run must
// always exit clean, even when the plan itself has a conflict, reporting the
// conflicting paths and mutating nothing.
func TestSyncDryRunOverConflictDoesNotError(t *testing.T) {
	name, p, id := heldFixture(t)
	local := config.ExpandRoot(p.LocalRoot)
	remote := config.ExpandRoot(p.RemoteRoot)
	_ = os.WriteFile(filepath.Join(local, "keep.txt"), []byte("LOCAL"), 0o644)
	_ = os.WriteFile(filepath.Join(remote, "keep.txt"), []byte("REMOTE"), 0o644)

	r := Runner{Syncer: lcSyncer{}, ToolVersion: "test"}
	rep, err := r.Sync(context.Background(), name, p, id, "", Options{DryRun: true})
	if err != nil {
		t.Fatalf("dry-run sync over a conflict must not error, got: %v", err)
	}
	if len(rep.Conflicts) == 0 {
		t.Error("dry-run report should list the would-be conflict")
	}
	if got, _ := os.ReadFile(filepath.Join(remote, "keep.txt")); string(got) != "REMOTE" {
		t.Errorf("remote must be byte-unchanged under dry-run, got %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(local, "keep.txt")); string(got) != "LOCAL" {
		t.Errorf("local must be byte-unchanged under dry-run, got %q", got)
	}
}

// TestSyncScopedPreservesOutOfScopeBaseline is the C1 regression: a scoped sync
// (relpath narrower than the held set) must not clobber the baseline entries
// for paths outside that scope with their current — possibly un-synced — local
// content, since that poisons the three-way ancestor for the next full sync.
func TestSyncScopedPreservesOutOfScopeBaseline(t *testing.T) {
	t.Setenv("NETCHECKOUT_STATE", t.TempDir())
	root := t.TempDir()
	local := filepath.Join(root, "local")
	remote := filepath.Join(root, "remote")
	_ = os.MkdirAll(filepath.Join(local, "sub"), 0o755)
	_ = os.MkdirAll(filepath.Join(remote, "sub"), 0o755)

	origTop := []byte("original top")
	origSub := []byte("original sub")
	_ = os.WriteFile(filepath.Join(local, "top.txt"), origTop, 0o644)
	_ = os.WriteFile(filepath.Join(remote, "top.txt"), origTop, 0o644)
	_ = os.WriteFile(filepath.Join(local, "sub", "y.txt"), origSub, 0o644)
	_ = os.WriteFile(filepath.Join(remote, "sub", "y.txt"), origSub, 0o644)

	id := ident.Ident{By: "me@host", Host: "host"}
	if err := marker.Write(remote, &marker.Marker{CheckedOutBy: id.By, Host: id.Host, Profile: "work", Relpaths: []string{"."}}); err != nil {
		t.Fatal(err)
	}
	origFiles, err := baseline.Snapshot(local, []string{"."})
	if err != nil {
		t.Fatal(err)
	}
	if err := baseline.Save(&baseline.Baseline{Profile: "work", Relpaths: []string{"."}, Files: origFiles, LastSyncAt: time.Unix(0, 0)}); err != nil {
		t.Fatal(err)
	}
	originalTopHash := origFiles["top.txt"].Hash

	name := "work"
	p := config.Profile{LocalRoot: local, RemoteRoot: remote}

	// Edit BOTH files locally; only "sub" will be synced.
	if err := os.WriteFile(filepath.Join(local, "top.txt"), []byte("EDITED top, never synced"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(local, "sub", "y.txt"), []byte("EDITED sub"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := Runner{Syncer: lcSyncer{}, ToolVersion: "test"}
	if _, err := r.Sync(context.Background(), name, p, id, "sub", Options{}); err != nil {
		t.Fatalf("scoped sync: %v", err)
	}

	// The out-of-scope remote file must be untouched.
	if got, _ := os.ReadFile(filepath.Join(remote, "top.txt")); string(got) != string(origTop) {
		t.Errorf("remote top.txt = %q, want untouched original %q", got, origTop)
	}

	nb, ok, err := baseline.Load(name)
	if err != nil || !ok {
		t.Fatalf("load baseline: ok=%v err=%v", ok, err)
	}

	gotTop, ok := nb.Files["top.txt"]
	if !ok {
		t.Fatal("top.txt missing from the reconciled baseline")
	}
	if gotTop.Hash != originalTopHash {
		t.Errorf("top.txt baseline hash = %s, want the ORIGINAL hash %s (out-of-scope entry must be preserved, not overwritten with the un-synced local edit)", gotTop.Hash, originalTopHash)
	}

	wantSubHash, err := baseline.HashFile(filepath.Join(local, "sub", "y.txt"))
	if err != nil {
		t.Fatal(err)
	}
	gotSub, ok := nb.Files["sub/y.txt"]
	if !ok {
		t.Fatal("sub/y.txt missing from the reconciled baseline")
	}
	if gotSub.Hash != wantSubHash {
		t.Errorf("sub/y.txt baseline hash = %s, want %s (post-sync content)", gotSub.Hash, wantSubHash)
	}
	if gotSub.Hash == originalTopHash {
		t.Error("sub/y.txt baseline hash unexpectedly matches the unrelated original top.txt hash")
	}
}
