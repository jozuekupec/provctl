package ui

import (
	"context"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"provctl/internal/domain"
)

func (m appModel) openTargetForm() appModel {
	w, ok := m.selectedWebsite()
	if !ok || (w.Type != domain.WebsiteProxy && w.Type != domain.WebsiteRedirect) {
		m.status = "select a proxy or redirect domain to edit its target"
		return m
	}
	in := textinput.New()
	in.Prompt = ""
	in.SetValue(w.Target)
	in.Focus()
	m.targetForm = targetFormState{open: true, website: w, input: in, code: w.RedirectCode}
	return m
}
func (m appModel) handleTargetFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.targetForm = targetFormState{}
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "left", "right":
		if m.targetForm.website.Type == domain.WebsiteRedirect {
			if m.targetForm.code == 301 {
				m.targetForm.code = 302
			} else {
				m.targetForm.code = 301
			}
			return m, nil
		}
	case "ctrl+s":
		f := m.targetForm
		m.targetForm = targetFormState{}
		return m.askConfirm(confirmState{action: "set-target", domain: f.website.PrimaryDomain, value: f.input.Value(), enabled: f.code == 302, title: "Change target", lines: []string{"Domain: " + f.website.PrimaryDomain, "Target: " + f.input.Value()}}), nil
	}
	in, cmd := m.targetForm.input.Update(msg)
	m.targetForm.input = in
	return m, cmd
}
func (m appModel) changeTarget(ctx context.Context, c confirmState, report func(int)) tea.Msg {
	s, ok := m.selectedSubscription()
	if !ok || m.deps.SetWebsiteTarget == nil {
		return websiteTargetChangedMsg{err: context.Canceled}
	}
	code := 301
	if c.enabled {
		code = 302
	}
	_, err := m.deps.SetWebsiteTarget(ctx, s.Name, c.domain, c.value, code)
	if err != nil || m.deps.LoadWebsites == nil {
		return websiteTargetChangedMsg{err: err, domain: c.domain}
	}
	report(1)
	items, err := m.deps.LoadWebsites(ctx, s.ID)
	return websiteTargetChangedMsg{err: err, domain: c.domain, items: items}
}
func (m appModel) targetCmd(c confirmState) tea.Cmd {
	return steppedCmd("Change target", []string{"apply Apache configuration", "refresh domain list"}, func(ctx context.Context, r func(int)) tea.Msg { return m.changeTarget(ctx, c, r) })
}
func (m appModel) targetFormPopup() string {
	f := m.targetForm
	body := []string{"Domain: " + f.website.PrimaryDomain, "", "Target:", f.input.View()}
	if f.website.Type == domain.WebsiteRedirect {
		body = append(body, "", "Redirect code: "+map[bool]string{true: "302", false: "301"}[f.code == 302])
	}
	return popupBox(m, popupOpts{Size: popupSmall, Fit: fitAuto, Title: "Change target", Body: body, Footer: []string{dimStyle.Render("←/→ redirect code · ctrl+s continue · esc cancel")}})
}
