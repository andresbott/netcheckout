package tui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/andresbott/netcheckout/internal/config"
	"github.com/andresbott/netcheckout/internal/sanity"
	tea "github.com/charmbracelet/bubbletea"
)

func TestInitDispatchesChecks(t *testing.T) {
	m := newModel("/tmp/x.yaml", testConfig())
	if m.Init() == nil {
		t.Fatal("Init should return a command batch when profiles exist")
	}
}

func TestSanityCmdChecksFilesystem(t *testing.T) {
	msg := sanityCmd("p", config.Profile{LocalRoot: t.TempDir(), RemoteRoot: t.TempDir()})()
	res, ok := msg.(sanityResultMsg)
	if !ok {
		t.Fatalf("sanityCmd produced %T, want sanityResultMsg", msg)
	}
	if !res.result.LocalRoot || !res.result.RemoteRoot {
		t.Errorf("result = %+v, want both roots present", res.result)
	}
}

func TestSanityResultStored(t *testing.T) {
	m := newModel("/tmp/x.yaml", testConfig())
	m = update(t, m, sanityResultMsg{name: "alpha", result: sanity.Result{LocalRoot: true, RemoteRoot: true}})
	got, ok := m.checks["alpha"]
	if !ok || got == nil {
		t.Fatal("sanityResultMsg should store a result for alpha")
	}
	if !got.LocalRoot || !got.RemoteRoot {
		t.Errorf("stored result = %+v, want both roots true", *got)
	}
}

func TestDetailsShowsCheckingThenResult(t *testing.T) {
	m := newModel("/tmp/x.yaml", testConfig())
	m = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	if !strings.Contains(m.View(), "…") {
		t.Errorf("before results, Details should show '…':\n%s", m.View())
	}
	m = update(t, m, sanityResultMsg{name: "alpha", result: sanity.Result{LocalRoot: true, RemoteRoot: true}})
	if !strings.Contains(m.View(), "✓") {
		t.Errorf("after result, Details should show a ✓ mark:\n%s", m.View())
	}
}

func TestDeleteDropsCheck(t *testing.T) {
	m := newModel(filepath.Join(t.TempDir(), "c.yaml"), testConfig())
	m.checks["alpha"] = &sanity.Result{LocalRoot: true}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")}) // delete-confirm on alpha
	m = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")}) // confirm
	if _, ok := m.checks["alpha"]; ok {
		t.Error("deleting a profile should drop its sanity cache entry")
	}
}
