package ui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"provctl/internal/domain"
	"provctl/internal/service"
)

func TestModel_LoadAndNavigateSubscriptions(t *testing.T) {
	m := New(Deps{LoadSubscriptions: func(context.Context) ([]domain.Subscription, error) {
		return []domain.Subscription{{Name: "acme", Status: "active"}, {Name: "beta", Status: "archived"}}, nil
	}})
	loaded := m.Init()().(subscriptionsLoadedMsg)
	updated, _ := m.Update(loaded)
	m = updated.(appModel)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if got := updated.(appModel).cursor; got != 1 {
		t.Errorf("cursor = %d, want 1", got)
	}
}

func TestModel_LoadWebsitesForSelectedSubscription(t *testing.T) {
	m := New(Deps{LoadWebsites: func(_ context.Context, id int64) ([]domain.Website, error) {
		if id != 7 {
			t.Fatalf("subscription ID = %d, want 7", id)
		}
		return []domain.Website{{PrimaryDomain: "example.test", Type: domain.WebsiteStatic, Enabled: true}}, nil
	}})
	m.items = []domain.Subscription{{ID: 7, Name: "acme"}}
	loaded := m.loadWebsites(context.Background(), 0).(websitesLoadedMsg)
	updated, _ := m.Update(loaded)
	if got := updated.(appModel).websites[0].PrimaryDomain; got != "example.test" {
		t.Errorf("domain = %q", got)
	}
}

func TestModel_IgnoresStaleWebsiteResponse(t *testing.T) {
	m := New(Deps{})
	m.items = []domain.Subscription{{ID: 1, Name: "acme"}, {ID: 2, Name: "beta"}}
	m.websitesLoad.generation = 1
	m.cursor = 1
	m = m.clearSelectionDetails()
	updated, _ := m.Update(websitesLoadedMsg{generation: 1, items: []domain.Website{{PrimaryDomain: "acme.test"}}})
	if got := updated.(appModel).websites; len(got) != 0 {
		t.Fatalf("stale websites = %#v, want none", got)
	}
}

func TestModel_SwitchingSubscriptionClearsDependentData(t *testing.T) {
	m := New(Deps{})
	m.items = []domain.Subscription{{ID: 1, Name: "acme"}, {ID: 2, Name: "beta"}}
	m.websites = []domain.Website{{PrimaryDomain: "acme.test"}}
	m.databases = []domain.Database{{Name: "acme_main"}}
	m.workspace, m.showWebsites, m.focus = true, true, focusSubscriptions
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(appModel)
	if m.cursor != 1 || m.showWebsites || len(m.websites) != 0 || len(m.databases) != 0 {
		t.Fatalf("selection state = %#v", m)
	}
}

func TestModel_RefreshCancelsPreviousRead(t *testing.T) {
	m := New(Deps{LoadSubscriptions: func(context.Context) ([]domain.Subscription, error) { return nil, nil }})
	first, firstCommand := m.startSubscriptions()
	second, secondCommand := first.startSubscriptions()
	firstMessage := firstCommand().(subscriptionsLoadedMsg)
	secondMessage := secondCommand().(subscriptionsLoadedMsg)
	if !second.subscriptions.stale(firstMessage.generation) {
		t.Fatal("first request is not stale")
	}
	if second.subscriptions.stale(secondMessage.generation) {
		t.Fatal("second request is unexpectedly stale")
	}
}

func TestModel_DetailUsesSelectedWebsite(t *testing.T) {
	m := New(Deps{})
	m.showWebsites = true
	m.items = []domain.Subscription{{Name: "acme", Home: "/vhosts/acme", PHPVersion: "8.4"}}
	m.websites = []domain.Website{{PrimaryDomain: "example.test", Type: domain.WebsitePHPFPM, DocumentRoot: "/vhosts/acme/sites/example.test/public", Enabled: true, SSLEnabled: true, ForceHTTPS: true, Aliases: []string{"www.example.test"}}}
	if got := m.detail(); !strings.Contains(got, "Domain: example.test") || !strings.Contains(got, "PHP-FPM: 8.4") || !strings.Contains(got, "Home: /vhosts/acme") || !strings.Contains(got, "TLS: enabled") || !strings.Contains(got, "www.example.test") {
		t.Errorf("detail = %q", got)
	}
}

func TestModel_ViewUsesMinimumSizeGuard(t *testing.T) {
	m := New(Deps{})
	m.ready, m.width, m.height = true, minWidth-1, minHeight
	if got := m.View(); !strings.Contains(got, "needs a terminal") {
		t.Errorf("View() = %q", got)
	}
}

func TestModel_PickerRendersFullScreenSubscriptionList(t *testing.T) {
	m := New(Deps{})
	m.ready, m.width, m.height = true, 100, 28
	m.items = []domain.Subscription{{Name: "acme", Status: "active", PHPVersion: "8.4"}}
	view := m.View()
	if !strings.Contains(view, "Subscriptions") || strings.Contains(view, "Domains") {
		t.Errorf("picker view =\n%s", view)
	}
}

func TestModel_WorkspaceUsesExactTerminalFrame(t *testing.T) {
	m := New(Deps{})
	m.ready, m.width, m.height, m.workspace = true, 101, 28, true
	m.items = []domain.Subscription{{Name: "acme", Status: "active", PHPVersion: "8.4"}}
	for index, line := range strings.Split(m.View(), "\n") {
		if got := lipgloss.Width(line); got != m.width {
			t.Fatalf("line %d width = %d, want %d: %q", index, got, m.width, line)
		}
	}
	if got := len(strings.Split(m.View(), "\n")); got != m.height {
		t.Fatalf("line count = %d, want %d", got, m.height)
	}
}

func TestModel_TruncatePreservesUnicodeAndANSI(t *testing.T) {
	value := selectedStyle.Render("žluťoučký")
	got := truncate(value, 5)
	if width := lipgloss.Width(got); width != 5 {
		t.Errorf("truncate width = %d, want 5; %q", width, got)
	}
	if !strings.Contains(got, "…") {
		t.Errorf("truncate result = %q, want ellipsis", got)
	}
}

func TestModel_RefreshDropsDependentDataWhenSelectionDisappears(t *testing.T) {
	m := New(Deps{})
	m.items = []domain.Subscription{{ID: 1, Name: "acme"}}
	m.websites = []domain.Website{{PrimaryDomain: "acme.test"}}
	m.databases = []domain.Database{{Name: "acme_main"}}
	m.showWebsites = true
	updated, _ := m.Update(subscriptionsLoadedMsg{items: []domain.Subscription{{ID: 2, Name: "beta"}}})
	m = updated.(appModel)
	if m.showWebsites || len(m.websites) != 0 || len(m.databases) != 0 {
		t.Fatalf("dependent state after refresh = %#v", m)
	}
}

func TestModel_ConfirmWebsiteToggleCallsDependency(t *testing.T) {
	called := false
	m := New(Deps{SetWebsiteEnabled: func(_ context.Context, subscription, domain string, enabled bool) (int64, error) {
		called = subscription == "acme" && domain == "example.test" && !enabled
		return 1, nil
	}})
	m.items = []domain.Subscription{{ID: 1, Name: "acme"}}
	m.workspace, m.websites, m.showWebsites = true, []domain.Website{{PrimaryDomain: "example.test", Enabled: true}}, true
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m = updated.(appModel)
	if m.confirm.action == "" {
		t.Fatal("toggle did not request confirmation")
	}
	updated, command := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = runProgress(t, updated.(appModel), command)
	if !called {
		t.Fatal("SetWebsiteEnabled was not called")
	}
	if m.progress.active {
		t.Fatal("progress is still active")
	}
}

func TestModel_FilteredWebsiteToggleUsesVisibleSelection(t *testing.T) {
	var changedDomain string
	m := New(Deps{SetWebsiteEnabled: func(_ context.Context, _, domain string, _ bool) (int64, error) {
		changedDomain = domain
		return 1, nil
	}})
	m.items = []domain.Subscription{{ID: 1, Name: "acme"}}
	m.workspace, m.showWebsites, m.focus = true, true, focusWebsites
	m.websites = []domain.Website{{PrimaryDomain: "first.test", Enabled: true}, {PrimaryDomain: "second.test", Enabled: true}}
	m.websiteFilter.input.SetValue("second")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m = updated.(appModel)
	updated, command := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = runProgress(t, updated.(appModel), command)
	if changedDomain != "second.test" {
		t.Fatalf("changed domain = %q, want filtered selection", changedDomain)
	}
	if len(m.websites) != 2 {
		t.Fatalf("filter mutated website source: %#v", m.websites)
	}
}

func TestModel_HelpOpensFiltersAndCloses(t *testing.T) {
	m := New(Deps{})
	m.ready, m.width, m.height = true, 100, 28
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = updated.(appModel)
	if !m.help.open || !strings.Contains(m.View(), "Help") {
		t.Fatalf("help did not open: %#v", m.help)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = updated.(appModel)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = updated.(appModel)
	if m.help.filter.query() != "d" {
		t.Fatalf("help filter = %q", m.help.filter.query())
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(appModel)
	if !m.help.open || m.help.filter.query() != "" {
		t.Fatalf("escape did not clear active help filter: %#v", m.help)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if updated.(appModel).help.open {
		t.Fatal("second escape did not close help")
	}
}

func TestModel_ConfirmSubscriptionSuspendCallsDependency(t *testing.T) {
	called := false
	m := New(Deps{SetSubscriptionStatus: func(_ context.Context, name, status string) (int64, error) {
		called = name == "acme" && status == "suspended"
		return 1, nil
	}})
	m.items = []domain.Subscription{{ID: 1, Name: "acme", Status: "active"}}
	m.workspace = true
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = updated.(appModel)
	if m.confirm.action != "suspended" {
		t.Fatalf("confirmation = %#v", m.confirm)
	}
	updated, command := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = runProgress(t, updated.(appModel), command)
	if !called {
		t.Fatal("SetSubscriptionStatus was not called")
	}
}

func TestModel_ArchiveSubscriptionCallsDependency(t *testing.T) {
	var status string
	m := New(Deps{SetSubscriptionStatus: func(_ context.Context, _, value string) (int64, error) {
		status = value
		return 1, nil
	}})
	m.items = []domain.Subscription{{Name: "acme", Status: "active"}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = updated.(appModel)
	if m.confirm.action != "archived" {
		t.Fatalf("confirmation = %#v", m.confirm)
	}
	updated, command := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	_ = runProgress(t, updated.(appModel), command)
	if status != "archived" {
		t.Fatalf("status = %q, want archived", status)
	}
}

func TestModel_DeleteArchivedSubscriptionRequiresTypedName(t *testing.T) {
	deleted := false
	m := New(Deps{DeleteSubscription: func(_ context.Context, name string, force bool) (int64, error) {
		deleted = name == "acme" && !force
		return 1, nil
	}})
	m.items = []domain.Subscription{{Name: "acme", Status: "archived"}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = updated.(appModel)
	if m.confirm.word != "acme" {
		t.Fatalf("delete confirmation = %#v", m.confirm)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(appModel)
	if m.confirm.err == "" || deleted {
		t.Fatalf("empty confirmation unexpectedly deleted: %#v", m.confirm)
	}
	for _, character := range "acme" {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{character}})
		m = updated.(appModel)
	}
	updated, command := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_ = runProgress(t, updated.(appModel), command)
	if !deleted {
		t.Fatal("DeleteSubscription was not called after typed confirmation")
	}
}

func TestFocusNavigationMovesAcrossWorkspacePanels(t *testing.T) {
	if got := focusLeft(focusWebsites); got != focusSubscriptions {
		t.Fatalf("left from domains = %v, want subscriptions", got)
	}
	if got := focusRight(focusLogs); got != focusOutput {
		t.Fatalf("right from logs = %v, want output", got)
	}
}

func TestModel_ConfirmationCannotStartDuplicateMutation(t *testing.T) {
	m := New(Deps{SetSubscriptionStatus: func(context.Context, string, string) (int64, error) { return 1, nil }})
	m.items = []domain.Subscription{{Name: "acme", Status: "active"}}
	m.workspace = true
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = updated.(appModel)
	updated, command := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = updated.(appModel)
	if m.confirm.action != "" {
		t.Fatalf("confirmation = %#v, want cleared", m.confirm)
	}
	updated, next := m.Update(command())
	m = updated.(appModel)
	if !m.progress.active || m.confirm.action != "" {
		t.Fatalf("progress/confirmation = %#v/%#v", m.progress, m.confirm)
	}
	updated, duplicate := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if duplicate != nil || updated.(appModel).progress.active != m.progress.active {
		t.Fatal("duplicate confirmation was accepted during progress")
	}
	_ = next
}

func runProgress(t *testing.T, model appModel, command tea.Cmd) appModel {
	t.Helper()
	for range 4 { // start, running step, completed step, final operation message
		message := command()
		updated, next := model.Update(message)
		model, command = updated.(appModel), next
	}
	return model
}

func TestModel_HealthWritesChecksToOutput(t *testing.T) {
	m := New(Deps{RunHealth: func(_ context.Context, name, domain string) ([]service.Check, error) {
		if name != "acme" || domain != "" {
			t.Fatalf("scope = %q/%q", name, domain)
		}
		return []service.Check{{Name: "Apache", Status: service.CheckOK, Detail: "active"}}, nil
	}})
	m.items = []domain.Subscription{{Name: "acme"}}
	m.workspace = true
	updated, command := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	updated, _ = updated.(appModel).Update(command())
	m = updated.(appModel)
	if m.focus != focusOutput || !strings.Contains(strings.Join(m.output.lines, "\n"), "Apache") {
		t.Errorf("model = %#v", m)
	}
}

func TestModel_LoadDatabasesShowsDetail(t *testing.T) {
	m := New(Deps{LoadDatabases: func(_ context.Context, name string) ([]domain.Database, error) {
		if name != "acme" {
			t.Fatalf("name = %q", name)
		}
		return []domain.Database{{Name: "acme_main"}}, nil
	}})
	m.items = []domain.Subscription{{Name: "acme"}}
	m.workspace = true
	updated, command := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	updated, _ = updated.(appModel).Update(command())
	m = updated.(appModel)
	if m.focus != focusDetail || !strings.Contains(m.detail(), "acme_main") {
		t.Errorf("detail = %q", m.detail())
	}
}

func TestModel_LoadWebsiteLogsWritesOutput(t *testing.T) {
	m := New(Deps{ReadWebsiteLogs: func(_ context.Context, subscription, domain string, errorLog bool, lines int) (string, error) {
		if subscription != "acme" || domain != "example.test" || errorLog || lines != 100 {
			t.Fatalf("log request = %q %q %t %d", subscription, domain, errorLog, lines)
		}
		return "first\nsecond\n", nil
	}})
	m.items = []domain.Subscription{{Name: "acme"}}
	m.workspace, m.websites, m.showWebsites = true, []domain.Website{{PrimaryDomain: "example.test"}}, true
	updated, command := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	updated, _ = updated.(appModel).Update(command())
	m = updated.(appModel)
	if m.focus != focusLogs || strings.Join(m.logs.lines, "\n") != "first\nsecond" {
		t.Errorf("logs = %#v", m.logs)
	}
}
