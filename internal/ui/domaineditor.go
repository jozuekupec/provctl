package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"provctl/internal/domain"
)

var domainEditorTabs = []string{"Overview", "Content", "Runtime", "TLS", "Routing", "Logs"}

func (m appModel) openDomainEditor() appModel {
	if _, ok := m.selectedWebsite(); !ok {
		m.status = "select a domain to edit"
		return m
	}
	m.domainEditor = domainEditorState{open: true}
	m.status = ""
	return m
}

func (m appModel) handleDomainEditorKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	action := actionFor(m.domainEditorContext(), msg.String())
	switch action {
	case actionQuit:
		return m, tea.Quit
	case actionPicker:
		m.domainEditor = domainEditorState{}
		m.workspace, m.focus, m.status = false, focusSubscriptions, "subscription picker"
		return m, nil
	case actionEditorClose:
		m.domainEditor, m.status = domainEditorState{}, "domain editor closed"
		return m, nil
	case actionEditorTabNext:
		m.domainEditor.tab = (m.domainEditor.tab + 1) % len(domainEditorTabs)
		return m, nil
	case actionEditorTabPrev:
		m.domainEditor.tab = (m.domainEditor.tab + len(domainEditorTabs) - 1) % len(domainEditorTabs)
		return m, nil
	case actionHelp:
		m.help = helpState{open: true, filter: newFilter()}
		return m, nil
	}
	return m.domainEditorAction(msg)
}

func (m appModel) domainEditorAction(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	website, ok := m.selectedWebsite()
	if !ok {
		return m, nil
	}
	switch actionFor(m.domainEditorContext(), msg.String()) {
	case actionToggleEnabled:
		m = m.askConfirm(confirmState{action: "set-enabled", enabled: !website.Enabled, domain: website.PrimaryDomain, title: "Update domain", lines: []string{"Domain: " + website.PrimaryDomain, "Set enabled: " + map[bool]string{true: "yes", false: "no"}[!website.Enabled]}})
	case actionDelete:
		m = m.askDeleteWebsite()
	case actionDocumentRoot:
		m = m.openDocumentRootForm()
	case actionPHPVersion:
		if website.Type != domain.WebsitePHPFPM {
			m.status = "runtime settings apply only to PHP-FPM domains"
			return m, nil
		}
		m.phpPicker = phpPickerState{open: true, loading: true}
		m.status = "loading installed PHP-FPM versions…"
		m, command := m.startPHPVersions()
		return m, command
	case actionToggleTLS:
		enabled := !website.SSLEnabled
		lines := []string{"Domain: " + website.PrimaryDomain}
		if enabled {
			lines = append(lines, "Issue a certificate and redirect HTTP to HTTPS.", "Public DNS and HTTP reachability are required.")
		} else {
			lines = append(lines, "Disable TLS without deleting the certificate.")
		}
		m = m.askConfirm(confirmState{action: "set-tls", enabled: enabled, domain: website.PrimaryDomain, title: map[bool]string{true: "Enable TLS", false: "Disable TLS"}[enabled], lines: lines})
	case actionAddAlias:
		m = m.openAliasForm(true)
	case actionRemoveAlias:
		m = m.openAliasForm(false)
	case actionTarget:
		m = m.openTargetForm()
	case actionAccessLog:
		m.status = "loading access log…"
		m, command := m.startWebsiteLogs(false)
		return m, command
	case actionErrorLog:
		m.status = "loading error log…"
		m, command := m.startWebsiteLogs(true)
		return m, command
	case actionLogDirectory:
		m = m.openLogDirectoryForm()
	}
	return m, nil
}

func (m appModel) domainEditorContext() shortcutContext {
	return shortcutEditorOverview + shortcutContext(m.domainEditor.tab)
}

func (m appModel) domainEditorTabs() string {
	parts := make([]string, len(domainEditorTabs))
	for index, label := range domainEditorTabs {
		if index == m.domainEditor.tab {
			parts[index] = tabActiveStyle.Render(label)
		} else {
			parts[index] = tabInactiveStyle.Render(label)
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Bottom, parts...)
}
