package ui

import tea "github.com/charmbracelet/bubbletea"

// handleKey routes a key by the current interaction mode. Keeping this apart
// from Update makes the value-model message router easy to audit and test.
func (m appModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.progress.active {
		switch msg.String() {
		case "esc":
			if m.progress.cancel != nil {
				m.progress.cancel()
			}
			m.status = "cancelling change…"
		case "q", "ctrl+c":
			if m.progress.cancel != nil {
				m.progress.cancel()
			}
			return m, tea.Quit
		}
		return m, nil
	}
	if m.confirm.action != "" {
		switch msg.String() {
		case "y":
			m.status = "applying change…"
			confirm := m.confirm
			m.confirm = confirmState{}
			if confirm.action == "active" || confirm.action == "suspended" {
				return m, m.changeSubscriptionCmd(confirm)
			}
			return m, m.changeWebsiteCmd(confirm)
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
			next := clamp(m.websiteCursor+1, len(m.websites))
			if next != m.websiteCursor {
				m.websiteCursor = next
				m.logsLoad.invalidate()
			}
		} else if m.focus == focusDetail {
			m.detailScroll++
		} else if m.focus == focusOutput {
			m.outputScroll++
		} else {
			next := clamp(m.cursor+1, len(m.items))
			if next != m.cursor {
				m.cursor = next
				m = m.clearSelectionDetails()
			}
		}
	case "k", "up":
		if m.focus == focusWebsites {
			next := clamp(m.websiteCursor-1, len(m.websites))
			if next != m.websiteCursor {
				m.websiteCursor = next
				m.logsLoad.invalidate()
			}
		} else if m.focus == focusDetail {
			m.detailScroll = max(0, m.detailScroll-1)
		} else if m.focus == focusOutput {
			m.outputScroll = max(0, m.outputScroll-1)
		} else {
			next := clamp(m.cursor-1, len(m.items))
			if next != m.cursor {
				m.cursor = next
				m = m.clearSelectionDetails()
			}
		}
	case "r":
		m.status = "loading subscriptions…"
		m, command := m.startSubscriptions()
		return m, command
	case "h":
		m.status = "running health checks…"
		m, command := m.startHealth()
		return m, command
	case "b":
		m.status = "loading databases…"
		m, command := m.startDatabases()
		return m, command
	case "l":
		if m.showWebsites && len(m.websites) > 0 {
			m.status = "loading access log…"
			m, command := m.startWebsiteLogs(false)
			return m, command
		}
	case "L":
		if m.showWebsites && len(m.websites) > 0 {
			m.status = "loading error log…"
			m, command := m.startWebsiteLogs(true)
			return m, command
		}
	case "enter":
		m.showWebsites, m.focus, m.status = true, focusWebsites, "loading websites…"
		m, command := m.startWebsites()
		return m, command
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
		m = m.clearSelectionDetails()
		m.focus = focusSubscriptions
	}
	return m, nil
}
