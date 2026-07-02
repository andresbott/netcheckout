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
	var b strings.Builder
	title := "Add profile"
	if f.origName != "" {
		title = "Edit profile: " + f.origName
	}
	b.WriteString(titleStyle.Render(title))
	b.WriteString("\n\n")
	labels := []string{"Name", "Local root", "Remote root"}
	for i := range f.inputs {
		b.WriteString(labelStyle.Render(labels[i]))
		b.WriteString("\n")
		b.WriteString(f.inputs[i].View())
		b.WriteString("\n\n")
	}
	if f.err != "" {
		b.WriteString(errStyle.Render(f.err))
		b.WriteString("\n\n")
	}
	b.WriteString(helpStyle.Render("tab: next • enter: save • esc: cancel"))
	return appStyle.Render(b.String())
}
