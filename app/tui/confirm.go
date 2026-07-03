package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// confirmModal is the delete-confirmation window. termWidth caps the box so a
// long profile name can't push it past the edge of the screen (titledBox clips
// the question text to fit the box).
func confirmModal(name string, termWidth int) string {
	if termWidth <= 0 {
		termWidth = 80 // matches mainView's pre-resize fallback
	}
	question := "Delete profile \"" + name + "\"?"
	width := lipgloss.Width(question) + 6
	if max := termWidth - 8; width > max {
		width = max
	}
	if width < 30 {
		width = 30
	}
	body := question + "\n\n" + helpTextStyle.Render("y delete · n/esc cancel")
	return titledBox("Confirm delete", body, width, lipgloss.Height(body)+2, true)
}

// updateConfirm handles the delete-confirmation modal: only y/Y deletes;
// n/esc cancel; every other key (including enter) is ignored to avoid an
// accidental-delete footgun.
func (m model) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "y", "Y":
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
	case "n", "N", "esc":
		m.mode = modeMain
		return m, nil
	}
	return m, nil
}
