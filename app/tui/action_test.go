package tui

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/andresbott/netcheckout/internal/config"
	"github.com/andresbott/netcheckout/internal/ident"
	"github.com/andresbott/netcheckout/internal/lifecycle"
	"github.com/andresbott/netcheckout/internal/marker"
	"github.com/andresbott/netcheckout/internal/rsync"
	tea "github.com/charmbracelet/bubbletea"
)

type tuiFakeSyncer struct{}

func (tuiFakeSyncer) Sync(_ context.Context, j rsync.Job) (rsync.Result, error) {
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
	return rsync.Result{Changes: []rsync.Change{{Path: "f", Type: rsync.Created}}}, nil
}
func (tuiFakeSyncer) Diff(_ context.Context, _ rsync.Job) (rsync.Diff, error) {
	return rsync.Diff{}, nil
}

func TestCheckoutCmdProducesMarker(t *testing.T) {
	t.Setenv("NETCHECKOUT_STATE", t.TempDir())
	root := t.TempDir()
	local := filepath.Join(root, "local")
	remote := filepath.Join(root, "remote")
	_ = os.MkdirAll(remote, 0o755)
	_ = os.WriteFile(filepath.Join(remote, "f"), []byte("x"), 0o644)

	runner := lifecycle.Runner{Syncer: tuiFakeSyncer{}, ToolVersion: "test"}
	p := config.Profile{LocalRoot: local, RemoteRoot: remote}
	msg := checkoutCmd(runner, ident.Ident{By: "me@host", Host: "host"}, "work", p, lifecycle.Options{})()
	res, ok := msg.(actionResultMsg)
	if !ok {
		t.Fatalf("want actionResultMsg, got %T", msg)
	}
	if res.err != nil {
		t.Fatalf("checkout cmd err: %v", res.err)
	}
	if _, ok, _ := marker.Read(remote); !ok {
		t.Error("checkout cmd did not write a marker")
	}
}

func TestToggleForceAndClean(t *testing.T) {
	m := model{sub: subActions}
	m.profile = newProfileView("work")
	m.profile.cursor = 1 // Checkout row (not toggled by these keys)
	m2, _ := m.updateProfile(keyMsg("f"))
	if !m2.(model).actForce {
		t.Error("f should toggle force on")
	}
	m3, _ := m2.(model).updateProfile(keyMsg("c"))
	if !m3.(model).actClean {
		t.Error("c should toggle clean on")
	}
}

func keyMsg(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
