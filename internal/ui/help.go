package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m appModel) handleHelpKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.help.filter.active {
		switch msg.String() {
		case "esc":
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
	case "?", "q":
		m.help.open = false
		return m, nil
	case "esc":
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

type helpSection struct {
	title string
	rows  []string
}

func helpSections() []helpSection {
	return []helpSection{
		{title: "Navigation", rows: []string{
			"↑/↓       select an item or scroll the focused panel",
			"←/→       move between workspace panels",
			"enter     open selected subscription",
			"esc       clear a focused filter or return to picker",
		}},
		{title: "Subscriptions", rows: []string{
			"r         refresh subscriptions",
			"/         filter subscriptions or domains",
			"s         suspend or resume selected subscription",
			"a         archive selected subscription",
			"d         permanently delete selected archived subscription",
		}},
		{title: "Domains", rows: []string{
			"e         enable or disable selected domain",
			"t         enable or disable TLS for selected domain",
			"l / L     load access or error log",
			"b         load subscription databases",
			"h         run health checks",
		}},
		{title: "General", rows: []string{
			"?         open this help",
			"q         quit",
		}},
	}
}

func (m appModel) filteredHelpRows() []string {
	needle := strings.ToLower(m.help.filter.query())
	filtered := make([]string, 0, 24)
	for _, section := range helpSections() {
		matches := make([]string, 0, len(section.rows))
		for _, row := range section.rows {
			if needle == "" || strings.Contains(strings.ToLower(section.title+" "+row), needle) {
				matches = append(matches, row)
			}
		}
		if len(matches) == 0 {
			continue
		}
		if len(filtered) > 0 {
			filtered = append(filtered, "")
		}
		filtered = append(filtered, section.title)
		filtered = append(filtered, matches...)
	}
	if needle != "" && len(filtered) == 0 {
		return []string{"No matching shortcut."}
	}
	return filtered
}

func (m appModel) helpMaxScroll() int {
	return max(0, len(m.filteredHelpRows())-m.helpBodyRows())
}

func (m appModel) helpBodyRows() int {
	_, height := m.helpPopupDimensions()
	// panel body plus a separator and persistent footer; the filter line is
	// popup chrome too and may not push shortcut rows below its border.
	rows := height - 2 - 2
	if m.help.filter.active || m.help.filter.query() != "" {
		rows--
	}
	return max(1, rows)
}

func (m appModel) helpPopup() string {
	width, height := m.helpPopupDimensions()
	lines := make([]string, 0, height-2)
	if m.help.filter.active {
		lines = append(lines, filterLabelStyle.Render("Filter help:")+" "+m.help.filter.input.View()+dimStyle.Render("  enter apply · esc keep"))
	} else if query := m.help.filter.query(); query != "" {
		lines = append(lines, filterLabelStyle.Render("Filter help: /"+query)+dimStyle.Render("  esc clear"))
	}
	lines = append(lines, popupWindow(m.filteredHelpRows(), m.helpBodyRows(), m.help.scroll)...)
	footer := dimStyle.Render("/ filter this help · ↑/↓ scroll · esc close")
	return panel("Help", popupFooter(lines, footer, height), width, height, true)
}
