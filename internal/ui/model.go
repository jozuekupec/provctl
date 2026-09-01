// Package ui provides the read-mostly terminal interface for provctl.
package ui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"provctl/internal/domain"
	"provctl/internal/service"
)

type Deps struct {
	LoadSubscriptions     func(context.Context) ([]domain.Subscription, error)
	LoadWebsites          func(context.Context, int64) ([]domain.Website, error)
	LoadDatabases         func(context.Context, string) ([]domain.Database, error)
	SetWebsiteEnabled     func(context.Context, string, string, bool) (int64, error)
	SetSubscriptionStatus func(context.Context, string, string) (int64, error)
	RunHealth             func(context.Context, string, string) ([]service.Check, error)
}
type websitesLoadedMsg struct {
	items []domain.Website
	err   error
}
type databasesLoadedMsg struct {
	items []domain.Database
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
type subscriptionChangedMsg struct {
	err          error
	name, status string
}
type healthLoadedMsg struct {
	checks []service.Check
	err    error
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
	databases     []domain.Database
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

func (m appModel) changeSubscription() tea.Msg {
	if m.deps.SetSubscriptionStatus == nil || len(m.items) == 0 {
		return subscriptionChangedMsg{err: context.Canceled}
	}
	subscription := m.items[clamp(m.cursor, len(m.items))]
	_, err := m.deps.SetSubscriptionStatus(context.Background(), subscription.Name, m.confirm.action)
	return subscriptionChangedMsg{err: err, name: subscription.Name, status: m.confirm.action}
}

func (m appModel) loadHealth() tea.Msg {
	if m.deps.RunHealth == nil || len(m.items) == 0 {
		return healthLoadedMsg{err: context.Canceled}
	}
	subscription := m.items[clamp(m.cursor, len(m.items))]
	checks, err := m.deps.RunHealth(context.Background(), subscription.Name, "")
	return healthLoadedMsg{checks: checks, err: err}
}

func (m appModel) loadWebsites() tea.Msg {
	if m.deps.LoadWebsites == nil || len(m.items) == 0 {
		return websitesLoadedMsg{err: context.Canceled}
	}
	items, err := m.deps.LoadWebsites(context.Background(), m.items[clamp(m.cursor, len(m.items))].ID)
	return websitesLoadedMsg{items: items, err: err}
}

func (m appModel) loadDatabases() tea.Msg {
	if m.deps.LoadDatabases == nil || len(m.items) == 0 {
		return databasesLoadedMsg{err: context.Canceled}
	}
	subscription := m.items[clamp(m.cursor, len(m.items))]
	items, err := m.deps.LoadDatabases(context.Background(), subscription.Name)
	return databasesLoadedMsg{items: items, err: err}
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
