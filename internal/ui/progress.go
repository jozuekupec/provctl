package ui

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type stepState int

const (
	stepPending stepState = iota
	stepRunning
	stepDone
	stepFailed
)

type progressStep struct {
	label string
	state stepState
}

type progressState struct {
	title  string
	steps  []progressStep
	active bool
	ch     chan tea.Msg
	cancel context.CancelFunc
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
	ch := make(chan tea.Msg, len(steps)*2+2)
	return func() tea.Msg {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			ch <- progressStartMsg{title: title, steps: steps, ch: ch, cancel: cancel}
			current := -1
			report := func(index int) {
				if index < 0 || index >= len(steps) || index == current {
					return
				}
				if current >= 0 {
					ch <- progressStepMsg{index: current, state: stepDone}
				}
				current = index
				ch <- progressStepMsg{index: current, state: stepRunning}
			}
			report(0)
			result := work(ctx, report)
			if current >= 0 {
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
	case websiteChangedMsg:
		return result.err != nil
	case websiteTLSChangedMsg:
		return result.err != nil
	case subscriptionChangedMsg:
		return result.err != nil
	case websitePHPChangedMsg:
		return result.err != nil
	case websiteDocumentRootChangedMsg:
		return result.err != nil
	}
	return true
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
		}
		lines = append(lines, glyph+" "+step.label)
	}
	return strings.Join(lines, "\n")
}

// progressPopup renders progress as the active modal surface. Output remains
// a persistent operation log and is not repurposed as a transient checklist.
func (m appModel) progressPopup() string {
	width := min(72, max(38, m.width-12))
	lines := []string{m.progress.render(), "", dimStyle.Render("esc cancel · q quit")}
	height := min(max(7, len(lines)+2), max(7, m.height-6))
	return panel(m.progress.title, strings.Join(lines, "\n"), width, height, true)
}
