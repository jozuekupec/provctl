// Package ui provides the read-mostly terminal interface for provctl.
package ui

import (
	"context"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"provctl/internal/domain"
	"provctl/internal/service"
)

type Deps struct {
	LoadSubscriptions     func(context.Context) ([]domain.Subscription, error)
	LoadWebsites          func(context.Context, int64) ([]domain.Website, error)
	LoadDatabases         func(context.Context, string) ([]domain.Database, error)
	ReadWebsiteLogs       func(context.Context, string, string, bool, int) (string, error)
	SetWebsiteEnabled     func(context.Context, string, string, bool) (int64, error)
	SetSubscriptionStatus func(context.Context, string, string) (int64, error)
	DeleteSubscription    func(context.Context, string, bool) (int64, error)
	RunHealth             func(context.Context, string, string) ([]service.Check, error)
}
type websitesLoadedMsg struct {
	items      []domain.Website
	err        error
	generation uint64
}
type databasesLoadedMsg struct {
	items      []domain.Database
	err        error
	generation uint64
}

type subscriptionsLoadedMsg struct {
	items      []domain.Subscription
	err        error
	generation uint64
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
type subscriptionDeletedMsg struct {
	err  error
	name string
}
type healthLoadedMsg struct {
	checks     []service.Check
	err        error
	generation uint64
}
type websiteLogsLoadedMsg struct {
	contents   string
	err        error
	generation uint64
}

type focus int

const (
	focusSubscriptions focus = iota
	focusWebsites
	focusDetail
	focusLogs
	focusOutput
)

type outputState struct{ lines []string }
type confirmState struct {
	action  string
	enabled bool
	domain  string
	title   string
	lines   []string
	word    string
	input   textinput.Model
	err     string
}

type helpState struct {
	open   bool
	scroll int
	filter filterState
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
	deps               Deps
	width, height      int
	ready              bool
	items              []domain.Subscription
	cursor             int
	websites           []domain.Website
	databases          []domain.Database
	websiteCursor      int
	showWebsites       bool
	workspace          bool
	focus              focus
	output             outputState
	logs               outputState
	subscriptionFilter filterState
	websiteFilter      filterState
	help               helpState
	status             string
	confirm            confirmState
	progress           progressState
	detailScroll       int
	outputScroll       int
	subscriptions      opSlot
	websitesLoad       opSlot
	databasesLoad      opSlot
	healthLoad         opSlot
	logsLoad           opSlot
}

func (m appModel) changeWebsite(ctx context.Context, confirm confirmState) tea.Msg {
	website, websiteOK := m.selectedWebsite()
	subscription, subscriptionOK := m.selectedSubscription()
	if m.deps.SetWebsiteEnabled == nil || !subscriptionOK || !websiteOK {
		return websiteChangedMsg{err: context.Canceled}
	}
	_, err := m.deps.SetWebsiteEnabled(ctx, subscription.Name, website.PrimaryDomain, confirm.enabled)
	return websiteChangedMsg{err: err, enabled: confirm.enabled, domain: website.PrimaryDomain}
}

func (m appModel) changeWebsiteCmd(confirm confirmState) tea.Cmd {
	return steppedCmd("Update website", []string{"apply generated configuration"}, func(ctx context.Context) tea.Msg { return m.changeWebsite(ctx, confirm) })
}

func (m appModel) changeSubscription(ctx context.Context, confirm confirmState) tea.Msg {
	subscription, ok := m.selectedSubscription()
	if m.deps.SetSubscriptionStatus == nil || !ok {
		return subscriptionChangedMsg{err: context.Canceled}
	}
	_, err := m.deps.SetSubscriptionStatus(ctx, subscription.Name, confirm.action)
	return subscriptionChangedMsg{err: err, name: subscription.Name, status: confirm.action}
}

func (m appModel) changeSubscriptionCmd(confirm confirmState) tea.Cmd {
	return steppedCmd("Update subscription", []string{"apply subscription state"}, func(ctx context.Context) tea.Msg { return m.changeSubscription(ctx, confirm) })
}

func (m appModel) deleteSubscription(ctx context.Context, confirm confirmState) tea.Msg {
	subscription, ok := m.selectedSubscription()
	if m.deps.DeleteSubscription == nil || !ok || subscription.Name != confirm.domain {
		return subscriptionDeletedMsg{err: context.Canceled}
	}
	_, err := m.deps.DeleteSubscription(ctx, subscription.Name, false)
	return subscriptionDeletedMsg{err: err, name: subscription.Name}
}

func (m appModel) deleteSubscriptionCmd(confirm confirmState) tea.Cmd {
	return steppedCmd("Delete subscription", []string{"remove subscription and managed resources"}, func(ctx context.Context) tea.Msg { return m.deleteSubscription(ctx, confirm) })
}

func (m appModel) askDeleteSubscription() appModel {
	subscription, ok := m.selectedSubscription()
	if !ok || subscription.Status != "archived" {
		m.status = "archive the selected subscription before deletion"
		return m
	}
	return m.askConfirm(confirmState{
		action: "delete", domain: subscription.Name, word: subscription.Name,
		title: "Delete subscription", lines: []string{
			"Subscription: " + subscription.Name,
			"This permanently removes managed resources and the subscription home.",
		},
	})
}

func (m appModel) loadHealth(ctx context.Context, generation uint64) tea.Msg {
	subscription, ok := m.selectedSubscription()
	if m.deps.RunHealth == nil || !ok {
		return healthLoadedMsg{err: context.Canceled, generation: generation}
	}
	checks, err := m.deps.RunHealth(ctx, subscription.Name, "")
	return healthLoadedMsg{checks: checks, err: err, generation: generation}
}

func (m appModel) loadWebsites(ctx context.Context, generation uint64) tea.Msg {
	subscription, ok := m.selectedSubscription()
	if m.deps.LoadWebsites == nil || !ok {
		return websitesLoadedMsg{err: context.Canceled, generation: generation}
	}
	items, err := m.deps.LoadWebsites(ctx, subscription.ID)
	return websitesLoadedMsg{items: items, err: err, generation: generation}
}

func (m appModel) loadDatabases(ctx context.Context, generation uint64) tea.Msg {
	subscription, ok := m.selectedSubscription()
	if m.deps.LoadDatabases == nil || !ok {
		return databasesLoadedMsg{err: context.Canceled, generation: generation}
	}
	items, err := m.deps.LoadDatabases(ctx, subscription.Name)
	return databasesLoadedMsg{items: items, err: err, generation: generation}
}

func (m appModel) loadWebsiteLogs(ctx context.Context, generation uint64, errorLog bool) tea.Msg {
	subscription, subscriptionOK := m.selectedSubscription()
	website, websiteOK := m.selectedWebsite()
	if m.deps.ReadWebsiteLogs == nil || !subscriptionOK || !websiteOK {
		return websiteLogsLoadedMsg{err: context.Canceled, generation: generation}
	}
	contents, err := m.deps.ReadWebsiteLogs(ctx, subscription.Name, website.PrimaryDomain, errorLog, 100)
	return websiteLogsLoadedMsg{contents: contents, err: err, generation: generation}
}

func New(deps Deps) appModel {
	return appModel{deps: deps, focus: focusSubscriptions, status: "loading subscriptions…", subscriptionFilter: newFilter(), websiteFilter: newFilter(), help: helpState{filter: newFilter()}}
}

func (m appModel) Init() tea.Cmd {
	return func() tea.Msg { return m.loadSubscriptions(context.Background(), 0) }
}

func (m appModel) loadSubscriptions(ctx context.Context, generation uint64) tea.Msg {
	if m.deps.LoadSubscriptions == nil {
		return subscriptionsLoadedMsg{err: context.Canceled, generation: generation}
	}
	items, err := m.deps.LoadSubscriptions(ctx)
	return subscriptionsLoadedMsg{items: items, err: err, generation: generation}
}

func (m appModel) startSubscriptions() (appModel, tea.Cmd) {
	ctx, generation := m.subscriptions.start()
	return m, func() tea.Msg { return m.loadSubscriptions(ctx, generation) }
}

func (m appModel) startWebsites() (appModel, tea.Cmd) {
	ctx, generation := m.websitesLoad.start()
	return m, func() tea.Msg { return m.loadWebsites(ctx, generation) }
}

func (m appModel) startDatabases() (appModel, tea.Cmd) {
	ctx, generation := m.databasesLoad.start()
	return m, func() tea.Msg { return m.loadDatabases(ctx, generation) }
}

func (m appModel) startHealth() (appModel, tea.Cmd) {
	ctx, generation := m.healthLoad.start()
	return m, func() tea.Msg { return m.loadHealth(ctx, generation) }
}

func (m appModel) startWebsiteLogs(errorLog bool) (appModel, tea.Cmd) {
	ctx, generation := m.logsLoad.start()
	return m, func() tea.Msg { return m.loadWebsiteLogs(ctx, generation, errorLog) }
}

func (m appModel) clearSelectionDetails() appModel {
	m.websitesLoad.invalidate()
	m.databasesLoad.invalidate()
	m.healthLoad.invalidate()
	m.logsLoad.invalidate()
	m.websites, m.databases = nil, nil
	m.websiteCursor, m.detailScroll, m.outputScroll = 0, 0, 0
	m.showWebsites = false
	return m
}

func (m appModel) selectedSubscriptionID() int64 {
	subscription, ok := m.selectedSubscription()
	if !ok {
		return 0
	}
	return subscription.ID
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
