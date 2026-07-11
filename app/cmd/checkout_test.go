package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
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

func TestCheckoutRegisteredOnRoot(t *testing.T) {
	for _, c := range newRootCommand().Commands() {
		if c.Name() == "checkout" {
			return
		}
	}
	t.Fatal("checkout command not registered on root")
}
