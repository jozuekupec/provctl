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
	if m.showWebsites {
		lines = append(lines, "", "Websites")
		for _, website := range m.websites {
			lines = append(lines, fmt.Sprintf("  %-28s %-10s %t", website.PrimaryDomain, website.Type, website.Enabled))
		}
		if len(m.websites) == 0 {
			lines = append(lines, "  no websites")
		}
	}
	lines = append(lines, "", m.status)
	return strings.Join(lines, "\n")
}
