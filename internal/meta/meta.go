// Package meta centralizes product identity and system-owned paths.
package meta

const (
	Name         = "provctl"
	ConfigDir    = "/etc/provctl"
	ConfigFile   = ConfigDir + "/config.toml"
	StateDir     = "/var/lib/provctl"
	DatabaseFile = StateDir + "/provctl.db"
	LogDir       = "/var/log/provctl"
	TemplateDir  = "/usr/share/provctl/templates"
	PHPConfigDir = "/etc/php"
	FilePrefix   = "provctl-"
)

// Version is set during release builds with -ldflags -X provctl/internal/meta.Version=….
var Version = "dev"
