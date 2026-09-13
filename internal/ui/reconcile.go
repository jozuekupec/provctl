package ui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
)

func (m appModel) reconcile(ctx context.Context, confirm confirmState, report func(int)) tea.Msg {
	subscription, ok := m.selectedSubscription()
	if !ok || subscription.Name != confirm.domain || m.deps.Reconcile == nil {
		return reconcileFinishedMsg{err: context.Canceled, subscription: confirm.domain}
	}
	operationID, err := m.deps.Reconcile(ctx, subscription.Name)
	if err != nil || m.deps.LoadWebsites == nil {
		return reconcileFinishedMsg{err: err, subscription: subscription.Name, operationID: operationID}
	}
	report(1)
	websites, err := m.deps.LoadWebsites(ctx, subscription.ID)
	return reconcileFinishedMsg{err: err, subscription: subscription.Name, operationID: operationID, websites: websites}
}

func (m appModel) reconcileCmd(confirm confirmState) tea.Cmd {
	return steppedCmd("Reconcile configuration", []string{"regenerate Apache configuration", "refresh domain list"}, func(ctx context.Context, report func(int)) tea.Msg {
		return m.reconcile(ctx, confirm, report)
	})
}
