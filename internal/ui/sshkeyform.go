package ui

import (
	"context"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func (m appModel) openSSHKeyForm() appModel {
	if _, ok := m.selectedSubscription(); !ok {
		m.status = "select a subscription before adding an SSH key"
		return m
	}
	input := textinput.New()
	input.Prompt = ""
	input.Focus()
	m.sshKeyForm, m.status = sshKeyFormState{open: true, input: input}, ""
	return m
}

func (m appModel) handleSSHKeyFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.sshKeyForm, m.status = sshKeyFormState{}, "cancelled"
		return m, nil
	case "enter":
		return m.openPathPickerForSSHKey()
	case "ctrl+s":
		path := m.sshKeyForm.input.Value()
		m.sshKeyForm = sshKeyFormState{}
		return m.askConfirm(confirmState{action: "add-ssh-key", value: path, title: "Add SSH key", lines: []string{
			"Subscription: " + m.selectedSubscriptionName(),
			"Public key file: " + path,
			"The key will be validated before it is added.",
		}}), nil
	}
	input, command := m.sshKeyForm.input.Update(msg)
	m.sshKeyForm.input = input
	return m, command
}

func (m appModel) selectedSubscriptionName() string {
	subscription, ok := m.selectedSubscription()
	if !ok {
		return ""
	}
	return subscription.Name
}

func (m appModel) addSSHKey(ctx context.Context, confirm confirmState, report func(int)) tea.Msg {
	subscription, ok := m.selectedSubscription()
	if !ok || m.deps.AddSSHKeyFromFile == nil {
		return sshKeyAddedMsg{err: context.Canceled, path: confirm.value}
	}
	_, err := m.deps.AddSSHKeyFromFile(ctx, subscription.Name, confirm.value)
	if err != nil || m.deps.LoadSSHKeys == nil {
		return sshKeyAddedMsg{err: err, path: confirm.value}
	}
	report(1)
	items, err := m.deps.LoadSSHKeys(ctx, subscription.Name)
	return sshKeyAddedMsg{err: err, path: confirm.value, items: items}
}

func (m appModel) addSSHKeyCmd(confirm confirmState) tea.Cmd {
	return steppedCmd("Add SSH key", []string{"validate and write authorized keys", "refresh SSH key list"}, func(ctx context.Context, report func(int)) tea.Msg {
		return m.addSSHKey(ctx, confirm, report)
	})
}

func (m appModel) sshKeyFormPopup() string {
	body := []string{"Public key file:", m.sshKeyForm.input.View(), "", dimStyle.Render("Enter browse · Ctrl+S continue · Esc cancel")}
	return popupBox(m, popupOpts{Size: popupMedium, Fit: fitAuto, Title: "Add SSH key", Body: body, Footer: []string{dimStyle.Render("enter browse · ctrl+s continue · esc cancel")}})
}
