package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// askConfirm makes confirmation a modal state: no background action can see a
// keystroke while the user is deciding.
func (m appModel) askConfirm(confirm confirmState) appModel {
	m.confirm = confirm
	m.status = ""
	return m
}

func (m appModel) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "y":
		confirm := m.confirm
		m.confirm, m.status = confirmState{}, "applying change…"
		if confirm.action == "active" || confirm.action == "suspended" {
			return m, m.changeSubscriptionCmd(confirm)
		}
		return m, m.changeWebsiteCmd(confirm)
	default:
		m.confirm, m.status = confirmState{}, "cancelled"
		return m, nil
	}
}

func (m appModel) confirmPopup() string {
	lines := append([]string{}, m.confirm.lines...)
	lines = append(lines, "", confirmStyle.Render("Confirm: y = yes · any other key = cancel"))
	return panel(m.confirm.title, strings.Join(lines, "\n"), m.popupWidth(), len(lines)+2, true)
}
