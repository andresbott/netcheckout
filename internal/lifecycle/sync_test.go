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
		if err := os.WriteFile(filepath.Join(dst, f), data, 0o644); err != nil {
			return rsync.Result{}, err
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
