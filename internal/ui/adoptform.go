package ui

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"provctl/internal/domain"
	"provctl/internal/fsbrowse"
	"provctl/internal/service"
)

var adoptWebsiteTypes = []domain.WebsiteType{
	domain.WebsitePHPFPM,
	domain.WebsiteStatic,
	domain.WebsiteProxy,
	domain.WebsiteRedirect,
}

func (m appModel) openSubscriptionAdoptForm() appModel {
	newInput := func() textinput.Model {
		input := textinput.New()
		input.Prompt, input.Width = "", 42
		return input
	}
	form := subscriptionAdoptFormState{open: true, backup: true, redirectCode: 301, name: newInput(), domain: newInput(), source: newInput(), target: newInput()}
	form.name.Focus()
	m.subscriptionAdoptForm, m.status = form, ""
	return m
}

func (m appModel) adoptType() domain.WebsiteType {
	return adoptWebsiteTypes[m.subscriptionAdoptForm.typeIndex]
}

func (m appModel) adoptIsDataBearing() bool {
	kind := m.adoptType()
	return kind == domain.WebsitePHPFPM || kind == domain.WebsiteStatic
}

func (m appModel) adoptFieldCount() int {
	if m.adoptIsDataBearing() {
		return 6 // name, domain, type, source, transfer, backup
	}
	if m.adoptType() == domain.WebsiteRedirect {
		return 5 // name, domain, type, target, status code
	}
	return 4 // name, domain, type, target
}

func (m appModel) focusSubscriptionAdoptField() appModel {
	m.subscriptionAdoptForm.name.Blur()
	m.subscriptionAdoptForm.domain.Blur()
	m.subscriptionAdoptForm.source.Blur()
	m.subscriptionAdoptForm.target.Blur()
	switch m.subscriptionAdoptForm.field {
	case 0:
		m.subscriptionAdoptForm.name.Focus()
	case 1:
		m.subscriptionAdoptForm.domain.Focus()
	case 3:
		if m.adoptIsDataBearing() {
			m.subscriptionAdoptForm.source.Focus()
		} else {
			m.subscriptionAdoptForm.target.Focus()
		}
	}
	return m
}

func (m appModel) handleSubscriptionAdoptFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	form := m.subscriptionAdoptForm
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.subscriptionAdoptForm, m.status = subscriptionAdoptFormState{}, "cancelled"
		return m, nil
	case "tab", "down":
		m.subscriptionAdoptForm.field = (form.field + 1) % m.adoptFieldCount()
		return m.focusSubscriptionAdoptField(), nil
	case "shift+tab", "up":
		m.subscriptionAdoptForm.field = (form.field - 1 + m.adoptFieldCount()) % m.adoptFieldCount()
		return m.focusSubscriptionAdoptField(), nil
	case "left", "right":
		if form.field == 2 {
			delta := 1
			if msg.String() == "left" {
				delta = -1
			}
			m.subscriptionAdoptForm.typeIndex = (form.typeIndex + delta + len(adoptWebsiteTypes)) % len(adoptWebsiteTypes)
			m.subscriptionAdoptForm.field = min(m.subscriptionAdoptForm.field, m.adoptFieldCount()-1)
			return m.focusSubscriptionAdoptField(), nil
		}
		if m.adoptIsDataBearing() && form.field == 4 {
			m.subscriptionAdoptForm.copy = !form.copy
			return m, nil
		}
		if m.adoptIsDataBearing() && form.field == 5 {
			m.subscriptionAdoptForm.backup = !form.backup
			return m, nil
		}
		if m.adoptType() == domain.WebsiteRedirect && form.field == 4 {
			m.subscriptionAdoptForm.redirectCode = map[int]int{301: 302, 302: 301}[form.redirectCode]
			return m, nil
		}
	case "enter":
		if m.adoptIsDataBearing() && form.field == 3 {
			m = m.openPathPickerForAdoptSource()
			return m, m.listPathCmd(m.subscriptionAdoptForm.source.Value(), m.pathPicker.generation)
		}
	case "ctrl+s":
		options := service.SubscriptionAdoptOptions{Domain: strings.TrimSpace(form.domain.Value()), Type: m.adoptType(), Copy: form.copy, Backup: form.backup}
		if m.adoptIsDataBearing() {
			options.Source = strings.TrimSpace(form.source.Value())
		} else {
			options.Target = strings.TrimSpace(form.target.Value())
		}
		if options.Type == domain.WebsiteRedirect {
			options.RedirectCode = form.redirectCode
		}
		if strings.TrimSpace(form.name.Value()) == "" || options.Domain == "" || (m.adoptIsDataBearing() && options.Source == "") || (!m.adoptIsDataBearing() && options.Target == "") {
			m.status = "name, domain, and the required source or target must be set"
			return m, nil
		}
		lines := []string{"Subscription: " + strings.TrimSpace(form.name.Value()), "Domain: " + options.Domain, "Type: " + string(options.Type)}
		if m.adoptIsDataBearing() {
			transfer := "move source atomically"
			if options.Copy {
				transfer = "copy source; leave original in place"
			}
			lines = append(lines, "Source: "+options.Source, "Transfer: "+transfer, "Backup before transfer: "+map[bool]string{true: "yes", false: "no"}[options.Backup], "All adopted files will be assigned to the new subscription UID:GID.")
		} else {
			lines = append(lines, "Target: "+options.Target)
			if options.Type == domain.WebsiteRedirect {
				lines = append(lines, "Redirect status: "+map[int]string{301: "301 permanent", 302: "302 temporary"}[options.RedirectCode])
			}
			lines = append(lines, "No application files or container runtime configuration will be changed.")
		}
		m.subscriptionAdoptForm = subscriptionAdoptFormState{}
		return m.askConfirm(confirmState{action: "adopt-subscription", domain: strings.TrimSpace(form.name.Value()), title: "Adopt website", lines: lines, word: "ADOPT", adopt: options}), nil
	}

	var input textinput.Model
	switch form.field {
	case 0:
		input = form.name
	case 1:
		input = form.domain
	case 3:
		if m.adoptIsDataBearing() {
			input = form.source
		} else {
			input = form.target
		}
	default:
		return m, nil
	}
	updated, command := input.Update(msg)
	switch form.field {
	case 0:
		m.subscriptionAdoptForm.name = updated
	case 1:
		m.subscriptionAdoptForm.domain = updated
	case 3:
		if m.adoptIsDataBearing() {
			m.subscriptionAdoptForm.source = updated
		} else {
			m.subscriptionAdoptForm.target = updated
		}
	}
	return m, command
}

func (m appModel) openPathPickerForAdoptSource() appModel {
	input := newFilter().input
	input.Prompt = ""
	input.SetValue(m.subscriptionAdoptForm.source.Value())
	m.pathPicker = pathPickerState{open: true, target: pathPickerAdoptSource, mode: fsbrowse.Dirs, loading: true, generation: m.pathPicker.generation + 1, filter: newFilter(), pathInput: input}
	return m
}

func (m appModel) adoptSubscription(ctx context.Context, confirm confirmState, report func(int)) tea.Msg {
	if m.deps.AdoptSubscription == nil {
		return subscriptionAdoptedMsg{err: context.Canceled, name: confirm.domain}
	}
	report(0)
	_, err := m.deps.AdoptSubscription(ctx, confirm.domain, confirm.adopt)
	return subscriptionAdoptedMsg{err: err, name: confirm.domain}
}

func (m appModel) adoptSubscriptionCmd(confirm confirmState) tea.Cmd {
	return steppedCmd("Adopt website", []string{"prepare adoption plan", "refresh subscriptions"}, func(ctx context.Context, report func(int)) tea.Msg {
		message := m.adoptSubscription(ctx, confirm, report)
		if !operationFailed(message) {
			report(1)
		}
		return message
	})
}

func (m appModel) subscriptionAdoptFormPopup() string {
	form, kind := m.subscriptionAdoptForm, m.adoptType()
	rows := []string{
		"Subscription: " + form.name.View(),
		"Domain:       " + form.domain.View(),
		"Type:         " + formSelector([]string{"php-fpm", "static", "proxy", "redirect"}, form.typeIndex),
	}
	if m.adoptIsDataBearing() {
		rows = append(rows, "Source:       "+form.source.View(), "Transfer:     "+formSelector([]string{"move", "copy"}, boolIndex(form.copy)), "Backup:       "+formSelector([]string{"yes", "no"}, boolIndex(!form.backup)), "", dimStyle.Render("Enter on Source opens the directory picker. ctrl+s shows the plan."))
	} else {
		rows = append(rows, "Target:       "+form.target.View())
		if kind == domain.WebsiteRedirect {
			rows = append(rows, "Status:       "+formSelector([]string{"301 permanent", "302 temporary"}, boolIndex(form.redirectCode == 302)))
		}
		rows = append(rows, "", dimStyle.Render("Proxy and redirect adoption never modifies container runtime files."))
	}
	return popupBox(m, popupOpts{Size: popupMedium, Fit: fitAuto, Title: "Adopt website", Body: rows, Footer: []string{dimStyle.Render("↑/↓ fields · ←/→ choices · ctrl+s continue · esc cancel")}})
}

func boolIndex(value bool) int {
	if value {
		return 1
	}
	return 0
}

func formSelector(items []string, selected int) string {
	parts := make([]string, len(items))
	for index, item := range items {
		if index == selected {
			parts[index] = "[" + item + "]"
		} else {
			parts[index] = item
		}
	}
	return strings.Join(parts, " ")
}
