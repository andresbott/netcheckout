package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// profileActions are the operations the Actions box will offer once the
// checkout engine exists. They are placeholders — selecting one does nothing yet.
var profileActions = []string{"Status", "Checkout", "Check-in", "Sync"}

// profileModel is the state for the Actions box: which profile is open and
// which action row is selected. The profile's roots are looked up fresh from
// cfg at render time, so only the name is stored here.
type profileModel struct {
	name   string
	cursor int
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
