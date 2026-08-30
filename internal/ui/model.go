// Package ui provides the read-mostly terminal interface for provctl.
package ui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"provctl/internal/domain"
)

type Deps struct {
	LoadSubscriptions func(context.Context) ([]domain.Subscription, error)
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
	status        string
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
