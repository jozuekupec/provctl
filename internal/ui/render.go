package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m appModel) View() string {
	if !m.ready {
		return "loading…"
	}
	if m.width < minWidth || m.height < minHeight {
		return fmt.Sprintf("provctl needs a terminal of at least %d×%d; current size is %d×%d", minWidth, minHeight, m.width, m.height)
	}
	layout := computeLayout(m.width, m.height)
	left := lipgloss.JoinVertical(lipgloss.Left,
		panel("Subscriptions", m.renderSubscriptions(layout.subscriptions-3), layout.leftWidth, layout.subscriptions, m.focus == focusSubscriptions),
		panel("Websites", m.renderWebsites(layout.websites-3), layout.leftWidth, layout.websites, m.focus == focusWebsites),
	)
	output := m.outputLines(layout.output - 3)
	if m.progress.active {
		output = m.progress.render()
	}
	right := lipgloss.JoinVertical(lipgloss.Left,
		panel("Detail", m.detailLines(layout.detail-3), layout.rightWidth, layout.detail, m.focus == focusDetail),
		panel("Output", output, layout.rightWidth, layout.output, m.focus == focusOutput),
	)
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	status := dimStyle.Render(truncate(m.status, m.width))
	keybar := keybarStyle.Render(truncate(m.keybar(), m.width))
	if m.confirm.action != "" {
		keybar = confirmStyle.Render(truncate(m.confirmationText(), m.width))
	}
	return lipgloss.JoinVertical(lipgloss.Left, body, status, keybar)
}

func (m appModel) renderSubscriptions(rows int) string {
	if len(m.items) == 0 {
		return "No subscriptions."
	}
	start, end := listWindow(m.cursor, len(m.items), rows)
	lines := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		item := m.items[index]
		line := fmt.Sprintf("  %-18s %-10s %s", item.Name, item.Status, valueOrDash(item.PHPVersion))
		if index == m.cursor {
			line = selectedStyle.Render("> " + strings.TrimPrefix(line, "  "))
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (m appModel) renderWebsites(rows int) string {
	if !m.showWebsites {
		return "Enter opens websites for the selected subscription."
	}
	if len(m.websites) == 0 {
		return "No websites."
	}
	start, end := listWindow(m.websiteCursor, len(m.websites), rows)
	lines := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		website := m.websites[index]
		state := "off"
		if website.Enabled {
			state = "on"
		}
		line := fmt.Sprintf("  %-27s %-8s %s", website.PrimaryDomain, website.Type, state)
		if index == m.websiteCursor {
			line = selectedStyle.Render("> " + strings.TrimPrefix(line, "  "))
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (m appModel) detailLines(rows int) string {
	return window(m.detail(), rows, m.detailScroll, false)
}

func (m appModel) outputLines(rows int) string {
	if len(m.output.lines) == 0 {
		return "No output yet."
	}
	return window(strings.Join(m.output.lines, "\n"), rows, m.outputScroll, true)
}

func window(contents string, rows, scroll int, fromBottom bool) string {
	lines := strings.Split(contents, "\n")
	if rows <= 0 || len(lines) == 0 {
		return ""
	}
	maxScroll := max(0, len(lines)-rows)
	scroll = min(max(0, scroll), maxScroll)
	start := scroll
	if fromBottom {
		start = maxScroll - scroll
	}
	end := min(len(lines), start+rows)
	return strings.Join(lines[start:end], "\n")
}

func (m appModel) confirmationText() string {
	if m.confirm.action == "active" || m.confirm.action == "suspended" {
		return fmt.Sprintf("Set subscription %s to %s?  y confirm • esc cancel", m.confirm.domain, m.confirm.action)
	}
	return fmt.Sprintf("Set website %s enabled=%t?  y confirm • esc cancel", m.confirm.domain, m.confirm.enabled)
}

func (m appModel) keybar() string {
	switch m.focus {
	case focusSubscriptions:
		return "↑/↓ select • enter websites • s suspend/resume • h health • b databases • r refresh • q quit"
	case focusWebsites:
		return "↑/↓ select • e enable/disable • l access log • L error log • d detail • esc back • q quit"
	case focusDetail:
		return "↑/↓ scroll • tab next panel • b databases • esc subscriptions • q quit"
	default:
		return "↑/↓ scroll • tab next panel • o output • esc subscriptions • q quit"
	}
}
