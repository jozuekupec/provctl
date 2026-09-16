package ui

import (
	"context"
	"path/filepath"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"provctl/internal/meta"
)

func (m appModel) openLogDirectoryForm() appModel {
	website, ok := m.selectedWebsite()
	if !ok {
		m.status = "select a domain to change its log directory"
		return m
	}
	input := textinput.New()
	input.Prompt = ""
	if website.LogDirectory != "" {
		input.SetValue(website.LogDirectory)
	} else {
		subscription, _ := m.selectedSubscription()
		input.SetValue(filepath.Join(meta.LogDir, subscription.Name, website.PrimaryDomain))
	}
	input.Focus()
	m.logDirectoryForm, m.status = logDirectoryFormState{open: true, website: website, input: input}, ""
	return m
}

func (m appModel) handleLogDirectoryFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.logDirectoryForm, m.status = logDirectoryFormState{}, "cancelled"
		return m, nil
	case "enter":
		return m.openPathPickerForLogDirectory()
	case "ctrl+s":
		path := m.logDirectoryForm.input.Value()
		m.logDirectoryForm.input.Blur()
		website := m.logDirectoryForm.website
		m.logDirectoryForm = logDirectoryFormState{}
		return m.askConfirm(confirmState{action: "set-log-directory", domain: website.PrimaryDomain, value: path, title: "Change log directory", lines: []string{"Domain: " + website.PrimaryDomain, "New directory: " + path, "The directory will be created as root with subscription-group access."}}), nil
	}
	input, command := m.logDirectoryForm.input.Update(msg)
	m.logDirectoryForm.input = input
	return m, command
}

func (m appModel) changeWebsiteLogDirectory(ctx context.Context, confirm confirmState, report func(int)) tea.Msg {
	subscription, ok := m.selectedSubscription()
	if !ok || m.deps.SetWebsiteLogDirectory == nil {
		return websiteLogDirectoryChangedMsg{err: context.Canceled}
	}
	_, err := m.deps.SetWebsiteLogDirectory(ctx, subscription.Name, confirm.domain, confirm.value)
	if err != nil || m.deps.LoadWebsites == nil {
		return websiteLogDirectoryChangedMsg{err: err, domain: confirm.domain, path: confirm.value}
	}
	report(1)
	items, err := m.deps.LoadWebsites(ctx, subscription.ID)
	return websiteLogDirectoryChangedMsg{err: err, domain: confirm.domain, path: confirm.value, items: items}
}

func (m appModel) changeWebsiteLogDirectoryCmd(confirm confirmState) tea.Cmd {
	return steppedCmd("Change log directory", []string{"create log directory", "apply Apache configuration", "refresh domain list"}, func(ctx context.Context, report func(int)) tea.Msg {
		return m.changeWebsiteLogDirectory(ctx, confirm, report)
	})
}

func (m appModel) logDirectoryFormPopup() string {
	body := []string{"Domain: " + m.logDirectoryForm.website.PrimaryDomain, "", "Log directory:", m.logDirectoryForm.input.View(), "", dimStyle.Render("The path must stay within this subscription's provctl log directory.")}
	return popupBox(m, popupOpts{Size: popupMedium, Fit: fitAuto, Title: "Change log directory", Body: body, Footer: []string{dimStyle.Render("enter browse · ctrl+s continue · esc cancel")}})
}
