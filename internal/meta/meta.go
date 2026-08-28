// Package meta centralizes product identity and system-owned paths.
package meta

const (
	Name        = "provctl"
	ConfigDir   = "/etc/provctl"
	StateDir    = "/var/lib/provctl"
	LogDir      = "/var/log/provctl"
	TemplateDir = "/usr/share/provctl/templates"
	FilePrefix  = "provctl-"
)

// Version is set during release builds with -ldflags -X provctl/internal/meta.Version=….
var Version = "dev"
