package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/andresbott/netcheckout/internal/baseline"
	"github.com/andresbott/netcheckout/internal/config"
	"github.com/andresbott/netcheckout/internal/marker"
)

// TestCheckinReleasesWhenInSync: heldFixture leaves local == remote == baseline,
// so the profile is already in sync; checkin releases it — removing the marker and
// clearing the baseline — without moving any data.
func TestCheckinReleasesWhenInSync(t *testing.T) {
	name, p, id := heldFixture(t)
	remote := config.ExpandRoot(p.RemoteRoot)

	r := Runner{Syncer: lcSyncer{}, ToolVersion: "test"}
	rep, err := r.Checkin(context.Background(), name, p, id, Options{})
	if err != nil {
		t.Fatalf("checkin of an in-sync profile: %v", err)
	}
	if _, ok, _ := marker.Read(remote); ok {
		t.Error("marker must be removed after checkin")
	}
	if _, ok, _ := baseline.Load(name); ok {
		t.Error("baseline must be cleared after checkin")
	}
	if !rep.Released {
		t.Error("report should mark the profile released")
	}
}

func TestCheckinCleanRemovesLocalCopy(t *testing.T) {
	name, p, id := heldFixture(t)
	local := config.ExpandRoot(p.LocalRoot)
	r := Runner{Syncer: lcSyncer{}, ToolVersion: "test"}
	if _, err := r.Checkin(context.Background(), name, p, id, Options{Clean: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(local, "keep.txt")); !os.IsNotExist(err) {
		t.Error("--clean should remove the local working copy")
	}
}

// TestCheckinRefusesUnsyncedChanges: a pending local edit (a push that sync would
// carry) makes the profile un-releasable. checkin fails, pushes nothing, surfaces
// the pending change, and leaves the marker in place. There is no --force.
func TestCheckinRefusesUnsyncedChanges(t *testing.T) {
	name, p, id := heldFixture(t)
	local := config.ExpandRoot(p.LocalRoot)
	remote := config.ExpandRoot(p.RemoteRoot)
	_ = os.WriteFile(filepath.Join(local, "keep.txt"), []byte("EDITED"), 0o644)

	r := Runner{Syncer: lcSyncer{}, ToolVersion: "test"}
	rep, err := r.Checkin(context.Background(), name, p, id, Options{})
	if err == nil {
		t.Fatal("checkin must refuse a profile with unsynced changes")
	}
	if len(rep.Pushed) != 1 || rep.Pushed[0] != "keep.txt" {
		t.Errorf("report should surface the pending push, got Pushed=%v", rep.Pushed)
	}
	// Nothing was pushed to the remote, and the marker stays.
	if got, _ := os.ReadFile(filepath.Join(remote, "keep.txt")); string(got) != "base" {
		t.Errorf("checkin must not push; remote keep.txt = %q, want base", got)
	}
	if _, ok, _ := marker.Read(remote); !ok {
		t.Error("marker must remain when checkin is refused")
	}
}

func TestCheckinConflictKeepsMarker(t *testing.T) {
	name, p, id := heldFixture(t)
	local := config.ExpandRoot(p.LocalRoot)
	remote := config.ExpandRoot(p.RemoteRoot)
	_ = os.WriteFile(filepath.Join(local, "keep.txt"), []byte("L"), 0o644)
	_ = os.WriteFile(filepath.Join(remote, "keep.txt"), []byte("R"), 0o644)
	r := Runner{Syncer: lcSyncer{}, ToolVersion: "test"}
	rep, err := r.Checkin(context.Background(), name, p, id, Options{})
	if err == nil {
		t.Fatal("want a refusal when the same file changed on both sides")
	}
	if len(rep.Conflicts) == 0 {
		t.Error("report should surface the conflicting path")
	}
	if _, ok, _ := marker.Read(remote); !ok {
		t.Error("marker must remain after a conflict-stopped checkin")
	}
}
