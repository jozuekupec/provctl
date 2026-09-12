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
	if m.pathPicker.open {
		view = m.overlayCenter(m.pathPickerPopup(), view)
		if m.pathPicker.confirm != "" {
			return m.overlayCenter(m.pathPickerConfirmPopup(), view)
		}
		return view
	}
	if m.documentRootForm.open {
		return m.overlayCenter(m.documentRootFormPopup(), view)
	}
	if m.settings.open {
		return m.overlayCenter(m.settingsPopup(), view)
	}
	if m.phpPicker.open {
		return m.overlayCenter(m.phpPickerPopup(), view)
	}
	if m.confirm.action != "" {
		return m.overlayCenter(m.confirmPopup(), view)
	}
	if m.progress.active {
		return m.overlayCenter(m.progressPopup(), view)
	}
	return view
}

func (m appModel) renderPicker() string {
	body := panel(m.subscriptionPanelTitle(), m.renderSubscriptions(m.height-4), m.width, m.height-2, true)
	return m.renderFrame(body)
}

func (m appModel) renderWorkspace() string {
	layout := computeLayout(m.width, m.height, m.focus)
	left := strings.Join([]string{
		panel(m.subscriptionPanelTitle(), m.subscriptionMetadata(), layout.leftWidth, layout.metadata, m.focus == focusSubscriptions),
		panel(m.websitePanelTitle(), m.renderWebsites(layout.domains-2), layout.leftWidth, layout.domains, m.focus == focusWebsites),
		panel("Detail", m.detailLines(layout.detail-2), layout.leftWidth, layout.detail, m.focus == focusDetail),
	}, "\n")
	output := m.outputLines(layout.output - 2)
	right := strings.Join([]string{
		panel("Logs", m.logsLines(layout.logs-2), layout.rightWidth, layout.logs, m.focus == focusLogs),
		panel("Output", output, layout.rightWidth, layout.output, m.focus == focusOutput),
	}, "\n")
	body := joinColumns(left, right)
	return m.renderFrame(body)
}

func (m appModel) subscriptionPanelTitle() string {
	return m.filterPanelTitle("Subscriptions", m.subscriptionFilter, len(m.visibleSubscriptions()), len(m.items))
}

func (m appModel) websitePanelTitle() string {
	return m.filterPanelTitle("Domains", m.websiteFilter, len(m.visibleWebsites()), len(m.websites))
}

func (m appModel) filterPanelTitle(title string, filter filterState, visible, total int) string {
	if filter.query() == "" {
		return title
	}
	return fmt.Sprintf("%s · /%s · %d/%d", title, filter.query(), visible, total)
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
		line := fmt.Sprintf("  %-24s %s", item.Name, item.Status)
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
		"Home: " + valueOrDash(s.Home),
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
		return m.subscriptionFilter.activeSummary("subscriptions", len(m.visibleSubscriptions()), len(m.items))
	}
	if m.websiteFilter.active {
		return m.websiteFilter.activeSummary("domains", len(m.visibleWebsites()), len(m.websites))
	}
	if !m.workspace {
		if summary := m.subscriptionFilter.activeSummary("subscriptions", len(m.visibleSubscriptions()), len(m.items)); summary != "" {
			return summary
		}
		return "↑/↓ select · enter open · a archive · d delete · / filter · , settings · ? help · q quit"
	}
	if m.focus == focusSubscriptions {
		if summary := m.subscriptionFilter.activeSummary("subscriptions", len(m.visibleSubscriptions()), len(m.items)); summary != "" {
			return summary
		}
	}
	if m.focus == focusWebsites {
		if summary := m.websiteFilter.activeSummary("domains", len(m.visibleWebsites()), len(m.websites)); summary != "" {
			return summary
		}
	}
	switch m.focus {
	case focusSubscriptions:
		return "←/→ panels · ↑/↓ select · s suspend/resume · a archive · d delete · , settings · esc back"
	case focusWebsites:
		return "←/→ panels · ↑/↓ select · p PHP · e toggle · E root · t TLS · l/L logs · , settings · esc subscriptions"
	case focusDetail:
		return "←/→ panels · ↑/↓ scroll · b databases · esc subscriptions · ? help"
	case focusLogs:
		return "←/→ panels · ↑/↓ scroll · l/L logs · esc subscriptions · ? help"
	default:
		return "←/→ panels · ↑/↓ scroll · h health · esc subscriptions · ? help"
	}
}
