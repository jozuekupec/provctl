// Package service contains provctl's application use cases.
package service

import (
	"context"
	"fmt"
	"strings"

	"provctl/internal/config"
	"provctl/internal/meta"
	"provctl/internal/repository/sqlite"
	"provctl/internal/system"
)

type CheckStatus string

const (
	CheckOK   CheckStatus = "OK"
	CheckWarn CheckStatus = "WARN"
	CheckFail CheckStatus = "FAIL"
)

type Check struct {
	Name   string      `json:"name"`
	Status CheckStatus `json:"status"`
	Detail string      `json:"detail"`
	Hint   string      `json:"hint,omitempty"`
}

// Doctor is read-only. Its collaborators make every check usable in offline tests.
type Doctor struct {
	FS        system.FS
	Commander system.Commander
	Systemd   system.Systemd
	Identity  system.Identity
	Database  DatabaseInspector
	PHPDir    string
}

type DatabaseSchema struct {
	Current int
	Latest  int
}

type DatabaseInspector interface {
	Inspect(context.Context, string) (DatabaseSchema, error)
}

type sqliteInspector struct{}

func (sqliteInspector) Inspect(ctx context.Context, path string) (DatabaseSchema, error) {
	info, err := sqlite.InspectSchema(ctx, path)
	return DatabaseSchema{Current: info.Current, Latest: info.Latest}, err
}

func NewDoctor(fs system.FS, commander system.Commander, systemd system.Systemd, identity system.Identity) Doctor {
	return Doctor{FS: fs, Commander: commander, Systemd: systemd, Identity: identity, Database: sqliteInspector{}, PHPDir: meta.PHPConfigDir}
}

func NewProductionDoctor() Doctor {
	commander := system.ExecCommander{}
	return NewDoctor(system.OSFS{}, commander, system.CommandSystemd{Commander: commander}, system.OSIdentity{})
}

// Run performs diagnostics only. It never creates files, reloads services, or migrates SQLite.
func (doctor Doctor) Run(ctx context.Context, cfg config.Config) []Check {
	checks := []Check{
		doctor.checkRoot(), doctor.checkConfigVersion(cfg),
		doctor.checkDirectory(meta.ConfigDir, "configuration directory"),
		doctor.checkDirectory(meta.StateDir, "state directory"),
		doctor.checkDirectory(meta.LogDir, "log directory"),
		doctor.checkDirectory(cfg.Paths.VHosts, "vhosts root"),
		doctor.checkDatabase(ctx),
		doctor.checkCommand(ctx, "/usr/sbin/apachectl", "Apache"),
		doctor.checkService(ctx, cfg.Apache.Service, "Apache service"),
		doctor.checkApacheModules(ctx), doctor.checkCommand(ctx, "/usr/bin/certbot", "Certbot"),
		doctor.checkDeployHook(),
	}
	checks = append(checks, doctor.checkPHP(ctx)...)
	checks = append(checks, doctor.checkMariaDB(ctx, cfg))
	checks = append(checks, doctor.checkRenewal(ctx)...)
	return checks
}

func (doctor Doctor) checkDatabase(ctx context.Context) Check {
	info, err := doctor.FS.Stat(meta.DatabaseFile)
	if err != nil || info.Mode().Perm()&0o200 == 0 {
		return Check{Name: "SQLite database", Status: CheckFail, Detail: "database is unavailable or not writable", Hint: "run provctl bootstrap and restore database permissions"}
	}
	schema, err := doctor.Database.Inspect(ctx, meta.DatabaseFile)
	if err != nil {
		return Check{Name: "SQLite database", Status: CheckFail, Detail: fmt.Sprintf("cannot inspect schema: %v", err), Hint: "restore the provctl database"}
	}
	if schema.Current > schema.Latest {
		return Check{Name: "SQLite database", Status: CheckFail, Detail: fmt.Sprintf("schema %d is newer than supported %d", schema.Current, schema.Latest), Hint: "upgrade provctl"}
	}
	if schema.Current < schema.Latest {
		return Check{Name: "SQLite database", Status: CheckWarn, Detail: fmt.Sprintf("schema %d needs migration to %d", schema.Current, schema.Latest), Hint: "run an apply command to migrate the database"}
	}
	return Check{Name: "SQLite database", Status: CheckOK, Detail: fmt.Sprintf("schema version %d", schema.Current)}
}

func HasFailure(checks []Check) bool {
	for _, check := range checks {
		if check.Status == CheckFail {
			return true
		}
	}
	return false
}

func (doctor Doctor) checkRoot() Check {
	if doctor.Identity.EUID() == 0 {
		return Check{Name: "root", Status: CheckOK, Detail: "running as root"}
	}
	return Check{Name: "root", Status: CheckFail, Detail: "not running as root", Hint: "run provctl with sudo"}
}

func (doctor Doctor) checkConfigVersion(cfg config.Config) Check {
	if cfg.Meta.ConfigVersion == config.CurrentVersion {
		return Check{Name: "configuration version", Status: CheckOK, Detail: fmt.Sprintf("version %d", cfg.Meta.ConfigVersion)}
	}
	return Check{Name: "configuration version", Status: CheckFail, Detail: fmt.Sprintf("version %d is unsupported", cfg.Meta.ConfigVersion), Hint: "run provctl config migrate"}
}

func (doctor Doctor) checkDirectory(path, name string) Check {
	info, err := doctor.FS.Stat(path)
	if err != nil || !info.IsDir() {
		return Check{Name: name, Status: CheckFail, Detail: fmt.Sprintf("%s is unavailable", path), Hint: "run provctl bootstrap after installing prerequisites"}
	}
	if info.Mode().Perm()&0o200 == 0 {
		return Check{Name: name, Status: CheckFail, Detail: fmt.Sprintf("%s is not writable", path), Hint: "restore owner write permission"}
	}
	return Check{Name: name, Status: CheckOK, Detail: path}
}

func (doctor Doctor) checkCommand(ctx context.Context, binary, name string) Check {
	result, err := doctor.Commander.Run(ctx, binary, "--version")
	if err != nil {
		return Check{Name: name, Status: CheckFail, Detail: fmt.Sprintf("%s is unavailable: %v", binary, err), Hint: "install the required Debian package"}
	}
	return Check{Name: name, Status: CheckOK, Detail: firstLine(result.Stdout)}
}

func (doctor Doctor) checkService(ctx context.Context, unit, name string) Check {
	active, err := doctor.Systemd.IsActive(ctx, unit)
	if err != nil {
		return Check{Name: name, Status: CheckFail, Detail: fmt.Sprintf("cannot inspect %s: %v", unit, err), Hint: "install and start the service"}
	}
	if !active {
		return Check{Name: name, Status: CheckFail, Detail: unit + " is inactive", Hint: "start " + unit}
	}
	return Check{Name: name, Status: CheckOK, Detail: unit + " is active"}
}

func (doctor Doctor) checkApacheModules(ctx context.Context) Check {
	result, err := doctor.Commander.Run(ctx, "/usr/sbin/apachectl", "-M")
	if err != nil {
		return Check{Name: "Apache modules", Status: CheckFail, Detail: fmt.Sprintf("cannot list modules: %v", err), Hint: "enable required Apache modules"}
	}
	missing := make([]string, 0)
	for _, module := range []string{"proxy", "proxy_fcgi", "proxy_http", "ssl", "rewrite", "headers"} {
		if !strings.Contains(result.Stdout, module+"_module") {
			missing = append(missing, module)
		}
	}
	if len(missing) > 0 {
		return Check{Name: "Apache modules", Status: CheckFail, Detail: "missing: " + strings.Join(missing, ", "), Hint: "enable them with a2enmod"}
	}
	return Check{Name: "Apache modules", Status: CheckOK, Detail: "required modules are enabled"}
}

func (doctor Doctor) checkPHP(ctx context.Context) []Check {
	entries, err := doctor.FS.ReadDir(doctor.PHPDir)
	if err != nil {
		return []Check{{Name: "PHP-FPM", Status: CheckFail, Detail: fmt.Sprintf("cannot inspect %s: %v", doctor.PHPDir, err), Hint: "install PHP-FPM"}}
	}
	checks := make([]Check, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		version := entry.Name()
		if _, err := doctor.FS.Stat(doctor.PHPDir + "/" + version + "/fpm/pool.d"); err != nil {
			continue
		}
		checks = append(checks, doctor.checkCommand(ctx, "/usr/sbin/php-fpm"+version, "PHP-FPM "+version))
		checks = append(checks, doctor.checkService(ctx, "php"+version+"-fpm", "PHP-FPM "+version+" service"))
	}
	if len(checks) == 0 {
		return []Check{{Name: "PHP-FPM", Status: CheckFail, Detail: "no installed FPM pools detected", Hint: "install PHP-FPM"}}
	}
	return checks
}

func (doctor Doctor) checkMariaDB(ctx context.Context, cfg config.Config) Check {
	if !cfg.MariaDB.Enabled {
		return Check{Name: "MariaDB", Status: CheckWarn, Detail: "disabled by configuration"}
	}
	args := []string{"--batch", "--skip-column-names"}
	if cfg.MariaDB.DefaultsFile != "" {
		args = append(args, "--defaults-extra-file="+cfg.MariaDB.DefaultsFile)
	}
	result, err := doctor.Commander.RunWithStdin(ctx, strings.NewReader("SELECT 1;\n"), "/usr/bin/mysql", args...)
	if err != nil {
		return Check{Name: "MariaDB", Status: CheckFail, Detail: fmt.Sprintf("cannot connect: %v", err), Hint: "configure socket authentication or defaults_file"}
	}
	if strings.TrimSpace(result.Stdout) != "1" {
		return Check{Name: "MariaDB", Status: CheckFail, Detail: "connectivity query returned an unexpected result", Hint: "check MariaDB authentication"}
	}
	return Check{Name: "MariaDB", Status: CheckOK, Detail: "client and connectivity verified"}
}

func (doctor Doctor) checkDeployHook() Check {
	if _, err := doctor.FS.Stat(meta.DeployHook); err != nil {
		return Check{Name: "certbot deploy hook", Status: CheckFail, Detail: "provctl hook is missing", Hint: "run provctl bootstrap"}
	}
	return Check{Name: "certbot deploy hook", Status: CheckOK, Detail: meta.DeployHook}
}

func (doctor Doctor) checkRenewal(ctx context.Context) []Check {
	mechanisms := make([]string, 0, 3)
	active, err := doctor.Systemd.IsActive(ctx, "certbot.timer")
	if err == nil && active {
		mechanisms = append(mechanisms, "certbot.timer")
	}
	if _, err := doctor.FS.Stat(meta.CertbotCron); err == nil {
		mechanisms = append(mechanisms, meta.CertbotCron)
	}
	result, err := doctor.Commander.Run(ctx, "/usr/bin/crontab", "-l")
	if err == nil && strings.Contains(result.Stdout, "certbot") {
		mechanisms = append(mechanisms, "root crontab")
	}
	if len(mechanisms) == 1 {
		return []Check{{Name: "certificate renewal", Status: CheckOK, Detail: mechanisms[0]}}
	}
	if len(mechanisms) == 0 {
		return []Check{{Name: "certificate renewal", Status: CheckFail, Detail: "no renewal mechanism detected", Hint: "enable certbot.timer"}}
	}
	return []Check{{Name: "certificate renewal", Status: CheckWarn, Detail: "multiple mechanisms: " + strings.Join(mechanisms, ", "), Hint: "keep exactly one renewal mechanism"}}
}

func firstLine(value string) string {
	for index, character := range value {
		if character == '\n' {
			return value[:index]
		}
	}
	return value
}
