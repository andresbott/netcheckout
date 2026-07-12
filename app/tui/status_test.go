package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andresbott/netcheckout/internal/baseline"
	"github.com/andresbott/netcheckout/internal/config"
	"github.com/andresbott/netcheckout/internal/lifecycle"
	"github.com/andresbott/netcheckout/internal/sanity"
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

// runStatusResult runs cmd — the Status action returns a tea.Batch of the status
// compute and the local scan — and returns the statusResultMsg it produces,
// unwrapping the batch's sub-commands.
func runStatusResult(t *testing.T, cmd tea.Cmd) statusResultMsg {
	t.Helper()
	if cmd == nil {
		t.Fatal("want a command from Enter on Status")
	}
	if res, ok := cmd().(statusResultMsg); ok {
		return res
	}
	if batch, ok := cmd().(tea.BatchMsg); ok {
		for _, c := range batch {
			if res, ok := c().(statusResultMsg); ok {
				return res
			}
		}
	}
	t.Fatal("command should produce a statusResultMsg")
	return statusResultMsg{}
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
	m.profile.result = &status.ProfileStatus{CheckedOut: true, HasBaseline: true, Targets: []status.TargetStatus{{
		Push: []status.Change{{Path: "notes.txt", Modify: true}},
	}}}
	view := m.View()
	for _, want := range []string{"(root)", "modify", "→ remote", "notes.txt"} {
		if !strings.Contains(view, want) {
			t.Errorf("Activity result missing %q:\n%s", want, view)
		}
	}
}

// TestActivityStatusNoProfileName: the Status body does not repeat the profile
// name (the Details box already shows it).
func TestActivityStatusNoProfileName(t *testing.T) {
	body := statusBody(status.ProfileStatus{CheckedOut: true, HasBaseline: true, Targets: []status.TargetStatus{{
		Push: []status.Change{{Path: "notes.txt", Modify: true}},
	}}}, 40)
	if strings.Contains(body, "alpha") {
		t.Errorf("status body should not contain the profile name:\n%s", body)
	}
}

// TestActivityStatusVerbAndSide: each change reads as a verb and the side it
// lands on — pushes go to remote, pulls to local, deletes name their side, and
// the verb reflects add/modify/delete.
func TestActivityStatusVerbAndSide(t *testing.T) {
	body := statusBody(status.ProfileStatus{CheckedOut: true, HasBaseline: true, Targets: []status.TargetStatus{{
		Push:          []status.Change{{Path: "new.png", Modify: false}},
		Pull:          []status.Change{{Path: "cover.jpg", Modify: true}},
		RemoteDeletes: []string{"gone.png"},
	}}}, 40)
	for _, want := range []string{
		"add", "→ remote", "new.png",
		"delete", "gone.png",
		"modify", "→ local", "cover.jpg",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("status body missing %q:\n%s", want, body)
		}
	}
}

// TestActivityStatusLocalDeleteAndConflict: a remote-side deletion renders as a
// local delete, and a two-sided change renders as a conflict on "both".
func TestActivityStatusLocalDeleteAndConflict(t *testing.T) {
	body := statusBody(status.ProfileStatus{CheckedOut: true, HasBaseline: true, Targets: []status.TargetStatus{{
		LocalDeletes: []string{"old.txt"},
		Conflicts:    []string{"clash.txt"},
	}}}, 40)
	for _, want := range []string{
		"delete", "→ local", "old.txt",
		"conflict", "both", "clash.txt",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("status body missing %q:\n%s", want, body)
		}
	}
}

// TestActivityStatusInSyncPerTarget: an all-in-sync profile lists each target
// with its own "no changes" line rather than a single global summary.
func TestActivityStatusInSyncPerTarget(t *testing.T) {
	body := statusBody(status.ProfileStatus{CheckedOut: true, HasBaseline: true, Targets: []status.TargetStatus{
		{Subpath: "albums"},
		{Subpath: "stems"},
	}}, 40)
	if c := strings.Count(body, "no changes"); c != 2 {
		t.Errorf("want a 'no changes' line per in-sync target, got %d:\n%s", c, body)
	}
	for _, want := range []string{"albums", "stems", "───"} {
		if !strings.Contains(body, want) {
			t.Errorf("in-sync body missing %q:\n%s", want, body)
		}
	}
}

// TestActivityStatusDivider: multiple targets are separated by a divider line,
// but no divider trails the last target.
func TestActivityStatusDivider(t *testing.T) {
	body := statusBody(status.ProfileStatus{CheckedOut: true, HasBaseline: true, Targets: []status.TargetStatus{
		{Subpath: "a", Push: []status.Change{{Path: "x"}}},
		{Subpath: "b", Push: []status.Change{{Path: "y"}}},
	}}, 40)
	if n := strings.Count(body, "───"); n < 1 {
		t.Errorf("two targets should be divided by a rule:\n%s", body)
	}
	if strings.HasSuffix(strings.TrimRight(body, " "), "───") {
		t.Errorf("no divider should trail the last target:\n%s", body)
	}
}

// TestActivityStatusNoBaseline: checked out without a local baseline is called
// out rather than shown as a change list.
func TestActivityStatusNoBaseline(t *testing.T) {
	body := statusBody(status.ProfileStatus{CheckedOut: true, HasBaseline: false}, 40)
	if !strings.Contains(body, "no local baseline") {
		t.Errorf("want a no-baseline note, got %q", body)
	}
}

// TestStatusClearsStaleActionReport: after a mutating action leaves a report on
// the profile ("sync: 0 pulled"), running Status drops that report so the fresh
// status result is shown instead of the stale action outcome.
func TestStatusClearsStaleActionReport(t *testing.T) {
	cfg := mountedConfig(t)
	saveEmptyBaseline(t, cfg) // empty trees + empty baseline: in sync
	m := openActions(t, cfg)
	m.checks["alpha"] = &sanity.Result{CheckedOut: true}
	m.profile.actionReport = &lifecycle.Report{Action: "sync"} // as if a sync just finished
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})        // Enter on Status
	m = nm.(model)
	if m.profile.actionReport != nil {
		t.Fatal("running Status should clear the stale action report")
	}
	m = update(t, m, runStatusResult(t, cmd))
	view := m.View()
	if strings.Contains(view, "pulled") {
		t.Errorf("Activity should show the status result, not the stale action report:\n%s", view)
	}
	if !strings.Contains(view, "no changes") {
		t.Errorf("in-sync Status should show 'no changes':\n%s", view)
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
		m.profile.result = &status.ProfileStatus{CheckedOut: true, HasBaseline: true, Targets: []status.TargetStatus{{
			Push: []status.Change{{Path: long, Modify: false}},
		}}}
		if got := lipgloss.Width(m.View()); got > w {
			t.Errorf("width=%d: view renders %d cols, overflow %d", w, got, got-w)
		}
	}
}

// manyChangesResult builds a checked-out status with n push changes, enough to
// overflow the Activity pane so scrolling applies.
func manyChangesResult(n int) *status.ProfileStatus {
	changes := make([]status.Change, n)
	for i := range changes {
		changes[i] = status.Change{Path: fmt.Sprintf("file%02d.txt", i), Modify: false}
	}
	return &status.ProfileStatus{CheckedOut: true, HasBaseline: true, Targets: []status.TargetStatus{{Push: changes}}}
}

// TestActivityStatusScrolls: a change list taller than the pane hides its tail
// until PgDn scrolls it into view (and shows the scroll hint), and PgUp returns
// to the top.
func TestActivityStatusScrolls(t *testing.T) {
	m := openActions(t, testConfig())
	m.profile.result = manyChangesResult(40)

	if v := m.View(); !strings.Contains(v, "file00.txt") || strings.Contains(v, "file39.txt") {
		t.Fatalf("initial view should show the head, not the tail:\n%s", v)
	}
	if !strings.Contains(m.View(), "Scroll") {
		t.Errorf("footer should offer the scroll hint when content overflows:\n%s", m.View())
	}

	m = update(t, m, tea.KeyMsg{Type: tea.KeyPgDown})
	m = update(t, m, tea.KeyMsg{Type: tea.KeyPgDown})
	if v := m.View(); !strings.Contains(v, "file39.txt") {
		t.Errorf("after paging down the tail should be visible:\n%s", v)
	}

	m = update(t, m, tea.KeyMsg{Type: tea.KeyPgUp})
	m = update(t, m, tea.KeyMsg{Type: tea.KeyPgUp})
	if m.profile.statusScroll != 0 {
		t.Errorf("paging up should return to the top, got scroll=%d", m.profile.statusScroll)
	}
	if v := m.View(); !strings.Contains(v, "file00.txt") {
		t.Errorf("back at the top the head should be visible:\n%s", v)
	}
}

// mountedConfig builds a single-profile "alpha" config whose roots are real,
// existing temp directories with a checkout marker on the remote, and points
// NETCHECKOUT_STATE at a temp state dir so status.Compute resolves its baseline
// there.
func mountedConfig(t *testing.T) *config.Config {
	t.Helper()
	local := filepath.Join(t.TempDir(), "local")
	remote := filepath.Join(t.TempDir(), "remote")
	for _, d := range []string{local, remote} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(remote, ".netcheckout.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NETCHECKOUT_STATE", t.TempDir())
	return &config.Config{Profiles: map[string]config.Profile{
		"alpha": {LocalRoot: local, RemoteRoot: remote},
	}}
}

// saveEmptyBaseline snapshots the alpha profile's current local tree and saves it
// as the profile's baseline.
func saveEmptyBaseline(t *testing.T, cfg *config.Config) {
	t.Helper()
	local := cfg.Profiles["alpha"].LocalRoot
	files, err := baseline.Snapshot(local, []string{"."})
	if err != nil {
		t.Fatal(err)
	}
	if err := baseline.Save(&baseline.Baseline{Profile: "alpha", Relpaths: []string{"."}, Files: files}); err != nil {
		t.Fatal(err)
	}
}

// TestStatusEnterRunsCompute: Enter on Status marks the profile checking and
// returns a command; running it yields a statusResultMsg whose result renders.
func TestStatusEnterRunsCompute(t *testing.T) {
	cfg := mountedConfig(t)
	saveEmptyBaseline(t, cfg)
	// A brand-new local file is a push the three-way plan surfaces.
	if err := os.WriteFile(filepath.Join(cfg.Profiles["alpha"].LocalRoot, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := openActions(t, cfg)
	m.checks["alpha"] = &sanity.Result{CheckedOut: true} // Status is offered only when checked out
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if !m.profile.checking {
		t.Fatal("want profile.checking after Enter on Status")
	}
	res := runStatusResult(t, cmd)
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

// TestStatusEnterScansLocalTree: Enter on Status also kicks off a local file
// scan (marking the profile scanning) and, once the result arrives, populates
// fileStats so the Details box shows the folder/file/size summary.
func TestStatusEnterScansLocalTree(t *testing.T) {
	cfg := mountedConfig(t)
	// Give the local root some content so the scan has something to count.
	local := cfg.Profiles["alpha"].LocalRoot
	if err := os.MkdirAll(filepath.Join(local, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(local, "sub", "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	saveEmptyBaseline(t, cfg) // baseline now matches local; status is in sync
	m := openActions(t, cfg)
	m.checks["alpha"] = &sanity.Result{CheckedOut: true}
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if !m.profile.scanning {
		t.Fatal("want profile.scanning after Enter on Status")
	}
	res := runLocalStatResult(t, cmd)
	if res.err != nil {
		t.Fatalf("unexpected scan error: %v", res.err)
	}
	m = update(t, m, res)
	if m.profile.scanning {
		t.Fatal("scanning should clear once the scan result arrives")
	}
	if m.profile.fileStats == nil || m.profile.fileStats.Files != 1 {
		t.Fatalf("want fileStats with 1 file, got %+v", m.profile.fileStats)
	}
	if !strings.Contains(m.View(), "Contents") || !strings.Contains(m.View(), "Files") {
		t.Errorf("Details should show the contents summary:\n%s", m.View())
	}
}

// runLocalStatResult runs the Status batch command and returns the
// localStatResultMsg it produces.
func runLocalStatResult(t *testing.T, cmd tea.Cmd) localStatResultMsg {
	t.Helper()
	if cmd == nil {
		t.Fatal("want a command from Enter on Status")
	}
	if batch, ok := cmd().(tea.BatchMsg); ok {
		for _, c := range batch {
			if res, ok := c().(localStatResultMsg); ok {
				return res
			}
		}
	}
	t.Fatal("command should produce a localStatResultMsg")
	return localStatResultMsg{}
}

// TestStatusEnterSurfacesError: when the remote root is not mounted, Compute
// errors and the Activity box shows that error.
func TestStatusEnterSurfacesError(t *testing.T) {
	m := openActions(t, testConfig())                    // testConfig roots do not exist on disk
	m.checks["alpha"] = &sanity.Result{CheckedOut: true} // Status is offered only when checked out
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	res := runStatusResult(t, cmd)
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

func TestActivityShowsNotCheckedOut(t *testing.T) {
	m := openActions(t, testConfig())
	m.profile.result = &status.ProfileStatus{CheckedOut: false}
	if !strings.Contains(m.View(), "not checked out") {
		t.Errorf("Activity should show 'not checked out':\n%s", m.View())
	}
}
