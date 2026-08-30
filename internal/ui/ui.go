package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"provctl/internal/domain"
)

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height, m.ready = msg.Width, msg.Height, true
	case tea.KeyMsg:
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
		case "enter":
			m.showWebsites, m.focus, m.status = true, focusWebsites, "loading websites…"
			return m, m.loadWebsites
		case "d":
			m.focus = focusDetail
		case "o":
			m.focus = focusOutput
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
	}
	return m, nil
}

func Program(deps Deps) *tea.Program { return tea.NewProgram(New(deps), tea.WithAltScreen()) }
