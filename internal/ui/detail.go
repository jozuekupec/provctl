package ui

import (
	"fmt"
	"strings"
)

func (m appModel) detail() string {
	if m.showWebsites {
		website, websiteOK := m.selectedWebsite()
		if !websiteOK {
			return "No domain selected."
		}
		subscription, _ := m.selectedSubscription()
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
	subscription, ok := m.selectedSubscription()
	if !ok {
		return "No selection."
	}
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
