package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"provctl/internal/domain"
)

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height, m.ready = msg.Width, msg.Height, true
	case tea.KeyMsg:
		if m.confirm.action != "" {
			switch msg.String() {
			case "y":
				m.status = "applying change…"
				if m.confirm.action == "active" || m.confirm.action == "suspended" {
					return m, m.changeSubscription
				}
				return m, m.changeWebsite
			case "esc", "n", "q":
				m.confirm, m.status = confirmState{}, "cancelled"
				return m, nil
			}
			return m, nil
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "j", "down":
			if m.focus == focusWebsites {
				m.websiteCursor = clamp(m.websiteCursor+1, len(m.websites))
			} else {
				m.cursor = clamp(m.cursor+1, len(m.items))
			}
		case "k", "up":
			if m.focus == focusWebsites {
				m.websiteCursor = clamp(m.websiteCursor-1, len(m.websites))
			} else {
				m.cursor = clamp(m.cursor-1, len(m.items))
			}
		case "r":
			m.status = "loading subscriptions…"
			return m, m.loadSubscriptions
		case "h":
			m.status = "running health checks…"
			return m, m.loadHealth
		case "enter":
			m.showWebsites, m.focus, m.status = true, focusWebsites, "loading websites…"
			return m, m.loadWebsites
		case "d":
			m.focus = focusDetail
		case "o":
			m.focus = focusOutput
		case "e":
			if m.showWebsites && len(m.websites) > 0 {
				website := m.websites[clamp(m.websiteCursor, len(m.websites))]
				m.confirm = confirmState{action: "set-enabled", enabled: !website.Enabled, domain: website.PrimaryDomain}
				m.status = "confirm with y; esc cancels"
			}
		case "s":
			if !m.showWebsites && len(m.items) > 0 {
				subscription := m.items[clamp(m.cursor, len(m.items))]
				if subscription.Status == "active" {
					m.confirm = confirmState{action: "suspended", domain: subscription.Name}
				} else if subscription.Status == "suspended" {
					m.confirm = confirmState{action: "active", domain: subscription.Name}
				}
				if m.confirm.action != "" {
					m.status = "confirm with y; esc cancels"
				}
			}
		case "tab":
			m.focus = (m.focus + 1) % 4
		case "esc":
			m.showWebsites, m.focus = false, focusSubscriptions
		}
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
		m.websites, m.websiteCursor, m.status = append([]domain.Website(nil), msg.items...), 0, "esc subscriptions • d detail • o output • q quit"
		m.output = m.output.append("websites loaded")
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
	}
	return m, nil
}

func Program(deps Deps) *tea.Program { return tea.NewProgram(New(deps), tea.WithAltScreen()) }
