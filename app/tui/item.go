package tui

// profileItem adapts a profile to the bubbles list.DefaultItem interface.
type profileItem struct {
	name   string
	local  string
	remote string
}

func (i profileItem) Title() string       { return i.name }
func (i profileItem) Description() string { return i.local + "  →  " + i.remote }
func (i profileItem) FilterValue() string { return i.name }
