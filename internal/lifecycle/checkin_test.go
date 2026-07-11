package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/andresbott/netcheckout/internal/config"
	"github.com/andresbott/netcheckout/internal/marker"
)

func TestCheckinReconcilesThenReleases(t *testing.T) {
	name, p, id := heldFixture(t) // from sync_test.go (same package)
	local := config.ExpandRoot(p.LocalRoot)
	remote := config.ExpandRoot(p.RemoteRoot)
	_ = os.WriteFile(filepath.Join(local, "keep.txt"), []byte("FINAL"), 0o644)

	r := Runner{Syncer: lcSyncer{}, ToolVersion: "test"}
	rep, err := r.Checkin(context.Background(), name, p, id, Options{})
	if err != nil {
		t.Fatalf("checkin: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(remote, "keep.txt")); string(got) != "FINAL" {
		t.Errorf("remote should have the pushed content, got %q", got)
	}
	if _, ok, _ := marker.Read(remote); ok {
		t.Error("marker must be removed after checkin")
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

func TestCheckinConflictKeepsMarker(t *testing.T) {
	name, p, id := heldFixture(t)
	local := config.ExpandRoot(p.LocalRoot)
	remote := config.ExpandRoot(p.RemoteRoot)
	_ = os.WriteFile(filepath.Join(local, "keep.txt"), []byte("L"), 0o644)
	_ = os.WriteFile(filepath.Join(remote, "keep.txt"), []byte("R"), 0o644)
	r := Runner{Syncer: lcSyncer{}, ToolVersion: "test"}
	if _, err := r.Checkin(context.Background(), name, p, id, Options{}); err == nil {
		t.Fatal("want a conflict error")
	}
	if _, ok, _ := marker.Read(remote); !ok {
		t.Error("marker must remain after a conflict-stopped checkin")
	}
}
