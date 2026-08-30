// Package ui provides the read-mostly terminal interface for provctl.
package ui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"provctl/internal/domain"
)

type Deps struct {
	LoadSubscriptions func(context.Context) ([]domain.Subscription, error)
	LoadWebsites      func(context.Context, int64) ([]domain.Website, error)
}
type websitesLoadedMsg struct {
	items []domain.Website
	err   error
}

type subscriptionsLoadedMsg struct {
	items []domain.Subscription
	err   error
}

type appModel struct {
	deps          Deps
	width, height int
	ready         bool
	items         []domain.Subscription
	cursor        int
	websites      []domain.Website
	websiteCursor int
	showWebsites  bool
	status        string
}

func (m appModel) loadWebsites() tea.Msg {
	if m.deps.LoadWebsites == nil || len(m.items) == 0 {
		return websitesLoadedMsg{err: context.Canceled}
	}
	items, err := m.deps.LoadWebsites(context.Background(), m.items[clamp(m.cursor, len(m.items))].ID)
	return websitesLoadedMsg{items: items, err: err}
}

func New(deps Deps) appModel { return appModel{deps: deps, status: "loading subscriptions…"} }

func (m appModel) Init() tea.Cmd { return m.loadSubscriptions }

func (m appModel) loadSubscriptions() tea.Msg {
	if m.deps.LoadSubscriptions == nil {
		return subscriptionsLoadedMsg{err: context.Canceled}
	}
	items, err := m.deps.LoadSubscriptions(context.Background())
	return subscriptionsLoadedMsg{items: items, err: err}
}

func clamp(cursor, length int) int {
	if length == 0 || cursor < 0 {
		return 0
	}
	if cursor >= length {
		return length - 1
	}
	return cursor
}
