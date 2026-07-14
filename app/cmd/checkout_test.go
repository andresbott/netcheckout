package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andresbott/netcheckout/internal/config"
	"github.com/andresbott/netcheckout/internal/lifecycle"
	"github.com/andresbott/netcheckout/internal/marker"
	"github.com/andresbott/netcheckout/internal/rsync"
)

// checkoutFakeSyncer emulates a pull by copying remote -> local.
type checkoutFakeSyncer struct{}

func (checkoutFakeSyncer) Sync(_ context.Context, j rsync.Job) (rsync.Result, error) {
	_ = filepath.Walk(j.Remote.Path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(j.Remote.Path, p)
		target := filepath.Join(j.Local.Path, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, _ := os.ReadFile(p)
		return os.WriteFile(target, data, 0o644)
	})
	if j.OnChange != nil {
		j.OnChange(rsync.Change{Path: "file.txt", Type: rsync.Created})
	}
	return rsync.Result{Changes: []rsync.Change{{Path: "file.txt", Type: rsync.Created}}}, nil
}

func (checkoutFakeSyncer) Diff(_ context.Context, _ rsync.Job) (rsync.Diff, error) {
	return rsync.Diff{Changes: []rsync.Change{{Path: "file.txt", Type: rsync.Created}}}, nil
}

func TestCheckoutCommandWritesMarker(t *testing.T) {
	t.Setenv("NETCHECKOUT_STATE", t.TempDir())
	root := t.TempDir()
	local := filepath.Join(root, "local")
	remote := filepath.Join(root, "remote")
	_ = os.MkdirAll(remote, 0o755)
	_ = os.WriteFile(filepath.Join(remote, "file.txt"), []byte("x"), 0o644)

	cfgPath := writeStatusTestConfig(t, map[string]config.Profile{
		"work": {LocalRoot: local, RemoteRoot: remote},
	})
	runner := lifecycle.Runner{Syncer: checkoutFakeSyncer{}, ToolVersion: "test"}
	cmd := newCheckoutCmdWithRunner(&cfgPath, runner)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"work"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := marker.Read(remote); !ok {
		t.Fatal("expected a marker after checkout")
	}
}

func TestCheckoutCommandRefusesNonEmptyLocal(t *testing.T) {
	t.Setenv("NETCHECKOUT_STATE", t.TempDir())
	root := t.TempDir()
	local := filepath.Join(root, "local")
	remote := filepath.Join(root, "remote")
	_ = os.MkdirAll(remote, 0o755)
	_ = os.WriteFile(filepath.Join(remote, "file.txt"), []byte("x"), 0o644)
	// Pre-existing local content must block the checkout.
	_ = os.MkdirAll(local, 0o755)
	_ = os.WriteFile(filepath.Join(local, "keep.dat"), []byte("mine"), 0o644)

	cfgPath := writeStatusTestConfig(t, map[string]config.Profile{
		"work": {LocalRoot: local, RemoteRoot: remote},
	})
	runner := lifecycle.Runner{Syncer: checkoutFakeSyncer{}, ToolVersion: "test"}
	cmd := newCheckoutCmdWithRunner(&cfgPath, runner)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	// --force must not bypass the guard.
	cmd.SetArgs([]string{"work", "--force"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected checkout to refuse a non-empty local target")
	}
	if !strings.Contains(err.Error(), "not empty") {
		t.Errorf("want a 'not empty' error, got: %v", err)
	}
	if _, ok, _ := marker.Read(remote); ok {
		t.Error("refused checkout must not write a marker")
	}
}

func TestCheckoutRegisteredOnRoot(t *testing.T) {
	for _, c := range newRootCommand().Commands() {
		if c.Name() == "checkout" {
			return
		}
	}
	t.Fatal("checkout command not registered on root")
}

func TestCheckoutCommandPrintsLockedMessage(t *testing.T) {
	t.Setenv("NETCHECKOUT_STATE", t.TempDir())
	root := t.TempDir()
	local := filepath.Join(root, "local")
	remote := filepath.Join(root, "remote")
	_ = os.MkdirAll(remote, 0o755)
	_ = os.WriteFile(filepath.Join(remote, "file.txt"), []byte("x"), 0o644)

	cfgPath := writeStatusTestConfig(t, map[string]config.Profile{
		"work": {LocalRoot: local, RemoteRoot: remote},
	})
	runner := lifecycle.Runner{Syncer: checkoutFakeSyncer{}, ToolVersion: "test"}
	cmd := newCheckoutCmdWithRunner(&cfgPath, runner)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"work"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	// Checkout only locks now: it reports the lock and points at sync, and copies
	// no per-file changes.
	if !strings.Contains(out, "checked out") || !strings.Contains(out, "sync") {
		t.Errorf("want a 'checked out … run sync' summary, got:\n%s", out)
	}
	if entries, err := os.ReadDir(local); err == nil && len(entries) != 0 {
		t.Errorf("checkout must not populate the local dir, got %d entries", len(entries))
	}
}

func TestCheckoutDryRunPrintsNoChangeLines(t *testing.T) {
	t.Setenv("NETCHECKOUT_STATE", t.TempDir())
	root := t.TempDir()
	local := filepath.Join(root, "local")
	remote := filepath.Join(root, "remote")
	_ = os.MkdirAll(remote, 0o755)
	_ = os.WriteFile(filepath.Join(remote, "file.txt"), []byte("x"), 0o644)

	cfgPath := writeStatusTestConfig(t, map[string]config.Profile{
		"work": {LocalRoot: local, RemoteRoot: remote},
	})
	runner := lifecycle.Runner{Syncer: checkoutFakeSyncer{}, ToolVersion: "test"}
	cmd := newCheckoutCmdWithRunner(&cfgPath, runner)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"work", "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "→") {
		t.Errorf("dry run must not stream applied changes, got:\n%s", buf.String())
	}
}
