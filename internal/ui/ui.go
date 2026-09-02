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
	case subscriptionsLoadedMsg:
		if msg.err != nil {
			m.status = "load failed: " + msg.err.Error()
			return m, nil
		}
		m.items, m.cursor, m.status = append([]domain.Subscription(nil), msg.items...), clamp(m.cursor, len(msg.items)), "r refresh • enter websites • d detail • o output • q quit"
		m.output = m.output.append("subscriptions refreshed")
	case websitesLoadedMsg:
		if msg.err != nil {
			m.status = "website load failed: " + msg.err.Error()
			return m, nil
		}
		m.websites, m.websiteCursor, m.status = append([]domain.Website(nil), msg.items...), 0, "e toggle • l access log • L error log • esc subscriptions • d detail • o output • q quit"
		m.output = m.output.append("websites loaded")
	case databasesLoadedMsg:
		if msg.err != nil {
			m.status = "database load failed: " + msg.err.Error()
			return m, nil
		}
		m.databases, m.focus, m.status = append([]domain.Database(nil), msg.items...), focusDetail, "databases loaded"
	case websiteChangedMsg:
		if msg.err != nil {
			m.status = "website change failed: " + msg.err.Error()
			return m, nil
		}
		m.confirm = confirmState{}
		m.status = "website " + msg.domain + " updated; refreshing…"
		m.output = m.output.append("website " + msg.domain + " enabled=" + fmt.Sprint(msg.enabled))
		return m, m.loadWebsites
	case subscriptionChangedMsg:
		if msg.err != nil {
			m.status = "subscription change failed: " + msg.err.Error()
			return m, nil
		}
		m.confirm, m.status = confirmState{}, "subscription "+msg.name+" updated; refreshing…"
		m.output = m.output.append("subscription " + msg.name + " status=" + msg.status)
		return m, m.loadSubscriptions
	case healthLoadedMsg:
		if msg.err != nil {
			m.status = "health failed: " + msg.err.Error()
			return m, nil
		}
		for _, check := range msg.checks {
			m.output = m.output.append(string(check.Status) + " " + check.Name + ": " + check.Detail)
		}
		m.focus, m.status = focusOutput, "health checks completed"
	case websiteLogsLoadedMsg:
		if msg.err != nil {
			m.status = "log load failed: " + msg.err.Error()
			return m, nil
		}
		m.output, m.focus, m.status = outputState{}, focusOutput, "website log loaded"
		for _, line := range strings.Split(strings.TrimSuffix(msg.contents, "\n"), "\n") {
			if line != "" {
				m.output = m.output.append(line)
			}
		}
	}
	return m, nil
}

func Program(deps Deps) *tea.Program { return tea.NewProgram(New(deps), tea.WithAltScreen()) }
