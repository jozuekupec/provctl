package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"provctl/internal/system"
)

var phpVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+$`)

type PHPFPMVersion struct {
	Version string
	Binary  string
	Service string
	Active  bool
}

// DiscoverPHPFPM finds installed FPM versions without assuming a Debian PHP version.
func DiscoverPHPFPM(ctx context.Context, fs system.FS, systemd system.Systemd) ([]PHPFPMVersion, error) {
	entries, err := fs.ReadDir("/etc/php")
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read PHP configuration directory: %w", err)
	}
	var versions []PHPFPMVersion
	for _, entry := range entries {
		if !entry.IsDir() || !phpVersion.MatchString(entry.Name()) {
			continue
		}
		version := entry.Name()
		poolDir := filepath.Join("/etc/php", version, "fpm", "pool.d")
		if _, err := fs.Stat(poolDir); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("inspect PHP-FPM pool directory for %s: %w", version, err)
		}
		binary := "/usr/sbin/php-fpm" + version
		if _, err := fs.Stat(binary); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("inspect PHP-FPM binary for %s: %w", version, err)
		}
		unit := "php" + version + "-fpm.service"
		active, err := systemd.IsActive(ctx, unit)
		if err != nil {
			return nil, fmt.Errorf("inspect PHP-FPM service %s: %w", unit, err)
		}
		versions = append(versions, PHPFPMVersion{Version: version, Binary: binary, Service: unit, Active: active})
	}
	sort.Slice(versions, func(i, j int) bool { return comparePHPVersion(versions[i].Version, versions[j].Version) < 0 })
	return versions, nil
}

// SelectPHPFPM chooses the configured version or the highest discovered one.
func SelectPHPFPM(configured string, available []PHPFPMVersion) (PHPFPMVersion, error) {
	if len(available) == 0 {
		return PHPFPMVersion{}, fmt.Errorf("no PHP-FPM versions found under /etc/php")
	}
	if configured == "" {
		return available[len(available)-1], nil
	}
	for _, version := range available {
		if version.Version == configured {
			return version, nil
		}
	}
	names := make([]string, len(available))
	for index, version := range available {
		names[index] = version.Version
	}
	return PHPFPMVersion{}, fmt.Errorf("configured PHP-FPM version %q is unavailable; available versions: %s", configured, strings.Join(names, ", "))
}

func comparePHPVersion(left, right string) int {
	var leftMajor, leftMinor, rightMajor, rightMinor int
	_, _ = fmt.Sscanf(left, "%d.%d", &leftMajor, &leftMinor)
	_, _ = fmt.Sscanf(right, "%d.%d", &rightMajor, &rightMinor)
	if leftMajor != rightMajor {
		if leftMajor < rightMajor {
			return -1
		}
		return 1
	}
	if leftMinor < rightMinor {
		return -1
	}
	if leftMinor > rightMinor {
		return 1
	}
	return 0
}
