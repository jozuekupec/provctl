// Package meta centralizes product identity and system-owned paths.
package meta

const (
	Name         = "provctl"
	ConfigDir    = "/etc/provctl"
	ConfigFile   = ConfigDir + "/config.toml"
	StateDir     = "/var/lib/provctl"
	DatabaseFile = StateDir + "/provctl.db"
	LockFile     = "/run/provctl.lock"
	LogDir       = "/var/log/provctl"
	TemplateDir  = "/usr/share/provctl/templates"
	PHPConfigDir = "/etc/php"
	CertbotCron  = "/etc/cron.d/certbot"
	DeployHook   = "/etc/letsencrypt/renewal-hooks/deploy/00-provctl.sh"
	FilePrefix   = "provctl-"
)

// Version is set during release builds with -ldflags -X provctl/internal/meta.Version=….
var Version = "dev"
