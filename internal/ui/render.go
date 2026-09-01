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
		if i == m.cursor && m.focus == focusSubscriptions {
			marker = ">"
		}
		lines = append(lines, fmt.Sprintf("%s %-20s %-10s %s", marker, item.Name, item.Status, item.PHPVersion))
	}
	if len(m.items) == 0 {
		lines = append(lines, "  no subscriptions")
	}
	if m.showWebsites {
		lines = append(lines, "", "Websites")
		for i, website := range m.websites {
			marker := " "
			if i == m.websiteCursor && m.focus == focusWebsites {
				marker = ">"
			}
			lines = append(lines, fmt.Sprintf("%s %-28s %-10s %t", marker, website.PrimaryDomain, website.Type, website.Enabled))
		}
		if len(m.websites) == 0 {
			lines = append(lines, "  no websites")
		}
	}
	if m.focus == focusDetail {
		lines = append(lines, "", "Detail", m.detail())
	}
	if m.focus == focusOutput {
		lines = append(lines, "", "Output")
		lines = append(lines, m.output.lines...)
	}
	if m.confirm.action != "" {
		lines = append(lines, "", "Confirm", fmt.Sprintf("Set %s enabled=%t? Press y to continue, esc to cancel.", m.confirm.domain, m.confirm.enabled))
	}
	lines = append(lines, "", m.status)
	return strings.Join(lines, "\n")
}
