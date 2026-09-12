package ui

import "github.com/charmbracelet/lipgloss"

var (
	accentColor = lipgloss.Color("39")
	dimColor    = lipgloss.Color("245")
	faintColor  = lipgloss.Color("240")
	warnColor   = lipgloss.Color("214")
)

var (
	panelTitleStyle  = lipgloss.NewStyle().Foreground(dimColor)
	panelActiveStyle = lipgloss.NewStyle().Foreground(accentColor).Bold(true)
	panelBorderStyle = lipgloss.NewStyle().Foreground(faintColor)
	panelFocusBorder = lipgloss.NewStyle().Foreground(accentColor)
	selectedStyle    = lipgloss.NewStyle().Foreground(accentColor).Bold(true)
	dimStyle         = lipgloss.NewStyle().Foreground(dimColor)
	keybarStyle      = lipgloss.NewStyle().Foreground(dimColor)
	confirmStyle     = lipgloss.NewStyle().Foreground(warnColor).Bold(true)
	filterLabelStyle = lipgloss.NewStyle().Foreground(accentColor).Bold(true)
	filterCountStyle = lipgloss.NewStyle().Foreground(warnColor).Bold(true)

	// Settings scopes use the same connected-tab treatment as branchctl. The
	// active tab has no bottom line, so it reads as the selected form surface.
	tabActiveStyle   = lipgloss.NewStyle().Border(tabBorderWithBottom("┘", " ", "└")).BorderForeground(accentColor).Bold(true).Padding(0, 1)
	tabInactiveStyle = lipgloss.NewStyle().Border(tabBorderWithBottom("┴", "─", "┴")).BorderForeground(dimColor).Padding(0, 1)
)

func tabBorderWithBottom(left, middle, right string) lipgloss.Border {
	border := lipgloss.RoundedBorder()
	border.BottomLeft = left
	border.Bottom = middle
	border.BottomRight = right
	return border
}

const (
	minWidth  = 80
	minHeight = 24
)
