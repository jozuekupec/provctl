package ui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
)

var sshAccessModes = []string{"none", "key", "password", "key+password"}

func (m appModel) openSSHAccessForm() appModel {
	subscription, ok := m.selectedSubscription()
	if !ok {
		m.status = "select a subscription before changing SSH access"
		return m
	}
	index := 0
	for candidate, mode := range sshAccessModes {
		if mode == subscription.SSHAccess {
			index = candidate
			break
		}
	}
	m.sshAccessForm = sshAccessFormState{open: true, index: index}
	m.status = ""
	return m
}

func (m appModel) selectedSSHAccessMode() string {
	return sshAccessModes[clamp(m.sshAccessForm.index, len(sshAccessModes))]
}

func (m appModel) handleSSHAccessFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.sshAccessForm, m.status = sshAccessFormState{}, "cancelled"
		return m, nil
	case "up", "left", "k":
		m.sshAccessForm.index = (m.sshAccessForm.index - 1 + len(sshAccessModes)) % len(sshAccessModes)
		return m, nil
	case "down", "right", "j":
		m.sshAccessForm.index = (m.sshAccessForm.index + 1) % len(sshAccessModes)
		return m, nil
	case "enter", "ctrl+s":
		subscription, ok := m.selectedSubscription()
		if !ok {
			return m, nil
		}
		access := m.selectedSSHAccessMode()
		m.sshAccessForm = sshAccessFormState{}
		lines := []string{"Subscription: " + subscription.Name, "SSH access: " + subscription.SSHAccess + " → " + access}
		if access == "password" || access == "key+password" {
			lines = append(lines, "A generated password will be displayed once after success.")
		}
		return m.askConfirm(confirmState{action: "set-ssh-access", domain: subscription.Name, value: access, title: "Change SSH access", lines: lines}), nil
	}
	return m, nil
}

func (m appModel) changeSSHAccess(ctx context.Context, confirm confirmState, report func(int)) tea.Msg {
	subscription, ok := m.selectedSubscription()
	if !ok || subscription.Name != confirm.domain || m.deps.SetSSHAccess == nil {
		return sshAccessChangedMsg{err: context.Canceled, access: confirm.value}
	}
	password, _, err := m.deps.SetSSHAccess(ctx, subscription.Name, confirm.value)
	if err != nil || m.deps.LoadSubscriptions == nil {
		return sshAccessChangedMsg{err: err, access: confirm.value}
	}
	report(1)
	items, err := m.deps.LoadSubscriptions(ctx)
	return sshAccessChangedMsg{err: err, access: confirm.value, password: password, items: items}
}

func (m appModel) changeSSHAccessCmd(confirm confirmState) tea.Cmd {
	return steppedCmd("Change SSH access", []string{"apply SSH account access", "refresh subscription list"}, func(ctx context.Context, report func(int)) tea.Msg {
		return m.changeSSHAccess(ctx, confirm, report)
	})
}

func (m appModel) sshAccessFormPopup() string {
	rows := []string{"SSH access:"}
	for index, access := range sshAccessModes {
		if index == m.sshAccessForm.index {
			access = selectedStyle.Render("▸ " + access)
		} else {
			access = "  " + access
		}
		rows = append(rows, access)
	}
	rows = append(rows, "", dimStyle.Render("Key modes require at least one registered SSH key."))
	return popupBox(m, popupOpts{Size: popupSmall, Fit: fitAuto, Title: "SSH access", Body: rows, Footer: []string{dimStyle.Render("↑/↓ select · enter continue · esc cancel")}})
}

func (m appModel) handleSecretKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "enter", "esc", "q":
		m.secret = secretState{}
		m.status = "SSH access updated"
	}
	return m, nil
}

func (m appModel) secretPopup() string {
	body := []string{
		confirmStyle.Render("Save this password now. It will not be shown again."),
		"",
		m.secret.secret,
	}
	return popupBox(m, popupOpts{Size: popupMedium, Fit: fitAuto, Title: m.secret.title, Danger: true, Body: body, Footer: []string{dimStyle.Render("enter acknowledge")}})
}
