// Package ui provides the read-mostly terminal interface for provctl.
package ui

import (
	"context"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"provctl/internal/config"
	"provctl/internal/domain"
	"provctl/internal/fsbrowse"
	"provctl/internal/service"
)

type Deps struct {
	LoadSubscriptions      func(context.Context) ([]domain.Subscription, error)
	LoadWebsites           func(context.Context, int64) ([]domain.Website, error)
	LoadDatabases          func(context.Context, string) ([]domain.Database, error)
	LoadSSHKeys            func(context.Context, string) ([]domain.SSHKey, error)
	LoadCronJobs           func(context.Context, string) ([]domain.CronJob, error)
	LoadBackups            func(context.Context, string) ([]domain.Backup, error)
	SetSSHAccess           func(context.Context, string, string) (string, int64, error)
	ReadWebsiteLogs        func(context.Context, string, string, bool, int) (string, error)
	SetWebsiteEnabled      func(context.Context, string, string, bool) (int64, error)
	SetWebsiteTLS          func(context.Context, string, string, bool) error
	SetWebsiteDocumentRoot func(context.Context, string, string, string) (int64, error)
	CreateWebsite          func(context.Context, string, string, domain.WebsiteType, string, int) (int64, error)
	SetWebsiteAlias        func(context.Context, string, string, string, bool) (int64, error)
	SetWebsiteTarget       func(context.Context, string, string, string, int) (int64, error)
	DeleteWebsite          func(context.Context, string, string) (int64, error)
	CreateSubscription     func(context.Context, string) (int64, error)
	SetSubscriptionStatus  func(context.Context, string, string) (int64, error)
	DeleteSubscription     func(context.Context, string, bool) (int64, error)
	LoadPHPVersions        func(context.Context) ([]service.PHPFPMVersion, error)
	SetWebsitePHP          func(context.Context, string, string, service.PHPSetOptions) (int64, error)
	RunHealth              func(context.Context, string, string) ([]service.Check, error)
	Reconcile              func(context.Context, string) (int64, error)
	SaveConfig             func(context.Context, config.Config) error
	BrowsePath             func(context.Context, string, fsbrowse.Mode) (string, []fsbrowse.Entry, error)
	Config                 config.Config
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
type sshKeysLoadedMsg struct {
	items      []domain.SSHKey
	err        error
	generation uint64
}
type cronJobsLoadedMsg struct {
	items      []domain.CronJob
	err        error
	generation uint64
}
type backupsLoadedMsg struct {
	items      []domain.Backup
	err        error
	generation uint64
}
type sshAccessChangedMsg struct {
	err      error
	access   string
	password string
	items    []domain.Subscription
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
type websiteTLSChangedMsg struct {
	err     error
	enabled bool
	domain  string
}
type websiteDocumentRootChangedMsg struct {
	err    error
	domain string
	root   string
	items  []domain.Website
}
type websiteCreatedMsg struct {
	err    error
	domain string
	items  []domain.Website
}
type websiteAliasChangedMsg struct {
	err    error
	domain string
	alias  string
	add    bool
	items  []domain.Website
}
type websiteTargetChangedMsg struct {
	err    error
	domain string
	items  []domain.Website
}
type websiteDeletedMsg struct {
	err    error
	domain string
	items  []domain.Website
}
type subscriptionChangedMsg struct {
	err          error
	name, status string
}
type subscriptionDeletedMsg struct {
	err  error
	name string
}
type subscriptionCreatedMsg struct {
	err  error
	name string
}
type reconcileFinishedMsg struct {
	err          error
	subscription string
	operationID  int64
	websites     []domain.Website
}
type phpVersionsLoadedMsg struct {
	items      []service.PHPFPMVersion
	err        error
	generation uint64
}
type websitePHPChangedMsg struct {
	err             error
	subscription    string
	domain, version string
	items           []domain.Website
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
type settingsSavedMsg struct {
	cfg config.Config
	err error
}
type pathListedMsg struct {
	dir        string
	entries    []fsbrowse.Entry
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

type detailView uint8

const (
	detailDomain detailView = iota
	detailDatabases
	detailSSHKeys
	detailCronJobs
	detailBackups
)

type outputState struct{ lines []string }
type confirmState struct {
	action  string
	enabled bool
	domain  string
	value   string
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

type phpPickerState struct {
	open    bool
	loading bool
	cursor  int
	items   []service.PHPFPMVersion
}

type settingsState struct {
	open   bool
	scope  int
	field  int
	values map[int]string
	input  textinput.Model
}

type documentRootFormState struct {
	open    bool
	website domain.Website
	input   textinput.Model
}

type websiteCreateFormState struct {
	open      bool
	field     int
	typeIndex int
	domain    textinput.Model
	target    textinput.Model
}

type aliasFormState struct {
	open   bool
	add    bool
	domain string
	input  textinput.Model
}
type targetFormState struct {
	open    bool
	website domain.Website
	input   textinput.Model
	code    int
}

type subscriptionCreateFormState struct {
	open  bool
	input textinput.Model
}

type sshAccessFormState struct {
	open  bool
	index int
}

type secretState struct {
	open   bool
	title  string
	secret string
}

type pathPickerTarget uint8

const (
	pathPickerSettings pathPickerTarget = iota
	pathPickerDocumentRoot
)

// pathPickerState belongs to the picker, keeping its filter and asynchronous
// generation separate from the settings form it temporarily overlays.
type pathPickerState struct {
	open       bool
	field      int
	target     pathPickerTarget
	mode       fsbrowse.Mode
	dir        string
	entries    []pathEntry
	cursor     int
	scroll     int
	filter     filterState
	loading    bool
	generation uint64
	err        string
	confirm    string
	confirmDir bool
	pathInput  textinput.Model
	enterPath  bool
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
	deps                   Deps
	width, height          int
	ready                  bool
	items                  []domain.Subscription
	cursor                 int
	websites               []domain.Website
	databases              []domain.Database
	sshKeys                []domain.SSHKey
	cronJobs               []domain.CronJob
	backups                []domain.Backup
	websiteCursor          int
	showWebsites           bool
	workspace              bool
	focus                  focus
	output                 outputState
	logs                   outputState
	subscriptionFilter     filterState
	websiteFilter          filterState
	help                   helpState
	phpPicker              phpPickerState
	settings               settingsState
	documentRootForm       documentRootFormState
	websiteCreateForm      websiteCreateFormState
	aliasForm              aliasFormState
	targetForm             targetFormState
	subscriptionCreateForm subscriptionCreateFormState
	sshAccessForm          sshAccessFormState
	secret                 secretState
	pathPicker             pathPickerState
	status                 string
	confirm                confirmState
	progress               progressState
	detailScroll           int
	detailView             detailView
	outputScroll           int
	subscriptions          opSlot
	websitesLoad           opSlot
	databasesLoad          opSlot
	sshKeysLoad            opSlot
	cronJobsLoad           opSlot
	backupsLoad            opSlot
	healthLoad             opSlot
	logsLoad               opSlot
	phpVersionsLoad        opSlot
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
	return steppedCmd("Update website", []string{"apply generated configuration"}, func(ctx context.Context, _ func(int)) tea.Msg { return m.changeWebsite(ctx, confirm) })
}

func (m appModel) changeWebsiteTLS(ctx context.Context, confirm confirmState) tea.Msg {
	website, websiteOK := m.selectedWebsite()
	subscription, subscriptionOK := m.selectedSubscription()
	if m.deps.SetWebsiteTLS == nil || !subscriptionOK || !websiteOK {
		return websiteTLSChangedMsg{err: context.Canceled}
	}
	err := m.deps.SetWebsiteTLS(ctx, subscription.Name, website.PrimaryDomain, confirm.enabled)
	return websiteTLSChangedMsg{err: err, enabled: confirm.enabled, domain: website.PrimaryDomain}
}

func (m appModel) changeWebsiteTLSCmd(confirm confirmState) tea.Cmd {
	return steppedCmd("Update TLS", []string{"apply TLS configuration"}, func(ctx context.Context, _ func(int)) tea.Msg { return m.changeWebsiteTLS(ctx, confirm) })
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
	return steppedCmd("Update subscription", []string{"apply subscription state"}, func(ctx context.Context, _ func(int)) tea.Msg { return m.changeSubscription(ctx, confirm) })
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
	return steppedCmd("Delete subscription", []string{"remove subscription and managed resources"}, func(ctx context.Context, _ func(int)) tea.Msg { return m.deleteSubscription(ctx, confirm) })
}

func (m appModel) loadPHPVersions(ctx context.Context, generation uint64) tea.Msg {
	if m.deps.LoadPHPVersions == nil {
		return phpVersionsLoadedMsg{err: context.Canceled, generation: generation}
	}
	items, err := m.deps.LoadPHPVersions(ctx)
	return phpVersionsLoadedMsg{items: items, err: err, generation: generation}
}

func (m appModel) changeWebsitePHP(ctx context.Context, confirm confirmState, report func(int)) tea.Msg {
	subscription, subscriptionOK := m.selectedSubscription()
	website, websiteOK := m.selectedWebsite()
	if m.deps.SetWebsitePHP == nil || !subscriptionOK || !websiteOK {
		return websitePHPChangedMsg{err: context.Canceled}
	}
	_, err := m.deps.SetWebsitePHP(ctx, subscription.Name, website.PrimaryDomain, service.PHPSetOptions{Version: confirm.domain})
	if err != nil || m.deps.LoadWebsites == nil {
		return websitePHPChangedMsg{err: err, subscription: subscription.Name, domain: website.PrimaryDomain, version: confirm.domain}
	}
	report(1)
	items, err := m.deps.LoadWebsites(ctx, subscription.ID)
	return websitePHPChangedMsg{err: err, subscription: subscription.Name, domain: website.PrimaryDomain, version: confirm.domain, items: items}
}

func (m appModel) changeWebsitePHPCmd(confirm confirmState) tea.Cmd {
	return steppedCmd("Switch PHP-FPM", []string{"apply domain PHP-FPM configuration", "refresh domain list"}, func(ctx context.Context, report func(int)) tea.Msg {
		return m.changeWebsitePHP(ctx, confirm, report)
	})
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

func (m appModel) loadSSHKeys(ctx context.Context, generation uint64) tea.Msg {
	subscription, ok := m.selectedSubscription()
	if m.deps.LoadSSHKeys == nil || !ok {
		return sshKeysLoadedMsg{err: context.Canceled, generation: generation}
	}
	items, err := m.deps.LoadSSHKeys(ctx, subscription.Name)
	return sshKeysLoadedMsg{items: items, err: err, generation: generation}
}

func (m appModel) startSSHKeys() (appModel, tea.Cmd) {
	ctx, generation := m.sshKeysLoad.start()
	return m, func() tea.Msg { return m.loadSSHKeys(ctx, generation) }
}

func (m appModel) loadCronJobs(ctx context.Context, generation uint64) tea.Msg {
	subscription, ok := m.selectedSubscription()
	if m.deps.LoadCronJobs == nil || !ok {
		return cronJobsLoadedMsg{err: context.Canceled, generation: generation}
	}
	items, err := m.deps.LoadCronJobs(ctx, subscription.Name)
	return cronJobsLoadedMsg{items: items, err: err, generation: generation}
}

func (m appModel) startCronJobs() (appModel, tea.Cmd) {
	ctx, generation := m.cronJobsLoad.start()
	return m, func() tea.Msg { return m.loadCronJobs(ctx, generation) }
}

func (m appModel) loadBackups(ctx context.Context, generation uint64) tea.Msg {
	subscription, ok := m.selectedSubscription()
	if m.deps.LoadBackups == nil || !ok {
		return backupsLoadedMsg{err: context.Canceled, generation: generation}
	}
	items, err := m.deps.LoadBackups(ctx, subscription.Name)
	return backupsLoadedMsg{items: items, err: err, generation: generation}
}

func (m appModel) startBackups() (appModel, tea.Cmd) {
	ctx, generation := m.backupsLoad.start()
	return m, func() tea.Msg { return m.loadBackups(ctx, generation) }
}

func (m appModel) startHealth() (appModel, tea.Cmd) {
	ctx, generation := m.healthLoad.start()
	return m, func() tea.Msg { return m.loadHealth(ctx, generation) }
}

func (m appModel) startWebsiteLogs(errorLog bool) (appModel, tea.Cmd) {
	ctx, generation := m.logsLoad.start()
	return m, func() tea.Msg { return m.loadWebsiteLogs(ctx, generation, errorLog) }
}

func (m appModel) startPHPVersions() (appModel, tea.Cmd) {
	ctx, generation := m.phpVersionsLoad.start()
	return m, func() tea.Msg { return m.loadPHPVersions(ctx, generation) }
}

func (m appModel) clearSelectionDetails() appModel {
	m.websitesLoad.invalidate()
	m.databasesLoad.invalidate()
	m.sshKeysLoad.invalidate()
	m.cronJobsLoad.invalidate()
	m.backupsLoad.invalidate()
	m.healthLoad.invalidate()
	m.logsLoad.invalidate()
	m.websites, m.databases, m.sshKeys, m.cronJobs, m.backups = nil, nil, nil, nil, nil
	m.websiteCursor, m.detailScroll, m.outputScroll, m.detailView = 0, 0, 0, detailDomain
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
