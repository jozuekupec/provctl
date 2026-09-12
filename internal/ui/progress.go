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

// steppedCmd reports only UI-owned stages. Domain services do not expose their
// internal plan yet, so the UI must not pretend it can show them.
func steppedCmd(title string, labels []string, work func(context.Context) tea.Msg) tea.Cmd {
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
			ch <- progressStepMsg{index: 0, state: stepRunning}
			result := work(ctx)
			if operationFailed(result) {
				ch <- progressStepMsg{index: 0, state: stepFailed}
			} else {
				ch <- progressStepMsg{index: 0, state: stepDone}
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
	case subscriptionChangedMsg:
		return result.err != nil
	case websitePHPChangedMsg:
		return result.err != nil
	}
	return true
}

func (progress progressState) render() string {
	lines := []string{progress.title}
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
