package ui

import (
	"fmt"
	"strings"
)

func (m appModel) View() string {
	if !m.ready {
		return "loading…"
	}
	lines := []string{"provctl — Subscriptions", ""}
	for i, item := range m.items {
		marker := " "
		if i == m.cursor {
			marker = ">"
		}
		lines = append(lines, fmt.Sprintf("%s %-20s %-10s %s", marker, item.Name, item.Status, item.PHPVersion))
	}
	if len(m.items) == 0 {
		lines = append(lines, "  no subscriptions")
	}
	lines = append(lines, "", m.status)
	return strings.Join(lines, "\n")
}
