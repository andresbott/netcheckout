package tui

import (
	"fmt"
	"os"
	"sort"

	"github.com/andresbott/netcheckout/app/metainfo"
	"github.com/andresbott/netcheckout/internal/config"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type mode int

const (
	modeMain mode = iota
	modeForm
	modeConfirm
)

type pane int

const (
	paneList pane = iota
	paneDetails
)

type model struct {
	path        string
	cfg         *config.Config
	version     string
	identity    string
	width       int
	height      int
	focus       pane
	list        listModel
	mode        mode
	form        formModel
	confirmName string
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
	m := model{
		path:     path,
		cfg:      cfg,
		version:  metainfo.Version,
		identity: identityString(cfg),
		mode:     modeMain,
		focus:    paneList,
		list:     newList(nil),
	}
	m.refreshList()
	return m
}

// identityString is the header's right-hand text: the configured identity, or
// "$USER@$HOSTNAME" as GOALS.md specifies for the default.
func identityString(cfg *config.Config) string {
	if cfg.Identity != "" {
		return cfg.Identity
	}
	user := os.Getenv("USER")
	host, _ := os.Hostname()
	switch {
	case user != "" && host != "":
		return user + "@" + host
	case host != "":
		return host
	default:
		return "unknown"
	}
}

func (m *model) refreshList() {
	names := make([]string, 0, len(m.cfg.Profiles))
	for name := range m.cfg.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	m.list.setNames(names)
}

func (m *model) resize(ws tea.WindowSizeMsg) {
	m.width = ws.Width
	m.height = ws.Height
	m.form.setWidth(ws.Width)
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "ctrl+c" {
		return m, tea.Quit
	}
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.resize(ws)
		return m, nil
	}
	switch m.mode {
	case modeForm:
		return m.updateForm(msg)
	case modeConfirm:
		return m.updateConfirm(msg)
	default:
		return m.updateMain(msg)
	}
}

func (m model) updateMain(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "q":
		return m, tea.Quit
	case "esc":
		if m.focus == paneDetails {
			m.focus = paneList
			return m, nil
		}
		return m, tea.Quit
	case "tab":
		if m.focus == paneList {
			m.focus = paneDetails
		} else {
			m.focus = paneList
		}
		return m, nil
	case "enter":
		if _, ok := m.list.selected(); ok {
			m.focus = paneDetails
		}
		return m, nil
	case "a":
		return m.openForm("", config.Profile{})
	case "e":
		if name, ok := m.list.selected(); ok {
			return m.openForm(name, m.cfg.Profiles[name])
		}
		return m, nil
	case "d":
		if name, ok := m.list.selected(); ok {
			m.confirmName = name
			m.mode = modeConfirm
		}
		return m, nil
	case "up", "k":
		if m.focus == paneList {
			m.list.moveUp()
		}
		return m, nil
	case "down", "j":
		if m.focus == paneList {
			m.list.moveDown()
		}
		return m, nil
	}
	return m, nil
}

func (m model) openForm(origName string, p config.Profile) (tea.Model, tea.Cmd) {
	m.form = newForm(origName, p)
	m.form.setWidth(m.width)
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
		m.mode = modeMain
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
	prev := cloneProfiles(m.cfg.Profiles)
	if m.form.origName != "" && m.form.origName != name {
		delete(m.cfg.Profiles, m.form.origName)
	}
	m.cfg.Profiles[name] = p
	if err := commitProfiles(m.path, m.cfg, prev); err != nil {
		m.form.err = "save failed: " + err.Error()
		return m, nil
	}
	m.refreshList()
	m.mode = modeMain
	m.err = nil
	return m, nil
}

// cloneProfiles returns a shallow copy of p so a mutation can be snapshotted
// before a save and rolled back if the save fails.
func cloneProfiles(p map[string]config.Profile) map[string]config.Profile {
	out := make(map[string]config.Profile, len(p))
	for k, v := range p {
		out[k] = v
	}
	return out
}

// commitProfiles saves cfg to path. On failure it restores prev into
// cfg.Profiles so in-memory state never diverges from what is on disk.
func commitProfiles(path string, cfg *config.Config, prev map[string]config.Profile) error {
	if err := config.Save(path, cfg); err != nil {
		cfg.Profiles = prev
		return err
	}
	return nil
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
	switch m.mode {
	case modeForm:
		return m.overlayModal(m.form.View())
	case modeConfirm:
		return m.overlayModal(confirmModal(m.confirmName))
	default:
		return m.mainView(false)
	}
}

// mainView composes the header, the two panels, and the footer to exactly the
// terminal size. When dim is true both panels render unfocused (used as the
// dimmed backdrop behind a modal in Task 6).
func (m model) mainView(dim bool) string {
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

	listFocused := !dim && m.focus == paneList
	detailsFocused := !dim && m.focus == paneDetails

	name, _ := m.list.selected()
	listBody := m.list.view(leftW-2, bodyH-2)
	detailsBody := renderDetails(name, m.cfg.Profiles[name], rightW-2)

	left := titledBox("Profiles", listBody, leftW, bodyH, listFocused)
	right := titledBox("Details", detailsBody, rightW, bodyH, detailsFocused)
	panels := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	view := renderHeader(w, m.version, m.identity) + "\n" + panels + "\n" + renderFooter(w)
	if m.err != nil {
		view += "\n" + errStyle.Render("save failed: "+m.err.Error())
	}
	return view
}

// overlayModal centers box over a backdrop the size of the terminal.
// Task 6 replaces the blank backdrop with the dimmed main view.
func (m model) overlayModal(box string) string {
	w, h := m.width, m.height
	if w == 0 {
		w, h = 80, 24
	}
	backdrop := lipgloss.NewStyle().Width(w).Height(h).Render("")
	return placeCenter(backdrop, box)
}
