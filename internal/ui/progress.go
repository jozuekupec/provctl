package ui

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"provctl/internal/plan"
)

type stepState int

const (
	stepPending stepState = iota
	stepRunning
	stepDone
	stepFailed
	stepRolledBack
)

type progressStep struct {
	label string
	state stepState
}

type progressState struct {
	title           string
	steps           []progressStep
	active          bool
	awaitingDismiss bool
	pendingMsg      tea.Msg
	ch              chan tea.Msg
	cancel          context.CancelFunc
}

type progressStartMsg struct {
	title  string
	steps  []progressStep
	ch     chan tea.Msg
	cancel context.CancelFunc
}

type progressStepMsg struct {
	index int
	state stepState
}

// progressPlanMsg transports executor-owned plan transitions into Update.
// The executor never knows about Bubble Tea or terminal rendering.
type progressPlanMsg struct{ event plan.ProgressEvent }

func waitProgress(ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg { return <-ch }
}

// steppedCmd reports UI-owned stages. Domain services do not expose their
// internal plan yet, so callers may report only work they actually perform.
func steppedCmd(title string, labels []string, work func(context.Context, func(int)) tea.Msg) tea.Cmd {
	steps := make([]progressStep, len(labels))
	for index, label := range labels {
		steps[index] = progressStep{label: label, state: stepPending}
	}
	ch := make(chan tea.Msg, 256)
	return func() tea.Msg {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			planStepCount := 0
			current := -1
			activePlanStep := -1
			ctx = plan.WithProgress(ctx, func(event plan.ProgressEvent) {
				if len(event.Steps) > 0 {
					planStepCount = len(event.Steps)
					current = -1
				}
				if event.Index >= 0 {
					switch event.Status {
					case plan.StepRunning:
						activePlanStep = event.Index
					case plan.StepDone, plan.StepFailed, plan.StepRolledBack:
						if activePlanStep == event.Index {
							activePlanStep = -1
						}
					}
				}
				ch <- progressPlanMsg{event: event}
			})
			ch <- progressStartMsg{title: title, steps: steps, ch: ch, cancel: cancel}
			report := func(index int) {
				if index < 0 || index >= len(steps) || index == current {
					return
				}
				if current >= 0 {
					ch <- progressStepMsg{index: current, state: stepDone}
				}
				current = index
				if planStepCount > 0 && index > 0 {
					current = planStepCount + index - 1
				}
				ch <- progressStepMsg{index: current, state: stepRunning}
			}
			report(0)
			result := work(ctx, report)
			if operationFailed(result) && activePlanStep >= 0 {
				ch <- progressStepMsg{index: activePlanStep, state: stepFailed}
			} else if current >= 0 {
				state := stepDone
				if operationFailed(result) {
					state = stepFailed
				}
				ch <- progressStepMsg{index: current, state: state}
			}
			ch <- result
		}()
		return <-ch
	}
}

func operationFailed(message tea.Msg) bool {
	switch result := message.(type) {
	case sshAccessChangedMsg:
		return result.err != nil
	case sshKeyAddedMsg:
		return result.err != nil
	case sshKeyRemovedMsg:
		return result.err != nil
	case cronJobCreatedMsg:
		return result.err != nil
	case cronJobUpdatedMsg:
		return result.err != nil
	case cronJobRemovedMsg:
		return result.err != nil
	case backupCreatedMsg:
		return result.err != nil
	case subscriptionAdoptedMsg:
		return result.err != nil
	case websiteChangedMsg:
		return result.err != nil
	case websiteTLSChangedMsg:
		return result.err != nil
	case subscriptionChangedMsg:
		return result.err != nil
	case subscriptionCreatedMsg:
		return result.err != nil
	case reconcileFinishedMsg:
		return result.err != nil
	case websitePHPChangedMsg:
		return result.err != nil
	case websiteDocumentRootChangedMsg:
		return result.err != nil
	case websiteLogDirectoryChangedMsg:
		return result.err != nil
	case websiteCreatedMsg:
		return result.err != nil
	case websiteAliasChangedMsg:
		return result.err != nil
	case websiteTargetChangedMsg:
		return result.err != nil
	case websiteDeletedMsg:
		return result.err != nil
	case subscriptionDeletedMsg:
		return result.err != nil
	case databaseCreatedMsg:
		return result.err != nil
	case databasePasswordChangedMsg:
		return result.err != nil
	case databaseDeletedMsg:
		return result.err != nil
	}
	return true
}

// isMutationResult separates a stepped command's terminal result from its
// progress stream messages and unrelated read operations.
func isMutationResult(message tea.Msg) bool {
	switch message.(type) {
	case sshAccessChangedMsg, sshKeyAddedMsg, sshKeyRemovedMsg,
		cronJobCreatedMsg, cronJobUpdatedMsg, cronJobRemovedMsg, backupCreatedMsg,
		websiteChangedMsg, websiteTLSChangedMsg, websiteDocumentRootChangedMsg,
		websiteLogDirectoryChangedMsg, websiteCreatedMsg, websiteAliasChangedMsg,
		websiteTargetChangedMsg, websiteDeletedMsg, subscriptionChangedMsg,
		subscriptionDeletedMsg, subscriptionCreatedMsg, subscriptionAdoptedMsg, databaseCreatedMsg,
		databasePasswordChangedMsg, databaseDeletedMsg, reconcileFinishedMsg,
		websitePHPChangedMsg:
		return true
	}
	return false
}

func progressStepState(status plan.StepStatus) stepState {
	switch status {
	case plan.StepRunning:
		return stepRunning
	case plan.StepDone:
		return stepDone
	case plan.StepFailed:
		return stepFailed
	case plan.StepRolledBack:
		return stepRolledBack
	default:
		return stepPending
	}
}

func (progress progressState) render() string {
	lines := make([]string, 0, len(progress.steps))
	for _, step := range progress.steps {
		glyph := "○"
		switch step.state {
		case stepRunning:
			glyph = "●"
		case stepDone:
			glyph = "✓"
		case stepFailed:
			glyph = "!"
		case stepRolledBack:
			glyph = "↶"
		}
		lines = append(lines, glyph+" "+step.label)
	}
	return strings.Join(lines, "\n")
}

// progressPopup renders progress as the active modal surface. Output remains
// a persistent operation log and is not repurposed as a transient checklist.
func (m appModel) progressPopup() string {
	width := min(72, max(38, m.width-12))
	footer := "esc cancel · q quit"
	if m.progress.awaitingDismiss {
		footer = "enter/esc dismiss · q quit"
	}
	lines := []string{m.progress.render(), "", dimStyle.Render(footer)}
	height := min(max(7, len(lines)+2), max(7, m.height-6))
	return panel(m.progress.title, strings.Join(lines, "\n"), width, height, true)
}
