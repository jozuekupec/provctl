package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"provctl/internal/domain"
	"provctl/internal/fsbrowse"
	"provctl/internal/service"
)

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height, m.ready = msg.Width, msg.Height, true
	case tea.KeyMsg:
		return m.handleKey(msg)
	case pathListedMsg:
		if !m.pathPicker.open || msg.generation != m.pathPicker.generation {
			return m, nil
		}
		m.pathPicker.loading = false
		if msg.err != nil {
			m.pathPicker.err = msg.err.Error()
			return m, nil
		}
		entries := make([]pathEntry, 0, len(msg.entries)+1)
		if _, hasParent := fsbrowse.Parent(msg.dir); hasParent {
			entries = append(entries, pathEntry{Entry: fsbrowse.Entry{Name: "..", Dir: true}, parent: true})
		}
		for _, entry := range msg.entries {
			entries = append(entries, pathEntry{Entry: entry})
		}
		m.pathPicker.dir, m.pathPicker.entries = msg.dir, entries
		m.pathPicker.cursor = clamp(m.pathPicker.cursor, len(m.visiblePathEntries()))
	case settingsSavedMsg:
		if msg.err != nil {
			m.status = "settings save failed: " + msg.err.Error()
			return m, nil
		}
		m.deps.Config = msg.cfg
		m.settings, m.status = settingsState{}, "settings saved; restart TUI to apply non-TLS settings"
		m.output = m.output.append("configuration saved")
		return m, nil
	case progressStartMsg:
		m.progress = progressState{title: msg.title, steps: append([]progressStep(nil), msg.steps...), active: true, ch: msg.ch, cancel: msg.cancel}
		// The progress overlay is modal; changing focus beneath it would make the
		// user lose the panel from which the operation was started.
		m.confirm, m.status = confirmState{}, "applying change…"
		return m, waitProgress(msg.ch)
	case progressStepMsg:
		if msg.index >= 0 && msg.index < len(m.progress.steps) {
			m.progress.steps[msg.index].state = msg.state
		}
		return m, waitProgress(m.progress.ch)
	case subscriptionsLoadedMsg:
		if m.subscriptions.stale(msg.generation) {
			return m, nil
		}
		m.subscriptions.finish(msg.generation)
		if msg.err != nil {
			m.status = "load failed: " + msg.err.Error()
			m.output = m.output.append(m.status)
			return m, nil
		}
		selectedID := m.selectedSubscriptionID()
		m.items = append([]domain.Subscription(nil), msg.items...)
		m.cursor = clamp(m.cursor, len(m.visibleSubscriptions()))
		if m.selectedSubscriptionID() != selectedID {
			m = m.clearSelectionDetails()
		}
		m.status = "r refresh • enter websites • d detail • o output • q quit"
		m.output = m.output.append("subscriptions refreshed")
	case websitesLoadedMsg:
		if m.websitesLoad.stale(msg.generation) {
			return m, nil
		}
		m.websitesLoad.finish(msg.generation)
		if msg.err != nil {
			m.status = "website load failed: " + msg.err.Error()
			m.output = m.output.append(m.status)
			return m, nil
		}
		m.websites, m.websiteCursor, m.status = append([]domain.Website(nil), msg.items...), 0, "domains loaded"
		m.output = m.output.append("websites loaded")
	case databasesLoadedMsg:
		if m.databasesLoad.stale(msg.generation) {
			return m, nil
		}
		m.databasesLoad.finish(msg.generation)
		if msg.err != nil {
			m.status = "database load failed: " + msg.err.Error()
			m.output = m.output.append(m.status)
			return m, nil
		}
		m.databases, m.detailView, m.focus, m.status = append([]domain.Database(nil), msg.items...), detailDatabases, focusDetail, "databases loaded"
	case sshKeysLoadedMsg:
		if m.sshKeysLoad.stale(msg.generation) {
			return m, nil
		}
		m.sshKeysLoad.finish(msg.generation)
		if msg.err != nil {
			m.status = "SSH key load failed: " + msg.err.Error()
			m.output = m.output.append(m.status)
			return m, nil
		}
		m.sshKeys, m.detailView, m.focus, m.status = append([]domain.SSHKey(nil), msg.items...), detailSSHKeys, focusDetail, "SSH keys loaded"
	case websiteChangedMsg:
		m.progress.active = false
		if msg.err != nil {
			m.status = "website change failed: " + msg.err.Error()
			m.output = m.output.append(m.status)
			return m, nil
		}
		m.confirm = confirmState{}
		m.status = "website " + msg.domain + " updated; refreshing…"
		m.output = m.output.append("website " + msg.domain + " enabled=" + fmt.Sprint(msg.enabled))
		m, command := m.startWebsites()
		return m, command
	case websiteTLSChangedMsg:
		m.progress.active = false
		if msg.err != nil {
			m.status = "TLS change failed: " + msg.err.Error()
			m.output = m.output.append(m.status)
			return m, nil
		}
		m.status = "TLS for " + msg.domain + " updated; refreshing…"
		m.output = m.output.append("TLS for " + msg.domain + " enabled=" + fmt.Sprint(msg.enabled))
		m, command := m.startWebsites()
		return m, command
	case websiteDocumentRootChangedMsg:
		m.progress.active = false
		if msg.err != nil {
			m.status = "document root change failed: " + msg.err.Error()
			m.output = m.output.append(m.status)
			return m, nil
		}
		m.documentRootForm = documentRootFormState{}
		m.status = "document root for " + msg.domain + " updated"
		m.output = m.output.append("document root updated: " + msg.root)
		if msg.items != nil {
			m.websites = append([]domain.Website(nil), msg.items...)
		}
		return m, nil
	case websiteCreatedMsg:
		m.progress.active = false
		if msg.err != nil {
			m.status = "domain creation failed: " + msg.err.Error()
			m.output = m.output.append(m.status)
			return m, nil
		}
		m.status = "domain " + msg.domain + " created"
		m.output = m.output.append("domain created: " + msg.domain)
		if msg.items != nil {
			m.websites = append([]domain.Website(nil), msg.items...)
			m.websiteCursor = clamp(m.websiteCursor, len(m.visibleWebsites()))
		}
		return m, nil
	case websiteAliasChangedMsg:
		m.progress.active = false
		if msg.err != nil {
			m.status = "alias change failed: " + msg.err.Error()
			m.output = m.output.append(m.status)
			return m, nil
		}
		verb := map[bool]string{true: "added", false: "removed"}[msg.add]
		m.status = "alias " + msg.alias + " " + verb
		m.output = m.output.append("alias " + verb + ": " + msg.alias)
		if msg.items != nil {
			m.websites = append([]domain.Website(nil), msg.items...)
		}
		return m, nil
	case websiteTargetChangedMsg:
		m.progress.active = false
		if msg.err != nil {
			m.status = "target change failed: " + msg.err.Error()
			return m, nil
		}
		m.status = "target for " + msg.domain + " updated"
		if msg.items != nil {
			m.websites = append([]domain.Website(nil), msg.items...)
		}
		return m, nil
	case websiteDeletedMsg:
		m.progress.active = false
		if msg.err != nil {
			m.status = "domain deletion failed: " + msg.err.Error()
			m.output = m.output.append(m.status)
			return m, nil
		}
		m.status = "domain " + msg.domain + " deleted; site data and logs retained"
		m.output = m.output.append("domain configuration deleted: " + msg.domain)
		m.websites = append([]domain.Website(nil), msg.items...)
		m.websiteCursor = clamp(m.websiteCursor, len(m.visibleWebsites()))
		return m, nil
	case subscriptionChangedMsg:
		m.progress.active = false
		if msg.err != nil {
			m.status = "subscription change failed: " + msg.err.Error()
			m.output = m.output.append(m.status)
			return m, nil
		}
		m.confirm, m.status = confirmState{}, "subscription "+msg.name+" updated; refreshing…"
		m.output = m.output.append("subscription " + msg.name + " status=" + msg.status)
		m, command := m.startSubscriptions()
		return m, command
	case subscriptionDeletedMsg:
		m.progress.active = false
		if msg.err != nil {
			m.status = "subscription deletion failed: " + msg.err.Error()
			m.output = m.output.append(m.status)
			return m, nil
		}
		m = m.clearSelectionDetails()
		m.workspace, m.focus = false, focusSubscriptions
		m.status = "subscription " + msg.name + " deleted; refreshing…"
		m.output = m.output.append("subscription " + msg.name + " deleted")
		m, command := m.startSubscriptions()
		return m, command
	case subscriptionCreatedMsg:
		m.progress.active = false
		if msg.err != nil {
			m.status = "subscription creation failed: " + msg.err.Error()
			m.output = m.output.append(m.status)
			return m, nil
		}
		m.status = "subscription " + msg.name + " created; refreshing…"
		m.output = m.output.append("subscription created: " + msg.name)
		m, command := m.startSubscriptions()
		return m, command
	case reconcileFinishedMsg:
		m.progress.active = false
		if msg.err != nil {
			m.status = "reconcile failed: " + msg.err.Error()
			m.output = m.output.append(m.status)
			return m, nil
		}
		if msg.operationID == 0 {
			m.status = "nothing to reconcile"
			m.output = m.output.append("reconcile: no generated drift")
		} else {
			m.status = "reconciled generated configuration"
			m.output = m.output.append("reconciled subscription " + msg.subscription)
		}
		if msg.websites != nil {
			m.websites = append([]domain.Website(nil), msg.websites...)
			m.websiteCursor = clamp(m.websiteCursor, len(m.visibleWebsites()))
		}
		return m, nil
	case phpVersionsLoadedMsg:
		if m.phpVersionsLoad.stale(msg.generation) {
			return m, nil
		}
		m.phpVersionsLoad.finish(msg.generation)
		m.phpPicker.loading = false
		if msg.err != nil {
			m.phpPicker.open = false
			m.status = "PHP version load failed: " + msg.err.Error()
			m.output = m.output.append(m.status)
			return m, nil
		}
		m.phpPicker.items = append([]service.PHPFPMVersion(nil), msg.items...)
		m.phpPicker.cursor = clamp(m.phpPicker.cursor, len(m.phpPicker.items))
		if website, ok := m.selectedWebsite(); ok {
			for index, item := range m.phpPicker.items {
				if item.Version == website.PHPVersion {
					m.phpPicker.cursor = index
					break
				}
			}
		}
		if len(m.phpPicker.items) == 0 {
			m.status = "no installed PHP-FPM versions found"
		}
		return m, nil
	case websitePHPChangedMsg:
		m.progress.active = false
		if msg.err != nil {
			m.status = "PHP version change failed: " + msg.err.Error()
			m.output = m.output.append(m.status)
			return m, nil
		}
		m.status = "domain " + msg.domain + " now uses PHP-FPM " + msg.version + "; refreshing…"
		m.output = m.output.append("domain " + msg.subscription + "/" + msg.domain + " PHP-FPM=" + msg.version)
		if msg.items != nil {
			m.websites = append([]domain.Website(nil), msg.items...)
			m.websiteCursor = clamp(m.websiteCursor, len(m.visibleWebsites()))
			m.status = "domain " + msg.domain + " now uses PHP-FPM " + msg.version
			return m, nil
		}
		m, command := m.startWebsites()
		return m, command
	case healthLoadedMsg:
		if m.healthLoad.stale(msg.generation) {
			return m, nil
		}
		m.healthLoad.finish(msg.generation)
		if msg.err != nil {
			m.status = "health failed: " + msg.err.Error()
			m.output = m.output.append(m.status)
			return m, nil
		}
		for _, check := range msg.checks {
			m.output = m.output.append(string(check.Status) + " " + check.Name + ": " + check.Detail)
		}
		m.focus, m.status = focusOutput, "health checks completed"
	case websiteLogsLoadedMsg:
		if m.logsLoad.stale(msg.generation) {
			return m, nil
		}
		m.logsLoad.finish(msg.generation)
		if msg.err != nil {
			m.status = "log load failed: " + msg.err.Error()
			m.output = m.output.append(m.status)
			return m, nil
		}
		m.focus, m.status = focusLogs, "website log loaded"
		m.output = m.output.append("website log loaded")
		m.logs = outputState{}
		for _, line := range strings.Split(strings.TrimSuffix(msg.contents, "\n"), "\n") {
			if line != "" {
				m.logs = m.logs.append(line)
			}
		}
	}
	return m, nil
}

func Program(deps Deps) *tea.Program { return tea.NewProgram(New(deps), tea.WithAltScreen()) }
