package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"provctl/internal/config"
	"provctl/internal/domain"
	"provctl/internal/fsbrowse"
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

func TestModel_SettingsEditsAndSavesSSLConfiguration(t *testing.T) {
	initial := config.Config{Meta: config.Meta{ConfigVersion: 1}, Paths: config.Paths{VHosts: "/vhosts", Backups: "/backups", ACMEChallenge: "/acme"}, Apache: config.Apache{Service: "apache2", SitesAvailable: "/available", SitesEnabled: "/enabled"}, Users: config.Users{UIDMin: 1, UIDMax: 2}, Limits: config.Limits{LockTimeoutSeconds: 1}, SSL: config.SSL{Email: "old@example.test"}}
	var got config.Config
	m := New(Deps{
		Config: initial,
		SaveConfig: func(_ context.Context, updated config.Config) error {
			got = updated
			return nil
		},
	})
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(",")})
	m = updated.(appModel)
	if !m.settings.open || m.settings.values[20] != "old@example.test" {
		t.Fatalf("settings = %#v, want opened with current values", m.settings)
	}
	m = m.changeSettingsScope(6)
	m.settings.input.SetValue("ops@example.test")
	m = m.moveSettingsField(1)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight}) // staging
	m = updated.(appModel)
	updated, command := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil {
		t.Fatal("save did not return a command")
	}
	updated, _ = updated.(appModel).Update(command())
	m = updated.(appModel)
	if got.SSL.Email != "ops@example.test" || !got.SSL.Staging {
		t.Fatalf("saved settings = %#v", got.SSL)
	}
	if m.settings.open || m.deps.Config.SSL != got.SSL || !strings.HasPrefix(m.status, "settings saved") {
		t.Fatalf("saved model = settings:%#v config:%#v status:%q", m.settings, m.deps.Config.SSL, m.status)
	}
}

func TestModel_SettingsPopupFitsTerminal(t *testing.T) {
	m := New(Deps{Config: config.Config{SSL: config.SSL{Email: "ops@example.test", Staging: true}}})
	m.ready, m.width, m.height = true, 100, 28
	m = m.openSettings()
	for index, line := range strings.Split(m.View(), "\n") {
		if got := lipgloss.Width(line); got != m.width {
			t.Fatalf("line %d width = %d, want %d: %q", index, got, m.width, line)
		}
	}
}

func TestModel_SettingsTabsFitMinimumTerminal(t *testing.T) {
	m := New(Deps{})
	m.width, m.height = minWidth, minHeight
	m = m.openSettings()
	tabs := m.settingsTabs()
	if got, want := lipgloss.Width(tabs), m.popupInnerW(popupLarge); got > want {
		t.Fatalf("tab strip width = %d, inner popup width = %d:\n%s", got, want, tabs)
	}
	if got := lipgloss.Height(tabs); got != 3 {
		t.Fatalf("tab strip height = %d, want bordered tabs with height 3:\n%s", got, tabs)
	}
	for _, label := range settingTabLabels {
		if !strings.Contains(tabs, label) {
			t.Errorf("tab strip does not contain %q:\n%s", label, tabs)
		}
	}
}

func TestModel_PathPickerSelectsAbsoluteDirectoryForSetting(t *testing.T) {
	root := t.TempDir()
	selected := filepath.Join(root, "project")
	if err := os.Mkdir(selected, 0o755); err != nil {
		t.Fatal(err)
	}
	m := New(Deps{
		Config: config.Config{Paths: config.Paths{VHosts: root}},
		BrowsePath: func(_ context.Context, path string, mode fsbrowse.Mode) (string, []fsbrowse.Entry, error) {
			return fsbrowse.Browse(path, mode)
		},
	})
	m.ready, m.width, m.height = true, 100, 28
	m = m.openSettings()
	m = m.changeSettingsScope(1) // Paths; VHosts is the first field.

	updated, command := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil {
		t.Fatal("opening a path field did not return a listing command")
	}
	updated, _ = updated.(appModel).Update(command())
	m = updated.(appModel)
	if !m.pathPicker.open || len(m.pathPicker.entries) < 2 {
		t.Fatalf("picker state = %#v", m.pathPicker)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // Skip synthetic parent.
	updated, _ = updated.(appModel).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m = updated.(appModel)
	if m.pathPicker.confirm != selected {
		t.Fatalf("confirmation path = %q, want %q", m.pathPicker.confirm, selected)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(appModel)
	if m.pathPicker.open || m.settings.values[1] != selected {
		t.Fatalf("selected setting = %q, picker open = %t", m.settings.values[1], m.pathPicker.open)
	}
}

func TestModel_PathPickerEnterNavigatesButDoesNotSelect(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	m := New(Deps{
		Config: config.Config{Paths: config.Paths{VHosts: root}},
		BrowsePath: func(_ context.Context, path string, mode fsbrowse.Mode) (string, []fsbrowse.Entry, error) {
			return fsbrowse.Browse(path, mode)
		},
	})
	m = m.openSettings()
	m = m.changeSettingsScope(1)
	updated, command := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated, _ = updated.(appModel).Update(command())
	updated, _ = updated.(appModel).Update(tea.KeyMsg{Type: tea.KeyDown})
	updated, command = updated.(appModel).Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil {
		t.Fatal("enter on a directory did not navigate")
	}
	updated, _ = updated.(appModel).Update(command())
	m = updated.(appModel)
	if m.pathPicker.dir != child || m.pathPicker.confirm != "" || m.settings.values[1] != root {
		t.Fatalf("after navigation: dir=%q confirm=%q value=%q", m.pathPicker.dir, m.pathPicker.confirm, m.settings.values[1])
	}
}

func TestModel_PathPickerPopupFitsTerminal(t *testing.T) {
	m := New(Deps{Config: config.Config{Paths: config.Paths{VHosts: "/does/not/exist"}}})
	m.ready, m.width, m.height = true, 100, 28
	m = m.openSettings()
	m = m.changeSettingsScope(1)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(appModel)
	for index, line := range strings.Split(m.View(), "\n") {
		if got := lipgloss.Width(line); got != m.width {
			t.Fatalf("line %d width = %d, want %d: %q", index, got, m.width, line)
		}
	}
}

func TestModel_DocumentRootFormConfirmsAndUpdatesSelectedDomain(t *testing.T) {
	var gotSubscription, gotDomain, gotRoot string
	m := New(Deps{
		SetWebsiteDocumentRoot: func(_ context.Context, subscription, domain, root string) (int64, error) {
			gotSubscription, gotDomain, gotRoot = subscription, domain, root
			return 1, nil
		},
		LoadWebsites: func(_ context.Context, _ int64) ([]domain.Website, error) {
			return []domain.Website{{ID: 2, PrimaryDomain: "example.test", Type: domain.WebsiteStatic, DocumentRoot: "/vhosts/acme/new-public"}}, nil
		},
	})
	m.items = []domain.Subscription{{ID: 1, Name: "acme"}}
	m.websites = []domain.Website{{ID: 2, PrimaryDomain: "example.test", Type: domain.WebsiteStatic, DocumentRoot: "/vhosts/acme/public"}}
	m.workspace, m.showWebsites, m.focus = true, true, focusWebsites
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("E")})
	m = updated.(appModel)
	if !m.documentRootForm.open {
		t.Fatal("document root form did not open")
	}
	m.documentRootForm.input.SetValue("/vhosts/acme/new-public")
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = updated.(appModel)
	if m.documentRootForm.open || m.confirm.action != "set-document-root" {
		t.Fatalf("form/confirmation = %#v / %#v", m.documentRootForm, m.confirm)
	}
	updated, command := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = runProgress(t, updated.(appModel), command)
	if gotSubscription != "acme" || gotDomain != "example.test" || gotRoot != "/vhosts/acme/new-public" {
		t.Fatalf("SetWebsiteDocumentRoot(%q, %q, %q)", gotSubscription, gotDomain, gotRoot)
	}
	if got := m.websites[0].DocumentRoot; got != "/vhosts/acme/new-public" {
		t.Errorf("refreshed root = %q", got)
	}
}

func TestModel_DocumentRootFormFitsTerminal(t *testing.T) {
	m := New(Deps{})
	m.ready, m.width, m.height = true, 100, 28
	m.items = []domain.Subscription{{ID: 1, Name: "acme"}}
	m.websites = []domain.Website{{ID: 2, PrimaryDomain: "example.test", Type: domain.WebsiteStatic, DocumentRoot: "/vhosts/acme/sites/example.test/public"}}
	m.workspace, m.showWebsites, m.focus = true, true, focusWebsites
	m = m.openDocumentRootForm()
	for index, line := range strings.Split(m.View(), "\n") {
		if got := lipgloss.Width(line); got != m.width {
			t.Fatalf("line %d width = %d, want %d: %q", index, got, m.width, line)
		}
	}
}

func TestModel_WebsiteCreateFormCreatesSelectedType(t *testing.T) {
	var gotSubscription, gotDomain, gotTarget string
	var gotType domain.WebsiteType
	m := New(Deps{
		CreateWebsite: func(_ context.Context, subscription, name string, kind domain.WebsiteType, target string, code int) (int64, error) {
			gotSubscription, gotDomain, gotType, gotTarget = subscription, name, kind, target
			if code != 301 {
				t.Errorf("redirect code = %d", code)
			}
			return 1, nil
		},
		LoadWebsites: func(context.Context, int64) ([]domain.Website, error) {
			return []domain.Website{{PrimaryDomain: "api.example.test", Type: domain.WebsiteProxy, Target: "http://127.0.0.1:8080"}}, nil
		},
	})
	m.items = []domain.Subscription{{ID: 1, Name: "acme"}}
	m.workspace, m.showWebsites, m.focus = true, true, focusWebsites
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = updated.(appModel)
	if !m.websiteCreateForm.open {
		t.Fatal("website create form did not open")
	}
	m.websiteCreateForm.domain.SetValue("api.example.test")
	m.websiteCreateForm.typeIndex, m.websiteCreateForm.field = 2, 2
	m.websiteCreateForm.target.SetValue("http://127.0.0.1:8080")
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = updated.(appModel)
	if m.websiteCreateForm.open || m.confirm.action != "create-website" {
		t.Fatalf("form/confirmation = %#v / %#v", m.websiteCreateForm, m.confirm)
	}
	updated, command := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = runProgress(t, updated.(appModel), command)
	if gotSubscription != "acme" || gotDomain != "api.example.test" || gotType != domain.WebsiteProxy || gotTarget != "http://127.0.0.1:8080" {
		t.Fatalf("CreateWebsite(%q, %q, %q, %q)", gotSubscription, gotDomain, gotType, gotTarget)
	}
	if len(m.websites) != 1 || m.websites[0].PrimaryDomain != "api.example.test" {
		t.Fatalf("refreshed domains = %#v", m.websites)
	}
}

func TestModel_AliasFormAppliesAndRefreshes(t *testing.T) {
	var gotSubscription, gotDomain, gotAlias string
	var gotAdd bool
	m := New(Deps{
		SetWebsiteAlias: func(_ context.Context, subscription, domain, alias string, add bool) (int64, error) {
			gotSubscription, gotDomain, gotAlias, gotAdd = subscription, domain, alias, add
			return 1, nil
		},
		LoadWebsites: func(context.Context, int64) ([]domain.Website, error) {
			return []domain.Website{{PrimaryDomain: "example.test", Type: domain.WebsiteStatic, Aliases: []string{"www.example.test"}}}, nil
		},
	})
	m.items = []domain.Subscription{{ID: 1, Name: "acme"}}
	m.websites = []domain.Website{{PrimaryDomain: "example.test", Type: domain.WebsiteStatic}}
	m.workspace, m.showWebsites, m.focus = true, true, focusWebsites
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = updated.(appModel)
	if !m.aliasForm.open || !m.aliasForm.add {
		t.Fatalf("alias form = %#v", m.aliasForm)
	}
	m.aliasForm.input.SetValue("www.example.test")
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = updated.(appModel)
	if m.aliasForm.open || m.confirm.action != "set-alias" {
		t.Fatalf("form/confirmation = %#v / %#v", m.aliasForm, m.confirm)
	}
	updated, command := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = runProgress(t, updated.(appModel), command)
	if gotSubscription != "acme" || gotDomain != "example.test" || gotAlias != "www.example.test" || !gotAdd {
		t.Fatalf("SetWebsiteAlias(%q, %q, %q, %t)", gotSubscription, gotDomain, gotAlias, gotAdd)
	}
	if got := m.websites[0].Aliases; len(got) != 1 || got[0] != "www.example.test" {
		t.Fatalf("refreshed aliases = %#v", got)
	}
}

func TestModel_TargetFormUpdatesRedirect(t *testing.T) {
	var target string
	var code int
	m := New(Deps{
		SetWebsiteTarget: func(_ context.Context, _, _, value string, redirectCode int) (int64, error) {
			target, code = value, redirectCode
			return 1, nil
		},
		LoadWebsites: func(context.Context, int64) ([]domain.Website, error) {
			return []domain.Website{{PrimaryDomain: "go.example.test", Type: domain.WebsiteRedirect, Target: "https://new.example.test", RedirectCode: 302}}, nil
		},
	})
	m.items = []domain.Subscription{{ID: 1, Name: "acme"}}
	m.websites = []domain.Website{{PrimaryDomain: "go.example.test", Type: domain.WebsiteRedirect, Target: "https://old.example.test", RedirectCode: 301}}
	m.workspace, m.showWebsites, m.focus = true, true, focusWebsites
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("T")})
	m = updated.(appModel)
	m.targetForm.input.SetValue("https://new.example.test")
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	updated, _ = updated.(appModel).Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	updated, command := updated.(appModel).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = runProgress(t, updated.(appModel), command)
	if target != "https://new.example.test" || code != 302 || m.websites[0].RedirectCode != 302 {
		t.Fatalf("target/code = %q/%d; domains=%#v", target, code, m.websites)
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
	m.sshKeys = []domain.SSHKey{{Fingerprint: "SHA256:old"}}
	m.detailView = detailSSHKeys
	m.workspace, m.showWebsites, m.focus = true, true, focusSubscriptions
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(appModel)
	if m.cursor != 1 || m.showWebsites || len(m.websites) != 0 || len(m.databases) != 0 || len(m.sshKeys) != 0 || m.detailView != detailDomain {
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

func TestModel_TLSToggleCallsDependency(t *testing.T) {
	var called bool
	m := New(Deps{SetWebsiteTLS: func(_ context.Context, subscription, domain string, enabled bool) error {
		called = subscription == "acme" && domain == "example.test" && enabled
		return nil
	}})
	m.items = []domain.Subscription{{ID: 1, Name: "acme"}}
	m.workspace, m.showWebsites, m.focus = true, true, focusWebsites
	m.websites = []domain.Website{{PrimaryDomain: "example.test", SSLEnabled: false}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	m = updated.(appModel)
	if m.confirm.action != "set-tls" || !m.confirm.enabled {
		t.Fatalf("TLS confirmation = %#v", m.confirm)
	}
	updated, command := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	_ = runProgress(t, updated.(appModel), command)
	if !called {
		t.Fatal("SetWebsiteTLS was not called")
	}
}

func TestModel_PHPVersionPickerChangesSelectedDomain(t *testing.T) {
	var changedSubscription, changedDomain, changedVersion string
	m := New(Deps{
		LoadPHPVersions: func(context.Context) ([]service.PHPFPMVersion, error) {
			return []service.PHPFPMVersion{{Version: "8.3", Active: true}, {Version: "8.4", Active: true}}, nil
		},
		SetWebsitePHP: func(_ context.Context, subscription, domain string, options service.PHPSetOptions) (int64, error) {
			changedSubscription, changedDomain, changedVersion = subscription, domain, options.Version
			return 1, nil
		},
	})
	m.items = []domain.Subscription{{ID: 1, Name: "acme", Status: "active"}}
	m.workspace, m.showWebsites, m.focus = true, true, focusWebsites
	m.websites = []domain.Website{{ID: 1, PrimaryDomain: "app.example.test", Type: domain.WebsitePHPFPM, PHPVersion: "8.3"}}
	updated, command := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m = updated.(appModel)
	if !m.phpPicker.open || !m.phpPicker.loading {
		t.Fatalf("PHP picker state = %#v", m.phpPicker)
	}
	updated, _ = m.Update(command())
	m = updated.(appModel)
	if m.phpPicker.loading || len(m.phpPicker.items) != 2 {
		t.Fatalf("loaded PHP picker = %#v", m.phpPicker)
	}
	if m.phpPicker.cursor != 0 || !strings.Contains(m.phpPickerPopup(), "selected") {
		t.Fatalf("PHP picker does not mark the current domain version: %#v\n%s", m.phpPicker, m.phpPickerPopup())
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(appModel)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(appModel)
	if m.confirm.action != "set-php" || m.confirm.domain != "8.4" {
		t.Fatalf("PHP confirmation = %#v", m.confirm)
	}
	updated, command = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	_ = runProgress(t, updated.(appModel), command)
	if changedSubscription != "acme" || changedDomain != "app.example.test" || changedVersion != "8.4" {
		t.Fatalf("SetWebsitePHP(%q, %q, %q), want acme, app.example.test, 8.4", changedSubscription, changedDomain, changedVersion)
	}
}

func TestModel_PHPVersionPickerRejectsCurrentVersionWithoutMutation(t *testing.T) {
	m := New(Deps{LoadPHPVersions: func(context.Context) ([]service.PHPFPMVersion, error) {
		return []service.PHPFPMVersion{{Version: "8.4", Active: true}}, nil
	}})
	m.items = []domain.Subscription{{ID: 1, Name: "acme", Status: "active"}}
	m.workspace, m.showWebsites, m.focus = true, true, focusWebsites
	m.websites = []domain.Website{{ID: 1, PrimaryDomain: "app.example.test", Type: domain.WebsitePHPFPM, PHPVersion: "8.4"}}
	updated, command := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m = updated.(appModel)
	updated, _ = m.Update(command())
	m = updated.(appModel)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(appModel)
	if m.phpPicker.open || m.confirm.action != "" || !strings.Contains(m.status, "already uses PHP-FPM 8.4") {
		t.Fatalf("current PHP selection = picker:%v confirm:%#v status:%q", m.phpPicker.open, m.confirm, m.status)
	}
}

func TestModel_SubscriptionMetadataDoesNotPresentPHPVersion(t *testing.T) {
	m := New(Deps{})
	m.items = []domain.Subscription{{ID: 1, Name: "acme", Home: "/vhosts/acme", PHPVersion: "8.4"}}
	if got := m.subscriptionMetadata(); strings.Contains(got, "PHP") {
		t.Fatalf("subscription metadata presents a domain-scoped PHP version: %q", got)
	}
	if got := m.renderSubscriptions(1); strings.Contains(got, "8.4") {
		t.Fatalf("subscription row presents a domain-scoped PHP version: %q", got)
	}
}

func TestModel_ProgressUsesModalAndPreservesOutputPanel(t *testing.T) {
	m := New(Deps{})
	m.ready, m.width, m.height, m.workspace = true, 100, 28, true
	m.items = []domain.Subscription{{ID: 1, Name: "acme"}}
	m.websites, m.showWebsites = []domain.Website{{PrimaryDomain: "app.example.test"}}, true
	m.output = outputState{}.append("previous operation")
	m.progress = progressState{title: "Switch PHP-FPM", active: true, steps: []progressStep{{label: "replace domain PHP-FPM pool", state: stepRunning}, {label: "regenerate Apache vhost", state: stepPending}}}

	view := m.View()
	if !strings.Contains(view, "Switch PHP-FPM") || !strings.Contains(view, "replace domain PHP-FPM pool") {
		t.Fatalf("progress modal missing from view:\n%s", view)
	}
	if strings.Contains(m.outputLines(3), "replace domain PHP-FPM pool") || !strings.Contains(m.outputLines(3), "previous operation") {
		t.Fatalf("progress leaked into persistent output: %#v", m.output)
	}
}

func TestModel_PHPChangeStreamsApplyAndRefreshSteps(t *testing.T) {
	m := New(Deps{
		SetWebsitePHP: func(context.Context, string, string, service.PHPSetOptions) (int64, error) { return 1, nil },
		LoadWebsites: func(context.Context, int64) ([]domain.Website, error) {
			return []domain.Website{{ID: 1, PrimaryDomain: "app.example.test", Type: domain.WebsitePHPFPM, PHPVersion: "8.3"}}, nil
		},
	})
	m.items = []domain.Subscription{{ID: 1, Name: "acme"}}
	m.websites, m.workspace, m.showWebsites, m.focus = []domain.Website{{ID: 1, PrimaryDomain: "app.example.test", Type: domain.WebsitePHPFPM, PHPVersion: "8.4"}}, true, true, focusWebsites

	command := m.changeWebsitePHPCmd(confirmState{domain: "8.3"})
	updated, next := m.Update(command())
	m = updated.(appModel)
	if !m.progress.active || m.progress.steps[0].state != stepPending {
		t.Fatalf("progress start = %#v", m.progress)
	}
	if m.focus != focusWebsites {
		t.Fatalf("progress changed operation focus to %v", m.focus)
	}
	updated, next = m.Update(next())
	m = updated.(appModel)
	if m.progress.steps[0].state != stepRunning {
		t.Fatalf("apply step = %#v", m.progress.steps)
	}
	updated, next = m.Update(next())
	m = updated.(appModel)
	if m.progress.steps[0].state != stepDone || m.progress.steps[1].state != stepPending {
		t.Fatalf("apply completion = %#v", m.progress.steps)
	}
	updated, next = m.Update(next())
	m = updated.(appModel)
	if m.progress.steps[1].state != stepRunning {
		t.Fatalf("refresh start = %#v", m.progress.steps)
	}
	updated, next = m.Update(next())
	m = updated.(appModel)
	if m.progress.steps[1].state != stepDone {
		t.Fatalf("refresh completion = %#v", m.progress.steps)
	}
	updated, _ = m.Update(next())
	m = updated.(appModel)
	if m.progress.active || m.websites[0].PHPVersion != "8.3" {
		t.Fatalf("final PHP refresh = progress:%#v websites:%#v", m.progress, m.websites)
	}
	if m.focus != focusWebsites {
		t.Fatalf("completed operation changed focus to %v", m.focus)
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
	if !m.help.open || m.help.filter.active || m.help.filter.query() != "d" {
		t.Fatalf("escape did not apply active help filter: %#v", m.help)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(appModel)
	if !m.help.open || m.help.filter.query() != "" {
		t.Fatal("second escape did not clear applied help filter")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if updated.(appModel).help.open {
		t.Fatal("third escape did not close help")
	}
}

func TestModel_HelpPopupFitsAndPreservesFooter(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {100, 28}, {160, 50}} {
		m := New(Deps{})
		m.ready, m.width, m.height = true, size[0], size[1]
		m.help = helpState{open: true, filter: newFilter()}
		popup := m.helpPopup()
		if got := lipgloss.Width(popup); got > m.width {
			t.Fatalf("popup width at %dx%d = %d", m.width, m.height, got)
		}
		if got := lipgloss.Height(popup); got > m.height {
			t.Fatalf("popup height at %dx%d = %d", m.width, m.height, got)
		}
		if !strings.Contains(popup, "filter this help") {
			t.Fatalf("help footer disappeared at %dx%d:\n%s", m.width, m.height, popup)
		}
	}
}

func TestModel_AppliedHelpFilterAlsoFitsPopup(t *testing.T) {
	m := New(Deps{})
	m.ready, m.width, m.height = true, minWidth, minHeight
	m.help = helpState{open: true, filter: newFilter()}
	m.help.filter.input.SetValue("subscription")
	popup := m.helpPopup()
	if got := lipgloss.Height(popup); got != 16 {
		t.Fatalf("applied filter popup height = %d, want fixed 16", got)
	}
	if !strings.Contains(popup, "esc clear") || !strings.Contains(popup, "filter this help") {
		t.Fatalf("applied filter lost chrome:\n%s", popup)
	}
}

func TestModel_HelpFilterDropsUnmatchedSections(t *testing.T) {
	m := New(Deps{})
	m.help = helpState{open: true, filter: newFilter()}
	m.help.filter.input.SetValue("TLS")
	rows := strings.Join(m.filteredHelpRows(), "\n")
	if !strings.Contains(rows, "Domains") || strings.Contains(rows, "Subscriptions") {
		t.Fatalf("filtered help sections = %q", rows)
	}
}

func TestModel_SubscriptionFilterShowsCountAndClearsFromWorkspace(t *testing.T) {
	m := New(Deps{})
	m.ready, m.width, m.height, m.workspace = true, 100, 28, true
	m.items = []domain.Subscription{{Name: "acme", Status: "active"}, {Name: "beta", Status: "archived"}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = updated.(appModel)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = updated.(appModel)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = updated.(appModel)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	m = updated.(appModel)
	if !strings.Contains(m.keybar(), "1/2 matches") || !strings.Contains(m.subscriptionPanelTitle(), "1/2") {
		t.Fatalf("filter indicators = %q / %q", m.keybar(), m.subscriptionPanelTitle())
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(appModel)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(appModel)
	if m.subscriptionFilter.query() != "" || m.cursor != 0 {
		t.Fatalf("escape did not clear applied subscription filter: %#v", m.subscriptionFilter)
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

func TestModel_CreateSubscriptionCallsDependency(t *testing.T) {
	var created string
	m := New(Deps{
		CreateSubscription: func(_ context.Context, name string) (int64, error) {
			created = name
			return 1, nil
		},
		LoadSubscriptions: func(context.Context) ([]domain.Subscription, error) {
			return []domain.Subscription{{ID: 1, Name: "acme", Status: "active"}}, nil
		},
	})
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = updated.(appModel)
	if !m.subscriptionCreateForm.open {
		t.Fatal("subscription creation form did not open")
	}
	for _, character := range "acme" {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{character}})
		m = updated.(appModel)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = updated.(appModel)
	if m.confirm.action != "create-subscription" || m.confirm.domain != "acme" {
		t.Fatalf("confirmation = %#v", m.confirm)
	}
	updated, command := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = runProgress(t, updated.(appModel), command)
	if created != "acme" {
		t.Fatalf("CreateSubscription name = %q, want acme", created)
	}
	if !strings.Contains(m.status, "refreshing") {
		t.Fatalf("status = %q, want refresh", m.status)
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

func TestModel_DeleteWebsiteRequiresTypedDomain(t *testing.T) {
	deleted := false
	m := New(Deps{
		DeleteWebsite: func(_ context.Context, subscription, domain string) (int64, error) {
			deleted = subscription == "acme" && domain == "api.example.test"
			return 1, nil
		},
		LoadWebsites: func(context.Context, int64) ([]domain.Website, error) { return nil, nil },
	})
	m.workspace, m.showWebsites, m.focus = true, true, focusWebsites
	m.items = []domain.Subscription{{ID: 1, Name: "acme", Status: "active"}}
	m.websites = []domain.Website{{ID: 2, PrimaryDomain: "api.example.test", Type: domain.WebsiteStatic}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("D")})
	m = updated.(appModel)
	if m.confirm.action != "delete-website" || m.confirm.word != "api.example.test" {
		t.Fatalf("delete confirmation = %#v", m.confirm)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(appModel)
	if deleted || m.confirm.err == "" {
		t.Fatalf("empty confirmation unexpectedly deleted: %#v", m.confirm)
	}
	for _, character := range "api.example.test" {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{character}})
		m = updated.(appModel)
	}
	updated, command := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = runProgress(t, updated.(appModel), command)
	if !deleted || len(m.websites) != 0 {
		t.Fatalf("website deletion = %t, websites = %#v", deleted, m.websites)
	}
}

func TestModel_ReconcileUsesSelectedSubscription(t *testing.T) {
	var reconciled string
	m := New(Deps{
		Reconcile: func(_ context.Context, subscription string) (int64, error) {
			reconciled = subscription
			return 17, nil
		},
		LoadWebsites: func(context.Context, int64) ([]domain.Website, error) { return nil, nil },
	})
	m.workspace, m.focus = true, focusSubscriptions
	m.items = []domain.Subscription{{ID: 1, Name: "acme", Status: "active"}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
	m = updated.(appModel)
	if m.confirm.action != "reconcile" || m.confirm.domain != "acme" {
		t.Fatalf("confirmation = %#v", m.confirm)
	}
	updated, command := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = runProgress(t, updated.(appModel), command)
	if reconciled != "acme" || !strings.Contains(m.status, "reconciled") {
		t.Fatalf("reconciled=%q status=%q", reconciled, m.status)
	}
}

func TestModel_LoadDatabasesShowsDatabaseDetail(t *testing.T) {
	m := New(Deps{LoadDatabases: func(_ context.Context, subscription string) ([]domain.Database, error) {
		if subscription != "acme" {
			t.Fatalf("LoadDatabases subscription = %q", subscription)
		}
		return []domain.Database{{Name: "acme_app", User: "acme_app", Host: "localhost", Charset: "utf8mb4"}}, nil
	}})
	m.workspace, m.showWebsites, m.focus = true, true, focusWebsites
	m.items = []domain.Subscription{{ID: 1, Name: "acme", Status: "active"}}
	m.websites = []domain.Website{{PrimaryDomain: "www.example.test", Type: domain.WebsiteStatic}, {PrimaryDomain: "api.example.test", Type: domain.WebsiteStatic}}
	updated, command := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	updated, _ = updated.(appModel).Update(command())
	m = updated.(appModel)
	if m.detailView != detailDatabases || m.focus != focusDetail || !strings.Contains(m.detail(), "acme_app") || m.detailTitle() != "Databases" {
		t.Fatalf("detail view = %v focus = %v detail = %q title = %q", m.detailView, m.focus, m.detail(), m.detailTitle())
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = updated.(appModel)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(appModel)
	if m.detailView != detailDomain {
		t.Fatal("changing the selected domain did not restore domain detail")
	}
}

func TestModel_LoadSSHKeysShowsSSHKeyDetail(t *testing.T) {
	m := New(Deps{LoadSSHKeys: func(_ context.Context, subscription string) ([]domain.SSHKey, error) {
		if subscription != "acme" {
			t.Fatalf("LoadSSHKeys subscription = %q", subscription)
		}
		return []domain.SSHKey{{Fingerprint: "SHA256:example", Comment: "operator@example.test"}}, nil
	}})
	m.workspace = true
	m.items = []domain.Subscription{{ID: 1, Name: "acme", Status: "active"}}
	updated, command := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("K")})
	updated, _ = updated.(appModel).Update(command())
	m = updated.(appModel)
	if m.detailView != detailSSHKeys || m.focus != focusDetail || !strings.Contains(m.detail(), "SHA256:example") || m.detailTitle() != "SSH keys" {
		t.Fatalf("detail view = %v focus = %v detail = %q title = %q", m.detailView, m.focus, m.detail(), m.detailTitle())
	}
}

func TestModel_LoadCronJobsShowsCronDetail(t *testing.T) {
	m := New(Deps{LoadCronJobs: func(_ context.Context, subscription string) ([]domain.CronJob, error) {
		if subscription != "acme" {
			t.Fatalf("LoadCronJobs subscription = %q", subscription)
		}
		return []domain.CronJob{{ID: 7, Schedule: "@daily", Command: "/usr/local/bin/backup", Comment: "backup"}}, nil
	}})
	m.workspace = true
	m.items = []domain.Subscription{{ID: 1, Name: "acme", Status: "active"}}
	updated, command := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	updated, _ = updated.(appModel).Update(command())
	m = updated.(appModel)
	if m.detailView != detailCronJobs || m.focus != focusDetail || !strings.Contains(m.detail(), "@daily") || m.detailTitle() != "Cron jobs" {
		t.Fatalf("detail view = %v focus = %v detail = %q title = %q", m.detailView, m.focus, m.detail(), m.detailTitle())
	}
}

func TestModel_LoadBackupsShowsBackupDetail(t *testing.T) {
	m := New(Deps{LoadBackups: func(_ context.Context, subscription string) ([]domain.Backup, error) {
		if subscription != "acme" {
			t.Fatalf("LoadBackups subscription = %q", subscription)
		}
		return []domain.Backup{{ID: 9, Status: "completed", SizeBytes: 2048}}, nil
	}})
	m.workspace = true
	m.items = []domain.Subscription{{ID: 1, Name: "acme", Status: "active"}}
	updated, command := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("V")})
	updated, _ = updated.(appModel).Update(command())
	m = updated.(appModel)
	if m.detailView != detailBackups || m.focus != focusDetail || !strings.Contains(m.detail(), "2048 bytes") || m.detailTitle() != "Backups" {
		t.Fatalf("detail view = %v focus = %v detail = %q title = %q", m.detailView, m.focus, m.detail(), m.detailTitle())
	}
}

func TestModel_SSHAccessShowsPasswordOnlyInSecretPopup(t *testing.T) {
	const password = "must-not-reach-output"
	m := New(Deps{
		SetSSHAccess: func(_ context.Context, subscription, access string) (string, int64, error) {
			if subscription != "acme" || access != "password" {
				t.Fatalf("SetSSHAccess(%q, %q)", subscription, access)
			}
			return password, 11, nil
		},
		LoadSubscriptions: func(context.Context) ([]domain.Subscription, error) {
			return []domain.Subscription{{ID: 1, Name: "acme", SSHAccess: "password"}}, nil
		},
	})
	m.workspace, m.focus = true, focusSubscriptions
	m.items = []domain.Subscription{{ID: 1, Name: "acme", SSHAccess: "none"}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	m = updated.(appModel)
	if !m.sshAccessForm.open {
		t.Fatal("SSH access form did not open")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(appModel)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(appModel)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(appModel)
	if m.confirm.action != "set-ssh-access" || m.confirm.value != "password" {
		t.Fatalf("confirmation = %#v", m.confirm)
	}
	updated, command := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = runProgress(t, updated.(appModel), command)
	if !m.secret.open || m.secret.secret != password {
		t.Fatalf("secret popup = %#v", m.secret)
	}
	if strings.Contains(strings.Join(m.output.lines, "\n"), password) {
		t.Fatal("generated SSH password leaked into persistent output")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(appModel)
	if m.secret.open {
		t.Fatal("secret popup did not acknowledge")
	}
}

func TestModel_PathPickerSetsSSHKeyFile(t *testing.T) {
	root := t.TempDir()
	key := filepath.Join(root, "operator.pub")
	if err := os.WriteFile(key, []byte("ssh-ed25519 AAAA example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := New(Deps{BrowsePath: func(_ context.Context, path string, mode fsbrowse.Mode) (string, []fsbrowse.Entry, error) {
		return fsbrowse.Browse(path, mode)
	}})
	m.sshKeyForm = sshKeyFormState{open: true, input: newFilter().input}
	m.sshKeyForm.input.SetValue(root)
	updated, command := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated, _ = updated.(appModel).Update(command())
	m = updated.(appModel)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // skip synthetic parent
	m = updated.(appModel)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m = updated.(appModel)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(appModel)
	if m.pathPicker.open || m.sshKeyForm.input.Value() != key {
		t.Fatalf("picker/form = %#v / %q", m.pathPicker, m.sshKeyForm.input.Value())
	}
}

func TestModel_AddSSHKeyUsesServiceAndRefreshesKeys(t *testing.T) {
	var path string
	m := New(Deps{
		AddSSHKeyFromFile: func(_ context.Context, subscription, file string) (int64, error) {
			if subscription != "acme" {
				t.Fatalf("subscription = %q", subscription)
			}
			path = file
			return 1, nil
		},
		LoadSSHKeys: func(context.Context, string) ([]domain.SSHKey, error) {
			return []domain.SSHKey{{Fingerprint: "SHA256:new"}}, nil
		},
	})
	m.workspace, m.focus, m.detailView = true, focusDetail, detailSSHKeys
	m.items = []domain.Subscription{{ID: 1, Name: "acme"}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("+")})
	m = updated.(appModel)
	if !m.sshKeyForm.open {
		t.Fatal("SSH key form did not open")
	}
	m.sshKeyForm.input.SetValue("/tmp/operator.pub")
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = updated.(appModel)
	updated, command := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = runProgress(t, updated.(appModel), command)
	if path != "/tmp/operator.pub" || len(m.sshKeys) != 1 || m.sshKeys[0].Fingerprint != "SHA256:new" {
		t.Fatalf("path=%q ssh keys=%#v", path, m.sshKeys)
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
	started := false
	for range 12 { // The number of progress messages follows the advertised steps.
		message := command()
		updated, next := model.Update(message)
		model, command = updated.(appModel), next
		if _, ok := message.(progressStartMsg); ok {
			started = true
		}
		if started && !model.progress.active {
			return model
		}
	}
	t.Fatal("progress did not finish")
	return appModel{}
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
