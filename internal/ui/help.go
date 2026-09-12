package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func (m appModel) handleHelpKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.help.filter.active {
		switch msg.String() {
		case "esc":
			m.help.filter.input.SetValue("")
			m.help.filter.input.Blur()
			m.help.filter.active = false
			m.help.scroll = 0
			return m, nil
		case "enter":
			m.help.filter.input.Blur()
			m.help.filter.active = false
			m.help.scroll = 0
			return m, nil
		default:
			input, command := m.help.filter.input.Update(msg)
			m.help.filter.input, m.help.scroll = input, 0
			return m, command
		}
	}
	switch msg.String() {
	case "?", "esc", "q":
		if m.help.filter.query() != "" {
			m.help.filter.input.SetValue("")
			m.help.scroll = 0
			return m, nil
		}
		m.help.open = false
		return m, nil
	case "/":
		m.help.filter.active = true
		m.help.filter.input.Focus()
		return m, nil
	case "j", "down":
		m.help.scroll = min(m.help.scroll+1, m.helpMaxScroll())
	case "k", "up":
		m.help.scroll = max(0, m.help.scroll-1)
	}
	return m, nil
}

func helpRows() []string {
	return []string{
		"Navigation",
		"↑/↓\tselect an item or scroll the focused panel",
		"←/→\tmove between workspace panels",
		"enter\topen selected subscription",
		"esc\treturn to the subscription picker",
		"",
		"Subscriptions",
		"r\trefresh subscriptions",
		"/\tfilter subscriptions or domains",
		"s\tsuspend or resume selected subscription",
		"a\tarchive selected subscription",
		"d\tpermanently delete selected archived subscription",
		"",
		"Domains",
		"e\tenable or disable selected domain",
		"t\tenable or disable TLS for selected domain",
		"l / L\tload access or error log",
		"b\tload subscription databases",
		"h\trun health checks",
		"",
		"General",
		"?\topen this help",
		"q\tquit",
	}
}

func (m appModel) filteredHelpRows() []string {
	needle := strings.ToLower(m.help.filter.query())
	if needle == "" {
		return helpRows()
	}
	rows := helpRows()
	filtered := make([]string, 0, len(rows))
	for _, row := range rows {
		if !strings.Contains(row, "\t") || strings.Contains(strings.ToLower(row), needle) {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func (m appModel) helpMaxScroll() int {
	return max(0, len(m.filteredHelpRows())-(m.height-8))
}

func (m appModel) helpPopup() string {
	rows := m.filteredHelpRows()
	bodyRows := max(4, m.height-8)
	start := min(m.help.scroll, max(0, len(rows)-bodyRows))
	end := min(len(rows), start+bodyRows)
	lines := append([]string{}, rows[start:end]...)
	if m.help.filter.active {
		lines = append(lines, "", m.help.filter.input.View()+dimStyle.Render("  enter apply · esc clear"))
	} else {
		lines = append(lines, "", dimStyle.Render("/ filter · ↑/↓ scroll · esc close"))
	}
	for index, line := range lines {
		lines[index] = ansi.Truncate(line, m.popupWidth()-4, "…")
	}
	return panel("Help", strings.Join(lines, "\n"), m.popupWidth(), len(lines)+2, true)
}
