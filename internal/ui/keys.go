package ui

import tea "github.com/charmbracelet/bubbletea"

// handleKey routes a key by the current interaction mode. Keeping this apart
// from Update makes the value-model message router easy to audit and test.
func (m appModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.confirm.action != "" {
		switch msg.String() {
		case "y":
			m.status = "applying change…"
			if m.confirm.action == "active" || m.confirm.action == "suspended" {
				return m, m.changeSubscription
			}
			return m, m.changeWebsite
		case "esc", "n", "q":
			m.confirm, m.status = confirmState{}, "cancelled"
			return m, nil
		}
		return m, nil
	}
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "j", "down":
		if m.focus == focusWebsites {
			m.websiteCursor = clamp(m.websiteCursor+1, len(m.websites))
		} else {
			m.cursor = clamp(m.cursor+1, len(m.items))
		}
	case "k", "up":
		if m.focus == focusWebsites {
			m.websiteCursor = clamp(m.websiteCursor-1, len(m.websites))
		} else {
			m.cursor = clamp(m.cursor-1, len(m.items))
		}
	case "r":
		m.status = "loading subscriptions…"
		return m, m.loadSubscriptions
	case "h":
		m.status = "running health checks…"
		return m, m.loadHealth
	case "b":
		m.status = "loading databases…"
		return m, m.loadDatabases
	case "l":
		if m.showWebsites && len(m.websites) > 0 {
			m.status = "loading access log…"
			return m, func() tea.Msg { return m.loadWebsiteLogs(false) }
		}
	case "L":
		if m.showWebsites && len(m.websites) > 0 {
			m.status = "loading error log…"
			return m, func() tea.Msg { return m.loadWebsiteLogs(true) }
		}
	case "enter":
		m.showWebsites, m.focus, m.status = true, focusWebsites, "loading websites…"
		return m, m.loadWebsites
	case "d":
		m.focus = focusDetail
	case "o":
		m.focus = focusOutput
	case "e":
		if m.showWebsites && len(m.websites) > 0 {
			website := m.websites[clamp(m.websiteCursor, len(m.websites))]
			m.confirm = confirmState{action: "set-enabled", enabled: !website.Enabled, domain: website.PrimaryDomain}
			m.status = "confirm with y; esc cancels"
		}
	case "s":
		if !m.showWebsites && len(m.items) > 0 {
			subscription := m.items[clamp(m.cursor, len(m.items))]
			if subscription.Status == "active" {
				m.confirm = confirmState{action: "suspended", domain: subscription.Name}
			} else if subscription.Status == "suspended" {
				m.confirm = confirmState{action: "active", domain: subscription.Name}
			}
			if m.confirm.action != "" {
				m.status = "confirm with y; esc cancels"
			}
		}
	case "tab":
		m.focus = (m.focus + 1) % 4
	case "esc":
		m.showWebsites, m.focus = false, focusSubscriptions
	}
	return m, nil
}
