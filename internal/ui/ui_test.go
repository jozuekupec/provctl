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
