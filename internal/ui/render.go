package ui

import (
	"fmt"
	"strings"
)

func (m appModel) View() string {
	if !m.ready {
		return "loading…"
	}
	if m.width < minWidth || m.height < minHeight {
		return fmt.Sprintf("provctl needs a terminal of at least %d×%d; current size is %d×%d", minWidth, minHeight, m.width, m.height)
	}
	if !m.workspace {
		return m.renderPicker()
	}
	return m.renderWorkspace()
}

func (m appModel) renderPicker() string {
	body := panel("Subscriptions", m.renderSubscriptions(m.height-4), m.width, m.height-2, true)
	return m.renderFrame(body)
}

func (m appModel) renderWorkspace() string {
	layout := computeLayout(m.width, m.height)
	left := strings.Join([]string{
		panel("Subscription", m.subscriptionMetadata(), layout.leftWidth, layout.metadata, m.focus == focusSubscriptions),
		panel("Domains", m.renderWebsites(layout.domains-2), layout.leftWidth, layout.domains, m.focus == focusWebsites),
	}, "\n")
	output := m.outputLines(layout.output - 2)
	if m.progress.active {
		output = m.progress.render()
	}
	right := strings.Join([]string{
		panel("Detail", m.detailLines(layout.detail-2), layout.rightWidth, layout.detail, m.focus == focusDetail),
		panel("Logs", m.logsLines(layout.logs-2), layout.rightWidth, layout.logs, m.focus == focusLogs),
		panel("Output", output, layout.rightWidth, layout.output, m.focus == focusOutput),
	}, "\n")
	body := joinColumns(left, right)
	return m.renderFrame(body)
}

func (m appModel) renderFrame(body string) string {
	status := dimStyle.Render(truncate(m.status, m.width))
	keybar := keybarStyle.Render(truncate(m.keybar(), m.width))
	if m.confirm.action != "" {
		keybar = confirmStyle.Render(truncate(m.confirmationText(), m.width))
	}
	return strings.Join([]string{body, fit(status, m.width), fit(keybar, m.width)}, "\n")
}

func joinColumns(left, right string) string {
	leftRows, rightRows := strings.Split(left, "\n"), strings.Split(right, "\n")
	rows := max(len(leftRows), len(rightRows))
	result := make([]string, rows)
	for index := range rows {
		var a, b string
		if index < len(leftRows) {
			a = leftRows[index]
		}
		if index < len(rightRows) {
			b = rightRows[index]
		}
		result[index] = a + b
	}
	return strings.Join(result, "\n")
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

func (m appModel) subscriptionMetadata() string {
	if len(m.items) == 0 {
		return "No subscription selected."
	}
	s := m.items[clamp(m.cursor, len(m.items))]
	return strings.Join([]string{
		"Name: " + s.Name, "Status: " + s.Status, "User: " + s.UnixUser,
		"Home: " + valueOrDash(s.Home), "PHP-FPM: " + valueOrDash(s.PHPVersion),
		"SSH: " + valueOrDash(s.SSHAccess),
	}, "\n")
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

func (m appModel) logsLines(rows int) string {
	if len(m.logs.lines) == 0 {
		return "Press l for access or L for error log."
	}
	return window(strings.Join(m.logs.lines, "\n"), rows, m.outputScroll, true)
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
	if !m.workspace {
		return "↑/↓ select • enter open • / filter • n new • e edit • s suspend/resume • a archive • d delete • ? help • q quit"
	}
	switch m.focus {
	case focusSubscriptions:
		return "← domains • ↑/↓ scroll • e edit • s suspend/resume • esc subscriptions • ? help"
	case focusWebsites:
		return "←/→ panels • ↑/↓ select • e enable/disable • l access • L error • esc subscriptions • ? help"
	case focusDetail:
		return "← domains • ↑/↓ scroll • b databases • esc subscriptions • ? help"
	case focusLogs:
		return "← domains • ↑/↓ scroll • l access • L error • esc subscriptions • ? help"
	default:
		return "← domains • ↑/↓ scroll • h health • esc subscriptions • ? help"
	}
}
