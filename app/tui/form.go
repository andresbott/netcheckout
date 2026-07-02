package tui

import (
	"strings"

	"github.com/andresbott/netcheckout/internal/config"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type formModel struct {
	inputs   []textinput.Model
	focus    int
	origName string // "" for add; the existing name for edit
	err      string
	width    int // terminal width last passed to setWidth
}

func newForm(origName string, p config.Profile) formModel {
	name := textinput.New()
	name.Placeholder = "profile name"
	name.CharLimit = 64
	name.SetValue(origName)

	local := textinput.New()
	local.Placeholder = "/absolute/local/root"
	local.SetValue(p.LocalRoot)

	remote := textinput.New()
	remote.Placeholder = "/absolute/remote/root"
	remote.SetValue(p.RemoteRoot)

	f := formModel{
		inputs:   []textinput.Model{name, local, remote},
		origName: origName,
	}
	f.inputs[0].Focus()
	return f
}

func (f *formModel) focusNext() tea.Cmd { return f.setFocus(f.focus + 1) }
func (f *formModel) focusPrev() tea.Cmd { return f.setFocus(f.focus - 1) }

// setWidth records the terminal width and stretches every input to fill the
// bordered box (see formContentWidth), so the form uses the available width
// instead of sizing to its content.
func (f *formModel) setWidth(w int) {
	f.width = w
	// Each input line is the "> " prompt (2 cols) plus the value area; the
	// bordered box also reserves 1 column of padding on each side.
	inputW := formContentWidth(w) - 4
	if inputW < 10 {
		inputW = 10
	}
	for i := range f.inputs {
		f.inputs[i].Width = inputW
	}
}

// formContentWidth returns the width to pass to the form's border style so
// the full view (title + border + help, inside appStyle) fills the terminal.
// Chrome: appStyle padding (4) + thick border (2) = 6.
func formContentWidth(termWidth int) int {
	w := termWidth - 6
	if w < 20 {
		w = 20
	}
	return w
}

func (f *formModel) setFocus(i int) tea.Cmd {
	n := len(f.inputs)
	i = (i%n + n) % n
	f.focus = i
	var cmd tea.Cmd
	for j := range f.inputs {
		if j == i {
			cmd = f.inputs[j].Focus()
		} else {
			f.inputs[j].Blur()
		}
	}
	return cmd
}

func (f formModel) updateInputs(msg tea.Msg) (formModel, tea.Cmd) {
	var cmd tea.Cmd
	f.inputs[f.focus], cmd = f.inputs[f.focus].Update(msg)
	return f, cmd
}

func (f formModel) values() (string, config.Profile) {
	return strings.TrimSpace(f.inputs[0].Value()),
		config.Profile{
			LocalRoot:  strings.TrimSpace(f.inputs[1].Value()),
			RemoteRoot: strings.TrimSpace(f.inputs[2].Value()),
		}
}

func (f formModel) View() string {
	title := "Add profile"
	if f.origName != "" {
		title = "Edit profile: " + f.origName
	}

	var content strings.Builder
	labels := []string{"Name", "Local root", "Remote root"}
	for i := range f.inputs {
		if i > 0 {
			content.WriteString("\n\n")
		}
		content.WriteString(labelStyle.Render(labels[i]))
		content.WriteString("\n")
		content.WriteString(f.inputs[i].View())
	}
	if f.err != "" {
		content.WriteString("\n\n")
		content.WriteString(errStyle.Render(f.err))
	}

	contentW := formContentWidth(f.width)
	exteriorW := contentW + 2 // + the thick border's own left/right columns
	box := borderStyle.Padding(0, 1).Width(contentW).Render(content.String())

	// Cap title/help to the box's exterior width too: unconstrained, either
	// line's natural length can exceed a narrow box and make appStyle size
	// the whole view to that line instead, overflowing the terminal.
	body := titleStyle.Width(exteriorW).Render(title) + "\n\n" + box + "\n\n" +
		helpStyle.Width(exteriorW).Render("tab: next • enter: save • esc: cancel")
	return appStyle.Render(body)
}
