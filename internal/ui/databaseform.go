package ui

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func (m appModel) openDatabaseCreateForm() appModel {
	if _, ok := m.selectedSubscription(); !ok {
		m.status = "select a subscription before creating a database"
		return m
	}
	name, credentials := textinput.New(), textinput.New()
	name.Prompt, credentials.Prompt = "", ""
	name.Focus()
	m.databaseCreateForm, m.status = databaseCreateFormState{open: true, name: name, creds: credentials}, ""
	return m
}

func (m appModel) focusDatabaseCreateField() appModel {
	m.databaseCreateForm.name.Blur()
	m.databaseCreateForm.creds.Blur()
	if m.databaseCreateForm.field == 0 {
		m.databaseCreateForm.name.Focus()
	} else {
		m.databaseCreateForm.creds.Focus()
	}
	return m
}

func (m appModel) handleDatabaseCreateFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.databaseCreateForm, m.status = databaseCreateFormState{}, "cancelled"
		return m, nil
	case "tab", "down", "shift+tab", "up":
		m.databaseCreateForm.field = 1 - m.databaseCreateForm.field
		return m.focusDatabaseCreateField(), nil
	case "ctrl+s":
		name, credentials := strings.TrimSpace(m.databaseCreateForm.name.Value()), strings.TrimSpace(m.databaseCreateForm.creds.Value())
		m.databaseCreateForm = databaseCreateFormState{}
		lines := []string{"Subscription: " + m.selectedSubscriptionName(), "Database suffix: " + name, "A generated password is shown exactly once after success."}
		if credentials != "" {
			lines = append(lines, "Write client credentials: "+credentials)
		}
		return m.askConfirm(confirmState{action: "create-database", domain: name, value: credentials, title: "Create database", lines: lines}), nil
	}
	if m.databaseCreateForm.field == 0 {
		input, command := m.databaseCreateForm.name.Update(msg)
		m.databaseCreateForm.name = input
		return m, command
	}
	input, command := m.databaseCreateForm.creds.Update(msg)
	m.databaseCreateForm.creds = input
	return m, command
}

func (m appModel) createDatabase(ctx context.Context, confirm confirmState, report func(int)) tea.Msg {
	subscription, ok := m.selectedSubscription()
	if !ok || m.deps.CreateDatabase == nil {
		return databaseCreatedMsg{err: context.Canceled, name: confirm.domain}
	}
	password, _, err := m.deps.CreateDatabase(ctx, subscription.Name, confirm.domain, confirm.value)
	if err != nil || m.deps.LoadDatabases == nil {
		return databaseCreatedMsg{err: err, name: confirm.domain}
	}
	report(1)
	items, err := m.deps.LoadDatabases(ctx, subscription.Name)
	return databaseCreatedMsg{err: err, name: subscription.Name + "_" + confirm.domain, password: password, items: items}
}

func (m appModel) createDatabaseCmd(confirm confirmState) tea.Cmd {
	return steppedCmd("Create database", []string{"create MariaDB database and user", "refresh database list"}, func(ctx context.Context, report func(int)) tea.Msg {
		return m.createDatabase(ctx, confirm, report)
	})
}

func (m appModel) databaseCreateFormPopup() string {
	body := []string{
		"Database suffix:  " + m.databaseCreateForm.name.View(),
		"Credentials file: " + m.databaseCreateForm.creds.View(), "",
		dimStyle.Render("The database name is <subscription>_<suffix>."),
		dimStyle.Render("Credentials file is optional and must be inside the subscription home."),
	}
	return popupBox(m, popupOpts{Size: popupMedium, Fit: fitAuto, Title: "Create database", Body: body, Footer: []string{dimStyle.Render("tab field · ctrl+s continue · esc cancel")}})
}
