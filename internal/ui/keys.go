package ui

import tea "github.com/charmbracelet/bubbletea"

// handleKey routes a key by the current interaction mode. Keeping this apart
// from Update makes the value-model message router easy to audit and test.
func (m appModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.subscriptionFilter.active || m.websiteFilter.active {
		return m.handleFilterKey(msg)
	}
	if !m.workspace {
		return m.handlePickerKey(msg)
	}
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
			next := clamp(m.websiteCursor+1, len(m.visibleWebsites()))
			if next != m.websiteCursor {
				m.websiteCursor = next
				m.logsLoad.invalidate()
			}
		} else if m.focus == focusDetail {
			m.detailScroll++
		} else if m.focus == focusOutput {
			m.outputScroll++
		} else {
			next := clamp(m.cursor+1, len(m.visibleSubscriptions()))
			if next != m.cursor {
				m.cursor = next
				m = m.clearSelectionDetails()
			}
		}
	case "k", "up":
		if m.focus == focusWebsites {
			next := clamp(m.websiteCursor-1, len(m.visibleWebsites()))
			if next != m.websiteCursor {
				m.websiteCursor = next
				m.logsLoad.invalidate()
			}
		} else if m.focus == focusDetail {
			m.detailScroll = max(0, m.detailScroll-1)
		} else if m.focus == focusOutput {
			m.outputScroll = max(0, m.outputScroll-1)
		} else {
			next := clamp(m.cursor-1, len(m.visibleSubscriptions()))
			if next != m.cursor {
				m.cursor = next
				m = m.clearSelectionDetails()
			}
		}
	case "r":
		m.status = "loading subscriptions…"
		m, command := m.startSubscriptions()
		return m, command
	case "/":
		if m.focus == focusWebsites {
			m.websiteFilter.active = true
			m.websiteFilter.input.Focus()
		}
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
		if m.showWebsites {
			website, ok := m.selectedWebsite()
			if !ok {
				break
			}
			m.confirm = confirmState{action: "set-enabled", enabled: !website.Enabled, domain: website.PrimaryDomain}
			m.status = "confirm with y; esc cancels"
		}
	case "s":
		if !m.showWebsites {
			subscription, ok := m.selectedSubscription()
			if !ok {
				break
			}
			if subscription.Status == "active" {
				m.confirm = confirmState{action: "suspended", domain: subscription.Name}
			} else if subscription.Status == "suspended" {
				m.confirm = confirmState{action: "active", domain: subscription.Name}
			}
			if m.confirm.action != "" {
				m.status = "confirm with y; esc cancels"
			}
		}
	case "right", "tab":
		m.focus = focusRight(m.focus)
	case "left", "shift+tab":
		m.focus = focusLeft(m.focus)
	case "esc":
		m.workspace, m.focus, m.status = false, focusSubscriptions, "subscription picker"
	}
	return m, nil
}

func (m appModel) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	filter := &m.subscriptionFilter
	if m.websiteFilter.active {
		filter = &m.websiteFilter
	}
	switch msg.String() {
	case "esc":
		filter.input.SetValue("")
		filter.active = false
	case "enter":
		filter.active = false
	default:
		input, _ := filter.input.Update(msg)
		filter.input = input
	}
	if m.subscriptionFilter.active || m.websiteFilter.active {
		return m, nil
	}
	m.cursor = clamp(m.cursor, len(m.visibleSubscriptions()))
	m.websiteCursor = clamp(m.websiteCursor, len(m.visibleWebsites()))
	return m, nil
}

func (m appModel) handlePickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "j", "down":
		m.cursor = clamp(m.cursor+1, len(m.visibleSubscriptions()))
	case "k", "up":
		m.cursor = clamp(m.cursor-1, len(m.visibleSubscriptions()))
	case "/":
		m.subscriptionFilter.active = true
		m.subscriptionFilter.input.Focus()
	case "r":
		m.status = "loading subscriptions…"
		m, command := m.startSubscriptions()
		return m, command
	case "enter":
		if len(m.items) == 0 {
			return m, nil
		}
		m.workspace, m.showWebsites, m.focus, m.status = true, true, focusWebsites, "loading domains…"
		m, command := m.startWebsites()
		return m, command
	}
	return m, nil
}

func focusRight(current focus) focus {
	switch current {
	case focusSubscriptions, focusWebsites:
		return focusDetail
	default:
		return current
	}
}

func focusLeft(current focus) focus {
	switch current {
	case focusDetail, focusLogs, focusOutput:
		return focusWebsites
	default:
		return current
	}
}
