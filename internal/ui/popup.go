package ui

import (
	"fmt"
	"strings"
)

// helpPopupDimensions reserves a stable, bounded box for Help. The box must
// not grow with new shortcuts: Help is intentionally scrollable.
func (m appModel) helpPopupDimensions() (int, int) {
	width := max(68, m.width*2/3)
	width = min(92, min(width, m.width-2))
	height := max(16, m.height*2/3)
	height = min(height, m.height)
	return width, height
}

// popupWindow returns at most rows lines and reserves one row to acknowledge
// overflow. A modal without a visible way out is worse than a short modal.
func popupWindow(lines []string, rows, scroll int) []string {
	if rows <= 0 || len(lines) == 0 {
		return nil
	}
	if len(lines) <= rows {
		return lines
	}
	contentRows := max(1, rows-1)
	maxStart := len(lines) - contentRows
	start := min(max(0, scroll), maxStart)
	end := min(len(lines), start+contentRows)
	result := append([]string(nil), lines[start:end]...)
	above, below := start, len(lines)-end
	switch {
	case above > 0 && below > 0:
		result = append(result, fmt.Sprintf("… %d above · %d below", above, below))
	case above > 0:
		result = append(result, fmt.Sprintf("… %d above", above))
	default:
		result = append(result, fmt.Sprintf("… %d more below", below))
	}
	return result
}

func popupFooter(lines []string, footer string, height int) string {
	rows := max(0, height-2)
	for len(lines) < rows-2 {
		lines = append(lines, "")
	}
	lines = append(lines, "", footer)
	return strings.Join(lines, "\n")
}
