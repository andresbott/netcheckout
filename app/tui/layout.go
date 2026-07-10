package tui

import (
	"fmt"
	"strings"

	"github.com/andresbott/netcheckout/internal/config"
	"github.com/andresbott/netcheckout/internal/sanity"
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
		hint("↵", "Actions"), hint("q", "Quit"),
	}
	return ansi.Truncate(" "+strings.Join(parts, "  "), width, "")
}

// renderProfileFooter is the actions-substate bottom key-hint bar.
func renderProfileFooter(width int) string {
	parts := []string{
		hint("↵", "Run"), hint("↑↓", "Select"), hint("esc", "Back"),
	}
	return ansi.Truncate(" "+strings.Join(parts, "  "), width, "")
}

// renderDetails is the profile summary body: the two roots and (when scoped) the
// subpaths, each prefixed with a sanity mark from res (nil while the check is
// still running). The mark sits at the start of the line so titledBox's width
// clip trims the path tail, never the mark.
func renderDetails(name string, p config.Profile, res *sanity.Result, width int) string {
	if name == "" {
		return helpTextStyle.Render("No profile selected.")
	}
	label := func(s string) string { return labelStyle.Render(fmt.Sprintf("%-7s", s)) }
	var b strings.Builder
	b.WriteString("  " + label("Name") + name + "\n")
	b.WriteString(existMark(res, res != nil && res.LocalRoot) + " " + label("Local") + p.LocalRoot + "\n")
	b.WriteString(existMark(res, res != nil && res.RemoteRoot) + " " + label("Remote") + p.RemoteRoot + "\n")
	b.WriteString(checkoutLine(res))
	if len(p.Subpaths) > 0 {
		b.WriteString("\n" + labelStyle.Render(fmt.Sprintf("Subpaths (%d)", len(p.Subpaths))))
		for i, sub := range p.Subpaths {
			b.WriteString("\n " + subpathMark(res, i) + " " + sub)
		}
	}
	return b.String()
}

// existMark is the glyph for a root existence check: "…" while pending (res nil),
// green ✓ if present, red ✗ if missing.
func existMark(res *sanity.Result, ok bool) string {
	switch {
	case res == nil:
		return helpTextStyle.Render("…")
	case ok:
		return okStyle.Render("✓")
	default:
		return errStyle.Render("✗")
	}
}

// checkoutLine is the one-line checkout state: pending, unknown (remote not
// mounted so no marker can be read), checked out, or the benign "not checked
// out" (dim, a normal state rather than an error).
func checkoutLine(res *sanity.Result) string {
	switch {
	case res == nil:
		return helpTextStyle.Render("… checkout")
	case !res.RemoteRoot:
		return helpTextStyle.Render("? checkout")
	case res.CheckedOut:
		return okStyle.Render("✓") + " checked out"
	default:
		return helpTextStyle.Render("✗ not checked out")
	}
}

// subpathMark is the glyph for the i-th declared subpath: pending, "?" when the
// remote isn't mounted (can't tell), green ✓ if present on the remote, red ✗ if
// missing.
func subpathMark(res *sanity.Result, i int) string {
	switch {
	case res == nil:
		return helpTextStyle.Render("…")
	case !res.RemoteRoot:
		return helpTextStyle.Render("?")
	case i < len(res.Subpaths) && res.Subpaths[i].Exists:
		return okStyle.Render("✓")
	default:
		return errStyle.Render("✗")
	}
}
