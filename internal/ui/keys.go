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
	if m.websiteCreateForm.open {
		return m.handleWebsiteCreateFormKey(msg)
	}
	if m.aliasForm.open {
		return m.handleAliasFormKey(msg)
	}
	if m.targetForm.open {
		return m.handleTargetFormKey(msg)
	}
	if m.subscriptionCreateForm.open {
		return m.handleSubscriptionCreateFormKey(msg)
	}
	if m.databaseCreateForm.open {
		return m.handleDatabaseCreateFormKey(msg)
	}
	if m.sshAccessForm.open {
		return m.handleSSHAccessFormKey(msg)
	}
	if m.sshKeyForm.open {
		return m.handleSSHKeyFormKey(msg)
	}
	if m.secret.open {
		return m.handleSecretKey(msg)
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
	if m.domainEditor.open {
		return m.handleDomainEditorKey(msg)
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
	switch actionFor(m.shortcutContext(), msg.String()) {
	case actionSettings:
		return m.openSettings(), nil
	case actionHelp:
		m.help = helpState{open: true, filter: newFilter()}
		m.status = ""
		return m, nil
	case actionQuit:
		return m, tea.Quit
	case actionMoveNext:
		if m.focus == focusWebsites {
			next := clamp(m.websiteCursor+1, len(m.visibleWebsites()))
			if next != m.websiteCursor {
				m.websiteCursor = next
				m.detailView = detailDomain
				m.logsLoad.invalidate()
			}
		} else if m.focus == focusOutput {
			m.outputScroll++
		}
	case actionMovePrevious:
		if m.focus == focusWebsites {
			next := clamp(m.websiteCursor-1, len(m.visibleWebsites()))
			if next != m.websiteCursor {
				m.websiteCursor = next
				m.detailView = detailDomain
				m.logsLoad.invalidate()
			}
		} else if m.focus == focusOutput {
			m.outputScroll = max(0, m.outputScroll-1)
		}
	case actionRefresh:
		m.status = "loading subscriptions…"
		m, command := m.startSubscriptions()
		return m, command
	case actionFilter:
		m.websiteFilter.active = true
		m.websiteFilter.input.Focus()
	case actionHealth:
		m.status = "running health checks…"
		m, command := m.startHealth()
		return m, command
	case actionReconcile:
		subscription, ok := m.selectedSubscription()
		if ok {
			m = m.askConfirm(confirmState{action: "reconcile", domain: subscription.Name, title: "Reconcile configuration", lines: []string{
				"Subscription: " + subscription.Name,
				"Regenerate managed Apache configuration from provctl state.",
			}})
		}
	case actionOpen:
		m = m.openDomainEditor()
	case actionCreate:
		m = m.openWebsiteCreateForm()
	case actionPicker:
		m.workspace, m.focus, m.status = false, focusSubscriptions, "subscription picker"
	case actionPanelNext:
		m.focus = focusRight(m.focus)
	case actionPanelPrevious:
		m.focus = focusLeft(m.focus)
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
	switch actionFor(shortcutPicker, msg.String()) {
	case actionSettings:
		return m.openSettings(), nil
	case actionQuit:
		return m, tea.Quit
	case actionMoveNext:
		next := clamp(m.cursor+1, len(m.visibleSubscriptions()))
		if next != m.cursor {
			m.cursor = next
			m = m.clearSelectionDetails()
		}
	case actionMovePrevious:
		next := clamp(m.cursor-1, len(m.visibleSubscriptions()))
		if next != m.cursor {
			m.cursor = next
			m = m.clearSelectionDetails()
		}
	case actionFilter:
		m.subscriptionFilter.active = true
		m.subscriptionFilter.input.Focus()
	case actionHelp:
		m.help = helpState{open: true, filter: newFilter()}
		m.status = ""
	case actionRefresh:
		m.status = "loading subscriptions…"
		m, command := m.startSubscriptions()
		return m, command
	case actionArchive:
		subscription, ok := m.selectedSubscription()
		if ok && subscription.Status != "archived" {
			m = m.askConfirm(confirmState{action: "archived", domain: subscription.Name, title: "Archive subscription", lines: []string{"Subscription: " + subscription.Name, "Archiving is required before permanent deletion."}})
		}
	case actionDelete:
		m = m.askDeleteSubscription()
	case actionSSHAccess:
		m = m.openSSHAccessForm()
	case actionCreate:
		m = m.openSubscriptionCreateForm()
	case actionOpen:
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
