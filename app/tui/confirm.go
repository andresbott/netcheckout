package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// confirmModal is the delete-confirmation window.
func confirmModal(name string) string {
	question := "Delete profile \"" + name + "\"?"
	body := question + "\n\n" + helpTextStyle.Render("y delete · n/esc cancel")
	width := lipgloss.Width(question) + 6
	if width < 30 {
		width = 30
	}
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
