package ui

import (
	"context"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// openSubscriptionCreateForm keeps creation in the picker, where there is no
// ambiguous selected domain or subscription target.
func (m appModel) openSubscriptionCreateForm() appModel {
	input := textinput.New()
	input.Prompt = ""
	input.Focus()
	m.subscriptionCreateForm = subscriptionCreateFormState{open: true, input: input}
	m.status = ""
	return m
}

func (m appModel) handleSubscriptionCreateFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.subscriptionCreateForm, m.status = subscriptionCreateFormState{}, "cancelled"
		return m, nil
	case "ctrl+s":
		name := m.subscriptionCreateForm.input.Value()
		m.subscriptionCreateForm = subscriptionCreateFormState{}
		return m.askConfirm(confirmState{action: "create-subscription", domain: name, title: "Create subscription", lines: []string{
			"Subscription: " + name,
			"Create a Unix user and isolated hosting home.",
			"Quotas use the configured defaults (unlimited).",
		}}), nil
	}
	input, command := m.subscriptionCreateForm.input.Update(msg)
	m.subscriptionCreateForm.input = input
	return m, command
}

func (m appModel) createSubscription(ctx context.Context, confirm confirmState) tea.Msg {
	if m.deps.CreateSubscription == nil {
		return subscriptionCreatedMsg{err: context.Canceled, name: confirm.domain}
	}
	_, err := m.deps.CreateSubscription(ctx, confirm.domain)
	return subscriptionCreatedMsg{err: err, name: confirm.domain}
}

func (m appModel) createSubscriptionCmd(confirm confirmState) tea.Cmd {
	return steppedCmd("Create subscription", []string{"create user and subscription home"}, func(ctx context.Context, _ func(int)) tea.Msg {
		return m.createSubscription(ctx, confirm)
	})
}

func (m appModel) subscriptionCreateFormPopup() string {
	body := []string{
		"Name:", m.subscriptionCreateForm.input.View(), "",
		dimStyle.Render("Lowercase letters, digits and hyphens; starts with a letter."),
		dimStyle.Render("The home and Unix user derive from the name."),
	}
	return popupBox(m, popupOpts{Size: popupSmall, Fit: fitAuto, Title: "Create subscription", Body: body, Footer: []string{dimStyle.Render("ctrl+s continue · esc cancel")}})
}
