package ui

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"provctl/internal/config"
)

const (
	settingText = "text"
	settingBool = "bool"
	settingInt  = "int"
	settingList = "list"
)

type settingField struct{ section, key, label, kind string }

var settingScopes = []string{"Meta", "Paths", "Apache", "PHP", "MariaDB", "Users", "SSL", "Logs", "Limits"}
var settingFields = []settingField{
	{"meta", "config_version", "Config version", settingInt},
	{"paths", "vhosts", "VHosts directory", settingText}, {"paths", "backups", "Backups directory", settingText}, {"paths", "acme_challenge", "ACME challenge directory", settingText},
	{"apache", "service", "Service", settingText}, {"apache", "sites_available", "Sites available", settingText}, {"apache", "sites_enabled", "Sites enabled", settingText}, {"apache", "proxy_timeout", "Proxy timeout (seconds)", settingInt}, {"apache", "allowed_proxy_hosts", "Allowed proxy hosts (CSV)", settingList},
	{"php", "default_version", "Default PHP version", settingText}, {"php", "max_children", "Max children", settingInt}, {"php", "memory_limit", "Memory limit", settingText}, {"php", "upload_max", "Upload maximum", settingText}, {"php", "max_exec_time", "Max execution time", settingInt},
	{"mariadb", "enabled", "Enabled", settingBool}, {"mariadb", "host", "Host", settingText}, {"mariadb", "defaults_file", "Defaults file", settingText},
	{"users", "uid_min", "Minimum UID", settingInt}, {"users", "uid_max", "Maximum UID", settingInt}, {"users", "shell", "Login shell", settingText},
	{"ssl", "email", "ACME email", settingText}, {"ssl", "staging", "ACME staging", settingBool}, {"ssl", "server", "ACME server URL", settingText},
	{"logs", "retention_days", "Retention days", settingInt}, {"logs", "compress", "Compress", settingBool},
	{"limits", "lock_timeout_seconds", "Lock timeout (seconds)", settingInt},
}

func (m appModel) openSettings() appModel {
	values := make(map[int]string, len(settingFields))
	for index := range settingFields {
		values[index] = settingValue(m.deps.Config, index)
	}
	m.settings = settingsState{open: true, values: values}
	m.status = ""
	return m.focusSettingInput()
}

func (m appModel) settingsFields() []int {
	section := strings.ToLower(settingScopes[m.settings.scope])
	fields := make([]int, 0, 5)
	for index, field := range settingFields {
		if field.section == section {
			fields = append(fields, index)
		}
	}
	return fields
}

func (m appModel) activeSetting() int {
	fields := m.settingsFields()
	if m.settings.field < 0 || m.settings.field >= len(fields) {
		return -1
	}
	return fields[m.settings.field]
}

func (m appModel) focusSettingInput() appModel {
	field := m.activeSetting()
	if field < 0 || settingFields[field].kind == settingBool {
		m.settings.input.Blur()
		return m
	}
	input := textinput.New()
	input.Prompt = ""
	input.SetValue(m.settings.values[field])
	input.Width = max(12, m.popupInnerW(popupLarge)-34)
	input.Focus()
	m.settings.input = input
	return m
}

func (m appModel) persistSettingInput() appModel {
	if field := m.activeSetting(); field >= 0 && settingFields[field].kind != settingBool {
		m.settings.values[field] = m.settings.input.Value()
	}
	return m
}

func (m appModel) changeSettingsScope(delta int) appModel {
	m = m.persistSettingInput()
	m.settings.scope = (m.settings.scope + delta + len(settingScopes)) % len(settingScopes)
	m.settings.field = 0
	return m.focusSettingInput()
}

func (m appModel) moveSettingsField(delta int) appModel {
	m = m.persistSettingInput()
	fields := m.settingsFields()
	m.settings.field = (m.settings.field + delta + len(fields)) % len(fields)
	return m.focusSettingInput()
}

func (m appModel) saveSettings(ctx context.Context) tea.Msg {
	m = m.persistSettingInput()
	cfg, err := buildSettingsConfig(m.deps.Config, m.settings.values)
	if err != nil {
		return settingsSavedMsg{err: err}
	}
	if m.deps.SaveConfig == nil {
		return settingsSavedMsg{err: context.Canceled}
	}
	return settingsSavedMsg{cfg: cfg, err: m.deps.SaveConfig(ctx, cfg)}
}

func (m appModel) saveSettingsCmd() tea.Cmd {
	return func() tea.Msg { return m.saveSettings(context.Background()) }
}

func (m appModel) handleSettingsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		m.settings, m.status = settingsState{}, "settings discarded"
		return m, nil
	case tea.KeyCtrlS, tea.KeyEnter:
		m.status = "saving settings…"
		return m, m.saveSettingsCmd()
	case tea.KeyTab, tea.KeyDown:
		return m.moveSettingsField(1), nil
	case tea.KeyShiftTab, tea.KeyUp:
		return m.moveSettingsField(-1), nil
	}
	switch msg.String() {
	case "shift+left":
		return m.changeSettingsScope(-1), nil
	case "shift+right":
		return m.changeSettingsScope(1), nil
	}
	field := m.activeSetting()
	if field < 0 {
		return m, nil
	}
	if settingFields[field].kind == settingBool {
		switch msg.String() {
		case "left", "right", " ":
			m.settings.values[field] = strconv.FormatBool(m.settings.values[field] != "true")
		}
		return m, nil
	}
	input, command := m.settings.input.Update(msg)
	m.settings.input = input
	return m, command
}

func (m appModel) settingsPopup() string {
	inner := m.popupInnerW(popupLarge)
	tabs := make([]string, len(settingScopes))
	for index, label := range settingScopes {
		if index == m.settings.scope {
			tabs[index] = selectedStyle.Render("[" + label + "]")
		} else {
			tabs[index] = dimStyle.Render(label)
		}
	}
	body := []string{strings.Join(tabs, " "), ""}
	for _, index := range m.settingsFields() {
		field, value := settingFields[index], m.settings.values[index]
		if index == m.activeSetting() && field.kind != settingBool {
			value = m.settings.input.View()
		}
		if field.kind == settingBool {
			value = map[bool]string{true: "yes", false: "no"}[value == "true"]
		}
		line := field.label + ": " + value
		if index == m.activeSetting() {
			line = selectedStyle.Render("▸ " + line)
		} else {
			line = "  " + line
		}
		body = append(body, line)
	}
	return popupBox(m, popupOpts{Size: popupLarge, Fit: fitFixed, Title: "Settings", Body: popupWrap(body, inner), Footer: []string{dimStyle.Render("⇧←/⇧→ scope · tab/↑/↓ field · ←/→ toggle · enter/ctrl+s save · esc cancel")}})
}

func settingValue(cfg config.Config, index int) string {
	f := settingFields[index]
	switch f.section + "." + f.key {
	case "meta.config_version":
		return strconv.Itoa(cfg.Meta.ConfigVersion)
	case "paths.vhosts":
		return cfg.Paths.VHosts
	case "paths.backups":
		return cfg.Paths.Backups
	case "paths.acme_challenge":
		return cfg.Paths.ACMEChallenge
	case "apache.service":
		return cfg.Apache.Service
	case "apache.sites_available":
		return cfg.Apache.SitesAvailable
	case "apache.sites_enabled":
		return cfg.Apache.SitesEnabled
	case "apache.proxy_timeout":
		return strconv.Itoa(cfg.Apache.ProxyTimeout)
	case "apache.allowed_proxy_hosts":
		return strings.Join(cfg.Apache.AllowedProxyHosts, ", ")
	case "php.default_version":
		return cfg.PHP.DefaultVersion
	case "php.max_children":
		return strconv.Itoa(cfg.PHP.MaxChildren)
	case "php.memory_limit":
		return cfg.PHP.MemoryLimit
	case "php.upload_max":
		return cfg.PHP.UploadMax
	case "php.max_exec_time":
		return strconv.Itoa(cfg.PHP.MaxExecTime)
	case "mariadb.enabled":
		return strconv.FormatBool(cfg.MariaDB.Enabled)
	case "mariadb.host":
		return cfg.MariaDB.Host
	case "mariadb.defaults_file":
		return cfg.MariaDB.DefaultsFile
	case "users.uid_min":
		return strconv.Itoa(cfg.Users.UIDMin)
	case "users.uid_max":
		return strconv.Itoa(cfg.Users.UIDMax)
	case "users.shell":
		return cfg.Users.Shell
	case "ssl.email":
		return cfg.SSL.Email
	case "ssl.staging":
		return strconv.FormatBool(cfg.SSL.Staging)
	case "ssl.server":
		return cfg.SSL.Server
	case "logs.retention_days":
		return strconv.Itoa(cfg.Logs.RetentionDays)
	case "logs.compress":
		return strconv.FormatBool(cfg.Logs.Compress)
	default:
		return strconv.Itoa(cfg.Limits.LockTimeoutSeconds)
	}
}

func buildSettingsConfig(cfg config.Config, values map[int]string) (config.Config, error) {
	for index, field := range settingFields {
		value := strings.TrimSpace(values[index])
		if field.kind == settingInt {
			if _, err := strconv.Atoi(value); err != nil {
				return cfg, fmt.Errorf("%s must be an integer", field.label)
			}
		}
		if field.kind == settingBool && value != "true" && value != "false" {
			return cfg, fmt.Errorf("%s must be yes or no", field.label)
		}
		switch field.section + "." + field.key {
		case "meta.config_version":
			cfg.Meta.ConfigVersion, _ = strconv.Atoi(value)
		case "paths.vhosts":
			cfg.Paths.VHosts = value
		case "paths.backups":
			cfg.Paths.Backups = value
		case "paths.acme_challenge":
			cfg.Paths.ACMEChallenge = value
		case "apache.service":
			cfg.Apache.Service = value
		case "apache.sites_available":
			cfg.Apache.SitesAvailable = value
		case "apache.sites_enabled":
			cfg.Apache.SitesEnabled = value
		case "apache.proxy_timeout":
			cfg.Apache.ProxyTimeout, _ = strconv.Atoi(value)
		case "apache.allowed_proxy_hosts":
			cfg.Apache.AllowedProxyHosts = splitCSV(value)
		case "php.default_version":
			cfg.PHP.DefaultVersion = value
		case "php.max_children":
			cfg.PHP.MaxChildren, _ = strconv.Atoi(value)
		case "php.memory_limit":
			cfg.PHP.MemoryLimit = value
		case "php.upload_max":
			cfg.PHP.UploadMax = value
		case "php.max_exec_time":
			cfg.PHP.MaxExecTime, _ = strconv.Atoi(value)
		case "mariadb.enabled":
			cfg.MariaDB.Enabled = value == "true"
		case "mariadb.host":
			cfg.MariaDB.Host = value
		case "mariadb.defaults_file":
			cfg.MariaDB.DefaultsFile = value
		case "users.uid_min":
			cfg.Users.UIDMin, _ = strconv.Atoi(value)
		case "users.uid_max":
			cfg.Users.UIDMax, _ = strconv.Atoi(value)
		case "users.shell":
			cfg.Users.Shell = value
		case "ssl.email":
			cfg.SSL.Email = value
		case "ssl.staging":
			cfg.SSL.Staging = value == "true"
		case "ssl.server":
			cfg.SSL.Server = value
		case "logs.retention_days":
			cfg.Logs.RetentionDays, _ = strconv.Atoi(value)
		case "logs.compress":
			cfg.Logs.Compress = value == "true"
		case "limits.lock_timeout_seconds":
			cfg.Limits.LockTimeoutSeconds, _ = strconv.Atoi(value)
		}
	}
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func splitCSV(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	for index := range parts {
		parts[index] = strings.TrimSpace(parts[index])
	}
	return parts
}
