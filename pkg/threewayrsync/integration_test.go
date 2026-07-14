//go:build integration

package threewayrsync_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	tw "github.com/andresbott/netcheckout/pkg/threewayrsync"
)

func requireRsync(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("rsync"); err != nil {
		t.Skip("rsync not on PATH")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newSyncer(t *testing.T) (*tw.Syncer, tw.Endpoint, tw.Endpoint) {
	t.Helper()
	root := t.TempDir()
	local := filepath.Join(root, "local")
	remote := filepath.Join(root, "remote")
	for _, d := range []string{local, remote} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	store := tw.FileStore{Path: filepath.Join(root, "state", "base.json")}
	return tw.New(store), tw.Endpoint{Path: local}, tw.Endpoint{Path: remote}
}

func TestIntegrationFullCycle(t *testing.T) {
	requireRsync(t)
	s, local, remote := newSyncer(t)
	ctx := context.Background()

	// Local has files, remote and base empty => push both to remote.
	writeFile(t, filepath.Join(local.Path, "a.txt"), "A")
	writeFile(t, filepath.Join(local.Path, "sub", "b.txt"), "BB")

	if _, err := s.Sync(ctx, local, remote, tw.Options{}); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if _, err := os.Stat(filepath.Join(remote.Path, "a.txt")); err != nil {
		t.Fatalf("a.txt should have been pushed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(remote.Path, "sub", "b.txt")); err != nil {
		t.Fatalf("sub/b.txt should have been pushed: %v", err)
	}

	// Second diff is in sync.
	plan, err := s.Diff(ctx, local, remote, tw.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.InSync {
		t.Fatalf("expected in sync, got %+v", plan)
	}

	// Edit the remote (larger content => size change) => next sync pulls it local.
	writeFile(t, filepath.Join(remote.Path, "a.txt"), "A-EDITED")
	plan, err = s.Diff(ctx, local, remote, tw.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Pull) != 1 || plan.Pull[0] != "a.txt" {
		t.Fatalf("expected a.txt pull, got %+v", plan)
	}
	if _, err := s.Sync(ctx, local, remote, tw.Options{}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(local.Path, "a.txt"))
	if err != nil || string(got) != "A-EDITED" {
		t.Fatalf("local a.txt = %q err %v", string(got), err)
	}

	// Delete sub/b.txt locally (remote unchanged) => sync removes it remotely.
	if err := os.Remove(filepath.Join(local.Path, "sub", "b.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Sync(ctx, local, remote, tw.Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(remote.Path, "sub", "b.txt")); !os.IsNotExist(err) {
		t.Fatalf("remote sub/b.txt should be gone, stat err = %v", err)
	}
}

func TestIntegrationConflictAbort(t *testing.T) {
	requireRsync(t)
	s, local, remote := newSyncer(t)
	ctx := context.Background()

	// Establish a shared baseline for x.txt.
	writeFile(t, filepath.Join(local.Path, "x.txt"), "orig")
	if _, err := s.Sync(ctx, local, remote, tw.Options{}); err != nil {
		t.Fatal(err)
	}
	// Edit both sides to different sizes => conflict.
	writeFile(t, filepath.Join(local.Path, "x.txt"), "local-change")
	writeFile(t, filepath.Join(remote.Path, "x.txt"), "R")

	_, err := s.Sync(ctx, local, remote, tw.Options{Conflict: tw.Abort})
	var ce *tw.ConflictError
	if !errors.As(err, &ce) || len(ce.Paths) != 1 || ce.Paths[0] != "x.txt" {
		t.Fatalf("want ConflictError for x.txt, got %v", err)
	}
	// Abort changed nothing: remote still "R".
	got, _ := os.ReadFile(filepath.Join(remote.Path, "x.txt"))
	if string(got) != "R" {
		t.Errorf("remote must be untouched on Abort, got %q", string(got))
	}
}

func TestIntegrationResumeIsIdempotent(t *testing.T) {
	requireRsync(t)
	s, local, remote := newSyncer(t)
	ctx := context.Background()

	writeFile(t, filepath.Join(local.Path, "a.txt"), "A")
	if _, err := s.Sync(ctx, local, remote, tw.Options{}); err != nil {
		t.Fatal(err)
	}
	// Running again re-derives from live state: everything has converged => no-op.
	res, err := s.Sync(ctx, local, remote, tw.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Applied.Push)+len(res.Applied.Pull)+len(res.Applied.RemoteDeletes)+len(res.Applied.LocalDeletes) != 0 {
		t.Errorf("second sync should apply nothing, got %+v", res.Applied)
	}
}
