package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"

	"provctl/internal/domain"
)

type filterState struct {
	input  textinput.Model
	active bool
}

func newFilter() filterState {
	input := textinput.New()
	input.Prompt = "/"
	return filterState{input: input}
}

func (filter filterState) query() string { return strings.TrimSpace(filter.input.Value()) }

func (filter filterState) activeSummary(label string, visible, total int) string {
	count := filterCountStyle.Render("%d/%d matches")
	if filter.active {
		return filterLabelStyle.Render("Filter "+label+":") + " " + filter.input.View() + "  " + fmt.Sprintf(count, visible, total) + dimStyle.Render(" · enter apply · esc clear")
	}
	if filter.query() == "" {
		return ""
	}
	return filterLabelStyle.Render("/"+filter.query()) + " " + fmt.Sprintf(count, visible, total) + dimStyle.Render(" · esc clear")
}

func (m appModel) visibleSubscriptions() []domain.Subscription {
	query := strings.ToLower(m.subscriptionFilter.query())
	if query == "" {
		return m.items
	}
	items := make([]domain.Subscription, 0, len(m.items))
	for _, item := range m.items {
		if strings.Contains(strings.ToLower(item.Name), query) || strings.Contains(strings.ToLower(item.Status), query) || strings.Contains(strings.ToLower(item.Home), query) {
			items = append(items, item)
		}
	}
	return items
}

func (m appModel) visibleWebsites() []domain.Website {
	query := strings.ToLower(m.websiteFilter.query())
	if query == "" {
		return m.websites
	}
	items := make([]domain.Website, 0, len(m.websites))
	for _, item := range m.websites {
		if strings.Contains(strings.ToLower(item.PrimaryDomain), query) || strings.Contains(strings.ToLower(string(item.Type)), query) || strings.Contains(strings.ToLower(strings.Join(item.Aliases, " ")), query) {
			items = append(items, item)
		}
	}
	return items
}

func (m appModel) selectedSubscription() (domain.Subscription, bool) {
	items := m.visibleSubscriptions()
	if len(items) == 0 {
		return domain.Subscription{}, false
	}
	return items[clamp(m.cursor, len(items))], true
}

func (m appModel) selectedWebsite() (domain.Website, bool) {
	items := m.visibleWebsites()
	if len(items) == 0 {
		return domain.Website{}, false
	}
	return items[clamp(m.websiteCursor, len(items))], true
}
