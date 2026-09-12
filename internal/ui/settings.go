package ui

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func (m appModel) openSettings() appModel {
	email := textinput.New()
	email.Prompt = ""
	email.SetValue(m.deps.SSLSettings.Email)
	email.Focus()
	m.settings = settingsState{open: true, email: email, staging: m.deps.SSLSettings.Staging}
	m.status = ""
	return m
}

func (m appModel) saveSettings(ctx context.Context) tea.Msg {
	if m.deps.SaveSSLSettings == nil {
		return settingsSavedMsg{err: context.Canceled}
	}
	return settingsSavedMsg{err: m.deps.SaveSSLSettings(ctx, strings.TrimSpace(m.settings.email.Value()), m.settings.staging)}
}

func (m appModel) saveSettingsCmd() tea.Cmd {
	return func() tea.Msg { return m.saveSettings(context.Background()) }
}

func (m appModel) handleSettingsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		m.settings, m.status = settingsState{}, "settings discarded"
		return m, nil
	case tea.KeyCtrlS, tea.KeyEnter:
		m.status = "saving settings…"
		return m, m.saveSettingsCmd()
	case tea.KeyTab, tea.KeyDown, tea.KeyUp:
		m.settings.field = 1 - m.settings.field
		if m.settings.field == 0 {
			m.settings.email.Focus()
		} else {
			m.settings.email.Blur()
		}
		return m, nil
	}
	if m.settings.field == 1 {
		switch msg.String() {
		case "left", "right", " ":
			m.settings.staging = !m.settings.staging
		}
		return m, nil
	}
	input, command := m.settings.email.Update(msg)
	m.settings.email = input
	return m, command
}

func (m appModel) settingsPopup() string {
	staging := "Production — public ACME certificates"
	if m.settings.staging {
		staging = "Staging — safe test certificates"
	}
	rows := []string{
		"  " + selectedStyle.Render("▸ ") + "ACME email: " + m.settings.email.View(),
		"  " + staging,
		"",
		dimStyle.Render("↑/↓ or tab = field · ←/→ = toggle · enter = save · esc = cancel"),
	}
	if m.settings.field == 1 {
		rows[0] = "  ACME email: " + m.settings.email.Value()
		rows[1] = "  " + selectedStyle.Render("▸ "+staging)
	}
	return panel("Settings · SSL", strings.Join(rows, "\n"), m.popupWidth(), len(rows)+2, true)
}
