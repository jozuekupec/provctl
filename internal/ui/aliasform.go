package ui

import (
	"context"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func (m appModel) openAliasForm(add bool) appModel {
	website, ok := m.selectedWebsite()
	if !ok {
		m.status = "select a domain before changing aliases"
		return m
	}
	input := textinput.New()
	input.Prompt = ""
	input.Focus()
	m.aliasForm = aliasFormState{open: true, add: add, domain: website.PrimaryDomain, input: input}
	m.status = ""
	return m
}

func (m appModel) handleAliasFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.aliasForm, m.status = aliasFormState{}, "cancelled"
		return m, nil
	case "ctrl+s":
		alias, add, domain := m.aliasForm.input.Value(), m.aliasForm.add, m.aliasForm.domain
		m.aliasForm = aliasFormState{}
		verb := map[bool]string{true: "Add", false: "Remove"}[add]
		line := map[bool]string{true: "TLS sites reissue the certificate with the complete SAN list.", false: "TLS sites reissue the certificate without this SAN."}[add]
		return m.askConfirm(confirmState{action: "set-alias", domain: domain, value: alias, enabled: add, title: verb + " alias", lines: []string{"Domain: " + domain, "Alias: " + alias, line}}), nil
	}
	input, command := m.aliasForm.input.Update(msg)
	m.aliasForm.input = input
	return m, command
}

func (m appModel) changeWebsiteAlias(ctx context.Context, confirm confirmState, report func(int)) tea.Msg {
	subscription, ok := m.selectedSubscription()
	if m.deps.SetWebsiteAlias == nil || !ok {
		return websiteAliasChangedMsg{err: context.Canceled}
	}
	_, err := m.deps.SetWebsiteAlias(ctx, subscription.Name, confirm.domain, confirm.value, confirm.enabled)
	if err != nil || m.deps.LoadWebsites == nil {
		return websiteAliasChangedMsg{err: err, domain: confirm.domain, alias: confirm.value, add: confirm.enabled}
	}
	report(1)
	items, err := m.deps.LoadWebsites(ctx, subscription.ID)
	return websiteAliasChangedMsg{err: err, domain: confirm.domain, alias: confirm.value, add: confirm.enabled, items: items}
}

func (m appModel) changeWebsiteAliasCmd(confirm confirmState) tea.Cmd {
	title := map[bool]string{true: "Add alias", false: "Remove alias"}[confirm.enabled]
	return steppedCmd(title, []string{"apply domain configuration", "refresh domain list"}, func(ctx context.Context, report func(int)) tea.Msg {
		return m.changeWebsiteAlias(ctx, confirm, report)
	})
}

func (m appModel) aliasFormPopup() string {
	verb := map[bool]string{true: "Add", false: "Remove"}[m.aliasForm.add]
	body := []string{"Domain: " + m.aliasForm.domain, "", "Alias:", m.aliasForm.input.View()}
	return popupBox(m, popupOpts{Size: popupSmall, Fit: fitAuto, Title: verb + " alias", Body: body, Footer: []string{dimStyle.Render("ctrl+s continue · esc cancel")}})
}
