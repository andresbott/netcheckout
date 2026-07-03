package tui

import (
	"strings"

	"github.com/andresbott/netcheckout/internal/config"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

// modalWidth is the form window's total width: capped, and clamped to a
// sensible minimum for narrow terminals.
func (f formModel) modalWidth() int {
	w := f.width - 8
	if w > 100 {
		w = 100
	}
	if w < 30 {
		w = 30
	}
	return w
}

// setWidth records the terminal width and sizes each input to the modal's
// inner content area.
func (f *formModel) setWidth(w int) {
	f.width = w
	inputW := f.modalWidth() - 6 // borders (2) + box padding (2) + prompt "> " (2)
	if inputW < 6 {
		inputW = 6
	}
	for i := range f.inputs {
		f.inputs[i].Width = inputW
	}
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
			content.WriteString("\n")
		}
		content.WriteString(labelStyle.Render(labels[i]))
		content.WriteString("\n")
		content.WriteString(f.inputs[i].View())
	}
	if f.err != "" {
		content.WriteString("\n\n")
		content.WriteString(errStyle.Render(f.err))
	}
	content.WriteString("\n\n")
	content.WriteString(helpTextStyle.Render("tab next · enter save · esc cancel"))

	// Inset the content one column from the border on each side; the input
	// widths (modalWidth-6) already reserve those two columns.
	body := lipgloss.NewStyle().Padding(0, 1).Render(content.String())
	return titledBox(title, body, f.modalWidth(), lipgloss.Height(body)+2, true)
}
