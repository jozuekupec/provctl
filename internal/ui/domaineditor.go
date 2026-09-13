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
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "s":
		m.domainEditor = domainEditorState{}
		m.workspace, m.focus, m.status = false, focusSubscriptions, "subscription picker"
		return m, nil
	case "esc":
		m.domainEditor, m.status = domainEditorState{}, "domain editor closed"
		return m, nil
	case "shift+right":
		m.domainEditor.tab = (m.domainEditor.tab + 1) % len(domainEditorTabs)
		return m, nil
	case "shift+left":
		m.domainEditor.tab = (m.domainEditor.tab + len(domainEditorTabs) - 1) % len(domainEditorTabs)
		return m, nil
	case "?":
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
	switch m.domainEditor.tab {
	case 0:
		if msg.String() == "enter" || msg.String() == "e" {
			m = m.askConfirm(confirmState{action: "set-enabled", enabled: !website.Enabled, domain: website.PrimaryDomain, title: "Update domain", lines: []string{"Domain: " + website.PrimaryDomain, "Set enabled: " + map[bool]string{true: "yes", false: "no"}[!website.Enabled]}})
		}
	case 1:
		if msg.String() == "enter" || msg.String() == "e" {
			m = m.openDocumentRootForm()
		}
	case 2:
		if msg.String() == "enter" || msg.String() == "p" {
			if website.Type != domain.WebsitePHPFPM {
				m.status = "runtime settings apply only to PHP-FPM domains"
				return m, nil
			}
			m.phpPicker = phpPickerState{open: true, loading: true}
			m.status = "loading installed PHP-FPM versions…"
			m, command := m.startPHPVersions()
			return m, command
		}
	case 3:
		if msg.String() == "enter" || msg.String() == "t" {
			enabled := !website.SSLEnabled
			lines := []string{"Domain: " + website.PrimaryDomain}
			if enabled {
				lines = append(lines, "Issue a certificate and redirect HTTP to HTTPS.", "Public DNS and HTTP reachability are required.")
			} else {
				lines = append(lines, "Disable TLS without deleting the certificate.")
			}
			m = m.askConfirm(confirmState{action: "set-tls", enabled: enabled, domain: website.PrimaryDomain, title: map[bool]string{true: "Enable TLS", false: "Disable TLS"}[enabled], lines: lines})
		}
	case 4:
		switch msg.String() {
		case "a":
			m = m.openAliasForm(true)
		case "A":
			m = m.openAliasForm(false)
		case "enter", "e":
			m = m.openTargetForm()
		}
	case 5:
		switch msg.String() {
		case "enter", "l":
			m.status = "loading access log…"
			m, command := m.startWebsiteLogs(false)
			return m, command
		case "L":
			m.status = "loading error log…"
			m, command := m.startWebsiteLogs(true)
			return m, command
		}
	}
	return m, nil
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
