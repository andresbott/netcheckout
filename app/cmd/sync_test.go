package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/andresbott/netcheckout/internal/baseline"
	"github.com/andresbott/netcheckout/internal/config"
	"github.com/andresbott/netcheckout/internal/lifecycle"
	"github.com/andresbott/netcheckout/internal/marker"
	"github.com/andresbott/netcheckout/internal/rsync"
)

// cmdCopySyncer copies listed files between roots (like reconcile's fake).
type cmdCopySyncer struct{}

func (cmdCopySyncer) Sync(_ context.Context, j rsync.Job) (rsync.Result, error) {
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
		_ = os.WriteFile(filepath.Join(dst, f), data, 0o644)
	}
	return rsync.Result{}, nil
}
func (cmdCopySyncer) Diff(context.Context, rsync.Job) (rsync.Diff, error) { return rsync.Diff{}, nil }

func heldCmdFixture(t *testing.T) (cfgPath, remote string) {
	t.Helper()
	t.Setenv("NETCHECKOUT_STATE", t.TempDir())
	root := t.TempDir()
	local := filepath.Join(root, "local")
	remote = filepath.Join(root, "remote")
	_ = os.MkdirAll(local, 0o755)
	_ = os.MkdirAll(remote, 0o755)
	_ = os.WriteFile(filepath.Join(local, "keep.txt"), []byte("base"), 0o644)
	_ = os.WriteFile(filepath.Join(remote, "keep.txt"), []byte("base"), 0o644)
	_ = marker.Write(remote, &marker.Marker{CheckedOutBy: hostIdentity(t), Host: hostName(t), Profile: "work", Relpaths: []string{"."}})
	files, _ := baseline.Snapshot(local, []string{"."})
	_ = baseline.Save(&baseline.Baseline{Profile: "work", Relpaths: []string{"."}, Files: files})
	cfgPath = writeStatusTestConfig(t, map[string]config.Profile{"work": {LocalRoot: local, RemoteRoot: remote}})
	return cfgPath, remote
}

func hostName(t *testing.T) string { t.Helper(); h, _ := os.Hostname(); return h }
func hostIdentity(t *testing.T) string {
	t.Helper()
	// config has no identity, so ident.Resolve yields $USER@$HOSTNAME.
	u := os.Getenv("USER")
	h, _ := os.Hostname()
	if u == "" {
		return h
	}
	return u + "@" + h
}

func TestSyncCommandPushesLocalEdit(t *testing.T) {
	cfgPath, remote := heldCmdFixture(t)
	// Edit locally.
	cfg, _ := config.Load(cfgPath)
	lroot := cfg.Profiles["work"].LocalRoot
	_ = os.WriteFile(filepath.Join(lroot, "keep.txt"), []byte("EDITED"), 0o644)

	cmd := newSyncCmdWithRunner(&cfgPath, lifecycle.Runner{Syncer: cmdCopySyncer{}, ToolVersion: "test"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"work"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(remote, "keep.txt")); string(got) != "EDITED" {
		t.Errorf("remote keep.txt = %q, want EDITED", got)
	}
	if _, ok, _ := marker.Read(remote); !ok {
		t.Error("sync must leave the marker in place")
	}
}

func TestSyncRegisteredOnRoot(t *testing.T) {
	for _, c := range newRootCommand().Commands() {
		if c.Name() == "sync" {
			return
		}
	}
	t.Fatal("sync not registered")
}
