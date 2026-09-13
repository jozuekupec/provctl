package ui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
)

// askDeleteWebsite requires the same exact-domain acknowledgement as the CLI.
// The service deliberately retains site data and logs, which the dialog states.
func (m appModel) askDeleteWebsite() appModel {
	website, ok := m.selectedWebsite()
	if !ok {
		m.status = "select a domain before deletion"
		return m
	}
	return m.askConfirm(confirmState{
		action: "delete-website", domain: website.PrimaryDomain, word: website.PrimaryDomain,
		title: "Delete domain configuration", lines: []string{
			"Domain: " + website.PrimaryDomain,
			"Apache configuration and managed TLS metadata will be removed.",
			"Site data and logs will be retained.",
		},
	})
}

func (m appModel) deleteWebsite(ctx context.Context, confirm confirmState, report func(int)) tea.Msg {
	subscription, subscriptionOK := m.selectedSubscription()
	website, websiteOK := m.selectedWebsite()
	if !subscriptionOK || !websiteOK || website.PrimaryDomain != confirm.domain || m.deps.DeleteWebsite == nil {
		return websiteDeletedMsg{err: context.Canceled, domain: confirm.domain}
	}
	_, err := m.deps.DeleteWebsite(ctx, subscription.Name, confirm.domain)
	if err != nil || m.deps.LoadWebsites == nil {
		return websiteDeletedMsg{err: err, domain: confirm.domain}
	}
	report(1)
	items, err := m.deps.LoadWebsites(ctx, subscription.ID)
	return websiteDeletedMsg{err: err, domain: confirm.domain, items: items}
}

func (m appModel) deleteWebsiteCmd(confirm confirmState) tea.Cmd {
	return steppedCmd("Delete domain", []string{"remove generated domain resources", "refresh domain list"}, func(ctx context.Context, report func(int)) tea.Msg {
		return m.deleteWebsite(ctx, confirm, report)
	})
}
