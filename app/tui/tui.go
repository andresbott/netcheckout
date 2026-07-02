package tui

import (
	"fmt"
	"sort"

	"github.com/andresbott/netcheckout/internal/config"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type mode int

const (
	modeTable   mode = iota
	modeForm         // wired in Task 8
	modeConfirm      // wired in Task 9
	modeCheckout
)

type model struct {
	path            string
	cfg             *config.Config
	mode            mode
	table           table.Model
	form            formModel
	confirmName     string // populated in confirm mode
	checkoutProfile string // populated when opening the checkout view
	err             error
	width           int // last known terminal width, for sizing the form's border
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
	t := table.New(
		table.WithColumns([]table.Column{
			{Title: "Name", Width: 16},
			{Title: "Remote Root", Width: 24},
			{Title: "Local Root", Width: 24},
		}),
		table.WithFocused(true),
		table.WithHeight(20),
	)
	t.SetStyles(table.DefaultStyles())
	m := model{path: path, cfg: cfg, table: t, mode: modeTable}
	m.refreshRows()
	return m
}

func (m *model) refreshRows() {
	names := make([]string, 0, len(m.cfg.Profiles))
	for name := range m.cfg.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	rows := make([]table.Row, 0, len(names))
	for _, name := range names {
		p := m.cfg.Profiles[name]
		rows = append(rows, table.Row{name, p.RemoteRoot, p.LocalRoot})
	}
	m.table.SetRows(rows)
}

// resize fits the table to the terminal: height fills the screen minus the
// title/help/border/padding chrome, and the two root columns share the width
// left over after the fixed-width Name column.
func (m *model) resize(ws tea.WindowSizeMsg) {
	m.width = ws.Width

	const chrome = 8 // title + blank lines + help + border + app padding
	height := ws.Height - chrome
	if height < 3 {
		height = 3
	}
	m.table.SetHeight(height)

	const nameW = 16
	// Horizontal chrome: app padding (4) + thick border (2) + per-column cell
	// padding from table.DefaultStyles (2 cols x 3 columns = 6).
	usable := ws.Width - 12
	rootW := (usable - nameW) / 2
	if rootW < 12 {
		rootW = 12
	}
	m.table.SetColumns([]table.Column{
		{Title: "Name", Width: nameW},
		{Title: "Remote Root", Width: rootW},
		{Title: "Local Root", Width: rootW},
	})

	m.form.setWidth(m.width)
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
	case modeCheckout:
		return m.updateCheckout(msg)
	default:
		return m.updateTable(msg)
	}
}

func (m model) updateTable(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "q", "esc":
			return m, tea.Quit
		case "a":
			return m.openForm("", config.Profile{})
		case "e":
			if name, ok := m.selectedName(); ok {
				return m.openForm(name, m.cfg.Profiles[name])
			}
			return m, nil
		case "enter":
			if name, ok := m.selectedName(); ok {
				m.checkoutProfile = name
				m.mode = modeCheckout
			}
			return m, nil
		case "d":
			if name, ok := m.selectedName(); ok {
				m.confirmName = name
				m.mode = modeConfirm
			}
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// selectedName returns the profile name for the highlighted row, or false when
// the table is empty.
func (m model) selectedName() (string, bool) {
	row := m.table.SelectedRow()
	if row == nil {
		return "", false
	}
	return row[0], true
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
		m.mode = modeTable
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
	m.refreshRows()
	m.mode = modeTable
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

func (m model) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "y", "Y":
		prev := cloneProfiles(m.cfg.Profiles)
		delete(m.cfg.Profiles, m.confirmName)
		if err := commitProfiles(m.path, m.cfg, prev); err != nil {
			m.err = err
			m.mode = modeTable
			return m, nil
		}
		m.refreshRows()
		m.mode = modeTable
		m.err = nil
		return m, nil
	case "n", "N", "esc":
		m.mode = modeTable
		return m, nil
	}
	return m, nil
}

// updateCheckout handles the checkout placeholder view: esc returns to the
// table; every other key (besides the global ctrl+c handled in Update) is a
// no-op, since there are no actions here yet.
func (m model) updateCheckout(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if key.String() == "esc" {
		m.mode = modeTable
	}
	return m, nil
}

func (m model) confirmView() string {
	body := titleStyle.Render("Delete profile") + "\n\n" +
		"Delete profile \"" + m.confirmName + "\"?\n\n" +
		helpStyle.Render("y: delete • n/esc: cancel")
	return appStyle.Render(body)
}

// checkoutView is a placeholder for the checkout/sync/check-in feature
// described in GOALS.md: it shows the selected profile's roots but performs
// no file operations yet.
func (m model) checkoutView() string {
	p := m.cfg.Profiles[m.checkoutProfile]

	content := labelStyle.Render(fmt.Sprintf("%-13s", "Local root")) + p.LocalRoot + "\n" +
		labelStyle.Render(fmt.Sprintf("%-13s", "Remote root")) + p.RemoteRoot + "\n\n" +
		"Checkout / sync / check-in coming soon."

	contentW := boxContentWidth(m.width)
	exteriorW := contentW + 2 // + the thick border's own left/right columns
	box := borderStyle.Padding(0, 1).Width(contentW).Render(content)

	body := titleStyle.Width(exteriorW).Render(m.checkoutProfile) + "\n\n" + box + "\n\n" +
		helpStyle.Width(exteriorW).Render("esc: back")
	return appStyle.Render(body)
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
		return m.form.View()
	case modeConfirm:
		return m.confirmView()
	case modeCheckout:
		return m.checkoutView()
	default:
		return m.tableView()
	}
}

func (m model) tableView() string {
	body := titleStyle.Render("Profiles") + "\n\n" +
		borderStyle.Render(m.table.View()) + "\n\n" +
		helpStyle.Render("a add • e edit • d delete • q quit")
	if m.err != nil {
		body += "\n" + errStyle.Render("save failed: "+m.err.Error())
	}
	return appStyle.Render(body)
}
