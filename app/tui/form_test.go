package tui

import (
	"path/filepath"
	"testing"

	"github.com/andresbott/netcheckout/internal/config"
	tea "github.com/charmbracelet/bubbletea"
)

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

	if m.mode != modeList {
		t.Fatalf("want modeList after save, got %d", m.mode)
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
	if m.mode != modeList {
		t.Fatal("esc should return to the list")
	}
}
