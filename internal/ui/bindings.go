package ui

import "strings"

// shortcutContext describes a stable interactive surface. The keybar and help
// are both projections of bindings, never separately maintained strings.
type shortcutContext uint8

const (
	shortcutPicker shortcutContext = iota
	shortcutDomains
	shortcutDetail
	shortcutLogs
	shortcutOutput
	shortcutEditorOverview
	shortcutEditorContent
	shortcutEditorRuntime
	shortcutEditorTLS
	shortcutEditorRouting
	shortcutEditorLogs
	shortcutAdminOverview
	shortcutAdminDatabases
	shortcutAdminSSH
	shortcutAdminCron
	shortcutAdminBackups
)

type shortcutAction string

const (
	actionMoveNext      shortcutAction = "move-next"
	actionMovePrevious  shortcutAction = "move-previous"
	actionOpen          shortcutAction = "open"
	actionEdit          shortcutAction = "edit"
	actionCreate        shortcutAction = "create"
	actionArchive       shortcutAction = "archive"
	actionDelete        shortcutAction = "delete"
	actionSSHAccess     shortcutAction = "ssh-access"
	actionRefresh       shortcutAction = "refresh"
	actionFilter        shortcutAction = "filter"
	actionSettings      shortcutAction = "settings"
	actionHelp          shortcutAction = "help"
	actionQuit          shortcutAction = "quit"
	actionPicker        shortcutAction = "picker"
	actionPanelNext     shortcutAction = "panel-next"
	actionPanelPrevious shortcutAction = "panel-previous"
	actionHealth        shortcutAction = "health"
	actionReconcile     shortcutAction = "reconcile"
	actionEditorTabNext shortcutAction = "editor-tab-next"
	actionEditorTabPrev shortcutAction = "editor-tab-previous"
	actionEditorClose   shortcutAction = "editor-close"
	actionToggleEnabled shortcutAction = "toggle-enabled"
	actionDocumentRoot  shortcutAction = "document-root"
	actionPHPVersion    shortcutAction = "php-version"
	actionToggleTLS     shortcutAction = "toggle-tls"
	actionAddAlias      shortcutAction = "add-alias"
	actionRemoveAlias   shortcutAction = "remove-alias"
	actionTarget        shortcutAction = "target"
	actionAccessLog     shortcutAction = "access-log"
	actionErrorLog      shortcutAction = "error-log"
	actionLogDirectory  shortcutAction = "log-directory"
	actionAdmin         shortcutAction = "subscription-admin"
	actionAdminTabNext  shortcutAction = "admin-tab-next"
	actionAdminTabPrev  shortcutAction = "admin-tab-previous"
	actionAdminClose    shortcutAction = "admin-close"
	actionRotateSecret  shortcutAction = "rotate-secret"
)

type binding struct {
	Action   shortcutAction
	Group    string
	Keys     []string
	Label    string
	Help     string
	Bar      string
	Short    string
	Priority int
	Contexts []shortcutContext
}

var (
	contextsAll       = []shortcutContext{shortcutPicker, shortcutDomains, shortcutDetail, shortcutLogs, shortcutOutput, shortcutAdminOverview, shortcutAdminDatabases, shortcutAdminSSH, shortcutAdminCron, shortcutAdminBackups}
	contextsWorkspace = []shortcutContext{shortcutDomains, shortcutDetail, shortcutLogs, shortcutOutput}
	contextsEditor    = []shortcutContext{shortcutEditorOverview, shortcutEditorContent, shortcutEditorRuntime, shortcutEditorTLS, shortcutEditorRouting, shortcutEditorLogs}
	contextsAdmin     = []shortcutContext{shortcutAdminOverview, shortcutAdminDatabases, shortcutAdminSSH, shortcutAdminCron, shortcutAdminBackups}
)

// bindings is the single source of truth for normal-mode shortcuts. Form and
// popup inputs intentionally keep their widget-local bindings.
var bindings = []binding{
	{Action: actionMoveNext, Group: "Navigation", Keys: []string{"j", "down"}, Label: "↑/↓", Help: "move selection or scroll", Bar: "↑/↓ move", Short: "↑/↓", Priority: 3, Contexts: contextsAll},
	{Action: actionMovePrevious, Group: "Navigation", Keys: []string{"k", "up"}, Label: "↑/↓", Help: "move selection or scroll", Contexts: contextsAll},
	{Action: actionPanelNext, Group: "Navigation", Keys: []string{"right", "tab"}, Label: "←/→", Help: "move to next workspace panel", Bar: "←/→ panels", Short: "panels", Priority: 5, Contexts: contextsWorkspace},
	{Action: actionPanelPrevious, Group: "Navigation", Keys: []string{"left", "shift+tab"}, Label: "←/→", Help: "move to previous workspace panel", Contexts: contextsWorkspace},
	{Action: actionPicker, Group: "Navigation", Keys: []string{"s", "esc"}, Label: "s / esc", Help: "return to subscription picker", Bar: "s subscriptions", Short: "s", Priority: 1, Contexts: contextsWorkspace},
	{Action: actionOpen, Group: "Actions", Keys: []string{"enter"}, Label: "enter", Help: "open selected subscription", Bar: "enter open", Short: "enter", Priority: 2, Contexts: []shortcutContext{shortcutPicker}},
	{Action: actionCreate, Group: "Actions", Keys: []string{"n"}, Label: "n", Help: "create subscription", Bar: "n create", Short: "n", Priority: 4, Contexts: []shortcutContext{shortcutPicker}},
	{Action: actionArchive, Group: "Actions", Keys: []string{"a"}, Label: "a", Help: "archive selected subscription", Bar: "a archive", Short: "a", Priority: 6, Contexts: []shortcutContext{shortcutPicker}},
	{Action: actionDelete, Group: "Actions", Keys: []string{"d"}, Label: "d", Help: "delete archived subscription", Bar: "d delete", Short: "d", Priority: 7, Contexts: []shortcutContext{shortcutPicker}},
	{Action: actionSSHAccess, Group: "Actions", Keys: []string{"u"}, Label: "u", Help: "set subscription SSH access", Bar: "u SSH", Short: "u", Priority: 7, Contexts: []shortcutContext{shortcutPicker}},
	{Action: actionRefresh, Group: "Actions", Keys: []string{"r"}, Label: "r", Help: "refresh subscriptions", Bar: "r refresh", Short: "r", Priority: 5, Contexts: contextsAll},
	{Action: actionFilter, Group: "Navigation", Keys: []string{"/"}, Label: "/", Help: "filter subscriptions", Bar: "/ filter", Short: "/", Priority: 4, Contexts: []shortcutContext{shortcutPicker}},
	{Action: actionOpen, Group: "Actions", Keys: []string{"enter"}, Label: "enter", Help: "open selected domain editor", Bar: "enter edit", Short: "enter", Priority: 2, Contexts: []shortcutContext{shortcutDomains}},
	{Action: actionCreate, Group: "Actions", Keys: []string{"n"}, Label: "n", Help: "create domain", Bar: "n create", Short: "n", Priority: 4, Contexts: []shortcutContext{shortcutDomains}},
	{Action: actionFilter, Group: "Navigation", Keys: []string{"/"}, Label: "/", Help: "filter domains", Bar: "/ filter", Short: "/", Priority: 4, Contexts: []shortcutContext{shortcutDomains}},
	{Action: actionHealth, Group: "Actions", Keys: []string{"h"}, Label: "h", Help: "run health checks", Bar: "h health", Short: "h", Priority: 7, Contexts: []shortcutContext{shortcutDomains}},
	{Action: actionReconcile, Group: "Actions", Keys: []string{"R"}, Label: "R", Help: "reconcile subscription configuration", Bar: "R reconcile", Short: "R", Priority: 8, Contexts: []shortcutContext{shortcutDomains}},
	{Action: actionAdmin, Group: "Actions", Keys: []string{"m"}, Label: "m", Help: "manage subscription resources", Bar: "m manage", Short: "m", Priority: 6, Contexts: []shortcutContext{shortcutDomains}},
	{Action: actionSettings, Group: "Other", Keys: []string{","}, Label: ",", Help: "open settings", Bar: ", settings", Short: ",", Priority: 8, Contexts: contextsAll},
	{Action: actionHelp, Group: "Other", Keys: []string{"?"}, Label: "?", Help: "open this help", Bar: "? help", Short: "?", Priority: 0, Contexts: contextsAll},
	{Action: actionQuit, Group: "Other", Keys: []string{"q", "ctrl+c"}, Label: "q", Help: "quit", Bar: "q quit", Short: "q", Priority: 0, Contexts: contextsAll},

	{Action: actionEditorTabPrev, Group: "Navigation", Keys: []string{"shift+left"}, Label: "Shift+←", Help: "previous editor tab", Bar: "⇧←/⇧→ tabs", Short: "tabs", Priority: 2, Contexts: contextsEditor},
	{Action: actionEditorTabNext, Group: "Navigation", Keys: []string{"shift+right"}, Label: "Shift+→", Help: "next editor tab", Contexts: contextsEditor},
	{Action: actionEditorClose, Group: "Navigation", Keys: []string{"esc"}, Label: "esc", Help: "return to workspace", Bar: "esc workspace", Short: "esc", Priority: 3, Contexts: contextsEditor},
	{Action: actionPicker, Group: "Navigation", Keys: []string{"s"}, Label: "s", Help: "return to subscription picker", Bar: "s subscriptions", Short: "s", Priority: 1, Contexts: contextsEditor},
	{Action: actionToggleEnabled, Group: "Actions", Keys: []string{"enter", "e"}, Label: "enter / e", Help: "enable or disable domain", Bar: "enter toggle", Short: "toggle", Priority: 4, Contexts: []shortcutContext{shortcutEditorOverview}},
	{Action: actionDelete, Group: "Actions", Keys: []string{"D"}, Label: "D", Help: "delete domain configuration", Bar: "D delete", Short: "D", Priority: 7, Contexts: []shortcutContext{shortcutEditorOverview}},
	{Action: actionDocumentRoot, Group: "Actions", Keys: []string{"enter", "e"}, Label: "enter / e", Help: "choose document root", Bar: "enter root", Short: "root", Priority: 4, Contexts: []shortcutContext{shortcutEditorContent}},
	{Action: actionPHPVersion, Group: "Actions", Keys: []string{"enter", "p"}, Label: "enter / p", Help: "choose PHP-FPM version", Bar: "enter PHP", Short: "PHP", Priority: 4, Contexts: []shortcutContext{shortcutEditorRuntime}},
	{Action: actionToggleTLS, Group: "Actions", Keys: []string{"enter", "t"}, Label: "enter / t", Help: "enable or disable TLS", Bar: "enter TLS", Short: "TLS", Priority: 4, Contexts: []shortcutContext{shortcutEditorTLS}},
	{Action: actionAddAlias, Group: "Actions", Keys: []string{"a"}, Label: "a", Help: "add domain alias", Bar: "a alias", Short: "a", Priority: 5, Contexts: []shortcutContext{shortcutEditorRouting}},
	{Action: actionRemoveAlias, Group: "Actions", Keys: []string{"A"}, Label: "A", Help: "remove domain alias", Bar: "A remove", Short: "A", Priority: 6, Contexts: []shortcutContext{shortcutEditorRouting}},
	{Action: actionTarget, Group: "Actions", Keys: []string{"enter", "e"}, Label: "enter / e", Help: "edit proxy or redirect target", Bar: "enter target", Short: "target", Priority: 4, Contexts: []shortcutContext{shortcutEditorRouting}},
	{Action: actionAccessLog, Group: "Actions", Keys: []string{"enter", "l"}, Label: "enter / l", Help: "load access log", Bar: "enter access", Short: "access", Priority: 4, Contexts: []shortcutContext{shortcutEditorLogs}},
	{Action: actionErrorLog, Group: "Actions", Keys: []string{"L"}, Label: "L", Help: "load error log", Bar: "L error", Short: "L", Priority: 5, Contexts: []shortcutContext{shortcutEditorLogs}},
	{Action: actionLogDirectory, Group: "Actions", Keys: []string{"e"}, Label: "e", Help: "change log directory", Bar: "e log dir", Short: "log dir", Priority: 6, Contexts: []shortcutContext{shortcutEditorLogs}},

	{Action: actionAdminTabPrev, Group: "Navigation", Keys: []string{"shift+left"}, Label: "Shift+←", Help: "previous administration tab", Bar: "⇧←/⇧→ tabs", Short: "tabs", Priority: 2, Contexts: contextsAdmin},
	{Action: actionAdminTabNext, Group: "Navigation", Keys: []string{"shift+right"}, Label: "Shift+→", Help: "next administration tab", Contexts: contextsAdmin},
	{Action: actionAdminClose, Group: "Navigation", Keys: []string{"esc"}, Label: "esc", Help: "return to workspace", Bar: "esc workspace", Short: "esc", Priority: 3, Contexts: contextsAdmin},
	{Action: actionPicker, Group: "Navigation", Keys: []string{"s"}, Label: "s", Help: "return to subscription picker", Bar: "s subscriptions", Short: "s", Priority: 1, Contexts: contextsAdmin},
	{Action: actionCreate, Group: "Actions", Keys: []string{"n"}, Label: "n", Help: "create database", Bar: "n create", Short: "create", Priority: 4, Contexts: []shortcutContext{shortcutAdminDatabases}},
	{Action: actionRotateSecret, Group: "Actions", Keys: []string{"p"}, Label: "p", Help: "rotate selected database password", Bar: "p password", Short: "password", Priority: 5, Contexts: []shortcutContext{shortcutAdminDatabases}},
	{Action: actionDelete, Group: "Actions", Keys: []string{"D"}, Label: "D", Help: "delete selected database", Bar: "D delete", Short: "delete", Priority: 6, Contexts: []shortcutContext{shortcutAdminDatabases}},
	{Action: actionCreate, Group: "Actions", Keys: []string{"n"}, Label: "n", Help: "add SSH public key", Bar: "n add key", Short: "add", Priority: 4, Contexts: []shortcutContext{shortcutAdminSSH}},
	{Action: actionDelete, Group: "Actions", Keys: []string{"D"}, Label: "D", Help: "remove selected SSH key", Bar: "D remove", Short: "remove", Priority: 5, Contexts: []shortcutContext{shortcutAdminSSH}},
	{Action: actionCreate, Group: "Actions", Keys: []string{"n"}, Label: "n", Help: "add cron job", Bar: "n add", Short: "add", Priority: 4, Contexts: []shortcutContext{shortcutAdminCron}},
	{Action: actionEdit, Group: "Actions", Keys: []string{"enter", "e"}, Label: "enter / e", Help: "edit selected cron job", Bar: "enter edit", Short: "edit", Priority: 5, Contexts: []shortcutContext{shortcutAdminCron}},
	{Action: actionDelete, Group: "Actions", Keys: []string{"D"}, Label: "D", Help: "remove selected cron job", Bar: "D remove", Short: "remove", Priority: 6, Contexts: []shortcutContext{shortcutAdminCron}},
	{Action: actionCreate, Group: "Actions", Keys: []string{"n"}, Label: "n", Help: "create subscription backup", Bar: "n backup", Short: "backup", Priority: 4, Contexts: []shortcutContext{shortcutAdminBackups}},
}

func actionFor(context shortcutContext, key string) shortcutAction {
	for _, binding := range bindings {
		if binding.inContext(context) && containsKey(binding.Keys, key) {
			return binding.Action
		}
	}
	return ""
}

func bindingsFor(context shortcutContext) []binding {
	result := make([]binding, 0, len(bindings))
	for _, binding := range bindings {
		if binding.inContext(context) {
			result = append(result, binding)
		}
	}
	return result
}

func (binding binding) inContext(context shortcutContext) bool {
	for _, candidate := range binding.Contexts {
		if candidate == context {
			return true
		}
	}
	return false
}

func containsKey(keys []string, key string) bool {
	for _, candidate := range keys {
		if candidate == key {
			return true
		}
	}
	return false
}

func keybarFor(context shortcutContext) string {
	parts := make([]string, 0, len(bindings))
	for _, binding := range bindingsFor(context) {
		if binding.Bar != "" {
			parts = append(parts, binding.Bar)
		}
	}
	return strings.Join(parts, " · ")
}

func helpFor(context shortcutContext) []helpSection {
	sections := make([]helpSection, 0, 3)
	positions := map[string]int{}
	for _, binding := range bindingsFor(context) {
		if binding.Group == "" {
			continue
		}
		position, ok := positions[binding.Group]
		if !ok {
			position = len(sections)
			positions[binding.Group] = position
			sections = append(sections, helpSection{title: binding.Group})
		}
		sections[position].rows = append(sections[position].rows, binding.Label+"  "+binding.Help)
	}
	return sections
}
