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

// mainSub selects what modeMain's top box shows. Enter (subList) reveals the
// selected profile's actions; esc (subActions) returns to the list.
type mainSub int

const (
	subList mainSub = iota
	subActions
)

type model struct {
	path         string
	cfg          *config.Config
	version      string
	identity     string
	width        int
	height       int
	list         listModel
	mode         mode
	sub          mainSub
	form         formModel
	profile      profileModel
	confirmName  string
	confirmFocus confirmFocus
	err          error
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
	m.form.termHeight = ws.Height
	if m.form.browsing {
		m.form.picker.setHeight(m.form.pickerHeight())
	}
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
	if m.sub == subActions {
		return m.updateProfile(msg)
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "q", "esc":
		return m, tea.Quit
	case "enter":
		if name, ok := m.list.selected(); ok {
			return m.openProfile(name)
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
			m.confirmFocus = confirmFocusCancel
			m.mode = modeConfirm
		}
		return m, nil
	case "up", "w":
		m.list.moveUp()
		return m, nil
	case "down", "s":
		m.list.moveDown()
		return m, nil
	}
	return m, nil
}

func (m model) openForm(origName string, p config.Profile) (tea.Model, tea.Cmd) {
	m.form = newForm(origName, p)
	m.form.setWidth(m.width)
	m.form.termHeight = m.height
	m.mode = modeForm
	return m, textinput.Blink
}

func (m model) openProfile(name string) (tea.Model, tea.Cmd) {
	m.profile = newProfileView(name)
	m.sub = subActions
	return m, nil
}

func (m model) updateProfile(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "esc":
		m.sub = subList
		return m, nil
	case "up", "w":
		m.profile.moveUp()
		return m, nil
	case "down", "s":
		m.profile.moveDown()
		return m, nil
	case "enter":
		// actions are not wired to the (nonexistent) checkout engine yet — no-op.
		return m, nil
	}
	return m, nil
}

func (m model) updateForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.form.browsing {
		var cmd tea.Cmd
		m.form, cmd = m.form.updatePicker(msg)
		return m, cmd
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmd tea.Cmd
		m.form, cmd = m.form.updateInputs(msg)
		return m, cmd
	}
	if cmd, ok := m.form.navKey(key.String()); ok {
		return m, cmd
	}
	switch key.String() {
	case "esc":
		m.mode = modeMain
		return m, nil
	case "enter":
		switch formSlots[m.form.focus].kind {
		case slotButton:
			return m, m.form.openPicker()
		case slotSave:
			return m.submitForm()
		case slotCancel:
			m.mode = modeMain
			return m, nil
		}
		// on a text input: enter does nothing
	case " ":
		switch formSlots[m.form.focus].kind {
		case slotButton:
			return m, m.form.openPicker()
		case slotSave:
			return m.submitForm()
		case slotCancel:
			m.mode = modeMain
			return m, nil
		}
		// on a text input: fall through to type the space
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
		return m.overlayModal(confirmModal(m.confirmName, m.confirmFocus, m.width))
	default:
		return m.mainView(false)
	}
}

// mainView composes the header, the two columns, and the footer to exactly the
// terminal size. The left column stacks a top box — the profile list, or (once
// a profile's actions are revealed) the Actions menu — over Details; the right
// column is the Activity placeholder. When dim is true the top box renders
// unfocused (used as the dimmed backdrop behind a modal).
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

	detailsH := bodyH / 2
	topH := bodyH - detailsH

	var topTitle, topBody, name string
	if m.sub == subActions {
		topTitle = "Actions"
		topBody = renderActions(m.profile.cursor, leftW-2)
		name = m.profile.name
	} else {
		topTitle = "Profiles"
		topBody = m.list.view(leftW-2, topH-2)
		name, _ = m.list.selected()
	}
	detailsBody := renderDetails(name, m.cfg.Profiles[name], leftW-2)

	top := titledBox(topTitle, topBody, leftW, topH, !dim)
	details := titledBox("Details", detailsBody, leftW, detailsH, false)
	left := lipgloss.JoinVertical(lipgloss.Left, top, details)
	right := titledBox("Activity", renderActivity(), rightW, bodyH, false)
	panels := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	footer := renderFooter(w)
	if m.sub == subActions {
		footer = renderProfileFooter(w)
	}
	view := renderHeader(w, m.version, m.identity) + "\n" + panels + "\n" + footer
	if m.err != nil {
		view += "\n" + errStyle.Render("save failed: "+m.err.Error())
	}
	return view
}

// overlayModal renders the main view dimmed (both panels unfocused) and
// composites box centered over it, so the modal reads as a floating window.
func (m model) overlayModal(box string) string {
	return placeCenter(m.mainView(true), box)
}
