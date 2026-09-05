package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"provctl/internal/system"
)

var bootstrapPackages = []string{"apache2", "certbot", "logrotate", "mariadb-server", "php-fpm"}

// BootstrapPackages installs only the fixed, official Debian prerequisites.
// It deliberately has no repository or arbitrary-package input.
type BootstrapPackages struct{ Commands system.Commander }

func NewProductionBootstrapPackages() BootstrapPackages {
	return BootstrapPackages{Commands: system.ExecCommander{}}
}

func (packages BootstrapPackages) Missing(ctx context.Context) ([]string, error) {
	if packages.Commands == nil {
		return nil, errors.New("package command runner is required")
	}
	missing := make([]string, 0, len(bootstrapPackages))
	for _, name := range bootstrapPackages {
		result, err := packages.Commands.Run(ctx, "/usr/bin/dpkg-query", "-W", "-f=${db:Status-Status}", name)
		if err != nil || strings.TrimSpace(result.Stdout) != "installed" {
			missing = append(missing, name)
		}
	}
	return missing, nil
}

func (packages BootstrapPackages) Install(ctx context.Context, missing []string) error {
	if len(missing) == 0 {
		return nil
	}
	if !samePackages(missing) {
		return errors.New("refuse to install packages outside the bootstrap allowlist")
	}
	if packages.Commands == nil {
		return errors.New("package command runner is required")
	}
	result, err := packages.Commands.Run(ctx, "/usr/bin/flock", "-n", "/var/lib/dpkg/lock-frontend", "/usr/bin/true")
	if err != nil {
		return commandError("check dpkg lock", result, err)
	}
	env, ok := packages.Commands.(system.EnvCommander)
	if !ok {
		return errors.New("package command runner does not support explicit environment")
	}
	args := append([]string{"install", "-y", "--no-install-recommends"}, missing...)
	result, err = env.RunWithEnv(ctx, []string{"DEBIAN_FRONTEND=noninteractive"}, "/usr/bin/apt-get", args...)
	if err != nil {
		return commandError("install bootstrap packages", result, err)
	}
	return nil
}

func samePackages(values []string) bool {
	for _, value := range values {
		found := false
		for _, allowed := range bootstrapPackages {
			if value == allowed {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func BootstrapInstallHint(missing []string) string {
	return "sudo apt-get install " + strings.Join(missing, " ")
}

func (packages BootstrapPackages) String() string { return fmt.Sprint(bootstrapPackages) }
