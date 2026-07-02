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

func TestNewModelBuildsRows(t *testing.T) {
	m := newModel("/tmp/x.yaml", testConfig())
	if len(m.table.Rows()) != 2 {
		t.Fatalf("want 2 rows, got %d", len(m.table.Rows()))
	}
}

func TestTableNavigation(t *testing.T) {
	m := newModel("/tmp/x.yaml", testConfig())
	m = update(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.table.Cursor() != 1 {
		t.Fatalf("want cursor 1 after down, got %d", m.table.Cursor())
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

// TestViewFitsWindowWidth guards against the table overflowing the terminal:
// after a resize, the rendered view (thick border included) must fit within
// the reported width, otherwise the right border is pushed off-screen.
func TestViewFitsWindowWidth(t *testing.T) {
	for _, w := range []int{80, 100, 120} {
		m := newModel("/tmp/x.yaml", testConfig())
		m = update(t, m, tea.WindowSizeMsg{Width: w, Height: 30})
		if got := lipgloss.Width(m.View()); got > w {
			t.Errorf("width=%d: view renders %d cols, overflow %d", w, got, got-w)
		}
	}
}
