package tui

import (
	"path/filepath"
	"testing"

	"github.com/andresbott/netcheckout/internal/config"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func testConfig() *config.Config {
	return &config.Config{Profiles: map[string]config.Profile{
		"alpha": {LocalRoot: "/l/a", RemoteRoot: "/r/a"},
		"beta":  {LocalRoot: "/l/b", RemoteRoot: "/r/b"},
	}}
}

func update(t *testing.T, m model, msg tea.Msg) model {
	t.Helper()
	nm, _ := m.Update(msg)
	got, ok := nm.(model)
	if !ok {
		t.Fatalf("Update returned %T, want model", nm)
	}
	return got
}

func TestListNavigation(t *testing.T) {
	m := newModel("/tmp/x.yaml", testConfig())
	m = update(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if name, _ := m.list.selected(); name != "beta" {
		t.Fatalf("want beta selected after down, got %q", name)
	}
}

// TestEditOpensPrefilledForm covers the selected-row -> profile lookup path:
// "alpha" sorts first, so the initial cursor selects it, and pressing "e" must
// open the form pre-filled with that profile.
func TestEditOpensPrefilledForm(t *testing.T) {
	m := newModel("/tmp/x.yaml", testConfig())
	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if m.mode != modeForm {
		t.Fatalf("want modeForm after e, got %d", m.mode)
	}
	name, p := m.form.values()
	if name != "alpha" {
		t.Fatalf("want form name alpha, got %q", name)
	}
	if p.LocalRoot != "/l/a" || p.RemoteRoot != "/r/a" {
		t.Fatalf("want roots /l/a,/r/a, got %q,%q", p.LocalRoot, p.RemoteRoot)
	}
}

// TestEnterFocusesDetails: enter no longer swaps to a checkout screen; it moves
// focus to the Details pane for the selected profile, staying in the main view.
func TestEnterFocusesDetails(t *testing.T) {
	m := newModel("/tmp/x.yaml", testConfig())
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeMain {
		t.Fatalf("want modeMain after enter, got %d", m.mode)
	}
	if m.focus != paneDetails {
		t.Fatalf("want focus paneDetails after enter, got %d", m.focus)
	}
}

func TestTabTogglesFocus(t *testing.T) {
	m := newModel("/tmp/x.yaml", testConfig())
	if m.focus != paneList {
		t.Fatalf("initial focus should be paneList, got %d", m.focus)
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.focus != paneDetails {
		t.Fatalf("want paneDetails after tab, got %d", m.focus)
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.focus != paneList {
		t.Fatalf("want paneList after second tab, got %d", m.focus)
	}
}

func TestDetailsEscReturnsToList(t *testing.T) {
	m := newModel("/tmp/x.yaml", testConfig())
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // focus Details
	if m.focus != paneDetails {
		t.Fatalf("want paneDetails after enter, got %d", m.focus)
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEsc}) // esc from Details returns to the list, does not quit
	if m.focus != paneList {
		t.Fatalf("want paneList after esc from details, got %d", m.focus)
	}
	if m.mode != modeMain {
		t.Fatalf("esc from details should stay in main view, got mode %d", m.mode)
	}
}

func TestListQuit(t *testing.T) {
	m := newModel("/tmp/x.yaml", testConfig())
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected a command from ctrl+c")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("expected QuitMsg")
	}
}

func TestListEscQuits(t *testing.T) {
	m := newModel("/tmp/x.yaml", testConfig())
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected a command from esc in list mode")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("expected QuitMsg")
	}
}

func TestFormCtrlCQuits(t *testing.T) {
	m := newModel("/tmp/x.yaml", testConfig())
	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")}) // open add form
	if m.mode != modeForm {
		t.Fatalf("want modeForm, got %d", m.mode)
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected a command from ctrl+c in form mode")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("expected QuitMsg")
	}
}

func TestListViewNotEmpty(t *testing.T) {
	m := newModel(filepath.Join(t.TempDir(), "x.yaml"), testConfig())
	if m.View() == "" {
		t.Fatal("list view should not be empty")
	}
}

// TestViewFitsWindowWidth guards against the two-pane main view overflowing the
// terminal: after a resize, the rendered view (both panel borders included) must
// fit within the reported width, otherwise the right border is pushed off-screen.
func TestViewFitsWindowWidth(t *testing.T) {
	for _, w := range []int{80, 100, 120} {
		m := newModel("/tmp/x.yaml", testConfig())
		m = update(t, m, tea.WindowSizeMsg{Width: w, Height: 30})
		if got := lipgloss.Width(m.View()); got > w {
			t.Errorf("width=%d: view renders %d cols, overflow %d", w, got, got-w)
		}
	}
}

// TestModalFitsAndIsCentered: the form modal view fills the terminal (it is
// composited over a full-size canvas) but the modal box itself is narrower than
// the terminal, i.e. centered rather than full-width.
func TestModalFitsAndIsCentered(t *testing.T) {
	for _, w := range []int{60, 100, 120} {
		m := newModel("/tmp/x.yaml", testConfig())
		m = update(t, m, tea.WindowSizeMsg{Width: w, Height: 30})
		m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
		if m.mode != modeForm {
			t.Fatalf("width=%d: want modeForm, got %d", w, m.mode)
		}
		if got := lipgloss.Width(m.View()); got > w {
			t.Errorf("width=%d: view renders %d cols, overflow %d", w, got, got-w)
		}
		if got := lipgloss.Width(m.form.View()); got >= w {
			t.Errorf("width=%d: modal box is %d cols, expected narrower (centered)", w, got)
		}
	}
}
