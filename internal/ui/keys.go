package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"provctl/internal/domain"
)

// handleKey routes a key by the current interaction mode. Keeping this apart
// from Update makes the value-model message router easy to audit and test.
func (m appModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.pathPicker.open {
		return m.handlePathPickerKey(msg)
	}
	if m.documentRootForm.open {
		return m.handleDocumentRootFormKey(msg)
	}
	if m.settings.open {
		return m.handleSettingsKey(msg)
	}
	if m.phpPicker.open {
		return m.handlePHPPickerKey(msg)
	}
	if m.help.open {
		return m.handleHelpKey(msg)
	}
	if m.confirm.action != "" {
		return m.handleConfirmKey(msg)
	}
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
	switch msg.String() {
	case ",":
		return m.openSettings(), nil
	case "?":
		m.help = helpState{open: true, filter: newFilter()}
		m.status = ""
		return m, nil
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
		if m.focus == focusSubscriptions {
			m.subscriptionFilter.active = true
			m.subscriptionFilter.input.Focus()
		} else if m.focus == focusWebsites {
			m.websiteFilter.active = true
			m.websiteFilter.input.Focus()
		}
	case "h":
		m.status = "running health checks…"
		m, command := m.startHealth()
		return m, command
	case "p":
		if m.focus == focusWebsites {
			website, ok := m.selectedWebsite()
			if !ok || website.Type != domain.WebsitePHPFPM {
				m.status = "select a PHP-FPM domain to change its PHP version"
				break
			}
			m.phpPicker = phpPickerState{open: true, loading: true}
			m.status = "loading installed PHP-FPM versions…"
			m, command := m.startPHPVersions()
			return m, command
		}
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
		if m.focus == focusSubscriptions {
			m = m.askDeleteSubscription()
		} else {
			m.focus = focusDetail
		}
	case "o":
		m.focus = focusOutput
	case "e":
		if m.showWebsites {
			website, ok := m.selectedWebsite()
			if !ok {
				break
			}
			m = m.askConfirm(confirmState{
				action: "set-enabled", enabled: !website.Enabled, domain: website.PrimaryDomain,
				title: "Update domain", lines: []string{"Domain: " + website.PrimaryDomain, "Set enabled: " + map[bool]string{true: "yes", false: "no"}[!website.Enabled]},
			})
		}
	case "E":
		if m.focus == focusWebsites {
			m = m.openDocumentRootForm()
		}
	case "t":
		if m.focus == focusWebsites {
			website, ok := m.selectedWebsite()
			if !ok {
				break
			}
			enabled := !website.SSLEnabled
			lines := []string{"Domain: " + website.PrimaryDomain}
			if enabled {
				lines = append(lines, "Issue a certificate and redirect HTTP to HTTPS.", "Public DNS and HTTP reachability are required.")
			} else {
				lines = append(lines, "Disable TLS without deleting the certificate.")
			}
			m = m.askConfirm(confirmState{action: "set-tls", enabled: enabled, domain: website.PrimaryDomain, title: map[bool]string{true: "Enable TLS", false: "Disable TLS"}[enabled], lines: lines})
		}
	case "s":
		if m.focus == focusSubscriptions {
			subscription, ok := m.selectedSubscription()
			if !ok {
				break
			}
			if subscription.Status == "active" {
				m = m.askConfirm(confirmState{action: "suspended", domain: subscription.Name, title: "Suspend subscription", lines: []string{"Subscription: " + subscription.Name, "Websites will be unavailable until resumed."}})
			} else if subscription.Status == "suspended" {
				m = m.askConfirm(confirmState{action: "active", domain: subscription.Name, title: "Resume subscription", lines: []string{"Subscription: " + subscription.Name, "Restore normal service."}})
			}
		}
	case "a":
		if m.focus == focusSubscriptions {
			subscription, ok := m.selectedSubscription()
			if ok && subscription.Status != "archived" {
				m = m.askConfirm(confirmState{action: "archived", domain: subscription.Name, title: "Archive subscription", lines: []string{"Subscription: " + subscription.Name, "Archiving is required before permanent deletion."}})
			}
		}
	case "right", "tab":
		m.focus = focusRight(m.focus)
	case "left", "shift+tab":
		m.focus = focusLeft(m.focus)
	case "esc":
		if m.focus == focusSubscriptions && m.subscriptionFilter.query() != "" {
			m.subscriptionFilter.input.SetValue("")
			m.cursor = 0
			return m, nil
		}
		if m.focus == focusWebsites && m.websiteFilter.query() != "" {
			m.websiteFilter.input.SetValue("")
			m.websiteCursor = 0
			return m, nil
		}
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
		filter.input.Blur()
	case "enter":
		filter.active = false
		filter.input.Blur()
	default:
		input, command := filter.input.Update(msg)
		filter.input = input
		m.cursor = clamp(m.cursor, len(m.visibleSubscriptions()))
		m.websiteCursor = clamp(m.websiteCursor, len(m.visibleWebsites()))
		return m, command
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
	case ",":
		return m.openSettings(), nil
	case "q", "ctrl+c":
		return m, tea.Quit
	case "j", "down":
		m.cursor = clamp(m.cursor+1, len(m.visibleSubscriptions()))
	case "k", "up":
		m.cursor = clamp(m.cursor-1, len(m.visibleSubscriptions()))
	case "/":
		m.subscriptionFilter.active = true
		m.subscriptionFilter.input.Focus()
	case "?":
		m.help = helpState{open: true, filter: newFilter()}
		m.status = ""
	case "r":
		m.status = "loading subscriptions…"
		m, command := m.startSubscriptions()
		return m, command
	case "a":
		subscription, ok := m.selectedSubscription()
		if ok && subscription.Status != "archived" {
			m = m.askConfirm(confirmState{action: "archived", domain: subscription.Name, title: "Archive subscription", lines: []string{"Subscription: " + subscription.Name, "Archiving is required before permanent deletion."}})
		}
	case "d":
		m = m.askDeleteSubscription()
	case "enter":
		if len(m.visibleSubscriptions()) == 0 {
			return m, nil
		}
		m.workspace, m.showWebsites, m.focus, m.status = true, true, focusWebsites, "loading domains…"
		m, command := m.startWebsites()
		return m, command
	}
	return m, nil
}

func (m appModel) handlePHPPickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.phpPicker.loading {
		if msg.String() == "esc" || msg.String() == "q" {
			m.phpVersionsLoad.invalidate()
			m.phpPicker, m.status = phpPickerState{}, "cancelled"
		}
		return m, nil
	}
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q":
		m.phpPicker, m.status = phpPickerState{}, "cancelled"
	case "j", "down":
		m.phpPicker.cursor = clamp(m.phpPicker.cursor+1, len(m.phpPicker.items))
	case "k", "up":
		m.phpPicker.cursor = clamp(m.phpPicker.cursor-1, len(m.phpPicker.items))
	case "enter":
		if len(m.phpPicker.items) == 0 {
			return m, nil
		}
		version := m.phpPicker.items[m.phpPicker.cursor].Version
		subscription, subscriptionOK := m.selectedSubscription()
		website, websiteOK := m.selectedWebsite()
		if !subscriptionOK || !websiteOK || website.Type != domain.WebsitePHPFPM {
			m.phpPicker = phpPickerState{}
			return m, nil
		}
		if website.PHPVersion == version {
			m.phpPicker, m.status = phpPickerState{}, "domain already uses PHP-FPM "+version
			return m, nil
		}
		m.phpPicker = phpPickerState{}
		m = m.askConfirm(confirmState{action: "set-php", domain: version, title: "Switch PHP-FPM", lines: []string{
			"Subscription: " + subscription.Name,
			"Domain: " + website.PrimaryDomain,
			"PHP-FPM: " + valueOrDash(website.PHPVersion) + " → " + version,
			"Only this domain's PHP-FPM pool and vhost will be updated.",
		}})
	}
	return m, nil
}

func focusRight(current focus) focus {
	switch current {
	case focusSubscriptions:
		return focusWebsites
	case focusWebsites:
		return focusDetail
	case focusDetail:
		return focusLogs
	case focusLogs:
		return focusOutput
	default:
		return current
	}
}

func focusLeft(current focus) focus {
	switch current {
	case focusWebsites:
		return focusSubscriptions
	case focusDetail:
		return focusWebsites
	case focusLogs:
		return focusDetail
	case focusOutput:
		return focusLogs
	default:
		return current
	}
}
