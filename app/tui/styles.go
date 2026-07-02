package tui

import "github.com/charmbracelet/lipgloss"

var (
	appStyle    = lipgloss.NewStyle().Padding(1, 2)
	titleStyle  = lipgloss.NewStyle().Bold(true)
	labelStyle  = lipgloss.NewStyle().Faint(true)
	helpStyle   = lipgloss.NewStyle().Faint(true)
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	borderStyle = lipgloss.NewStyle().Border(lipgloss.ThickBorder())
)

// boxContentWidth returns the width to pass to a bordered box's Width() so
// the full view (title + border + help, inside appStyle) fills the terminal.
// Chrome: appStyle padding (4) + thick border (2) = 6. Shared by every view
// that renders a full-width bordered box (the form, the checkout view).
func boxContentWidth(termWidth int) int {
	w := termWidth - 6
	if w < 20 {
		w = 20
	}
	return w
}
