package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type layout struct {
	width, height                           int
	leftWidth, rightWidth                   int
	metadata, domains, detail, logs, output int
}

func computeLayout(width, height int, active focus) layout {
	body := height - 2 // Status and keybar consume the final two rows.
	left := width / 2
	right := width - left
	metadata := 8
	domains, detail := splitLeftWorkspace(body-metadata, active)
	logs, output := splitRightWorkspace(body, active)
	// Joining bordered panels uses the line break that starts the next panel;
	// it does not consume a separate visual row.
	return layout{width: width, height: height, leftWidth: left, rightWidth: right, metadata: metadata, domains: domains, detail: detail, logs: logs, output: output}
}

// splitLeftWorkspace keeps Details below Domains. At rest Details occupy half
// of the whole workspace column; Domains receive the remaining available rows.
// A focused content panel receives the larger share without hiding its sibling.
func splitLeftWorkspace(remaining int, active focus) (domains, detail int) {
	if active == focusWebsites || active == focusDetail {
		return splitFocusedPair(remaining, active == focusWebsites)
	}
	// Details are intentionally half the full body, not half of the space left
	// after metadata. This gives the selected domain enough room for its fields.
	detail = (remaining + 8) / 2
	return remaining - detail, detail
}

// splitRightWorkspace leaves Logs and Output equal until either is focused.
func splitRightWorkspace(total int, active focus) (logs, output int) {
	if active == focusLogs || active == focusOutput {
		return splitFocusedPair(total, active == focusLogs)
	}
	return total / 2, total - total/2
}

func splitFocusedPair(total int, firstFocused bool) (first, second int) {
	const minimum = 4
	if total <= 2*minimum {
		first = total / 2
		return first, total - first
	}
	focused := (total*62 + 99) / 100
	if focused > total-minimum {
		focused = total - minimum
	}
	if firstFocused {
		return focused, total - focused
	}
	return total - focused, focused
}

// panel renders an exact outer rectangle. Layout calculations therefore use
// the same dimensions that are actually written to the terminal.
func panel(title, body string, width, height int, active bool) string {
	if width < 4 || height < 2 {
		return ""
	}
	border, titleStyle := panelBorderStyle, panelTitleStyle
	leftTop, horizontal, rightTop, vertical, leftBottom, rightBottom := "╭", "─", "╮", "│", "╰", "╯"
	if active {
		border, titleStyle = panelFocusBorder, panelActiveStyle
		leftTop, horizontal, rightTop, vertical, leftBottom, rightBottom = "┏", "━", "┓", "┃", "┗", "┛"
	}
	textWidth, textHeight := width-4, height-2
	lines := strings.Split(body, "\n")
	var result strings.Builder
	titleText := " " + ansi.Truncate(title, max(1, width-4), "…") + " "
	// The left corner plus its initial horizontal cell and the final right
	// corner consume three outer cells.
	titleFill := max(0, width-3-ansi.StringWidth(titleText))
	result.WriteString(border.Render(leftTop + horizontal))
	result.WriteString(titleStyle.Render(titleText))
	result.WriteString(border.Render(strings.Repeat(horizontal, titleFill) + rightTop))
	for index := 0; index < textHeight; index++ {
		line := ""
		if index < len(lines) {
			line = ansi.Truncate(lines[index], textWidth, "…")
		}
		result.WriteByte('\n')
		result.WriteString(border.Render(vertical))
		result.WriteString(" " + fit(line, textWidth) + " ")
		result.WriteString(border.Render(vertical))
	}
	result.WriteByte('\n')
	result.WriteString(border.Render(leftBottom + strings.Repeat(horizontal, width-2) + rightBottom))
	return result.String()
}

func truncate(value string, width int) string {
	if width <= 0 || lipgloss.Width(value) <= width {
		return value
	}
	return ansi.Truncate(value, width, "…")
}

func fit(value string, width int) string {
	value = ansi.Truncate(value, width, "…")
	return value + strings.Repeat(" ", max(0, width-ansi.StringWidth(value)))
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
