package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// askConfirm makes confirmation a modal state: no background action can see a
// keystroke while the user is deciding.
func (m appModel) askConfirm(confirm confirmState) appModel {
	if confirm.word != "" {
		input := textinput.New()
		input.Prompt = ""
		input.Focus()
		confirm.input = input
	}
	m.confirm = confirm
	m.status = ""
	return m
}

func (m appModel) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "n", "q":
		m.confirm, m.status = confirmState{}, "cancelled"
		return m, nil
	}
	if m.confirm.word != "" {
		if msg.String() == "enter" {
			if m.confirm.input.Value() != m.confirm.word {
				m.confirm.err = "type " + m.confirm.word + " to confirm"
				return m, nil
			}
			return m.runConfirmed()
		}
		input, command := m.confirm.input.Update(msg)
		m.confirm.input = input
		return m, command
	}
	if msg.String() == "y" {
		return m.runConfirmed()
	}
	m.confirm, m.status = confirmState{}, "cancelled"
	return m, nil
}

func (m appModel) runConfirmed() (tea.Model, tea.Cmd) {
	confirm := m.confirm
	m.confirm, m.status = confirmState{}, "applying change…"
	switch confirm.action {
	case "active", "suspended", "archived":
		return m, m.changeSubscriptionCmd(confirm)
	case "delete":
		return m, m.deleteSubscriptionCmd(confirm)
	case "set-tls":
		return m, m.changeWebsiteTLSCmd(confirm)
	default:
		return m, m.changeWebsiteCmd(confirm)
	}
}

func (m appModel) confirmPopup() string {
	lines := append([]string{}, m.confirm.lines...)
	if m.confirm.word != "" {
		lines = append(lines, "", confirmStyle.Render("Type "+m.confirm.word+" to confirm:"), m.confirm.input.View())
		if m.confirm.err != "" {
			lines = append(lines, confirmStyle.Render(m.confirm.err))
		}
		lines = append(lines, "", confirmStyle.Render("Enter confirms · esc cancels"))
	} else {
		lines = append(lines, "", confirmStyle.Render("Confirm: y = yes · any other key = cancel"))
	}
	return panel(m.confirm.title, strings.Join(lines, "\n"), m.popupWidth(), len(lines)+2, true)
}
