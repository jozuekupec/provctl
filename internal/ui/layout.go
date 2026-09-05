package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type layout struct {
	leftWidth, rightWidth   int
	subscriptions, websites int
	detail, output          int
}

func computeLayout(width, height int) layout {
	body := height - 2 // status and keybar consume the final two rows.
	left := width / 2
	right := width - left
	top := body / 2
	return layout{leftWidth: left, rightWidth: right, subscriptions: top, websites: body - top, detail: top, output: body - top}
}

func panel(title, body string, width, height int, active bool) string {
	if width < 4 || height < 3 {
		return ""
	}
	border := panelBorderStyle
	borderShape := lipgloss.RoundedBorder()
	titleStyle := panelTitleStyle
	if active {
		border, titleStyle = panelFocusBorder, panelActiveStyle
		borderShape = lipgloss.ThickBorder()
	}
	textWidth, textHeight := width-4, height-2
	lines := strings.Split(body, "\n")
	if len(lines) > textHeight {
		lines = lines[:textHeight]
	}
	for index, line := range lines {
		lines[index] = truncate(line, textWidth)
	}
	for len(lines) < textHeight {
		lines = append(lines, "")
	}
	contents := strings.Join(lines, "\n")
	return lipgloss.NewStyle().Border(borderShape).BorderForeground(border.GetForeground()).Padding(0, 1).Width(width - 2).Height(height - 2).Render(titleStyle.Render(title) + "\n" + contents)
}

func truncate(value string, width int) string {
	if width <= 0 || lipgloss.Width(value) <= width {
		return value
	}
	return ansi.Truncate(value, width, "…")
}

func listWindow(cursor, count, rows int) (int, int) {
	if count == 0 || rows <= 0 {
		return 0, 0
	}
	cursor = clamp(cursor, count)
	start := max(0, cursor-rows/2)
	end := min(count, start+rows)
	start = max(0, end-rows)
	return start, end
}
