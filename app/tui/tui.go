package tui

import (
	"fmt"
	"sort"

	"github.com/andresbott/netcheckout/internal/config"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type mode int

const (
	modeList    mode = iota
	modeForm         // wired in Task 8
	modeConfirm      // wired in Task 9
)

type model struct {
	path        string
	cfg         *config.Config
	mode        mode
	list        list.Model
	form        formModel
	confirmName string // populated in confirm mode (Task 9)
	err         error
}

// Run loads the config at path and starts the interactive TUI.
func Run(path string) error {
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	p := tea.NewProgram(newModel(path, cfg), tea.WithAltScreen())
	_, err = p.Run()
	return err
}

func newModel(path string, cfg *config.Config) model {
	l := list.New(nil, list.NewDefaultDelegate(), 80, 20)
	l.Title = "Profiles"
	l.SetFilteringEnabled(false)
	m := model{path: path, cfg: cfg, list: l, mode: modeList}
	m.refreshList()
	return m
}

func (m *model) refreshList() {
	names := make([]string, 0, len(m.cfg.Profiles))
	for name := range m.cfg.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	items := make([]list.Item, 0, len(names))
	for _, name := range names {
		p := m.cfg.Profiles[name]
		items = append(items, profileItem{name: name, local: p.LocalRoot, remote: p.RemoteRoot})
	}
	m.list.SetItems(items)
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.list.SetSize(ws.Width, ws.Height)
		return m, nil
	}
	switch m.mode {
	case modeForm:
		return m.updateForm(msg)
	default:
		return m.updateList(msg)
	}
}

func (m model) updateList(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "a":
			return m.openForm("", config.Profile{})
		case "e", "enter":
			if it, ok := m.list.SelectedItem().(profileItem); ok {
				return m.openForm(it.name, config.Profile{LocalRoot: it.local, RemoteRoot: it.remote})
			}
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) openForm(origName string, p config.Profile) (tea.Model, tea.Cmd) {
	m.form = newForm(origName, p)
	m.mode = modeForm
	return m, textinput.Blink
}

func (m model) updateForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmd tea.Cmd
		m.form, cmd = m.form.updateInputs(msg)
		return m, cmd
	}
	switch key.String() {
	case "esc":
		m.mode = modeList
		return m, nil
	case "enter":
		return m.submitForm()
	case "tab", "down":
		return m, m.form.focusNext()
	case "shift+tab", "up":
		return m, m.form.focusPrev()
	}
	var cmd tea.Cmd
	m.form, cmd = m.form.updateInputs(msg)
	return m, cmd
}

func (m model) submitForm() (tea.Model, tea.Cmd) {
	name, p := m.form.values()
	if err := validateProfile(m.cfg, m.form.origName, name, p); err != nil {
		m.form.err = err.Error()
		return m, nil
	}
	if m.form.origName != "" && m.form.origName != name {
		delete(m.cfg.Profiles, m.form.origName)
	}
	m.cfg.Profiles[name] = p
	return m.saveAndList()
}

func (m model) saveAndList() (tea.Model, tea.Cmd) {
	if err := config.Save(m.path, m.cfg); err != nil {
		m.err = err
	} else {
		m.err = nil
	}
	m.refreshList()
	m.mode = modeList
	return m, nil
}

func validateProfile(cfg *config.Config, origName, name string, p config.Profile) error {
	if err := config.ValidateName(name); err != nil {
		return err
	}
	if name != origName {
		if _, exists := cfg.Profiles[name]; exists {
			return fmt.Errorf("profile %q already exists", name)
		}
	}
	if err := config.ValidateRoot(p.LocalRoot); err != nil {
		return fmt.Errorf("local root: %w", err)
	}
	if err := config.ValidateRoot(p.RemoteRoot); err != nil {
		return fmt.Errorf("remote root: %w", err)
	}
	return nil
}

func (m model) View() string {
	if m.mode == modeForm {
		return m.form.View()
	}
	return m.listView()
}

func (m model) listView() string {
	if m.err != nil {
		return m.list.View() + "\n" + errStyle.Render("save failed: "+m.err.Error())
	}
	return m.list.View()
}
