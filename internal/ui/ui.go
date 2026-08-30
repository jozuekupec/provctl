package ui

import tea "github.com/charmbracelet/bubbletea"

import "provctl/internal/domain"

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height, m.ready = msg.Width, msg.Height, true
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "j", "down":
			m.cursor = clamp(m.cursor+1, len(m.items))
		case "k", "up":
			m.cursor = clamp(m.cursor-1, len(m.items))
		case "r":
			m.status = "loading subscriptions…"
			return m, m.loadSubscriptions
		case "enter":
			m.showWebsites, m.status = true, "loading websites…"
			return m, m.loadWebsites
		case "esc":
			m.showWebsites = false
		}
	case subscriptionsLoadedMsg:
		if msg.err != nil {
			m.status = "load failed: " + msg.err.Error()
			return m, nil
		}
		m.items, m.cursor, m.status = append([]domain.Subscription(nil), msg.items...), clamp(m.cursor, len(msg.items)), "r refresh • q quit"
	case websitesLoadedMsg:
		if msg.err != nil {
			m.status = "website load failed: " + msg.err.Error()
			return m, nil
		}
		m.websites, m.websiteCursor, m.status = append([]domain.Website(nil), msg.items...), 0, "esc subscriptions • q quit"
	}
	return m, nil
}

func Program(deps Deps) *tea.Program { return tea.NewProgram(New(deps), tea.WithAltScreen()) }
