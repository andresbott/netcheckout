package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestActionCursorMoves(t *testing.T) {
	p := newProfileView("alpha")
	if p.cursor != 0 {
		t.Fatalf("initial cursor = %d, want 0", p.cursor)
	}
	p.moveUp() // clamps at the top
	if p.cursor != 0 {
		t.Fatalf("cursor after up at top = %d, want 0", p.cursor)
	}
	for i := 0; i < len(profileActions)+2; i++ {
		p.moveDown() // clamps at the bottom
	}
	if p.cursor != len(profileActions)-1 {
		t.Fatalf("cursor after many downs = %d, want %d", p.cursor, len(profileActions)-1)
	}
	p.moveUp()
	if p.cursor != len(profileActions)-2 {
		t.Fatalf("cursor after up = %d, want %d", p.cursor, len(profileActions)-2)
	}
}

// TestActionsViewShowsPanels: once Actions is revealed, the unified view shows
// both boxes' content plus the Activity placeholder.
func TestActionsViewShowsPanels(t *testing.T) {
	m := newModel("/tmp/x.yaml", testConfig())
	m = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	view := m.View()
	for _, want := range []string{"Details", "Actions", "Activity", "alpha", "Checkout", "sync activity coming soon"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
}
