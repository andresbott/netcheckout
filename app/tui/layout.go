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

// hint renders one "key: label" entry — the key accent-coloured, the ": label"
// dim — shared by the footer bars and the add/edit form's help line.
func hint(k, label string) string {
	return helpKeyStyle.Render(k) + helpTextStyle.Render(": "+label)
}

// renderFooter is the main view's bottom key-hint bar.
func renderFooter(width int) string {
	parts := []string{
		hint("a", "Add"), hint("e", "Edit"), hint("d", "Delete"),
		hint("↵", "Open"), hint("q", "Quit"),
	}
	return ansi.Truncate(" "+strings.Join(parts, "  "), width, "")
}

// renderProfileFooter is the profile view's bottom key-hint bar.
func renderProfileFooter(width int) string {
	parts := []string{
		hint("↵", "Run"), hint("↑↓", "Select"), hint("esc", "Back"), hint("q", "Quit"),
	}
	return ansi.Truncate(" "+strings.Join(parts, "  "), width, "")
}

// renderDetails is the profile summary body: name, the two roots, and — when the
// profile is scoped — the subpaths under a "Subpaths (N)" header.
func renderDetails(name string, p config.Profile, width int) string {
	if name == "" {
		return helpTextStyle.Render("No profile selected.")
	}
	label := func(s string) string { return labelStyle.Render(fmt.Sprintf("%-8s", s)) }
	var b strings.Builder
	b.WriteString(label("Name") + name + "\n")
	b.WriteString(label("Local") + p.LocalRoot + "\n")
	b.WriteString(label("Remote") + p.RemoteRoot)
	if len(p.Subpaths) > 0 {
		b.WriteString("\n" + labelStyle.Render(fmt.Sprintf("Subpaths (%d)", len(p.Subpaths))))
		for _, sub := range p.Subpaths {
			b.WriteString("\n  " + sub)
		}
	}
	return b.String()
}
