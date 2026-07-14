package tui

import (
	"context"

	"github.com/andresbott/netcheckout/internal/lifecycle"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// confirmFocus is which control in the confirm dialog currently has focus.
// confirmFocusCancel is the zero value, so a freshly opened dialog is always
// safe by default: reaching the destructive action requires an explicit move,
// guarding against an accidental one-keystroke delete/check-in.
// confirmFocusClean is the "delete local copy" checkbox, present only in the
// check-in dialog.
type confirmFocus int

const (
	confirmFocusCancel confirmFocus = iota
	confirmFocusDelete
	confirmFocusClean
)

// confirmFocusRing is the Tab order for a dialog kind. The check-in dialog adds
// the "delete local copy" checkbox ahead of the buttons; delete has buttons only.
func confirmFocusRing(kind confirmKind) []confirmFocus {
	if kind == confirmCheckin {
		return []confirmFocus{confirmFocusClean, confirmFocusDelete, confirmFocusCancel}
	}
	return []confirmFocus{confirmFocusDelete, confirmFocusCancel}
}

// confirmFocusStep moves focus dir steps (+1/-1) around the kind's ring, wrapping.
func confirmFocusStep(kind confirmKind, cur confirmFocus, dir int) confirmFocus {
	ring := confirmFocusRing(kind)
	idx := 0
	for i, f := range ring {
		if f == cur {
			idx = i
			break
		}
	}
	n := len(ring)
	return ring[((idx+dir)%n+n)%n]
}

// confirmCheckbox renders the check-in dialog's "delete local copy" checkbox:
// a filled [x] / empty [ ] box plus its label, accent+bold when focused, dim
// otherwise. Mirrors confirmButton's focus styling.
func confirmCheckbox(checked, focused bool) string {
	box := "[ ]"
	if checked {
		box = "[x]"
	}
	st := lipgloss.NewStyle().Foreground(colDim)
	if focused {
		st = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	}
	return st.Render(box + " delete local copy")
}

// confirmKind selects which action the confirm modal is guarding: deleting a
// profile or checking one in. It changes the title, question, and
// activate-button label but shares all the layout/sizing and focus logic.
type confirmKind int

const (
	confirmDelete confirmKind = iota
	confirmCheckin
	confirmCheckout
	// confirmCancel guards stopping an already-running action (Sync/Checkout/
	// Check-in/Status) rather than starting one, so its "activate" button stops the
	// operation and its dismiss keeps it running.
	confirmCancel
)

// confirmButton renders a bracketed [ label ] button: accent+bold when it is the
// focused button, dim otherwise. Mirrors formModel.actionButton.
func confirmButton(label string, focused bool) string {
	st := lipgloss.NewStyle().Foreground(colDim)
	if focused {
		st = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	}
	return st.Render("[ " + label + " ]")
}

// confirmModal is the confirmation window shared by the delete and check-in
// flows; kind selects the title/question/activate-button wording. termWidth
// caps the box so a long profile name can't push it past the edge of the
// screen (titledBox clips the question text to fit the box). focus selects
// which button is highlighted.
func confirmModal(kind confirmKind, name string, focus confirmFocus, clean bool, termWidth int) string {
	if termWidth <= 0 {
		termWidth = 80 // matches mainView's pre-resize fallback
	}
	title, question, activate, dismiss := "Confirm delete", "Delete profile \""+name+"\"?", "Delete", "Cancel"
	switch kind {
	case confirmCheckin:
		title, question, activate = "Confirm check-in", "Check in and release \""+name+"\"?", "Check in"
	case confirmCheckout:
		title, question, activate = "Confirm checkout", "Check out \""+name+"\"?", "Check out"
	case confirmCancel:
		// This dialog guards stopping a run, not starting one, so "Cancel" would be
		// ambiguous — the dismiss button keeps the operation running instead.
		title, question, activate, dismiss = "Confirm cancel", "Stop the running operation?", "Stop", "Keep running"
	}
	buttons := lipgloss.JoinHorizontal(lipgloss.Top,
		confirmButton(activate, focus == confirmFocusDelete), "   ",
		confirmButton(dismiss, focus == confirmFocusCancel))
	sep := helpTextStyle.Render(" · ")
	escHint := "Cancel"
	if kind == confirmCancel {
		escHint = "Keep running"
	}
	help := hint("tab", "Move") + sep + hint("enter/space", "Activate") + sep + hint("esc", escHint)

	// The check-in dialog carries a "delete local copy" checkbox above the buttons.
	checkbox := ""
	if kind == confirmCheckin {
		checkbox = confirmCheckbox(clean, focus == confirmFocusClean)
	}

	width := lipgloss.Width(question) + 6
	// The help line's length is fixed (it doesn't depend on name), so the box
	// must be at least wide enough to show it without titledBox truncating it
	// mid-word — otherwise a short profile name would produce a box too narrow
	// for its own help line.
	if helpW := lipgloss.Width(help) + 2; helpW > width {
		width = helpW
	}
	if cbW := lipgloss.Width(checkbox) + 2; cbW > width {
		width = cbW
	}
	if max := termWidth - 8; width > max {
		width = max
	}
	if width < 30 {
		width = 30
	}
	buttonRow := lipgloss.NewStyle().Width(width - 2).Align(lipgloss.Center).Render(buttons)
	body := question
	if checkbox != "" {
		body += "\n\n" + checkbox
	}
	body += "\n\n" + buttonRow + "\n\n" + help
	return titledBox(title, body, width, lipgloss.Height(body)+2, true)
}

// updateConfirm handles the shared confirm modal (delete or check-in, per
// m.confirmKind). Tab/Shift+Tab cycle focus around the dialog's ring (default:
// Cancel); ←→ toggle between the two buttons. enter/space toggles the focused
// "delete local copy" checkbox (check-in only) or activates the focused button.
// y/Y always activates and n/N/esc always cancel, regardless of focus — the
// original direct shortcuts stay live alongside the controls.
func (m model) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "y", "Y":
		return m.activateConfirm()
	case "n", "N", "esc":
		m.mode = modeMain
		return m, nil
	case "tab":
		m.confirmFocus = confirmFocusStep(m.confirmKind, m.confirmFocus, 1)
		return m, nil
	case "shift+tab":
		m.confirmFocus = confirmFocusStep(m.confirmKind, m.confirmFocus, -1)
		return m, nil
	case "left", "right":
		// Toggle between the two buttons; ignored while on the checkbox.
		switch m.confirmFocus {
		case confirmFocusCancel:
			m.confirmFocus = confirmFocusDelete
		case confirmFocusDelete:
			m.confirmFocus = confirmFocusCancel
		}
		return m, nil
	case "up", "down":
		// Move between the checkbox row and the button row (check-in only, where
		// the checkbox exists). On a button, return to the checkbox; on the
		// checkbox, drop to the buttons.
		if m.confirmKind != confirmCheckin {
			return m, nil
		}
		if m.confirmFocus == confirmFocusClean {
			m.confirmFocus = confirmFocusDelete
		} else {
			m.confirmFocus = confirmFocusClean
		}
		return m, nil
	case "enter", " ":
		switch m.confirmFocus {
		case confirmFocusClean:
			m.checkinClean = !m.checkinClean
			return m, nil
		case confirmFocusDelete:
			return m.activateConfirm()
		default:
			m.mode = modeMain
			return m, nil
		}
	}
	return m, nil
}

// activateConfirm runs whichever mutating action the open confirm modal
// guards, branching on m.confirmKind.
func (m model) activateConfirm() (tea.Model, tea.Cmd) {
	switch m.confirmKind {
	case confirmCheckin:
		return m.checkinConfirmed()
	case confirmCheckout:
		return m.checkoutConfirmed()
	case confirmCancel:
		return m.cancelAction()
	}
	return m.deleteConfirmedProfile()
}

// cancelAction stops the in-flight action the confirm modal was guarding. For a
// streaming mutation it cancels the context — killing the live rsync via
// exec.CommandContext; for Status there is nothing to kill (m.cancel is nil), so
// it only abandons the result. Either way it bumps actionSeq so the run's
// straggler messages are dropped, marks the profile canceled for the "Canceled."
// note, and returns to the profile's actions view.
func (m model) cancelAction() (tea.Model, tea.Cmd) {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.actionSeq++
	m.profile.acting = false
	m.profile.checking = false
	m.profile.scanning = false
	m.profile.canceled = true
	m.mode = modeMain
	m.sub = subActions
	return m, nil
}

// deleteConfirmedProfile removes m.confirmName from the config and persists it,
// rolling back in memory if the save fails. Shared by the y/Y shortcut and the
// focused-Delete-button path.
func (m model) deleteConfirmedProfile() (tea.Model, tea.Cmd) {
	prev := cloneProfiles(m.cfg.Profiles)
	delete(m.cfg.Profiles, m.confirmName)
	if err := commitProfiles(m.path, m.cfg, prev); err != nil {
		m.err = err
		m.mode = modeMain
		return m, nil
	}
	m.refreshList()
	delete(m.checks, m.confirmName)
	m.mode = modeMain
	m.err = nil
	return m, nil
}

// checkinConfirmed runs lifecycle.Runner.Checkin for m.confirmName once the
// user has confirmed, returning to the profile's actions view with acting
// state reset so the Activity box shows a fresh in-progress state.
func (m model) checkinConfirmed() (tea.Model, tea.Cmd) {
	name := m.confirmName
	m.mode = modeMain
	m.sub = subActions
	m.profile.acting = true
	m.profile.actionErr = nil
	m.profile.actionReport = nil
	m.profile.applied = nil
	m.profile.canceled = false
	m.profile.statusScroll = 0
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.actionSeq++
	// checkin has no --force: it only releases a this-machine-owned, fully-synced
	// profile. The footer force toggle applies to checkout/sync, not here.
	opts := lifecycle.Options{Clean: m.checkinClean}
	return m, checkinCmd(ctx, m.runner, m.id, name, m.cfg.Profiles[name], m.actionSeq, opts)
}

// checkoutConfirmed runs lifecycle.Runner.Checkout for m.confirmName once the
// user has confirmed, returning to the profile's actions view with acting state
// reset so the Activity box shows a fresh in-progress state. Mirrors
// checkinConfirmed; there is no clean checkbox for checkout.
func (m model) checkoutConfirmed() (tea.Model, tea.Cmd) {
	name := m.confirmName
	m.mode = modeMain
	m.sub = subActions
	m.profile.acting = true
	m.profile.actionErr = nil
	m.profile.actionReport = nil
	m.profile.applied = nil
	m.profile.canceled = false
	m.profile.statusScroll = 0
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.actionSeq++
	opts := lifecycle.Options{Force: m.actForce}
	return m, checkoutCmd(ctx, m.runner, m.id, name, m.cfg.Profiles[name], m.actionSeq, opts)
}
