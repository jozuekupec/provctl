package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// placeOverlay draws a rectangular foreground without stripping ANSI styling
// from the surrounding panels.
func placeOverlay(x, y int, foreground, background string) string {
	foregroundLines := strings.Split(foreground, "\n")
	backgroundLines := strings.Split(background, "\n")
	foregroundWidth := 0
	for _, line := range foregroundLines {
		foregroundWidth = max(foregroundWidth, ansi.StringWidth(line))
	}
	for index, line := range foregroundLines {
		row := y + index
		if row < 0 || row >= len(backgroundLines) {
			continue
		}
		left := ansi.Truncate(backgroundLines[row], x, "")
		left += strings.Repeat(" ", max(0, x-ansi.StringWidth(left)))
		right := ansi.TruncateLeft(backgroundLines[row], x+foregroundWidth, "")
		backgroundLines[row] = left + fit(line, foregroundWidth) + right
	}
	return strings.Join(backgroundLines, "\n")
}

func (m appModel) overlayCenter(foreground, background string) string {
	foregroundLines := strings.Split(foreground, "\n")
	foregroundWidth := 0
	for _, line := range foregroundLines {
		foregroundWidth = max(foregroundWidth, ansi.StringWidth(line))
	}
	x := max(0, (m.width-foregroundWidth)/2)
	y := max(0, (m.height-len(foregroundLines))/2)
	return placeOverlay(x, y, foreground, background)
}

func (m appModel) popupWidth() int {
	return min(72, max(32, m.width-12))
}
