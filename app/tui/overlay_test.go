package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestOverlayPlacesForeground(t *testing.T) {
	bg := strings.Join([]string{".....", ".....", "....."}, "\n")
	got := overlay(bg, "XX", 1, 1)
	want := strings.Join([]string{".....", ".XX..", "....."}, "\n")
	if got != want {
		t.Fatalf("overlay = %q, want %q", got, want)
	}
}

func TestOverlayPreservesWidthOverStyledBackground(t *testing.T) {
	styled := lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	bg := strings.Join([]string{styled.Render("abcde"), styled.Render("fghij")}, "\n")
	got := overlay(bg, "##", 2, 0)
	// The colored background line still measures 5 cells after splicing.
	firstLine := strings.Split(got, "\n")[0]
	if w := lipgloss.Width(firstLine); w != 5 {
		t.Fatalf("width after overlay = %d, want 5", w)
	}
	if !strings.Contains(firstLine, "##") {
		t.Fatalf("overlay content missing: %q", firstLine)
	}
}

func TestPlaceCenter(t *testing.T) {
	bg := strings.Join([]string{"........", "........", "........", "........"}, "\n")
	got := placeCenter(bg, "AA")
	lines := strings.Split(got, "\n")
	// bg is 8 wide, 4 tall; "AA" is 2x1 -> x=3, y=1 (integer center, floor).
	if lines[1] != "...AA..." {
		t.Fatalf("centered line = %q, want %q", lines[1], "...AA...")
	}
}
