package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"provctl/internal/domain"
)

func (m appModel) openCronForm() appModel {
	if _, ok := m.selectedSubscription(); !ok {
		m.status = "select a subscription before adding a cron job"
		return m
	}
	return m.openCronFormFor(domain.CronJob{})
}

func (m appModel) openCronEditForm(job domain.CronJob) appModel {
	return m.openCronFormFor(job)
}

func (m appModel) openCronFormFor(job domain.CronJob) appModel {
	schedule, command, comment := textinput.New(), textinput.New(), textinput.New()
	schedule.Prompt, command.Prompt, comment.Prompt = "", "", ""
	schedule.SetValue(job.Schedule)
	command.SetValue(job.Command)
	comment.SetValue(job.Comment)
	schedule.Focus()
	m.cronForm, m.status = cronFormState{open: true, editingID: job.ID, schedule: schedule, command: command, comment: comment}, ""
	return m
}

func (m appModel) focusCronField() appModel {
	m.cronForm.schedule.Blur()
	m.cronForm.command.Blur()
	m.cronForm.comment.Blur()
	switch m.cronForm.field {
	case 0:
		m.cronForm.schedule.Focus()
	case 1:
		m.cronForm.command.Focus()
	default:
		m.cronForm.comment.Focus()
	}
	return m
}

func (m appModel) handleCronFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.cronForm, m.status = cronFormState{}, "cancelled"
		return m, nil
	case "tab", "down":
		m.cronForm.field = (m.cronForm.field + 1) % 3
		return m.focusCronField(), nil
	case "shift+tab", "up":
		m.cronForm.field = (m.cronForm.field + 2) % 3
		return m.focusCronField(), nil
	case "ctrl+s":
		schedule := strings.TrimSpace(m.cronForm.schedule.Value())
		command := strings.TrimSpace(m.cronForm.command.Value())
		comment := strings.TrimSpace(m.cronForm.comment.Value())
		editingID := m.cronForm.editingID
		m.cronForm = cronFormState{}
		action, title := "create-cron-job", "Create cron job"
		if editingID != 0 {
			action, title = "update-cron-job", "Update cron job"
		}
		return m.askConfirm(confirmState{action: action, jobID: editingID, domain: schedule, value: command, note: comment, title: title, lines: []string{"Subscription: " + m.selectedSubscriptionName(), "Schedule: " + schedule, "Command: " + command, "Comment: " + comment, cronSchedulePreview(schedule)}}), nil
	}
	var input textinput.Model
	var command tea.Cmd
	switch m.cronForm.field {
	case 0:
		input, command = m.cronForm.schedule.Update(msg)
		m.cronForm.schedule = input
	case 1:
		input, command = m.cronForm.command.Update(msg)
		m.cronForm.command = input
	default:
		input, command = m.cronForm.comment.Update(msg)
		m.cronForm.comment = input
	}
	return m, command
}

func cronSchedulePreview(schedule string) string {
	next, err := domain.NextCronRun(strings.TrimSpace(schedule), time.Now())
	if err != nil {
		return "Preview: " + err.Error()
	}
	return "Next run: " + next.Format("2006-01-02 15:04 MST") + " (server local time)"
}

func (m appModel) createCronJob(ctx context.Context, confirm confirmState, report func(int)) tea.Msg {
	subscription, ok := m.selectedSubscription()
	if !ok || m.deps.AddCronJob == nil {
		return cronJobCreatedMsg{err: context.Canceled}
	}
	_, err := m.deps.AddCronJob(ctx, subscription.Name, confirm.domain, confirm.value, confirm.note)
	if err != nil || m.deps.LoadCronJobs == nil {
		return cronJobCreatedMsg{err: err}
	}
	report(1)
	items, err := m.deps.LoadCronJobs(ctx, subscription.Name)
	return cronJobCreatedMsg{err: err, items: items}
}

func (m appModel) createCronJobCmd(confirm confirmState) tea.Cmd {
	return steppedCmd("Create cron job", []string{"write generated crontab", "refresh cron job list"}, func(ctx context.Context, report func(int)) tea.Msg { return m.createCronJob(ctx, confirm, report) })
}

func (m appModel) updateCronJob(ctx context.Context, confirm confirmState, report func(int)) tea.Msg {
	subscription, ok := m.selectedSubscription()
	if !ok || m.deps.UpdateCronJob == nil {
		return cronJobUpdatedMsg{err: context.Canceled}
	}
	_, err := m.deps.UpdateCronJob(ctx, subscription.Name, confirm.jobID, confirm.domain, confirm.value, confirm.note)
	if err != nil || m.deps.LoadCronJobs == nil {
		return cronJobUpdatedMsg{err: err, id: confirm.jobID}
	}
	report(1)
	items, err := m.deps.LoadCronJobs(ctx, subscription.Name)
	return cronJobUpdatedMsg{err: err, id: confirm.jobID, items: items}
}

func (m appModel) updateCronJobCmd(confirm confirmState) tea.Cmd {
	return steppedCmd("Update cron job", []string{"rewrite generated crontab", "refresh cron job list"}, func(ctx context.Context, report func(int)) tea.Msg { return m.updateCronJob(ctx, confirm, report) })
}

func (m appModel) cronFormPopup() string {
	title := "Create cron job"
	if m.cronForm.editingID != 0 {
		title = "Edit cron job #" + fmt.Sprint(m.cronForm.editingID)
	}
	body := []string{"Schedule: " + m.cronForm.schedule.View(), "Command:  " + m.cronForm.command.View(), "Comment:  " + m.cronForm.comment.View(), "", cronSchedulePreview(m.cronForm.schedule.Value())}
	return popupBox(m, popupOpts{Size: popupLarge, Fit: fitAuto, Title: title, Body: body, Footer: []string{dimStyle.Render("tab field · ctrl+s continue · esc cancel")}})
}
