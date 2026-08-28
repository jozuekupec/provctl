// Package service contains provctl's application use cases.
package service

import (
	"context"
	"fmt"
	"os"

	"provctl/internal/config"
	"provctl/internal/meta"
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

type Doctor struct {
	FS        system.FS
	Commander system.Commander
	EUID      func() int
	PHPDir    string
}

func NewDoctor(fs system.FS, commander system.Commander) Doctor {
	return Doctor{FS: fs, Commander: commander, EUID: os.Geteuid, PHPDir: meta.PHPConfigDir}
}

func NewProductionDoctor() Doctor {
	return NewDoctor(system.OSFS{}, system.ExecCommander{})
}

// Run performs diagnostics only. It does not create files, reload services,
// or otherwise alter system state.
func (doctor Doctor) Run(ctx context.Context, cfg config.Config) []Check {
	checks := []Check{doctor.checkRoot(), doctor.checkPath(cfg.Paths.VHosts, "vhosts root")}
	checks = append(checks, doctor.checkCommand(ctx, "/usr/sbin/apachectl", "Apache"))
	checks = append(checks, doctor.checkCommand(ctx, "/usr/bin/certbot", "Certbot"))
	checks = append(checks, doctor.checkPHP(ctx)...)
	return checks
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
	if doctor.EUID() == 0 {
		return Check{Name: "root", Status: CheckOK, Detail: "running as root"}
	}
	return Check{Name: "root", Status: CheckFail, Detail: "not running as root", Hint: "run provctl with sudo"}
}

func (doctor Doctor) checkPath(path, name string) Check {
	if _, err := doctor.FS.Stat(path); err != nil {
		return Check{Name: name, Status: CheckFail, Detail: fmt.Sprintf("%s is unavailable: %v", path, err), Hint: "run provctl bootstrap after installing prerequisites"}
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
		poolDirectory := doctor.PHPDir + "/" + entry.Name() + "/fpm/pool.d"
		if _, err := doctor.FS.Stat(poolDirectory); err != nil {
			continue
		}
		binary := "/usr/sbin/php-fpm" + entry.Name()
		checks = append(checks, doctor.checkCommand(ctx, binary, "PHP-FPM "+entry.Name()))
	}
	if len(checks) == 0 {
		return []Check{{Name: "PHP-FPM", Status: CheckFail, Detail: "no installed FPM pools detected", Hint: "install PHP-FPM"}}
	}
	return checks
}

func firstLine(value string) string {
	for index, character := range value {
		if character == '\n' {
			return value[:index]
		}
	}
	return value
}
