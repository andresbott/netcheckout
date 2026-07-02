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

// TestDeleteEnterDoesNotConfirm locks in spec §8: only y/Y confirms a delete.
// enter is edit's key in list mode; treating it as confirm here would be an
// accidental-delete footgun.
func TestDeleteEnterDoesNotConfirm(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	cfg := &config.Config{Profiles: map[string]config.Profile{
		"alpha": {LocalRoot: "/l/a", RemoteRoot: "/r/a"},
	}}
	m := newModel(p, cfg)
	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeConfirm {
		t.Fatalf("enter should not confirm delete, want modeConfirm, got %d", m.mode)
	}
	if _, exists := m.cfg.Profiles["alpha"]; !exists {
		t.Error("alpha should still exist; enter must not delete")
	}
}

// TestDeleteSaveFailureKeepsProfile covers spec §9: a save failure on delete
// must not silently diverge in-memory state from disk. The model's path
// points through a regular file so config.Save's MkdirAll fails
// deterministically.
func TestDeleteSaveFailureKeepsProfile(t *testing.T) {
	p := failingSavePath(t)
	cfg := &config.Config{Profiles: map[string]config.Profile{
		"alpha": {LocalRoot: "/l/a", RemoteRoot: "/r/a"},
	}}
	m := newModel(p, cfg)
	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})

	if m.mode != modeList {
		t.Fatalf("want modeList after failed delete save, got %d", m.mode)
	}
	if m.err == nil {
		t.Fatal("expected m.err to be set after a failed save")
	}
	if _, exists := m.cfg.Profiles["alpha"]; !exists {
		t.Error("alpha should still exist in memory when the delete save fails")
	}
}
