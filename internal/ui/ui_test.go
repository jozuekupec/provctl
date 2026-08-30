package ui

import (
	"context"
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
