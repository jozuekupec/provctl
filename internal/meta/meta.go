// Package meta centralizes product identity and system-owned paths.
package meta

const (
	Name                  = "provctl"
	ConfigDir             = "/etc/provctl"
	ConfigFile            = ConfigDir + "/config.toml"
	StateDir              = "/var/lib/provctl"
	DatabaseFile          = StateDir + "/provctl.db"
	LockFile              = "/run/provctl.lock"
	LogDir                = "/var/log/provctl"
	TemplateDir           = "/usr/share/provctl/templates"
	PHPConfigDir          = "/etc/php"
	CertbotCron           = "/etc/cron.d/certbot"
	DeployHook            = "/etc/letsencrypt/renewal-hooks/deploy/00-provctl.sh"
	LetsEncryptLiveDir    = "/etc/letsencrypt/live"
	FilePrefix            = "provctl-"
	DefaultSSLDir         = StateDir + "/default-ssl"
	DefaultSSLCertificate = DefaultSSLDir + "/certificate.pem"
	DefaultSSLKey         = DefaultSSLDir + "/private-key.pem"
	LogrotateConfig       = "/etc/logrotate.d/provctl"
	AuditLog              = LogDir + "/audit.jsonl"
	LoginShell            = "/bin/bash"
	NoLoginShell          = "/usr/sbin/nologin"
)

// Version is set during release builds with -ldflags -X provctl/internal/meta.Version=….
var Version = "dev"
