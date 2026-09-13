package ui

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"provctl/internal/service"
)

// openSubscriptionCreateForm keeps creation in the picker, where there is no
// ambiguous selected domain or subscription target.
func (m appModel) openSubscriptionCreateForm() appModel {
	inputs := make([]textinput.Model, 5)
	for index := range inputs {
		inputs[index] = textinput.New()
		inputs[index].Prompt = ""
		inputs[index].Width = 26
	}
	inputs[0].Focus()
	m.subscriptionCreateForm = subscriptionCreateFormState{open: true, inputs: inputs}
	m.status = ""
	return m
}

func (m appModel) focusSubscriptionCreateField() appModel {
	for index := range m.subscriptionCreateForm.inputs {
		m.subscriptionCreateForm.inputs[index].Blur()
	}
	m.subscriptionCreateForm.inputs[m.subscriptionCreateForm.field].Focus()
	return m
}

func (m appModel) handleSubscriptionCreateFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.subscriptionCreateForm, m.status = subscriptionCreateFormState{}, "cancelled"
		return m, nil
	case "ctrl+s":
		name := m.subscriptionCreateForm.inputs[0].Value()
		options, err := subscriptionCreateOptions(m.subscriptionCreateForm.inputs)
		if err != nil {
			m.status = "invalid quotas: " + err.Error()
			return m, nil
		}
		m.subscriptionCreateForm = subscriptionCreateFormState{}
		return m.askConfirm(confirmState{action: "create-subscription", domain: name, title: "Create subscription", lines: []string{
			"Subscription: " + name,
			"Create a Unix user and isolated hosting home.",
			"Disk: " + quotaSize(options.QuotaDiskBytes) + " · websites: " + quotaCount(options.QuotaWebsites),
			"Databases: " + quotaCount(options.QuotaDatabases) + " · backups: " + quotaCount(options.QuotaBackups),
		}, value: encodeSubscriptionCreateOptions(options)}), nil
	case "tab", "down":
		m.subscriptionCreateForm.field = (m.subscriptionCreateForm.field + 1) % len(m.subscriptionCreateForm.inputs)
		return m.focusSubscriptionCreateField(), nil
	case "shift+tab", "up":
		m.subscriptionCreateForm.field = (m.subscriptionCreateForm.field - 1 + len(m.subscriptionCreateForm.inputs)) % len(m.subscriptionCreateForm.inputs)
		return m.focusSubscriptionCreateField(), nil
	}
	index := m.subscriptionCreateForm.field
	input, command := m.subscriptionCreateForm.inputs[index].Update(msg)
	m.subscriptionCreateForm.inputs[index] = input
	return m, command
}

func (m appModel) createSubscription(ctx context.Context, confirm confirmState) tea.Msg {
	if m.deps.CreateSubscription == nil {
		return subscriptionCreatedMsg{err: context.Canceled, name: confirm.domain}
	}
	options, err := decodeSubscriptionCreateOptions(confirm.value)
	if err != nil {
		return subscriptionCreatedMsg{err: err, name: confirm.domain}
	}
	_, err = m.deps.CreateSubscription(ctx, confirm.domain, options)
	return subscriptionCreatedMsg{err: err, name: confirm.domain}
}

func (m appModel) createSubscriptionCmd(confirm confirmState) tea.Cmd {
	return steppedCmd("Create subscription", []string{"create user and subscription home"}, func(ctx context.Context, _ func(int)) tea.Msg {
		return m.createSubscription(ctx, confirm)
	})
}

func (m appModel) subscriptionCreateFormPopup() string {
	body := []string{
		"Name:             " + m.subscriptionCreateForm.inputs[0].View(),
		"Disk quota:       " + m.subscriptionCreateForm.inputs[1].View(),
		"Website quota:    " + m.subscriptionCreateForm.inputs[2].View(),
		"Database quota:   " + m.subscriptionCreateForm.inputs[3].View(),
		"Backup quota:     " + m.subscriptionCreateForm.inputs[4].View(), "",
		dimStyle.Render("Lowercase letters, digits and hyphens; starts with a letter."),
		dimStyle.Render("Leave quotas blank for unlimited. Disk accepts 20G, 500M, or bytes."),
	}
	return popupBox(m, popupOpts{Size: popupMedium, Fit: fitAuto, Title: "Create subscription", Body: body, Footer: []string{dimStyle.Render("tab field · ctrl+s continue · esc cancel")}})
}

var uiByteSize = regexp.MustCompile(`^([1-9][0-9]*)([KMGT]?)$`)

func subscriptionCreateOptions(inputs []textinput.Model) (service.SubscriptionCreateOptions, error) {
	if len(inputs) != 5 {
		return service.SubscriptionCreateOptions{}, fmt.Errorf("internal form state")
	}
	disk, err := parseQuotaBytes(inputs[1].Value())
	if err != nil {
		return service.SubscriptionCreateOptions{}, err
	}
	parseCount := func(value string) (int, error) {
		if value == "" {
			return 0, nil
		}
		count, err := strconv.Atoi(value)
		if err != nil || count < 0 {
			return 0, fmt.Errorf("counts must be zero or a positive integer")
		}
		return count, nil
	}
	websites, err := parseCount(inputs[2].Value())
	if err != nil {
		return service.SubscriptionCreateOptions{}, err
	}
	databases, err := parseCount(inputs[3].Value())
	if err != nil {
		return service.SubscriptionCreateOptions{}, err
	}
	backups, err := parseCount(inputs[4].Value())
	if err != nil {
		return service.SubscriptionCreateOptions{}, err
	}
	return service.SubscriptionCreateOptions{QuotaDiskBytes: disk, QuotaWebsites: websites, QuotaDatabases: databases, QuotaBackups: backups}, nil
}

func parseQuotaBytes(value string) (int64, error) {
	if value == "" {
		return 0, nil
	}
	match := uiByteSize.FindStringSubmatch(strings.ToUpper(value))
	if match == nil {
		return 0, fmt.Errorf("disk must be a positive size such as 20G")
	}
	amount, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil {
		return 0, err
	}
	for range strings.Index("KMGT", match[2]) + 1 {
		if amount > (1<<63-1)/1024 {
			return 0, fmt.Errorf("disk size is too large")
		}
		amount *= 1024
	}
	return amount, nil
}

func encodeSubscriptionCreateOptions(options service.SubscriptionCreateOptions) string {
	return fmt.Sprintf("%d,%d,%d,%d", options.QuotaDiskBytes, options.QuotaWebsites, options.QuotaDatabases, options.QuotaBackups)
}
func decodeSubscriptionCreateOptions(value string) (service.SubscriptionCreateOptions, error) {
	var options service.SubscriptionCreateOptions
	if _, err := fmt.Sscanf(value, "%d,%d,%d,%d", &options.QuotaDiskBytes, &options.QuotaWebsites, &options.QuotaDatabases, &options.QuotaBackups); err != nil {
		return options, err
	}
	return options, nil
}
func quotaCount(value int) string {
	if value == 0 {
		return "unlimited"
	}
	return strconv.Itoa(value)
}
func quotaSize(value int64) string {
	if value == 0 {
		return "unlimited"
	}
	return fmt.Sprintf("%d bytes", value)
}
