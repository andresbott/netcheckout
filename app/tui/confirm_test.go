package tui

import (
	"path/filepath"
	"testing"

	"github.com/andresbott/netcheckout/internal/config"
	tea "github.com/charmbracelet/bubbletea"
)

func TestDeleteConfirmed(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	cfg := &config.Config{Profiles: map[string]config.Profile{
		"alpha": {LocalRoot: "/l/a", RemoteRoot: "/r/a"},
		"beta":  {LocalRoot: "/l/b", RemoteRoot: "/r/b"},
	}}
	if err := config.Save(p, cfg); err != nil {
		t.Fatal(err)
	}
	m := newModel(p, cfg)
	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")}) // "alpha" is first
	if m.mode != modeConfirm {
		t.Fatalf("want modeConfirm, got %d", m.mode)
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if m.mode != modeList {
		t.Fatalf("want modeList after delete, got %d", m.mode)
	}
	if _, exists := m.cfg.Profiles["alpha"]; exists {
		t.Error("alpha should be deleted")
	}
	saved, _ := config.Load(p)
	if _, exists := saved.Profiles["alpha"]; exists {
		t.Error("delete should be persisted")
	}
}

func TestDeleteCancelled(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	cfg := &config.Config{Profiles: map[string]config.Profile{
		"alpha": {LocalRoot: "/l/a", RemoteRoot: "/r/a"},
	}}
	m := newModel(p, cfg)
	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if m.mode != modeList {
		t.Fatalf("want modeList after cancel, got %d", m.mode)
	}
	if _, exists := m.cfg.Profiles["alpha"]; !exists {
		t.Error("alpha should still exist after cancel")
	}
}
