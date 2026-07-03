package tui

import (
	"strings"
	"testing"

	"github.com/andresbott/netcheckout/internal/config"
	"github.com/charmbracelet/lipgloss"
)

func TestRenderHeaderFitsAndShowsIdentity(t *testing.T) {
	h := renderHeader(60, "1.2.3", "andres@thinkpad")
	if w := lipgloss.Width(h); w > 60 {
		t.Errorf("header width %d > 60", w)
	}
	if !strings.Contains(h, "1.2.3") || !strings.Contains(h, "andres@thinkpad") {
		t.Errorf("header missing version/identity: %q", h)
	}
}

func TestRenderFooterFitsAndHasHints(t *testing.T) {
	f := renderFooter(80)
	if w := lipgloss.Width(f); w > 80 {
		t.Errorf("footer width %d > 80", w)
	}
	if !strings.Contains(f, "Add") || !strings.Contains(f, "Quit") {
		t.Errorf("footer missing hints: %q", f)
	}
}

func TestRenderDetailsShowsRoots(t *testing.T) {
	d := renderDetails("photos", config.Profile{LocalRoot: "/home/me/pics", RemoteRoot: "/mnt/nas/pics"}, 40)
	for _, want := range []string{"photos", "/home/me/pics", "/mnt/nas/pics"} {
		if !strings.Contains(d, want) {
			t.Errorf("details missing %q:\n%s", want, d)
		}
	}
}
