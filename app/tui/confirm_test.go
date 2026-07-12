package tui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/andresbott/netcheckout/internal/config"
	"github.com/andresbott/netcheckout/internal/sanity"
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
	view := confirmModal(confirmDelete, "alpha", confirmFocusCancel, false, 80)
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

// TestCheckinModalShowsCleanCheckbox: the check-in dialog renders the "delete
// local copy" checkbox, empty when clean is false and filled when true.
func TestCheckinModalShowsCleanCheckbox(t *testing.T) {
	unchecked := confirmModal(confirmCheckin, "work", confirmFocusCancel, false, 80)
	if !strings.Contains(unchecked, "delete local copy") || !strings.Contains(unchecked, "[ ]") {
		t.Errorf("check-in modal should show an unchecked 'delete local copy' box:\n%s", unchecked)
	}
	checked := confirmModal(confirmCheckin, "work", confirmFocusClean, true, 80)
	if !strings.Contains(checked, "[x]") {
		t.Errorf("clean=true should render a checked box:\n%s", checked)
	}
}

// TestDeleteModalHasNoCleanCheckbox: the checkbox is check-in only; the delete
// dialog must never show it.
func TestDeleteModalHasNoCleanCheckbox(t *testing.T) {
	view := confirmModal(confirmDelete, "alpha", confirmFocusCancel, false, 80)
	if strings.Contains(view, "delete local copy") {
		t.Errorf("delete modal must not show the clean checkbox:\n%s", view)
	}
}

// TestCheckinFocusRingIncludesCheckbox: Tab cycles checkbox → Check in → Cancel
// → checkbox in the check-in dialog (the checkbox is absent from delete).
func TestCheckinFocusRingIncludesCheckbox(t *testing.T) {
	m := model{mode: modeConfirm, confirmKind: confirmCheckin, confirmFocus: confirmFocusCancel}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.confirmFocus != confirmFocusClean {
		t.Fatalf("tab from Cancel should reach the checkbox, got %d", m.confirmFocus)
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.confirmFocus != confirmFocusDelete {
		t.Fatalf("tab from checkbox should reach Check in, got %d", m.confirmFocus)
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.confirmFocus != confirmFocusCancel {
		t.Fatalf("tab from Check in should reach Cancel, got %d", m.confirmFocus)
	}
}

// TestCheckinCheckboxTogglesClean: space/enter on the focused checkbox flips the
// checkinClean state without activating the check-in.
func TestCheckinCheckboxTogglesClean(t *testing.T) {
	m := model{mode: modeConfirm, confirmKind: confirmCheckin, confirmFocus: confirmFocusClean}
	m = update(t, m, tea.KeyMsg{Type: tea.KeySpace})
	if !m.checkinClean {
		t.Error("space on the focused checkbox should toggle checkinClean on")
	}
	if m.mode != modeConfirm {
		t.Error("toggling the checkbox must not close the dialog")
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.checkinClean {
		t.Error("enter on the focused checkbox should toggle it back off")
	}
}

// TestCheckinUpDownMovesBetweenRows: up/down move focus between the checkbox row
// and the button row in the check-in dialog.
func TestCheckinUpDownMovesBetweenRows(t *testing.T) {
	m := model{mode: modeConfirm, confirmKind: confirmCheckin, confirmFocus: confirmFocusClean}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.confirmFocus != confirmFocusDelete {
		t.Fatalf("down from the checkbox should move to the buttons, got %d", m.confirmFocus)
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyUp})
	if m.confirmFocus != confirmFocusClean {
		t.Fatalf("up from a button should move to the checkbox, got %d", m.confirmFocus)
	}
	// From Cancel too, up returns to the checkbox.
	m.confirmFocus = confirmFocusCancel
	m = update(t, m, tea.KeyMsg{Type: tea.KeyUp})
	if m.confirmFocus != confirmFocusClean {
		t.Fatalf("up from Cancel should move to the checkbox, got %d", m.confirmFocus)
	}
}

// TestDeleteUpDownNoop: the delete dialog has no checkbox, so up/down do nothing.
func TestDeleteUpDownNoop(t *testing.T) {
	m := model{mode: modeConfirm, confirmKind: confirmDelete, confirmFocus: confirmFocusCancel}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.confirmFocus != confirmFocusCancel {
		t.Errorf("up/down should be a no-op in the delete dialog, got %d", m.confirmFocus)
	}
}

// TestCheckoutOpensConfirmModal: selecting Checkout no longer runs immediately —
// it opens the confirm modal, focused on Cancel (the safe default), without
// starting the action.
func TestCheckoutOpensConfirmModal(t *testing.T) {
	m := model{
		sub:    subActions,
		cfg:    &config.Config{Profiles: map[string]config.Profile{"work": {}}},
		checks: map[string]*sanity.Result{"work": {CheckedOut: false}},
	}
	m.profile = newProfileView("work")
	m.profile.cursor = actionIndex(visibleActions(m.checks["work"]), "Checkout")
	m2, _ := m.updateProfile(keyMsg("enter"))
	got := m2.(model)
	if got.mode != modeConfirm {
		t.Fatal("Checkout Enter should open the confirm modal")
	}
	if got.confirmKind != confirmCheckout {
		t.Errorf("confirmKind = %v, want confirmCheckout", got.confirmKind)
	}
	if got.confirmFocus != confirmFocusCancel {
		t.Errorf("checkout dialog should open focused on Cancel, got %d", got.confirmFocus)
	}
	if got.profile.acting {
		t.Error("opening the confirm dialog must not start the checkout")
	}
}

// TestCheckoutConfirmedStartsAction: activating the checkout dialog (y) closes it
// and starts the action (acting set), returning to the actions view.
func TestCheckoutConfirmedStartsAction(t *testing.T) {
	m := model{
		mode:        modeConfirm,
		confirmKind: confirmCheckout,
		confirmName: "work",
		cfg:         &config.Config{Profiles: map[string]config.Profile{"work": {}}},
	}
	m.profile = newProfileView("work")
	m2, cmd := m.activateConfirm()
	got := m2.(model)
	if got.mode != modeMain || got.sub != subActions {
		t.Fatalf("after confirming checkout want modeMain/subActions, got mode=%d sub=%d", got.mode, got.sub)
	}
	if !got.profile.acting {
		t.Error("confirming checkout should mark the profile as acting")
	}
	if cmd == nil {
		t.Error("confirming checkout should dispatch the checkout command")
	}
}

// TestCheckoutModalWording: the checkout dialog carries its own title/question/
// activate label and, like delete, shows no clean checkbox.
func TestCheckoutModalWording(t *testing.T) {
	view := confirmModal(confirmCheckout, "work", confirmFocusCancel, false, 80)
	for _, want := range []string{"Confirm checkout", "Check out \"work\"?", "[ Check out ]", "[ Cancel ]"} {
		if !strings.Contains(view, want) {
			t.Fatalf("checkout modal missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "delete local copy") {
		t.Errorf("checkout modal must not show the clean checkbox:\n%s", view)
	}
}
