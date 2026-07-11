package tui

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/andresbott/netcheckout/internal/baseline"
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

// actionIndex returns the cursor position of name within profileActions, so
// tests can position the cursor without hardcoding row indices.
func actionIndex(name string) int {
	for i, a := range profileActions {
		if a == name {
			return i
		}
	}
	return -1
}

// tuiHeldFixture mirrors internal/lifecycle's heldFixture (see
// internal/lifecycle/sync_test.go): a profile already checked out, with a
// marker and baseline snapshot on both sides, so Sync has a held checkout to
// reconcile.
func tuiHeldFixture(t *testing.T) (name string, p config.Profile, id ident.Ident) {
	t.Helper()
	root := t.TempDir()
	local := filepath.Join(root, "local")
	remote := filepath.Join(root, "remote")
	_ = os.MkdirAll(local, 0o755)
	_ = os.MkdirAll(remote, 0o755)
	_ = os.WriteFile(filepath.Join(local, "keep.txt"), []byte("base"), 0o644)
	_ = os.WriteFile(filepath.Join(remote, "keep.txt"), []byte("base"), 0o644)
	id = ident.Ident{By: "me@host", Host: "host"}
	_ = marker.Write(remote, &marker.Marker{CheckedOutBy: id.By, Host: id.Host, Profile: "work", Relpaths: []string{"."}})
	files, _ := baseline.Snapshot(local, []string{"."})
	_ = baseline.Save(&baseline.Baseline{Profile: "work", Relpaths: []string{"."}, Files: files, LastSyncAt: time.Unix(0, 0)})
	return "work", config.Profile{LocalRoot: local, RemoteRoot: remote}, id
}

func TestSyncCmdProducesResult(t *testing.T) {
	t.Setenv("NETCHECKOUT_STATE", t.TempDir())
	name, p, id := tuiHeldFixture(t)
	// Edit locally after checkout so Sync has something to push.
	_ = os.WriteFile(filepath.Join(p.LocalRoot, "keep.txt"), []byte("EDITED"), 0o644)

	runner := lifecycle.Runner{Syncer: tuiFakeSyncer{}, ToolVersion: "test"}
	msg := syncCmd(runner, id, name, p, lifecycle.Options{})()
	res, ok := msg.(actionResultMsg)
	if !ok {
		t.Fatalf("want actionResultMsg, got %T", msg)
	}
	if res.err != nil {
		t.Fatalf("sync cmd err: %v", res.err)
	}
	if len(res.report.Pushed) == 0 {
		t.Error("want a non-empty Pushed list after editing a local file")
	}
}

func TestCheckinOpensConfirmModal(t *testing.T) {
	m := model{sub: subActions, cfg: &config.Config{Profiles: map[string]config.Profile{"work": {}}}}
	m.profile = newProfileView("work")
	m.profile.cursor = actionIndex("Check-in")
	m2, _ := m.updateProfile(keyMsg("enter"))
	if m2.(model).mode != modeConfirm {
		t.Fatal("Check-in Enter should open the confirm modal")
	}
	if m2.(model).confirmKind != confirmCheckin {
		t.Errorf("confirmKind = %v, want confirmCheckin", m2.(model).confirmKind)
	}
}
