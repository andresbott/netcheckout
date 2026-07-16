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

// TestCheckinReleasesWhenInSync: heldFixture leaves local == remote == base,
// so the profile is already in sync; checkin releases it — removing the marker and
// clearing the state — without moving any data.
func TestCheckinReleasesWhenInSync(t *testing.T) {
	name, p, id := heldFixture(t)
	remote := config.ExpandRoot(p.RemoteRoot)

	rep, err := (Runner{ToolVersion: "test"}).Checkin(context.Background(), name, p, id, Options{})
	if err != nil {
		t.Fatalf("checkin of an in-sync profile: %v", err)
	}
	if _, ok, _ := marker.Read(remote); ok {
		t.Error("marker must be removed after checkin")
	}
	if _, ok, _ := baseline.Load(name); ok {
		t.Error("state must be cleared after checkin")
	}
	if !rep.Released {
		t.Error("report should mark the profile released")
	}
	// The data itself is untouched: the remote keeps its files.
	if _, err := os.Stat(filepath.Join(remote, "file.txt")); err != nil {
		t.Errorf("remote content must survive checkin: %v", err)
	}
}

func TestCheckinCleanRemovesLocalCopy(t *testing.T) {
	name, p, id := heldFixture(t)
	local := config.ExpandRoot(p.LocalRoot)
	if _, err := (Runner{ToolVersion: "test"}).Checkin(context.Background(), name, p, id, Options{Clean: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(local, "file.txt")); !os.IsNotExist(err) {
		t.Error("--clean should remove the local working copy")
	}
}

// --clean is os.RemoveAll of the local root: a config mistake pointing the
// local root at the home directory (or /) must refuse up front, before the
// marker or baseline are touched, so nothing is released and nothing deleted.
func TestCheckinCleanRefusesHomeDir(t *testing.T) {
	name, p, id := heldFixture(t)
	remote := config.ExpandRoot(p.RemoteRoot)
	// Pretend the user's home IS the local root (the dangerous misconfig).
	t.Setenv("HOME", config.ExpandRoot(p.LocalRoot))

	_, err := (Runner{ToolVersion: "test"}).Checkin(context.Background(), name, p, id, Options{Clean: true})
	if err == nil {
		t.Fatal("checkin --clean must refuse to remove the home directory")
	}
	// Nothing was released: marker and baseline still there, data intact.
	if _, ok, _ := marker.Read(remote); !ok {
		t.Error("marker must be left in place on refusal")
	}
	if _, ok, _ := baseline.Load(name); !ok {
		t.Error("baseline must be left in place on refusal")
	}
	if _, statErr := os.Stat(filepath.Join(config.ExpandRoot(p.LocalRoot), "file.txt")); statErr != nil {
		t.Error("local working copy must be untouched on refusal")
	}
}

func TestCheckinCleanRefusesShallowRoot(t *testing.T) {
	for _, root := range []string{"/", "/home", "/etc"} {
		if err := validateCleanTarget(root); err == nil {
			t.Errorf("validateCleanTarget(%q) must refuse", root)
		}
	}
	if err := validateCleanTarget("/home/someone/checkouts/photos"); err != nil {
		t.Errorf("a deep working-copy path must be allowed, got %v", err)
	}
}

// TestCheckinRefusesUnsyncedChanges: a pending local edit (a push that sync would
// carry) makes the profile un-releasable. checkin fails, pushes nothing, surfaces
// the pending change, and leaves the marker in place. There is no --force.
func TestCheckinRefusesUnsyncedChanges(t *testing.T) {
	name, p, id := heldFixture(t)
	local := config.ExpandRoot(p.LocalRoot)
	remote := config.ExpandRoot(p.RemoteRoot)
	if err := os.WriteFile(filepath.Join(local, "file.txt"), []byte("EDITED-LONGER"), 0o644); err != nil {
		t.Fatal(err)
	}

	rep, err := (Runner{ToolVersion: "test"}).Checkin(context.Background(), name, p, id, Options{})
	if err == nil {
		t.Fatal("checkin must refuse a profile with unsynced changes")
	}
	if len(rep.Pushed) != 1 || rep.Pushed[0] != "file.txt" {
		t.Errorf("report should surface the pending push, got Pushed=%v", rep.Pushed)
	}
	// Nothing was pushed to the remote, and the marker stays.
	if got, _ := os.ReadFile(filepath.Join(remote, "file.txt")); string(got) != "data" {
		t.Errorf("checkin must not push; remote file.txt = %q, want data", got)
	}
	if _, ok, _ := marker.Read(remote); !ok {
		t.Error("marker must remain when checkin is refused")
	}
}

func TestCheckinConflictKeepsMarker(t *testing.T) {
	name, p, id := heldFixture(t)
	local := config.ExpandRoot(p.LocalRoot)
	remote := config.ExpandRoot(p.RemoteRoot)
	if err := os.WriteFile(filepath.Join(local, "file.txt"), []byte("local-version"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(remote, "file.txt"), []byte("R"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := (Runner{ToolVersion: "test"}).Checkin(context.Background(), name, p, id, Options{})
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

func TestCheckinDryRunPreviewsWithoutReleasing(t *testing.T) {
	name, p, id := heldFixture(t)
	remote := config.ExpandRoot(p.RemoteRoot)
	rep, err := (Runner{ToolVersion: "test"}).Checkin(context.Background(), name, p, id, Options{DryRun: true})
	if err != nil {
		t.Fatalf("dry-run checkin: %v", err)
	}
	if rep.Released {
		t.Error("dry run must not release")
	}
	if _, ok, _ := marker.Read(remote); !ok {
		t.Error("marker must remain after a dry-run checkin")
	}
}
