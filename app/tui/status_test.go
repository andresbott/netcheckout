package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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

// fakeDiffer returns canned diffs per direction, mirroring the CLI status test.
type fakeDiffer struct {
	diffs map[rsync.Direction]rsync.Diff
	err   error
}

func (f fakeDiffer) Diff(_ context.Context, j rsync.Job) (rsync.Diff, error) {
	if f.err != nil {
		return rsync.Diff{}, f.err
	}
	return f.diffs[j.Direction], nil
}

// mountedConfig builds a single-profile config whose roots are real, existing
// temp directories, so status.Compute's mount/existence checks pass.
func mountedConfig(t *testing.T) *config.Config {
	t.Helper()
	local := filepath.Join(t.TempDir(), "local")
	remote := filepath.Join(t.TempDir(), "remote")
	for _, d := range []string{local, remote} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return &config.Config{Profiles: map[string]config.Profile{
		"alpha": {LocalRoot: local, RemoteRoot: remote},
	}}
}

// TestStatusEnterRunsCompute: Enter on Status marks the profile checking and
// returns a command; running it yields a statusResultMsg whose result renders.
func TestStatusEnterRunsCompute(t *testing.T) {
	m := openActions(t, mountedConfig(t))
	m.differ = fakeDiffer{diffs: map[rsync.Direction]rsync.Diff{
		rsync.Push: {Changes: []rsync.Change{{Path: "notes.txt", Type: rsync.Modified}}},
		rsync.Pull: {InSync: true},
	}}
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if !m.profile.checking {
		t.Fatal("want profile.checking after Enter on Status")
	}
	if cmd == nil {
		t.Fatal("want a command from Enter on Status")
	}
	res, ok := cmd().(statusResultMsg)
	if !ok {
		t.Fatal("command should produce a statusResultMsg")
	}
	if res.err != nil {
		t.Fatalf("unexpected compute error: %v", res.err)
	}
	m = update(t, m, res)
	if m.profile.checking {
		t.Fatal("checking should clear once the result arrives")
	}
	if !strings.Contains(m.View(), "notes.txt") {
		t.Errorf("Activity should show the computed change:\n%s", m.View())
	}
}

// TestStatusEnterSurfacesError: when the remote root is not mounted, Compute
// errors and the Activity box shows that error.
func TestStatusEnterSurfacesError(t *testing.T) {
	m := openActions(t, testConfig()) // testConfig roots do not exist on disk
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if cmd == nil {
		t.Fatal("want a command from Enter on Status")
	}
	res, ok := cmd().(statusResultMsg)
	if !ok {
		t.Fatal("command should produce a statusResultMsg")
	}
	if res.err == nil {
		t.Fatal("want an error for an unmounted remote root")
	}
	m = update(t, m, res)
	if m.profile.err == nil {
		t.Fatal("want profile.err set after an errored compute")
	}
	if !strings.Contains(m.View(), "not mounted") {
		t.Errorf("Activity should show the mount error:\n%s", m.View())
	}
}

// TestNonStatusActionIsInert: Enter on a non-Status action returns no command
// and does not start a compute.
func TestNonStatusActionIsInert(t *testing.T) {
	m := openActions(t, testConfig())
	m = update(t, m, tea.KeyMsg{Type: tea.KeyDown}) // move off Status to "Checkout"
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if cmd != nil {
		t.Fatal("non-Status action should not return a command")
	}
	if m.profile.checking {
		t.Fatal("non-Status action should not start a compute")
	}
}

// TestStaleStatusResultIgnored: a result for a profile the user has left is
// dropped rather than applied to the current profile.
func TestStaleStatusResultIgnored(t *testing.T) {
	m := openActions(t, testConfig())
	stale := statusResultMsg{name: "some-other-profile", err: errors.New("boom")}
	m = update(t, m, stale)
	if m.profile.err != nil {
		t.Fatal("a result for another profile must be ignored")
	}
}
