package ui

import (
	"fmt"
	"strings"
)

func (m appModel) detail() string {
	if m.detailView == detailDomain && m.showWebsites {
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
	if m.detailView == detailDatabases {
		if len(m.databases) == 0 {
			return "No databases."
		}
		lines := make([]string, 0, len(m.databases)+1)
		lines = append(lines, "Subscription: "+subscription.Name)
		for _, database := range m.databases {
			lines = append(lines, database.Name+" · "+database.User+"@"+database.Host+" · "+database.Charset)
		}
		return strings.Join(lines, "\n")
	}
	if m.detailView == detailSSHKeys {
		if len(m.sshKeys) == 0 {
			return "No SSH keys."
		}
		lines := make([]string, 0, len(m.sshKeys)+1)
		lines = append(lines, "Subscription: "+subscription.Name)
		for _, key := range m.sshKeys {
			line := key.Fingerprint
			if key.Comment != "" {
				line += " · " + key.Comment
			}
			lines = append(lines, line)
		}
		return strings.Join(lines, "\n")
	}
	return fmt.Sprintf("Subscription: %s\nStatus: %s\nUser: %s\nHome: %s\nWebsites: %d", subscription.Name, subscription.Status, subscription.UnixUser, subscription.Home, len(m.websites))
}

func (m appModel) detailTitle() string {
	if m.detailView == detailDatabases {
		return "Databases"
	}
	if m.detailView == detailSSHKeys {
		return "SSH keys"
	}
	return "Detail"
}

func valueOrDash(value string) string {
	if value == "" {
		return "—"
	}
	return value
}
