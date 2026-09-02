package ui

import (
	"fmt"
	"strings"
)

func (m appModel) detail() string {
	if m.showWebsites && len(m.websites) > 0 {
		website := m.websites[clamp(m.websiteCursor, len(m.websites))]
		return fmt.Sprintf("Domain: %s\nType: %s\nEnabled: %t\nDocument root: %s\nSSL: %t\nForce HTTPS: %t\nHSTS: %t", website.PrimaryDomain, website.Type, website.Enabled, website.DocumentRoot, website.SSLEnabled, website.ForceHTTPS, website.HSTS)
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
