package tui

import (
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

// confirmButton renders a bracketed [ label ] button: accent+bold when it is the
// focused button, dim otherwise. Mirrors formModel.actionButton.
func confirmButton(label string, focused bool) string {
	st := lipgloss.NewStyle().Foreground(colDim)
	if focused {
		st = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	}
	return st.Render("[ " + label + " ]")
}

// confirmModal is the delete-confirmation window. termWidth caps the box so a
// long profile name can't push it past the edge of the screen (titledBox clips
// the question text to fit the box). focus selects which button is highlighted.
func confirmModal(name string, focus confirmFocus, termWidth int) string {
	if termWidth <= 0 {
		termWidth = 80 // matches mainView's pre-resize fallback
	}
	question := "Delete profile \"" + name + "\"?"
	buttons := lipgloss.JoinHorizontal(lipgloss.Top,
		confirmButton("Delete", focus == confirmFocusDelete), "   ",
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
	return titledBox("Confirm delete", body, width, lipgloss.Height(body)+2, true)
}

// updateConfirm handles the delete-confirmation modal. Tab/Shift+Tab/←→/a/d
// toggle focus between the two buttons (default: Cancel); enter/space activates
// whichever is focused. y/Y always deletes and n/N/esc always cancel, regardless
// of focus — the original direct shortcuts stay live alongside the buttons.
func (m model) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "y", "Y":
		return m.deleteConfirmedProfile()
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
			return m.deleteConfirmedProfile()
		}
		m.mode = modeMain
		return m, nil
	}
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
	m.mode = modeMain
	m.err = nil
	return m, nil
}
