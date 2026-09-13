package ui

import (
	"fmt"
	"strings"

	"provctl/internal/domain"
	"provctl/internal/service"
)

func (m appModel) View() string {
	if !m.ready {
		return "loading…"
	}
	if m.width < minWidth || m.height < minHeight {
		return fmt.Sprintf("provctl needs a terminal of at least %d×%d; current size is %d×%d", minWidth, minHeight, m.width, m.height)
	}
	var view string
	if !m.workspace {
		view = m.renderPicker()
	} else if m.domainEditor.open {
		view = m.renderDomainEditor()
	} else {
		view = m.renderWorkspace()
	}
	if m.help.open {
		return m.overlayCenter(m.helpPopup(), view)
	}
	if m.pathPicker.open {
		view = m.overlayCenter(m.pathPickerPopup(), view)
		if m.pathPicker.confirm != "" {
			return m.overlayCenter(m.pathPickerConfirmPopup(), view)
		}
		return view
	}
	if m.documentRootForm.open {
		return m.overlayCenter(m.documentRootFormPopup(), view)
	}
	if m.websiteCreateForm.open {
		return m.overlayCenter(m.websiteCreateFormPopup(), view)
	}
	if m.aliasForm.open {
		return m.overlayCenter(m.aliasFormPopup(), view)
	}
	if m.targetForm.open {
		return m.overlayCenter(m.targetFormPopup(), view)
	}
	if m.subscriptionCreateForm.open {
		return m.overlayCenter(m.subscriptionCreateFormPopup(), view)
	}
	if m.databaseCreateForm.open {
		return m.overlayCenter(m.databaseCreateFormPopup(), view)
	}
	if m.sshAccessForm.open {
		return m.overlayCenter(m.sshAccessFormPopup(), view)
	}
	if m.sshKeyForm.open {
		return m.overlayCenter(m.sshKeyFormPopup(), view)
	}
	if m.secret.open {
		return m.overlayCenter(m.secretPopup(), view)
	}
	if m.settings.open {
		return m.overlayCenter(m.settingsPopup(), view)
	}
	if m.phpPicker.open {
		return m.overlayCenter(m.phpPickerPopup(), view)
	}
	if m.confirm.action != "" {
		return m.overlayCenter(m.confirmPopup(), view)
	}
	if m.progress.active {
		return m.overlayCenter(m.progressPopup(), view)
	}
	return view
}

func (m appModel) renderDomainEditor() string {
	website, ok := m.selectedWebsite()
	if !ok {
		return m.renderFrame(panel("Domain editor", "No domain selected.", m.width, m.height-2, true))
	}
	title := "Edit domain · " + website.PrimaryDomain
	body := strings.Join([]string{m.domainEditorTabs(), "", m.domainEditorBody(website)}, "\n")
	return m.renderFrame(panel(title, body, m.width, m.height-2, true))
}

func (m appModel) domainEditorBody(website domain.Website) string {
	subscription, _ := m.selectedSubscription()
	phpVersion := valueOrDash(website.PHPVersion)
	switch m.domainEditor.tab {
	case 0:
		return strings.Join([]string{"Domain: " + website.PrimaryDomain, "Subscription: " + subscription.Name, "Type: " + string(website.Type), "Status: " + map[bool]string{true: "enabled", false: "disabled"}[website.Enabled], "", "Enter  toggle enabled state"}, "\n")
	case 1:
		return strings.Join([]string{"Document root: " + valueOrDash(website.DocumentRoot), "Home: " + valueOrDash(subscription.Home), "", "Enter  choose a document root"}, "\n")
	case 2:
		if website.Type != domain.WebsitePHPFPM {
			return "This domain type has no PHP-FPM runtime."
		}
		return strings.Join([]string{"PHP-FPM: " + phpVersion, "", "Enter  choose an installed PHP-FPM version"}, "\n")
	case 3:
		return strings.Join([]string{"TLS: " + map[bool]string{true: "enabled", false: "disabled"}[website.SSLEnabled], "Force HTTPS: " + fmt.Sprint(website.ForceHTTPS), "HSTS: " + fmt.Sprint(website.HSTS), "", "Enter  enable or disable TLS"}, "\n")
	case 4:
		aliases := "—"
		if len(website.Aliases) > 0 {
			aliases = strings.Join(website.Aliases, ", ")
		}
		lines := []string{"Aliases: " + aliases}
		if website.Target != "" {
			lines = append(lines, "Target: "+website.Target)
		}
		lines = append(lines, "", "a/A    add or remove alias")
		if website.Type == domain.WebsiteProxy || website.Type == domain.WebsiteRedirect {
			lines = append(lines, "Enter  edit target")
		}
		return strings.Join(lines, "\n")
	default:
		return "Enter  load access log\nL      load error log"
	}
}

func (m appModel) renderPicker() string {
	body := panel(m.subscriptionPanelTitle(), m.renderSubscriptions(m.height-4), m.width, m.height-2, true)
	return m.renderFrame(body)
}

func (m appModel) renderWorkspace() string {
	layout := computeLayout(m.width, m.height, m.focus)
	left := strings.Join([]string{
		panel("Subscription", m.subscriptionMetadata(), layout.leftWidth, layout.metadata, false),
		panel(m.websitePanelTitle(), m.renderWebsites(layout.domains-2), layout.leftWidth, layout.domains, m.focus == focusWebsites),
		panel(m.detailTitle(), m.detailLines(layout.detail-2), layout.leftWidth, layout.detail, m.focus == focusDetail),
	}, "\n")
	output := m.outputLines(layout.output - 2)
	right := strings.Join([]string{
		panel("Logs", m.logsLines(layout.logs-2), layout.rightWidth, layout.logs, m.focus == focusLogs),
		panel("Output", output, layout.rightWidth, layout.output, m.focus == focusOutput),
	}, "\n")
	body := joinColumns(left, right)
	return m.renderFrame(body)
}

func (m appModel) subscriptionPanelTitle() string {
	return m.filterPanelTitle("Subscriptions", m.subscriptionFilter, len(m.visibleSubscriptions()), len(m.items))
}

func (m appModel) websitePanelTitle() string {
	return m.filterPanelTitle("Domains", m.websiteFilter, len(m.visibleWebsites()), len(m.websites))
}

func (m appModel) filterPanelTitle(title string, filter filterState, visible, total int) string {
	if filter.query() == "" {
		return title
	}
	return fmt.Sprintf("%s · /%s · %d/%d", title, filter.query(), visible, total)
}

func (m appModel) renderFrame(body string) string {
	status := dimStyle.Render(truncate(m.status, m.width))
	keybar := keybarStyle.Render(truncate(m.keybar(), m.width))
	return strings.Join([]string{body, fit(status, m.width), fit(keybar, m.width)}, "\n")
}

func joinColumns(left, right string) string {
	leftRows, rightRows := strings.Split(left, "\n"), strings.Split(right, "\n")
	rows := max(len(leftRows), len(rightRows))
	result := make([]string, rows)
	for index := range rows {
		var a, b string
		if index < len(leftRows) {
			a = leftRows[index]
		}
		if index < len(rightRows) {
			b = rightRows[index]
		}
		result[index] = a + b
	}
	return strings.Join(result, "\n")
}

func (m appModel) renderSubscriptions(rows int) string {
	items := m.visibleSubscriptions()
	if len(items) == 0 {
		if m.subscriptionFilter.query() != "" {
			return "No matching subscriptions."
		}
		return "No subscriptions."
	}
	start, end := listWindow(m.cursor, len(items), rows)
	lines := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		item := items[index]
		usage, known := m.usage[item.ID]
		line := fmt.Sprintf("  %-18s [%s] · %s", item.Name, item.Status, m.subscriptionSummary(item, usage, known))
		if index == m.cursor {
			line = selectedStyle.Render("> " + strings.TrimPrefix(line, "  "))
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (m appModel) renderWebsites(rows int) string {
	if !m.showWebsites {
		return "Enter opens websites for the selected subscription."
	}
	websites := m.visibleWebsites()
	if len(websites) == 0 {
		if m.websiteFilter.query() != "" {
			return "No matching domains."
		}
		return "No websites."
	}
	start, end := listWindow(m.websiteCursor, len(websites), rows)
	lines := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		website := websites[index]
		state := "off"
		if website.Enabled {
			state = "on"
		}
		tag := "[" + string(website.Type) + "]"
		line := fmt.Sprintf("  %-24s %-10s %s", website.PrimaryDomain, tag, state)
		if index == m.websiteCursor {
			line = selectedStyle.Render("> " + strings.TrimPrefix(line, "  "))
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (m appModel) subscriptionSummary(subscription domain.Subscription, usage service.SubscriptionUsage, known bool) string {
	websiteUsage := "sites —/" + quotaCount(subscription.QuotaWebsites)
	diskUsage := "disk —/" + quotaSize(subscription.QuotaDiskBytes)
	if known {
		websiteUsage = fmt.Sprintf("sites %d", usage.ActiveWebsites)
		if usage.DisabledWebsites > 0 {
			websiteUsage += fmt.Sprintf("+%d off", usage.DisabledWebsites)
		}
		websiteUsage += "/" + quotaCount(subscription.QuotaWebsites)
		if usage.DiskUsageKnown {
			diskUsage = quotaSize(usage.DiskUsedBytes) + "/" + quotaSize(subscription.QuotaDiskBytes)
		}
	}
	return websiteUsage + " · " + diskUsage + " · db " + quotaCount(subscription.QuotaDatabases)
}

func (m appModel) subscriptionMetadata() string {
	s, ok := m.selectedSubscription()
	if !ok {
		return "No subscription selected."
	}
	return strings.Join([]string{
		"Name: " + s.Name, "Status: " + s.Status, "User: " + s.UnixUser,
		"Home: " + valueOrDash(s.Home),
		"SSH: " + valueOrDash(s.SSHAccess),
	}, "\n")
}

func (m appModel) detailLines(rows int) string {
	return window(m.detail(), rows, m.detailScroll, false)
}

func (m appModel) outputLines(rows int) string {
	if len(m.output.lines) == 0 {
		return "No output yet."
	}
	return window(strings.Join(m.output.lines, "\n"), rows, m.outputScroll, true)
}

func (m appModel) logsLines(rows int) string {
	if len(m.logs.lines) == 0 {
		return "Press l for access or L for error log."
	}
	return window(strings.Join(m.logs.lines, "\n"), rows, m.outputScroll, true)
}

func window(contents string, rows, scroll int, fromBottom bool) string {
	lines := strings.Split(contents, "\n")
	if rows <= 0 || len(lines) == 0 {
		return ""
	}
	maxScroll := max(0, len(lines)-rows)
	scroll = min(max(0, scroll), maxScroll)
	start := scroll
	if fromBottom {
		start = maxScroll - scroll
	}
	end := min(len(lines), start+rows)
	return strings.Join(lines[start:end], "\n")
}

func (m appModel) keybar() string {
	if m.subscriptionFilter.active {
		return m.subscriptionFilter.activeSummary("subscriptions", len(m.visibleSubscriptions()), len(m.items))
	}
	if m.websiteFilter.active {
		return m.websiteFilter.activeSummary("domains", len(m.visibleWebsites()), len(m.websites))
	}
	if !m.workspace {
		if summary := m.subscriptionFilter.activeSummary("subscriptions", len(m.visibleSubscriptions()), len(m.items)); summary != "" {
			return summary
		}
		return "↑/↓ select · enter open · n create · a archive · d delete · / filter · , settings · ? help · q quit"
	}
	if m.domainEditor.open {
		return "⇧←/⇧→ tabs · enter action · esc workspace · s subscriptions · ? help · q quit"
	}
	if m.focus == focusWebsites {
		if summary := m.websiteFilter.activeSummary("domains", len(m.visibleWebsites()), len(m.websites)); summary != "" {
			return summary
		}
	}
	switch m.focus {
	case focusWebsites:
		return "←/→ panels · ↑/↓ select · enter edit · n create · D delete · a/A aliases · p PHP · e toggle · E root · T target · t TLS · l/L logs · s subscriptions"
	case focusDetail:
		return "←/→ panels · read-only preview · s subscriptions · ? help"
	case focusLogs:
		return "←/→ panels · ↑/↓ scroll · l/L logs · s subscriptions · ? help"
	default:
		return "←/→ panels · ↑/↓ scroll · h health · R reconcile · s subscriptions · ? help"
	}
}
