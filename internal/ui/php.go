package ui

import (
	"fmt"
	"strings"
)

func (m appModel) phpPickerPopup() string {
	width := min(68, max(44, m.width-12))
	height := min(16, max(10, m.height-8))
	lines := []string{"Choose an installed PHP-FPM version."}
	if m.phpPicker.loading {
		lines = append(lines, "", "Loading installed versions…")
	} else if len(m.phpPicker.items) == 0 {
		lines = append(lines, "", "No installed PHP-FPM versions found.")
	} else {
		lines = append(lines, "")
		website, _ := m.selectedWebsite()
		for index, item := range m.phpPicker.items {
			state := "inactive"
			if item.Active {
				state = "active"
			}
			if item.Version == website.PHPVersion {
				state += " · selected"
			}
			line := fmt.Sprintf("  PHP %-4s %s", item.Version, state)
			if index == m.phpPicker.cursor {
				line = selectedStyle.Render("> " + strings.TrimPrefix(line, "  "))
			}
			lines = append(lines, line)
		}
	}
	footer := dimStyle.Render("↑/↓ select · enter continue · esc cancel")
	return panel("PHP-FPM version", popupFooter(lines, footer, height), width, height, true)
}
