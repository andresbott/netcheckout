package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// profileActions are the operations the profile view will offer once the
// checkout engine exists. They are placeholders — selecting one does nothing yet.
var profileActions = []string{"Checkout", "Check-in", "Status", "Sync"}

// profileModel is the state for the full-screen per-profile view: which profile
// is open and which action row is selected. The profile's roots are looked up
// fresh from cfg at render time, so only the name is stored here.
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

// renderFileStatus is the right-panel placeholder body until the checkout engine
// exists. titledBox clips it to the panel width.
func renderFileStatus() string {
	return helpTextStyle.Render("file status coming soon")
}

// profileView is the full-screen per-profile screen: a focused Actions box stacked
// over a Details box on the left, and a full-height File status box on the right.
// It mirrors mainView's header/panels/footer composition and size arithmetic so it
// fills the terminal exactly (leftW + rightW == w keeps the joined view within width).
func (m model) profileView() string {
	w, h := m.width, m.height
	if w == 0 {
		w, h = 80, 24 // pre-resize fallback so the view is never empty
	}
	bodyH := h - 2 // header + footer
	if bodyH < 3 {
		bodyH = 3
	}
	leftW := w / 3
	if leftW < 16 {
		leftW = 16
	}
	rightW := w - leftW

	// Split the left column so Details + Actions together equal the right box height.
	actionsH := len(profileActions) + 2 // border + one row per action
	detailsH := bodyH - actionsH
	if detailsH < 3 {
		detailsH = 3
		actionsH = bodyH - detailsH
		if actionsH < 3 {
			actionsH = 3
		}
	}

	name := m.profile.name
	detailsBody := renderDetails(name, m.cfg.Profiles[name], leftW-2)
	actionsBody := renderActions(m.profile.cursor, leftW-2)

	details := titledBox("Details", detailsBody, leftW, detailsH, false)
	actions := titledBox("Actions", actionsBody, leftW, actionsH, true)
	left := lipgloss.JoinVertical(lipgloss.Left, actions, details)
	right := titledBox("File status", renderFileStatus(), rightW, bodyH, false)
	panels := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	return renderHeader(w, m.version, m.identity) + "\n" + panels + "\n" + renderProfileFooter(w)
}
