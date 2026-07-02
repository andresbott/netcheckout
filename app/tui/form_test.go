package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andresbott/netcheckout/internal/config"
	tea "github.com/charmbracelet/bubbletea"
)

// TestFormViewHasBorder guards visual consistency with the table view: the
// form's labeled inputs must render inside the same thick border.
func TestFormViewHasBorder(t *testing.T) {
	f := newForm("photos", config.Profile{LocalRoot: "/l", RemoteRoot: "/r"})
	if view := f.View(); !strings.Contains(view, "┏") || !strings.Contains(view, "┛") {
		t.Fatalf("form view missing thick border corners:\n%s", view)
	}
}

func typeRunes(t *testing.T, m model, s string) model {
	t.Helper()
	return update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
}

func TestAddProfilePersists(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	m := newModel(p, &config.Config{Profiles: map[string]config.Profile{}})

	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")}) // open add form
	if m.mode != modeForm {
		t.Fatalf("want modeForm, got %d", m.mode)
	}
	m = typeRunes(t, m, "photos")
	m = update(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m = typeRunes(t, m, "/home/me/pics")
	m = update(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m = typeRunes(t, m, "/mnt/nas/pics")
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // submit

	if m.mode != modeTable {
		t.Fatalf("want modeTable after save, got %d", m.mode)
	}
	saved, err := config.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	got := saved.Profiles["photos"]
	if got.LocalRoot != "/home/me/pics" || got.RemoteRoot != "/mnt/nas/pics" {
		t.Fatalf("persisted profile = %#v", got)
	}
}

func TestAddProfileValidationBlocks(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	m := newModel(p, &config.Config{Profiles: map[string]config.Profile{}})
	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // empty name
	if m.mode != modeForm {
		t.Fatal("empty name should keep the form open")
	}
	if m.form.err == "" {
		t.Fatal("expected a validation error message")
	}
}

func TestEditRenameReplacesKey(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	cfg := &config.Config{Profiles: map[string]config.Profile{
		"old": {LocalRoot: "/l", RemoteRoot: "/r"},
	}}
	if err := config.Save(p, cfg); err != nil {
		t.Fatal(err)
	}
	m := newModel(p, cfg)
	m.mode = modeForm
	m.form = newForm("old", cfg.Profiles["old"])
	m.form.inputs[0].SetValue("new")
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if _, exists := m.cfg.Profiles["old"]; exists {
		t.Error("old key should be removed after rename")
	}
	if _, exists := m.cfg.Profiles["new"]; !exists {
		t.Error("new key should exist after rename")
	}
}

func TestFormCancel(t *testing.T) {
	m := newModel("/tmp/x.yaml", testConfig())
	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != modeTable {
		t.Fatal("esc should return to the list")
	}
}

// failingSavePath returns a path whose parent directory can never be created:
// <dir>/notadir is a regular file, so config.Save's MkdirAll deterministically
// fails with "not a directory".
func failingSavePath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	blocker := filepath.Join(dir, "notadir")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(blocker, "config.yaml")
}

// TestAddProfileSaveFailureKeepsFormAndEdits covers spec §9: a save failure
// must be surfaced in the TUI without losing the user's edits or committing
// the change in memory ahead of disk.
func TestAddProfileSaveFailureKeepsFormAndEdits(t *testing.T) {
	p := failingSavePath(t)
	m := newModel(p, &config.Config{Profiles: map[string]config.Profile{}})

	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")}) // open add form
	m = typeRunes(t, m, "photos")
	m = update(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m = typeRunes(t, m, "/home/me/pics")
	m = update(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m = typeRunes(t, m, "/mnt/nas/pics")
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // submit; save should fail

	if m.mode != modeForm {
		t.Fatalf("want modeForm to stay open after save failure, got %d", m.mode)
	}
	if m.form.err == "" {
		t.Fatal("expected a save-failure error message on the form")
	}
	if got := m.form.inputs[0].Value(); got != "photos" {
		t.Fatalf("name input = %q, want it preserved", got)
	}
	if got := m.form.inputs[1].Value(); got != "/home/me/pics" {
		t.Fatalf("local root input = %q, want it preserved", got)
	}
	if got := m.form.inputs[2].Value(); got != "/mnt/nas/pics" {
		t.Fatalf("remote root input = %q, want it preserved", got)
	}
	if _, exists := m.cfg.Profiles["photos"]; exists {
		t.Error("profile should not be committed to memory when save fails")
	}
}

// TestEditRenameSaveFailureRollsBackProfiles covers the rename branch of
// submitForm: the old key is deleted and the new key set before saving, so a
// failed save must restore the original map wholesale (old key back, new key
// gone), not just undo one half of the mutation.
func TestEditRenameSaveFailureRollsBackProfiles(t *testing.T) {
	p := failingSavePath(t)
	cfg := &config.Config{Profiles: map[string]config.Profile{
		"old": {LocalRoot: "/l", RemoteRoot: "/r"},
	}}
	m := newModel(p, cfg)
	m.mode = modeForm
	m.form = newForm("old", cfg.Profiles["old"])
	m.form.inputs[0].SetValue("new")
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // submit; save should fail

	if m.mode != modeForm {
		t.Fatalf("want modeForm to stay open after save failure, got %d", m.mode)
	}
	if m.form.err == "" {
		t.Fatal("expected a save-failure error message on the form")
	}
	if _, exists := m.cfg.Profiles["old"]; !exists {
		t.Error("old profile should be restored when save fails")
	}
	if _, exists := m.cfg.Profiles["new"]; exists {
		t.Error("new profile should not be committed when save fails")
	}
}
