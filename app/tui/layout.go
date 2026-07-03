package tui

import (
	"fmt"
	"strings"

	"github.com/andresbott/netcheckout/internal/config"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// renderHeader is the top bar: "netcheckout <version>" left, identity right.
func renderHeader(width int, version, identity string) string {
	left := headerAppStyle.Render("netcheckout") + " " + headerIDStyle.Render(version)
	right := headerIDStyle.Render(identity)
	gap := width - 1 - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	line := " " + left + strings.Repeat(" ", gap) + right
	return ansi.Truncate(line, width, "")
}

// renderFooter is the bottom key-hint bar.
func renderFooter(width int) string {
	hint := func(k, label string) string {
		return helpKeyStyle.Render(k) + " " + helpTextStyle.Render(label)
	}
	parts := []string{
		hint("a", "Add"), hint("e", "Edit"), hint("d", "Delete"),
		hint("↵", "Open"), hint("tab", "Switch"), hint("q", "Quit"),
	}
	return ansi.Truncate(" "+strings.Join(parts, "  "), width, "")
}

// renderDetails is the right-panel body for the selected profile.
func renderDetails(name string, p config.Profile, width int) string {
	if name == "" {
		return helpTextStyle.Render("No profile selected.")
	}
	label := func(s string) string { return labelStyle.Render(fmt.Sprintf("%-8s", s)) }
	var b strings.Builder
	b.WriteString(label("Name") + name + "\n")
	b.WriteString(label("Local") + p.LocalRoot + "\n")
	b.WriteString(label("Remote") + p.RemoteRoot + "\n\n")
	b.WriteString(sectionStyle.Render("── Checkout") + "\n")
	b.WriteString(helpTextStyle.Render("sync / check-in coming soon"))
	return b.String()
}
