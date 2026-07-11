package tui

import (
	"github.com/andresbott/netcheckout/internal/lifecycle"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// confirmFocus is which button in the delete-confirmation dialog currently has
// focus. confirmFocusCancel is the zero value, so a freshly opened dialog is
// always safe by default: reaching the destructive action requires an explicit
// move, guarding against an accidental one-keystroke delete.
type confirmFocus int

const (
	confirmFocusCancel confirmFocus = iota
	confirmFocusDelete
)

// confirmKind selects which action the confirm modal is guarding: deleting a
// profile or checking one in. It changes the title, question, and
// activate-button label but shares all the layout/sizing and focus logic.
type confirmKind int

const (
	confirmDelete confirmKind = iota
	confirmCheckin
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
func confirmModal(kind confirmKind, name string, focus confirmFocus, termWidth int) string {
	if termWidth <= 0 {
		termWidth = 80 // matches mainView's pre-resize fallback
	}
	title, question, activate := "Confirm delete", "Delete profile \""+name+"\"?", "Delete"
	if kind == confirmCheckin {
		title, question, activate = "Confirm check-in", "Check in and release \""+name+"\"?", "Check in"
	}
	buttons := lipgloss.JoinHorizontal(lipgloss.Top,
		confirmButton(activate, focus == confirmFocusDelete), "   ",
		confirmButton("Cancel", focus == confirmFocusCancel))
	sep := helpTextStyle.Render(" · ")
	help := hint("tab", "Move") + sep + hint("enter/space", "Activate") + sep + hint("esc", "Cancel")

	width := lipgloss.Width(question) + 6
	// The help line's length is fixed (it doesn't depend on name), so the box
	// must be at least wide enough to show it without titledBox truncating it
	// mid-word — otherwise a short profile name would produce a box too narrow
	// for its own help line.
	if helpW := lipgloss.Width(help) + 2; helpW > width {
		width = helpW
	}
	if max := termWidth - 8; width > max {
		width = max
	}
	if width < 30 {
		width = 30
	}
	buttonRow := lipgloss.NewStyle().Width(width - 2).Align(lipgloss.Center).Render(buttons)
	body := question + "\n\n" + buttonRow + "\n\n" + help
	return titledBox(title, body, width, lipgloss.Height(body)+2, true)
}

// updateConfirm handles the shared confirm modal (delete or check-in, per
// m.confirmKind). Tab/Shift+Tab/←→/a/d toggle focus between the two buttons
// (default: Cancel); enter/space activates whichever is focused. y/Y always
// activates and n/N/esc always cancel, regardless of focus — the original
// direct shortcuts stay live alongside the buttons.
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
	case "tab", "shift+tab", "left", "right", "a", "d":
		if m.confirmFocus == confirmFocusCancel {
			m.confirmFocus = confirmFocusDelete
		} else {
			m.confirmFocus = confirmFocusCancel
		}
		return m, nil
	case "enter", " ":
		if m.confirmFocus == confirmFocusDelete {
			return m.activateConfirm()
		}
		m.mode = modeMain
		return m, nil
	}
	return m, nil
}

// activateConfirm runs whichever mutating action the open confirm modal
// guards, branching on m.confirmKind.
func (m model) activateConfirm() (tea.Model, tea.Cmd) {
	if m.confirmKind == confirmCheckin {
		return m.checkinConfirmed()
	}
	return m.deleteConfirmedProfile()
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
	opts := lifecycle.Options{Force: m.actForce, Clean: m.actClean}
	return m, checkinCmd(m.runner, m.id, name, m.cfg.Profiles[name], opts)
}
