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
)

const (
	minWidth  = 80
	minHeight = 24
)
