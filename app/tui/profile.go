package tui

import (
	"fmt"
	"strings"

	"github.com/andresbott/netcheckout/internal/lifecycle"
	"github.com/andresbott/netcheckout/internal/rsync"
	"github.com/andresbott/netcheckout/internal/status"
	"github.com/charmbracelet/x/ansi"
)

// profileActions are the operations the Actions box offers for a profile:
// Status runs internal/status; Checkout and Sync run their lifecycle.Runner
// action directly; Check-in opens the shared confirm modal first (see
// updateProfile and confirm.go).
var profileActions = []string{"Status", "Checkout", "Check-in", "Sync"}

// profileModel is the state for the Actions box: which profile is open, which
// action row is selected, and the outcome of the most recent Status run. The
// profile's roots are looked up fresh from cfg at render time, so only the name
// is stored here.
type profileModel struct {
	name         string
	cursor       int
	checking     bool                  // a Status compute is in flight
	result       *status.ProfileStatus // last successful Status result; nil until run
	err          error                 // last Status error; nil if none
	acting       bool                  // a mutating action (Checkout) is in flight
	actionReport *lifecycle.Report     // last successful action's outcome; nil until run
	actionErr    error                 // last action error; nil if none
}

func newProfileView(name string) profileModel { return profileModel{name: name} }

func (p *profileModel) moveUp() {
	if p.cursor > 0 {
		p.cursor--
	}
}

func (p *profileModel) moveDown() {
	if p.cursor < len(profileActions)-1 {
		p.cursor++
	}
}

// renderActions is the Actions box body: one row per action, the selected row
// marked and accented (mirrors listModel.view).
func renderActions(cursor, width int) string {
	var b strings.Builder
	for i, a := range profileActions {
		if i > 0 {
			b.WriteString("\n")
		}
		prefix := "  "
		line := a
		if i == cursor {
			prefix = "▸ "
			line = selectedRowStyle.Render(a)
		}
		b.WriteString(ansi.Truncate(prefix+line, width, ""))
	}
	return b.String()
}

// renderActivity is the right-column placeholder body until the sync/checkout
// engine exists. titledBox clips it to the panel width.
func renderActivity() string {
	return helpTextStyle.Render("sync activity coming soon")
}

// renderStatus is the Activity box body while a profile's actions are showing:
// the idle placeholder before Status has run, an in-flight "Checking…", a styled
// error, or the formatted result of the most recent Status run. titledBox clips
// the body to the panel, so no manual width handling is needed here.
//
// A stopped-on-conflict action carries both a report (with the conflicting
// paths) and an error, so the conflict path list takes precedence over the
// bare error message; any other action error (e.g. a lock/mount failure, where
// the report is empty) still renders as an error rather than an empty body.
func renderStatus(p profileModel) string {
	switch {
	case p.acting:
		return p.name + "\n  Working…"
	case p.actionReport != nil && len(p.actionReport.Conflicts) > 0:
		return actionBody(p.name, *p.actionReport)
	case p.actionErr != nil:
		return p.name + "\n  " + errStyle.Render(p.actionErr.Error())
	case p.actionReport != nil:
		return actionBody(p.name, *p.actionReport)
	case p.checking:
		return p.name + "\n  Checking…"
	case p.err != nil:
		return p.name + "\n  " + errStyle.Render(p.err.Error())
	case p.result != nil:
		return statusBody(p.name, *p.result)
	default:
		return renderActivity()
	}
}

// actionBody formats a completed mutating action's Report: conflicts (nothing
// written) take precedence, otherwise the action name and how many items were
// pulled.
func actionBody(name string, rep lifecycle.Report) string {
	var b strings.Builder
	b.WriteString(name)
	if len(rep.Conflicts) > 0 {
		b.WriteString("\n  conflicts — nothing written:")
		for _, c := range rep.Conflicts {
			fmt.Fprintf(&b, "\n    ! %s", c)
		}
		return b.String()
	}
	fmt.Fprintf(&b, "\n  %s: %d pulled", rep.Action, len(rep.Pulled))
	return b.String()
}

// statusBody formats a computed ProfileStatus: the profile name, then per target
// its label and the push/pull direction lines, each change prefixed with a mark.
func statusBody(name string, st status.ProfileStatus) string {
	var b strings.Builder
	b.WriteString(name)
	if !st.CheckedOut {
		b.WriteString("\n  not checked out")
		return b.String()
	}
	if st.InSync() {
		b.WriteString("\n  in sync")
		return b.String()
	}
	for _, t := range st.Targets {
		b.WriteString("\n  " + t.Label())
		if t.LocalMissing {
			fmt.Fprintf(&b, "\n    not checked out locally (%d items)", len(t.Pull.Changes))
			continue
		}
		writeDirection(&b, "push (local -> remote)", t.Push)
		writeDirection(&b, "pull (remote -> local)", t.Pull)
	}
	return b.String()
}

func writeDirection(b *strings.Builder, label string, d rsync.Diff) {
	if d.InSync {
		fmt.Fprintf(b, "\n    %s: in sync", label)
		return
	}
	fmt.Fprintf(b, "\n    %s: %d changes", label, len(d.Changes))
	for _, c := range d.Changes {
		fmt.Fprintf(b, "\n      %s %s", changeMark(c.Type), c.Path)
	}
}

func changeMark(t rsync.ChangeType) string {
	switch t {
	case rsync.Created:
		return "+"
	case rsync.Deleted:
		return "-"
	default:
		return "M"
	}
}
