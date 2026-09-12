package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestPopupBox_FixedLargeKeepsClassDimensions(t *testing.T) {
	m := New(Deps{})
	m.width, m.height = 100, 36
	short := popupBox(m, popupOpts{Size: popupLarge, Fit: fitFixed, Title: "Settings", Body: []string{"one"}, Footer: []string{"esc close"}})
	long := popupBox(m, popupOpts{Size: popupLarge, Fit: fitFixed, Title: "Settings", Body: []string{strings.Repeat("long value ", 40)}, Footer: []string{"esc close"}})
	if lipgloss.Width(short) != lipgloss.Width(long) || lipgloss.Height(short) != lipgloss.Height(long) {
		t.Fatalf("fixed popup dimensions changed: %dx%d vs %dx%d", lipgloss.Width(short), lipgloss.Height(short), lipgloss.Width(long), lipgloss.Height(long))
	}
}

func TestPopupBox_FixedFooterSitsAtBottom(t *testing.T) {
	m := New(Deps{})
	m.width, m.height = 80, 24
	popup := popupBox(m, popupOpts{Size: popupLarge, Fit: fitFixed, Title: "Settings", Body: []string{"value"}, Footer: []string{"esc cancel"}})
	lines := strings.Split(popup, "\n")
	if !strings.Contains(lines[len(lines)-2], "esc cancel") {
		t.Fatalf("footer is not on final content row:\n%s", popup)
	}
}
