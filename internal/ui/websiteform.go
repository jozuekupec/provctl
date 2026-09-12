package ui

import (
	"context"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"provctl/internal/domain"
)

var websiteTypes = []domain.WebsiteType{domain.WebsitePHPFPM, domain.WebsiteStatic, domain.WebsiteProxy, domain.WebsiteRedirect}

func (m appModel) openWebsiteCreateForm() appModel {
	if _, ok := m.selectedSubscription(); !ok {
		m.status = "select a subscription before creating a domain"
		return m
	}
	domainInput, targetInput := textinput.New(), textinput.New()
	domainInput.Prompt, targetInput.Prompt = "", ""
	domainInput.Focus()
	m.websiteCreateForm = websiteCreateFormState{open: true, domain: domainInput, target: targetInput}
	m.status = ""
	return m
}

func (m appModel) createFormType() domain.WebsiteType {
	return websiteTypes[clamp(m.websiteCreateForm.typeIndex, len(websiteTypes))]
}

func (m appModel) createFormNeedsTarget() bool {
	kind := m.createFormType()
	return kind == domain.WebsiteProxy || kind == domain.WebsiteRedirect
}

func (m appModel) createFormFieldCount() int {
	if m.createFormNeedsTarget() {
		return 3
	}
	return 2
}

func (m appModel) focusCreateFormField() appModel {
	m.websiteCreateForm.domain.Blur()
	m.websiteCreateForm.target.Blur()
	if m.websiteCreateForm.field == 0 {
		m.websiteCreateForm.domain.Focus()
	} else if m.websiteCreateForm.field == 2 {
		m.websiteCreateForm.target.Focus()
	}
	return m
}

func (m appModel) handleWebsiteCreateFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.websiteCreateForm, m.status = websiteCreateFormState{}, "cancelled"
		return m, nil
	case "tab", "down":
		m.websiteCreateForm.field = (m.websiteCreateForm.field + 1) % m.createFormFieldCount()
		return m.focusCreateFormField(), nil
	case "shift+tab", "up":
		m.websiteCreateForm.field = (m.websiteCreateForm.field - 1 + m.createFormFieldCount()) % m.createFormFieldCount()
		return m.focusCreateFormField(), nil
	case "left":
		if m.websiteCreateForm.field == 1 {
			m.websiteCreateForm.typeIndex = (m.websiteCreateForm.typeIndex - 1 + len(websiteTypes)) % len(websiteTypes)
			m.websiteCreateForm.field = min(m.websiteCreateForm.field, m.createFormFieldCount()-1)
			return m.focusCreateFormField(), nil
		}
	case "right":
		if m.websiteCreateForm.field == 1 {
			m.websiteCreateForm.typeIndex = (m.websiteCreateForm.typeIndex + 1) % len(websiteTypes)
			return m.focusCreateFormField(), nil
		}
	case "ctrl+s":
		subscription, ok := m.selectedSubscription()
		if !ok {
			return m, nil
		}
		kind, name, target := m.createFormType(), m.websiteCreateForm.domain.Value(), m.websiteCreateForm.target.Value()
		m.websiteCreateForm = websiteCreateFormState{}
		return m.askConfirm(confirmState{action: "create-website", domain: name, value: string(kind) + "\n" + target, title: "Create domain", lines: []string{"Subscription: " + subscription.Name, "Domain: " + name, "Type: " + string(kind), "Target: " + valueOrDash(target)}}), nil
	}
	if m.websiteCreateForm.field == 0 {
		input, command := m.websiteCreateForm.domain.Update(msg)
		m.websiteCreateForm.domain = input
		return m, command
	}
	if m.websiteCreateForm.field == 2 {
		input, command := m.websiteCreateForm.target.Update(msg)
		m.websiteCreateForm.target = input
		return m, command
	}
	return m, nil
}

func (m appModel) createWebsite(ctx context.Context, confirm confirmState, report func(int)) tea.Msg {
	subscription, ok := m.selectedSubscription()
	if m.deps.CreateWebsite == nil || !ok {
		return websiteCreatedMsg{err: context.Canceled}
	}
	parts := splitCreateValue(confirm.value)
	_, err := m.deps.CreateWebsite(ctx, subscription.Name, confirm.domain, domain.WebsiteType(parts[0]), parts[1], 301)
	if err != nil || m.deps.LoadWebsites == nil {
		return websiteCreatedMsg{err: err, domain: confirm.domain}
	}
	report(1)
	items, err := m.deps.LoadWebsites(ctx, subscription.ID)
	return websiteCreatedMsg{err: err, domain: confirm.domain, items: items}
}

func splitCreateValue(value string) [2]string {
	for index, character := range value {
		if character == '\n' {
			return [2]string{value[:index], value[index+1:]}
		}
	}
	return [2]string{value, ""}
}

func (m appModel) createWebsiteCmd(confirm confirmState) tea.Cmd {
	return steppedCmd("Create domain", []string{"provision domain resources", "refresh domain list"}, func(ctx context.Context, report func(int)) tea.Msg {
		return m.createWebsite(ctx, confirm, report)
	})
}

func (m appModel) websiteCreateFormPopup() string {
	labels := []string{"Domain", "Type", "Target"}
	rows := []string{labels[0] + ": " + m.websiteCreateForm.domain.View()}
	types := ""
	for index, kind := range websiteTypes {
		label := string(kind)
		if index == m.websiteCreateForm.typeIndex {
			label = selectedStyle.Render(label)
		}
		if types != "" {
			types += "  "
		}
		types += label
	}
	rows = append(rows, labels[1]+": "+types)
	if m.createFormNeedsTarget() {
		rows = append(rows, labels[2]+": "+m.websiteCreateForm.target.View())
	} else {
		rows = append(rows, dimStyle.Render("Document root uses the safe default; edit it after creation with E."))
	}
	rows = append(rows, "", dimStyle.Render("Static/PHP-FPM use <home>/sites/<domain>/public."))
	return popupBox(m, popupOpts{Size: popupMedium, Fit: fitAuto, Title: "Create domain", Body: rows, Footer: []string{dimStyle.Render("tab field · ←/→ type · ctrl+s continue · esc cancel")}})
}
