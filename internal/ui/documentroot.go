package ui

import (
	"context"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func (m appModel) openDocumentRootForm() appModel {
	website, ok := m.selectedWebsite()
	if !ok || (website.Type != "static" && website.Type != "php-fpm") {
		m.status = "select a static or PHP-FPM domain to change its document root"
		return m
	}
	input := textinput.New()
	input.Prompt = ""
	input.SetValue(website.DocumentRoot)
	input.Focus()
	m.documentRootForm = documentRootFormState{open: true, website: website, input: input}
	m.status = ""
	return m
}

func (m appModel) handleDocumentRootFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.documentRootForm, m.status = documentRootFormState{}, "cancelled"
		return m, nil
	case "enter":
		return m.openPathPickerForDocumentRoot()
	case "ctrl+s":
		root := m.documentRootForm.input.Value()
		m.documentRootForm.input.Blur()
		website := m.documentRootForm.website
		m.documentRootForm = documentRootFormState{}
		return m.askConfirm(confirmState{action: "set-document-root", domain: website.PrimaryDomain, value: root, title: "Change document root", lines: []string{"Domain: " + website.PrimaryDomain, "New root: " + root, "Files will not be moved."}}), nil
	}
	input, command := m.documentRootForm.input.Update(msg)
	m.documentRootForm.input = input
	return m, command
}

func (m appModel) changeWebsiteDocumentRoot(ctx context.Context, confirm confirmState, report func(int)) tea.Msg {
	subscription, subscriptionOK := m.selectedSubscription()
	if m.deps.SetWebsiteDocumentRoot == nil || !subscriptionOK {
		return websiteDocumentRootChangedMsg{err: context.Canceled}
	}
	_, err := m.deps.SetWebsiteDocumentRoot(ctx, subscription.Name, confirm.domain, confirm.value)
	if err != nil || m.deps.LoadWebsites == nil {
		return websiteDocumentRootChangedMsg{err: err, domain: confirm.domain, root: confirm.value}
	}
	report(1)
	items, err := m.deps.LoadWebsites(ctx, subscription.ID)
	return websiteDocumentRootChangedMsg{err: err, domain: confirm.domain, root: confirm.value, items: items}
}

func (m appModel) changeWebsiteDocumentRootCmd(confirm confirmState) tea.Cmd {
	return steppedCmd("Change document root", []string{"apply Apache configuration", "refresh domain list"}, func(ctx context.Context, report func(int)) tea.Msg {
		return m.changeWebsiteDocumentRoot(ctx, confirm, report)
	})
}

func (m appModel) documentRootFormPopup() string {
	body := []string{"Domain: " + m.documentRootForm.website.PrimaryDomain, "", "Document root:", m.documentRootForm.input.View(), "", dimStyle.Render("The directory must already exist inside the subscription home. Files are not moved.")}
	return popupBox(m, popupOpts{Size: popupMedium, Fit: fitAuto, Title: "Change document root", Body: body, Footer: []string{dimStyle.Render("enter browse · ctrl+s continue · esc cancel")}})
}
