package ui

import (
	"fmt"
	"strings"

	"provctl/internal/domain"
)

func (m appModel) detail() string {
	if m.showWebsites && len(m.websites) > 0 {
		website := m.websites[clamp(m.websiteCursor, len(m.websites))]
		subscription := domain.Subscription{}
		if len(m.items) > 0 {
			subscription = m.items[clamp(m.cursor, len(m.items))]
		}
		phpVersion := website.PHPVersion
		if phpVersion == "" {
			phpVersion = subscription.PHPVersion
		}
		aliases := "—"
		if len(website.Aliases) > 0 {
			aliases = strings.Join(website.Aliases, ", ")
		}
		lines := []string{
			"Domain: " + website.PrimaryDomain,
			"Subscription: " + subscription.Name,
			"Status: " + map[bool]string{true: "enabled", false: "disabled"}[website.Enabled],
			"Type: " + string(website.Type),
			"PHP-FPM: " + valueOrDash(phpVersion),
			"Home: " + valueOrDash(subscription.Home),
			"Document root: " + valueOrDash(website.DocumentRoot),
			"Aliases: " + aliases,
			"TLS: " + map[bool]string{true: "enabled", false: "disabled"}[website.SSLEnabled],
			"Force HTTPS: " + fmt.Sprint(website.ForceHTTPS),
			"HSTS: " + fmt.Sprint(website.HSTS),
		}
		if website.Target != "" {
			lines = append(lines, "Target: "+website.Target)
		}
		return strings.Join(lines, "\n")
	}
	if len(m.items) == 0 {
		return "No selection."
	}
	subscription := m.items[clamp(m.cursor, len(m.items))]
	names := make([]string, 0, len(m.databases))
	for _, database := range m.databases {
		names = append(names, database.Name)
	}
	return fmt.Sprintf("Subscription: %s\nStatus: %s\nUser: %s\nHome: %s\nPHP: %s\nWebsites: %d\nDatabases: %s", subscription.Name, subscription.Status, subscription.UnixUser, subscription.Home, subscription.PHPVersion, len(m.websites), strings.Join(names, ", "))
}

func valueOrDash(value string) string {
	if value == "" {
		return "—"
	}
	return value
}
