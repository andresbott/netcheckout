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

// fakeSyncer copies the source tree to the dest for Sync (so the local copy is
// materialized) and records the jobs it saw. Diff returns a canned diff.
type fakeSyncer struct {
	syncErr error
	diff    rsync.Diff
	jobs    []rsync.Job
}

func (f *fakeSyncer) Sync(_ context.Context, j rsync.Job) (rsync.Result, error) {
	f.jobs = append(f.jobs, j)
	if f.syncErr != nil {
		return rsync.Result{}, f.syncErr
	}
	// Emulate rsync pull: copy Remote -> Local.
	_ = copyTree(j.Remote.Path, j.Local.Path)
	return rsync.Result{Changes: []rsync.Change{{Path: "file.txt", Type: rsync.Created}}}, nil
}

func (f *fakeSyncer) Diff(_ context.Context, j rsync.Job) (rsync.Diff, error) {
	f.jobs = append(f.jobs, j)
	return f.diff, nil
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func fixture(t *testing.T) (local, remote string) {
	t.Helper()
	root := t.TempDir()
	local = filepath.Join(root, "local")
	remote = filepath.Join(root, "remote")
	if err := os.MkdirAll(remote, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(remote, "file.txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	return local, remote
}

func TestCheckoutWritesMarkerAndBaseline(t *testing.T) {
	t.Setenv("NETCHECKOUT_STATE", t.TempDir())
	local, remote := fixture(t)
	r := Runner{Syncer: &fakeSyncer{}, ToolVersion: "test", Now: func() time.Time { return time.Unix(0, 0).UTC() }}
	rep, err := r.Checkout(context.Background(), "work", config.Profile{LocalRoot: local, RemoteRoot: remote}, ident.Ident{By: "me@host", Host: "host"}, "", Options{})
	if err != nil {
		t.Fatalf("checkout: %v", err)
	}
	// Marker written, owned by us.
	m, ok, _ := marker.Read(remote)
	if !ok || !m.OwnedBy("me@host", "host") || m.ToolVersion != "test" {
		t.Fatalf("marker = %+v ok=%v", m, ok)
	}
	// Baseline written with the pulled file.
	b, ok, _ := baseline.Load("work")
	if !ok || b.Files["file.txt"].Hash == "" {
		t.Fatalf("baseline = %+v ok=%v", b, ok)
	}
	if len(rep.Pulled) == 0 {
		t.Error("report should list pulled files")
	}
}

func TestCheckoutRefusesForeignMarker(t *testing.T) {
	t.Setenv("NETCHECKOUT_STATE", t.TempDir())
	local, remote := fixture(t)
	_ = marker.Write(remote, &marker.Marker{CheckedOutBy: "alice@nas", Host: "nas", Profile: "work"})
	r := Runner{Syncer: &fakeSyncer{}, ToolVersion: "test"}
	_, err := r.Checkout(context.Background(), "work", config.Profile{LocalRoot: local, RemoteRoot: remote}, ident.Ident{By: "me@host", Host: "host"}, "", Options{})
	if err == nil {
		t.Fatal("want refusal on a foreign marker")
	}
}

func TestCheckoutForceOverridesForeignMarker(t *testing.T) {
	t.Setenv("NETCHECKOUT_STATE", t.TempDir())
	local, remote := fixture(t)
	_ = marker.Write(remote, &marker.Marker{CheckedOutBy: "alice@nas", Host: "nas", Profile: "work"})
	r := Runner{Syncer: &fakeSyncer{}, ToolVersion: "test"}
	if _, err := r.Checkout(context.Background(), "work", config.Profile{LocalRoot: local, RemoteRoot: remote}, ident.Ident{By: "me@host", Host: "host"}, "", Options{Force: true}); err != nil {
		t.Fatalf("force checkout: %v", err)
	}
	m, _, _ := marker.Read(remote)
	if !m.OwnedBy("me@host", "host") {
		t.Errorf("force should rewrite the marker to us, got %+v", m)
	}
}

func TestCheckoutDryRunWritesNothing(t *testing.T) {
	t.Setenv("NETCHECKOUT_STATE", t.TempDir())
	local, remote := fixture(t)
	fs := &fakeSyncer{diff: rsync.Diff{Changes: []rsync.Change{{Path: "file.txt", Type: rsync.Created}}}}
	r := Runner{Syncer: fs, ToolVersion: "test"}
	rep, err := r.Checkout(context.Background(), "work", config.Profile{LocalRoot: local, RemoteRoot: remote}, ident.Ident{By: "me@host", Host: "host"}, "", Options{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := marker.Read(remote); ok {
		t.Error("dry-run must not write a marker")
	}
	if _, ok, _ := baseline.Load("work"); ok {
		t.Error("dry-run must not write a baseline")
	}
	if !rep.DryRun || len(rep.Pulled) == 0 {
		t.Errorf("dry-run report = %+v", rep)
	}
}

func TestCheckoutRollsBackBaselineOnMarkerFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("cannot test write-permission failure as root")
	}
	t.Setenv("NETCHECKOUT_STATE", t.TempDir())
	local, remote := fixture(t)
	// Make the remote root read-only so marker.Write's os.CreateTemp(remoteRoot, ...) fails
	// AFTER the transfer and baseline write have succeeded. Restore perms so cleanup works.
	if err := os.Chmod(remote, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(remote, 0o755) })

	r := Runner{Syncer: &fakeSyncer{}, ToolVersion: "test"}
	_, err := r.Checkout(context.Background(), "work", config.Profile{LocalRoot: local, RemoteRoot: remote}, ident.Ident{By: "me@host", Host: "host"}, "", Options{})
	if err == nil {
		t.Fatal("expected checkout to fail when the marker cannot be written")
	}
	// Fresh checkout: the baseline written just before the failed marker write must be rolled back.
	if _, ok, _ := baseline.Load("work"); ok {
		t.Error("baseline should be rolled back after a marker-write failure on a fresh checkout")
	}
	// And no marker was left behind.
	if _, ok, _ := marker.Read(remote); ok {
		t.Error("no marker should exist after a failed marker write")
	}
}
