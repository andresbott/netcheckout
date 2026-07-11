package tui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/andresbott/netcheckout/internal/config"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	if m.mode != modeMain {
		t.Fatalf("want modeMain after delete, got %d", m.mode)
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
	if m.mode != modeMain {
		t.Fatalf("want modeMain after cancel, got %d", m.mode)
	}
	if _, exists := m.cfg.Profiles["alpha"]; !exists {
		t.Error("alpha should still exist after cancel")
	}
}

// TestDeleteEnterOnDefaultFocusCancelsNotDeletes: the dialog opens with Cancel
// focused (the safe default), so a bare enter right after opening activates
// Cancel — it must never delete on the very first keystroke.
func TestDeleteEnterOnDefaultFocusCancelsNotDeletes(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	cfg := &config.Config{Profiles: map[string]config.Profile{
		"alpha": {LocalRoot: "/l/a", RemoteRoot: "/r/a"},
	}}
	m := newModel(p, cfg)
	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if m.confirmFocus != confirmFocusCancel {
		t.Fatalf("dialog should open with Cancel focused, got %d", m.confirmFocus)
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeMain {
		t.Fatalf("enter on the default-focused Cancel should return to main, mode = %d", m.mode)
	}
	if _, exists := m.cfg.Profiles["alpha"]; !exists {
		t.Error("alpha should still exist; a bare enter must not delete")
	}
}

// TestDeleteFocusedButtonDeletes: moving focus to Delete (via Tab) and
// activating it removes the profile and persists it — the new button-driven
// path, held to the same persistence rigor as TestDeleteConfirmed's y/Y path.
func TestDeleteFocusedButtonDeletes(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	cfg := &config.Config{Profiles: map[string]config.Profile{
		"alpha": {LocalRoot: "/l/a", RemoteRoot: "/r/a"},
	}}
	if err := config.Save(p, cfg); err != nil {
		t.Fatal(err)
	}
	m := newModel(p, cfg)
	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = update(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.confirmFocus != confirmFocusDelete {
		t.Fatalf("tab should move focus to Delete, got %d", m.confirmFocus)
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeMain {
		t.Fatalf("enter on focused Delete should return to main, mode = %d", m.mode)
	}
	if _, exists := m.cfg.Profiles["alpha"]; exists {
		t.Error("alpha should be deleted")
	}
	saved, err := config.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := saved.Profiles["alpha"]; exists {
		t.Error("delete via the button path should be persisted")
	}
}

// TestDeleteFocusLeftRight: left/right toggle focus between Delete and Cancel,
// mirroring the add/edit form's Save/Cancel toggle.
func TestDeleteFocusLeftRight(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	cfg := &config.Config{Profiles: map[string]config.Profile{
		"alpha": {LocalRoot: "/l/a", RemoteRoot: "/r/a"},
	}}
	m := newModel(p, cfg)
	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = update(t, m, tea.KeyMsg{Type: tea.KeyRight})
	if m.confirmFocus != confirmFocusDelete {
		t.Fatalf("right from Cancel should focus Delete, got %d", m.confirmFocus)
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyLeft})
	if m.confirmFocus != confirmFocusCancel {
		t.Fatalf("left from Delete should focus Cancel, got %d", m.confirmFocus)
	}
}

// TestDeleteFocusTabCycles: tab and shift+tab also toggle focus between the two
// buttons.
func TestDeleteFocusTabCycles(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	cfg := &config.Config{Profiles: map[string]config.Profile{
		"alpha": {LocalRoot: "/l/a", RemoteRoot: "/r/a"},
	}}
	m := newModel(p, cfg)
	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = update(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.confirmFocus != confirmFocusDelete {
		t.Fatalf("tab from Cancel should focus Delete, got %d", m.confirmFocus)
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.confirmFocus != confirmFocusCancel {
		t.Fatalf("shift+tab from Delete should focus Cancel, got %d", m.confirmFocus)
	}
}

// TestConfirmModalHasDeleteCancelButtons guards the new button UI: the modal
// must render both bracketed buttons and the shared hint line.
func TestConfirmModalHasDeleteCancelButtons(t *testing.T) {
	view := confirmModal(confirmDelete, "alpha", confirmFocusCancel, 80)
	for _, want := range []string{"[ Delete ]", "[ Cancel ]", "Move", "Activate", "Cancel"} {
		if !strings.Contains(view, want) {
			t.Fatalf("confirm modal missing %q:\n%s", want, view)
		}
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

	if m.mode != modeMain {
		t.Fatalf("want modeMain after failed delete save, got %d", m.mode)
	}
	if m.err == nil {
		t.Fatal("expected m.err to be set after a failed save")
	}
	if _, exists := m.cfg.Profiles["alpha"]; !exists {
		t.Error("alpha should still exist in memory when the delete save fails")
	}
}

func TestConfirmModalFitsWidthWithLongName(t *testing.T) {
	longName := strings.Repeat("x", 60)
	cfg := &config.Config{Profiles: map[string]config.Profile{
		longName: {LocalRoot: "/l", RemoteRoot: "/r"},
	}}
	m := newModel("/tmp/x.yaml", cfg)
	m = update(t, m, tea.WindowSizeMsg{Width: 40, Height: 20})
	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if m.mode != modeConfirm {
		t.Fatalf("want modeConfirm, got %d", m.mode)
	}
	if got := lipgloss.Width(m.View()); got > 40 {
		t.Errorf("confirm view width %d > 40; long name should be capped", got)
	}
}
