package tui

import (
	"sort"

	"github.com/andresbott/netcheckout/internal/config"
	"github.com/charmbracelet/bubbles/list"
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
	return m.updateList(msg)
}

func (m model) updateList(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) View() string {
	return m.listView()
}

func (m model) listView() string {
	if m.err != nil {
		return m.list.View() + "\n" + errStyle.Render("save failed: "+m.err.Error())
	}
	return m.list.View()
}
