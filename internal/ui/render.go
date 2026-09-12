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
	var view string
	if !m.workspace {
		view = m.renderPicker()
	} else {
		view = m.renderWorkspace()
	}
	if m.help.open {
		return m.overlayCenter(m.helpPopup(), view)
	}
	if m.confirm.action != "" {
		return m.overlayCenter(m.confirmPopup(), view)
	}
	return view
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
	items := m.visibleSubscriptions()
	if len(items) == 0 {
		if m.subscriptionFilter.query() != "" {
			return "No matching subscriptions."
		}
		return "No subscriptions."
	}
	start, end := listWindow(m.cursor, len(items), rows)
	lines := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		item := items[index]
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
	websites := m.visibleWebsites()
	if len(websites) == 0 {
		if m.websiteFilter.query() != "" {
			return "No matching domains."
		}
		return "No websites."
	}
	start, end := listWindow(m.websiteCursor, len(websites), rows)
	lines := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		website := websites[index]
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
	s, ok := m.selectedSubscription()
	if !ok {
		return "No subscription selected."
	}
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

func (m appModel) keybar() string {
	if m.subscriptionFilter.active {
		return m.subscriptionFilter.input.View() + "  enter apply • esc clear"
	}
	if m.websiteFilter.active {
		return m.websiteFilter.input.View() + "  enter apply • esc clear"
	}
	if !m.workspace {
		return "↑/↓ select · enter open · / filter · r refresh · ? help · q quit"
	}
	switch m.focus {
	case focusSubscriptions:
		return "←/→ panels · ↑/↓ select · s suspend/resume · esc subscriptions · ? help"
	case focusWebsites:
		return "←/→ panels · ↑/↓ select · e toggle · l/L logs · esc subscriptions · ? help"
	case focusDetail:
		return "←/→ panels · ↑/↓ scroll · b databases · esc subscriptions · ? help"
	case focusLogs:
		return "←/→ panels · ↑/↓ scroll · l/L logs · esc subscriptions · ? help"
	default:
		return "←/→ panels · ↑/↓ scroll · h health · esc subscriptions · ? help"
	}
}
