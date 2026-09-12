package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"provctl/internal/domain"
)

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height, m.ready = msg.Width, msg.Height, true
	case tea.KeyMsg:
		return m.handleKey(msg)
	case progressStartMsg:
		m.progress = progressState{title: msg.title, steps: append([]progressStep(nil), msg.steps...), active: true, ch: msg.ch, cancel: msg.cancel}
		m.confirm, m.focus, m.status = confirmState{}, focusOutput, "applying change…"
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
		m.items, m.cursor = append([]domain.Subscription(nil), msg.items...), clamp(m.cursor, len(msg.items))
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
		m.websites, m.websiteCursor, m.status = append([]domain.Website(nil), msg.items...), 0, "e toggle • l access log • L error log • esc subscriptions • d detail • o output • q quit"
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
		m.databases, m.focus, m.status = append([]domain.Database(nil), msg.items...), focusDetail, "databases loaded"
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
