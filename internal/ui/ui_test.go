package ui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"provctl/internal/domain"
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
	loaded := m.loadWebsites().(websitesLoadedMsg)
	updated, _ := m.Update(loaded)
	if got := updated.(appModel).websites[0].PrimaryDomain; got != "example.test" {
		t.Errorf("domain = %q", got)
	}
}

func TestModel_DetailUsesSelectedWebsite(t *testing.T) {
	m := New(Deps{})
	m.showWebsites = true
	m.websites = []domain.Website{{PrimaryDomain: "example.test", Type: domain.WebsiteStatic, DocumentRoot: "/vhosts/acme/sites/example.test/public", Enabled: true}}
	if got := m.detail(); !strings.Contains(got, "Domain: example.test") {
		t.Errorf("detail = %q", got)
	}
}

func TestModel_ConfirmWebsiteToggleCallsDependency(t *testing.T) {
	called := false
	m := New(Deps{SetWebsiteEnabled: func(_ context.Context, subscription, domain string, enabled bool) (int64, error) {
		called = subscription == "acme" && domain == "example.test" && !enabled
		return 1, nil
	}})
	m.items = []domain.Subscription{{ID: 1, Name: "acme"}}
	m.websites, m.showWebsites = []domain.Website{{PrimaryDomain: "example.test", Enabled: true}}, true
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m = updated.(appModel)
	if m.confirm.action == "" {
		t.Fatal("toggle did not request confirmation")
	}
	updated, command := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if message := command(); message.(websiteChangedMsg).err != nil {
		t.Fatal(message.(websiteChangedMsg).err)
	}
	if !called {
		t.Fatal("SetWebsiteEnabled was not called")
	}
	_ = updated
}

func TestModel_ConfirmSubscriptionSuspendCallsDependency(t *testing.T) {
	called := false
	m := New(Deps{SetSubscriptionStatus: func(_ context.Context, name, status string) (int64, error) {
		called = name == "acme" && status == "suspended"
		return 1, nil
	}})
	m.items = []domain.Subscription{{ID: 1, Name: "acme", Status: "active"}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = updated.(appModel)
	if m.confirm.action != "suspended" {
		t.Fatalf("confirmation = %#v", m.confirm)
	}
	updated, command := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if message := command(); message.(subscriptionChangedMsg).err != nil {
		t.Fatal(message.(subscriptionChangedMsg).err)
	}
	if !called {
		t.Fatal("SetSubscriptionStatus was not called")
	}
	_ = updated
}
