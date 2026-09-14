package ui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"provctl/internal/domain"
)

var subscriptionAdminTabs = []string{"Overview", "Databases", "SSH", "Cron", "Backups"}

func (m appModel) openSubscriptionAdmin() (appModel, tea.Cmd) {
	if _, ok := m.selectedSubscription(); !ok {
		m.status = "select a subscription to manage"
		return m, nil
	}
	m.subscriptionAdmin = subscriptionAdminState{open: true, tab: 1}
	m.status = "loading databases…"
	return m.startDatabases()
}

func (m appModel) handleSubscriptionAdminKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	action := actionFor(m.subscriptionAdminContext(), msg.String())
	switch action {
	case actionQuit:
		return m, tea.Quit
	case actionPicker:
		m.subscriptionAdmin = subscriptionAdminState{}
		m.workspace, m.focus, m.status = false, focusSubscriptions, "subscription picker"
		return m, nil
	case actionAdminClose:
		m.subscriptionAdmin, m.status = subscriptionAdminState{}, "subscription administration closed"
		return m, nil
	case actionAdminTabNext:
		m.subscriptionAdmin.tab = (m.subscriptionAdmin.tab + 1) % len(subscriptionAdminTabs)
		return m.startSubscriptionAdminTab()
	case actionAdminTabPrev:
		m.subscriptionAdmin.tab = (m.subscriptionAdmin.tab + len(subscriptionAdminTabs) - 1) % len(subscriptionAdminTabs)
		return m.startSubscriptionAdminTab()
	case actionMoveNext:
		if m.subscriptionAdmin.tab == 1 {
			m.databaseCursor = clamp(m.databaseCursor+1, len(m.databases))
		} else if m.subscriptionAdmin.tab == 2 {
			m.sshKeyCursor = clamp(m.sshKeyCursor+1, len(m.sshKeys))
		}
	case actionMovePrevious:
		if m.subscriptionAdmin.tab == 1 {
			m.databaseCursor = clamp(m.databaseCursor-1, len(m.databases))
		} else if m.subscriptionAdmin.tab == 2 {
			m.sshKeyCursor = clamp(m.sshKeyCursor-1, len(m.sshKeys))
		}
	case actionCreate:
		if m.subscriptionAdmin.tab == 1 {
			return m.openDatabaseCreateForm(), nil
		} else if m.subscriptionAdmin.tab == 2 {
			return m.openSSHKeyForm(), nil
		}
	case actionRotateSecret:
		if database, ok := m.selectedDatabase(); ok {
			m = m.askConfirm(confirmState{action: "rotate-database-password", domain: database.Name, title: "Rotate database password", lines: []string{"Database: " + database.Name, "The current password will stop working immediately.", "The new password is shown exactly once."}})
		}
	case actionDelete:
		if database, ok := m.selectedDatabase(); ok {
			m = m.askConfirm(confirmState{action: "delete-database", domain: database.Name, word: database.Name, title: "Delete database", lines: []string{"Database: " + database.Name, "This permanently removes the MariaDB database and local user."}})
		} else if key, ok := m.selectedSSHKey(); ok {
			m = m.askConfirm(confirmState{action: "remove-ssh-key", domain: key.Fingerprint, word: key.Fingerprint, title: "Remove SSH key", lines: []string{"Fingerprint: " + key.Fingerprint, "This removes the public key from the subscription account."}})
		}
	case actionHelp:
		m.help = helpState{open: true, filter: newFilter()}
	}
	return m, nil
}

func (m appModel) subscriptionAdminContext() shortcutContext {
	return shortcutAdminOverview + shortcutContext(m.subscriptionAdmin.tab)
}

func (m appModel) renderSubscriptionAdmin() string {
	subscription, ok := m.selectedSubscription()
	if !ok {
		return m.renderFrame(panel("Subscription administration", "No subscription selected.", m.width, m.height-2, true))
	}
	body := strings.Join([]string{m.subscriptionAdminTabStrip(), "", m.subscriptionAdminBody(subscription)}, "\n")
	return m.renderFrame(panel("Manage subscription · "+subscription.Name, body, m.width, m.height-2, true))
}

func (m appModel) subscriptionAdminTabStrip() string {
	parts := make([]string, len(subscriptionAdminTabs))
	for index, label := range subscriptionAdminTabs {
		if index == m.subscriptionAdmin.tab {
			parts[index] = tabActiveStyle.Render(label)
		} else {
			parts[index] = tabInactiveStyle.Render(label)
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Bottom, parts...)
}

func (m appModel) subscriptionAdminBody(subscription domain.Subscription) string {
	switch m.subscriptionAdmin.tab {
	case 0:
		return fmt.Sprintf("Status: %s\nUser: %s\nHome: %s\nSSH: %s\n\nQuotas\nDisk: %s\nWebsites: %s\nDatabases: %s\nBackups: %s", subscription.Status, subscription.UnixUser, valueOrDash(subscription.Home), valueOrDash(subscription.SSHAccess), quotaSize(subscription.QuotaDiskBytes), quotaCount(subscription.QuotaWebsites), quotaCount(subscription.QuotaDatabases), quotaCount(subscription.QuotaBackups))
	case 1:
		if len(m.databases) == 0 {
			return "No databases.\n\nn  create a database"
		}
		lines := []string{"↑/↓  select database", ""}
		for index, database := range m.databases {
			line := fmt.Sprintf("  %s · %s@%s · %s", database.Name, database.User, database.Host, database.Charset)
			if index == m.databaseCursor {
				line = selectedStyle.Render("> " + strings.TrimPrefix(line, "  "))
			}
			lines = append(lines, line)
		}
		return strings.Join(lines, "\n")
	case 2:
		if len(m.sshKeys) == 0 {
			return "No SSH keys.\n\nn  add a public key"
		}
		lines := []string{"↑/↓  select SSH key", ""}
		for index, key := range m.sshKeys {
			line := "  " + key.Fingerprint
			if key.Comment != "" {
				line += " · " + key.Comment
			}
			if index == m.sshKeyCursor {
				line = selectedStyle.Render("> " + strings.TrimPrefix(line, "  "))
			}
			lines = append(lines, line)
		}
		return strings.Join(lines, "\n")
	case 3:
		return "Cron job management will move here in the next administration slice."
	default:
		return "Backup creation, inspection and restore will move here after their destructive workflow is designed."
	}
}

func (m appModel) selectedDatabase() (domain.Database, bool) {
	if m.databaseCursor < 0 || m.databaseCursor >= len(m.databases) {
		return domain.Database{}, false
	}
	return m.databases[m.databaseCursor], true
}

func (m appModel) loadDatabases(ctx context.Context, generation uint64) tea.Msg {
	subscription, ok := m.selectedSubscription()
	if !ok || m.deps.LoadDatabases == nil {
		return databasesLoadedMsg{err: context.Canceled, generation: generation}
	}
	items, err := m.deps.LoadDatabases(ctx, subscription.Name)
	return databasesLoadedMsg{items: items, err: err, generation: generation}
}

func (m appModel) startDatabases() (appModel, tea.Cmd) {
	ctx, generation := m.databasesLoad.start()
	return m, func() tea.Msg { return m.loadDatabases(ctx, generation) }
}

func (m appModel) startSubscriptionAdminTab() (appModel, tea.Cmd) {
	switch m.subscriptionAdmin.tab {
	case 1:
		m.status = "loading databases…"
		return m.startDatabases()
	case 2:
		m.status = "loading SSH keys…"
		return m.startSSHKeys()
	default:
		return m, nil
	}
}

func (m appModel) loadSSHKeys(ctx context.Context, generation uint64) tea.Msg {
	subscription, ok := m.selectedSubscription()
	if !ok || m.deps.LoadSSHKeys == nil {
		return sshKeysLoadedMsg{err: context.Canceled, generation: generation}
	}
	items, err := m.deps.LoadSSHKeys(ctx, subscription.Name)
	return sshKeysLoadedMsg{items: items, err: err, generation: generation}
}

func (m appModel) startSSHKeys() (appModel, tea.Cmd) {
	ctx, generation := m.sshKeysLoad.start()
	return m, func() tea.Msg { return m.loadSSHKeys(ctx, generation) }
}

func (m appModel) selectedSSHKey() (domain.SSHKey, bool) {
	if m.sshKeyCursor < 0 || m.sshKeyCursor >= len(m.sshKeys) {
		return domain.SSHKey{}, false
	}
	return m.sshKeys[m.sshKeyCursor], true
}

func (m appModel) removeSSHKey(ctx context.Context, confirm confirmState, report func(int)) tea.Msg {
	subscription, ok := m.selectedSubscription()
	if !ok || m.deps.RemoveSSHKey == nil {
		return sshKeyRemovedMsg{err: context.Canceled}
	}
	_, err := m.deps.RemoveSSHKey(ctx, subscription.Name, confirm.domain)
	if err != nil || m.deps.LoadSSHKeys == nil {
		return sshKeyRemovedMsg{err: err, fingerprint: confirm.domain}
	}
	report(1)
	items, err := m.deps.LoadSSHKeys(ctx, subscription.Name)
	return sshKeyRemovedMsg{err: err, fingerprint: confirm.domain, items: items}
}

func (m appModel) removeSSHKeyCmd(confirm confirmState) tea.Cmd {
	return steppedCmd("Remove SSH key", []string{"rewrite authorized keys", "refresh SSH key list"}, func(ctx context.Context, report func(int)) tea.Msg { return m.removeSSHKey(ctx, confirm, report) })
}

func (m appModel) changeDatabasePassword(ctx context.Context, confirm confirmState) tea.Msg {
	subscription, ok := m.selectedSubscription()
	if !ok || m.deps.ChangeDatabasePassword == nil {
		return databasePasswordChangedMsg{err: context.Canceled}
	}
	password, _, err := m.deps.ChangeDatabasePassword(ctx, subscription.Name, confirm.domain)
	return databasePasswordChangedMsg{err: err, name: confirm.domain, password: password}
}

func (m appModel) changeDatabasePasswordCmd(confirm confirmState) tea.Cmd {
	return steppedCmd("Rotate database password", []string{"replace MariaDB password"}, func(ctx context.Context, _ func(int)) tea.Msg { return m.changeDatabasePassword(ctx, confirm) })
}

func (m appModel) deleteDatabase(ctx context.Context, confirm confirmState, report func(int)) tea.Msg {
	subscription, ok := m.selectedSubscription()
	if !ok || m.deps.DeleteDatabase == nil {
		return databaseDeletedMsg{err: context.Canceled}
	}
	_, err := m.deps.DeleteDatabase(ctx, subscription.Name, confirm.domain)
	if err != nil || m.deps.LoadDatabases == nil {
		return databaseDeletedMsg{err: err, name: confirm.domain}
	}
	report(1)
	items, err := m.deps.LoadDatabases(ctx, subscription.Name)
	return databaseDeletedMsg{err: err, name: confirm.domain, items: items}
}

func (m appModel) deleteDatabaseCmd(confirm confirmState) tea.Cmd {
	return steppedCmd("Delete database", []string{"remove MariaDB database and local user", "refresh database list"}, func(ctx context.Context, report func(int)) tea.Msg { return m.deleteDatabase(ctx, confirm, report) })
}
