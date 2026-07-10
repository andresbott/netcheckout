package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/andresbott/netcheckout/internal/config"
	"github.com/andresbott/netcheckout/internal/rsync"
	"github.com/andresbott/netcheckout/internal/status"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// openActions builds a model at a usable size and reveals the first profile's
// action menu (cursor on Status).
func openActions(t *testing.T, cfg *config.Config) model {
	t.Helper()
	m := newModel("/tmp/x.yaml", cfg)
	m = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	return m
}

func TestActivityIdlePlaceholder(t *testing.T) {
	m := openActions(t, testConfig())
	if !strings.Contains(m.View(), "sync activity coming soon") {
		t.Errorf("idle Activity should keep the placeholder:\n%s", m.View())
	}
}

func TestActivityShowsChecking(t *testing.T) {
	m := openActions(t, testConfig())
	m.profile.checking = true
	if !strings.Contains(m.View(), "Checking…") {
		t.Errorf("Activity should show Checking… while a compute is in flight:\n%s", m.View())
	}
}

func TestActivityShowsError(t *testing.T) {
	m := openActions(t, testConfig())
	m.profile.err = errors.New("remote root /r/a is not mounted")
	if !strings.Contains(m.View(), "not mounted") {
		t.Errorf("Activity should show the error:\n%s", m.View())
	}
}

func TestActivityShowsStatusResult(t *testing.T) {
	m := openActions(t, testConfig())
	m.profile.result = &status.ProfileStatus{Targets: []status.TargetStatus{{
		Push: rsync.Diff{Changes: []rsync.Change{{Path: "notes.txt", Type: rsync.Modified}}},
		Pull: rsync.Diff{InSync: true},
	}}}
	view := m.View()
	for _, want := range []string{"(root)", "notes.txt", "in sync"} {
		if !strings.Contains(view, want) {
			t.Errorf("Activity result missing %q:\n%s", want, view)
		}
	}
}

// TestActivityStatusFitsWidth: a very long change path is clipped, so the view
// never overflows the terminal width.
func TestActivityStatusFitsWidth(t *testing.T) {
	long := strings.Repeat("very-long-path-segment/", 12) + "file.txt"
	for _, w := range []int{80, 100, 120} {
		m := newModel("/tmp/x.yaml", testConfig())
		m = update(t, m, tea.WindowSizeMsg{Width: w, Height: 30})
		m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
		m.profile.result = &status.ProfileStatus{Targets: []status.TargetStatus{{
			Push: rsync.Diff{Changes: []rsync.Change{{Path: long, Type: rsync.Created}}},
			Pull: rsync.Diff{InSync: true},
		}}}
		if got := lipgloss.Width(m.View()); got > w {
			t.Errorf("width=%d: view renders %d cols, overflow %d", w, got, got-w)
		}
	}
}
