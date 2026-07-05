package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

func TestProfileViewShowsPanels(t *testing.T) {
	m := newModel("/tmp/x.yaml", testConfig())
	m = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m.profile = newProfileView("alpha")
	view := m.profileView()
	for _, want := range []string{"Details", "Actions", "File status", "alpha", "Checkout", "file status coming soon"} {
		if !strings.Contains(view, want) {
			t.Errorf("profile view missing %q:\n%s", want, view)
		}
	}
}

func TestProfileViewFitsWindowWidth(t *testing.T) {
	for _, w := range []int{80, 100, 120} {
		m := newModel("/tmp/x.yaml", testConfig())
		m = update(t, m, tea.WindowSizeMsg{Width: w, Height: 30})
		m.profile = newProfileView("alpha")
		if got := lipgloss.Width(m.profileView()); got > w {
			t.Errorf("width=%d: profile view renders %d cols, overflow %d", w, got, got-w)
		}
	}
}
