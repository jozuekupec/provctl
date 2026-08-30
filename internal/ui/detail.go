package ui

import "fmt"

func (m appModel) detail() string {
	if m.showWebsites && len(m.websites) > 0 {
		website := m.websites[clamp(m.websiteCursor, len(m.websites))]
		return fmt.Sprintf("Domain: %s\nType: %s\nEnabled: %t\nDocument root: %s", website.PrimaryDomain, website.Type, website.Enabled, website.DocumentRoot)
	}
	if len(m.items) == 0 {
		return "No selection."
	}
	subscription := m.items[clamp(m.cursor, len(m.items))]
	return fmt.Sprintf("Subscription: %s\nStatus: %s\nUser: %s\nHome: %s\nPHP: %s", subscription.Name, subscription.Status, subscription.UnixUser, subscription.Home, subscription.PHPVersion)
}
