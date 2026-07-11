//go:build integration

package rsync_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/andresbott/netcheckout/internal/rsync"
)

func hasChange(changes []rsync.Change, path string, typ rsync.ChangeType) bool {
	for _, c := range changes {
		if c.Path == path && c.Type == typ {
			return true
		}
	}
	return false
}

func TestIntegrationPushDiffSyncDelete(t *testing.T) {
	if _, err := exec.LookPath("rsync"); err != nil {
		t.Skip("rsync not on PATH")
	}
	root := t.TempDir()
	local := filepath.Join(root, "local")
	remote := filepath.Join(root, "remote")
	for _, d := range []string{local, remote, filepath.Join(local, "sub")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(local, "a.txt"), []byte("A"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(local, "sub", "b.txt"), []byte("B"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := rsync.New()
	ctx := context.Background()
	push := rsync.Job{
		Local:     rsync.Endpoint{Path: local},
		Remote:    rsync.Endpoint{Path: remote},
		Direction: rsync.Push,
	}

	// Dry-run diff shows the new files.
	d, err := s.Diff(ctx, push)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if !hasChange(d.Changes, "a.txt", rsync.Created) || !hasChange(d.Changes, "sub/b.txt", rsync.Created) {
		t.Fatalf("diff changes = %#v", d.Changes)
	}

	// Real sync propagates them.
	if _, err := s.Sync(ctx, push); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if _, err := os.Stat(filepath.Join(remote, "a.txt")); err != nil {
		t.Fatalf("remote a.txt missing: %v", err)
	}

	// A second diff is in sync.
	d, err = s.Diff(ctx, push)
	if err != nil {
		t.Fatal(err)
	}
	if !d.InSync {
		t.Fatalf("expected in sync, got %#v", d.Changes)
	}

	// Remove a local file; with Delete the diff reports it and sync removes it remotely.
	if err := os.Remove(filepath.Join(local, "a.txt")); err != nil {
		t.Fatal(err)
	}
	pushDel := push
	pushDel.Options.Delete = true
	d, err = s.Diff(ctx, pushDel)
	if err != nil {
		t.Fatal(err)
	}
	if !hasChange(d.Changes, "a.txt", rsync.Deleted) {
		t.Fatalf("expected a.txt deleted, got %#v", d.Changes)
	}
	if _, err := s.Sync(ctx, pushDel); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(remote, "a.txt")); !os.IsNotExist(err) {
		t.Fatalf("remote a.txt should be gone, stat err = %v", err)
	}
}

func TestSyncFilesFromTransfersOnlyListed(t *testing.T) {
	if _, err := exec.LookPath("rsync"); err != nil {
		t.Skip("rsync not on PATH")
	}
	src := t.TempDir()
	dst := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "keep.txt"), []byte("k"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "skip.txt"), []byte("s"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := rsync.New()
	_, err := s.Sync(context.Background(), rsync.Job{
		Local:     rsync.Endpoint{Path: dst},
		Remote:    rsync.Endpoint{Path: src},
		Direction: rsync.Pull,
		Files:     []string{"keep.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "keep.txt")); err != nil {
		t.Errorf("keep.txt should have been pulled: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "skip.txt")); !os.IsNotExist(err) {
		t.Errorf("skip.txt should NOT have been pulled")
	}
}
