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
	SetWebsiteEnabled func(context.Context, string, string, bool) (int64, error)
}
type websitesLoadedMsg struct {
	items []domain.Website
	err   error
}

type subscriptionsLoadedMsg struct {
	items []domain.Subscription
	err   error
}
type websiteChangedMsg struct {
	err     error
	enabled bool
	domain  string
}

type focus int

const (
	focusSubscriptions focus = iota
	focusWebsites
	focusDetail
	focusOutput
)

type outputState struct{ lines []string }
type confirmState struct {
	action  string
	enabled bool
	domain  string
}

func (output outputState) append(line string) outputState {
	next := append([]string(nil), output.lines...)
	next = append(next, line)
	if len(next) > 100 {
		next = next[len(next)-100:]
	}
	return outputState{lines: next}
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
	focus         focus
	output        outputState
	status        string
	confirm       confirmState
}

func (m appModel) changeWebsite() tea.Msg {
	if m.deps.SetWebsiteEnabled == nil || len(m.items) == 0 || len(m.websites) == 0 {
		return websiteChangedMsg{err: context.Canceled}
	}
	website := m.websites[clamp(m.websiteCursor, len(m.websites))]
	subscription := m.items[clamp(m.cursor, len(m.items))]
	_, err := m.deps.SetWebsiteEnabled(context.Background(), subscription.Name, website.PrimaryDomain, m.confirm.enabled)
	return websiteChangedMsg{err: err, enabled: m.confirm.enabled, domain: website.PrimaryDomain}
}

func (m appModel) loadWebsites() tea.Msg {
	if m.deps.LoadWebsites == nil || len(m.items) == 0 {
		return websitesLoadedMsg{err: context.Canceled}
	}
	items, err := m.deps.LoadWebsites(context.Background(), m.items[clamp(m.cursor, len(m.items))].ID)
	return websitesLoadedMsg{items: items, err: err}
}

func New(deps Deps) appModel {
	return appModel{deps: deps, focus: focusSubscriptions, status: "loading subscriptions…"}
}

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
