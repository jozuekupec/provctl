package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"provctl/internal/system"
	"provctl/internal/system/fake"
)

func TestBootstrapPackages_MissingReturnsUninstalledPackages(t *testing.T) {
	commands := &fake.Commander{Result: system.Result{Stdout: "installed\n"}}
	missing, err := (BootstrapPackages{Commands: commands}).Missing(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 0 {
		t.Errorf("Missing() = %v, want none", missing)
	}
}

func TestBootstrapPackages_InstallUsesLockAndExplicitAptArguments(t *testing.T) {
	commands := &fake.Commander{}
	packages := BootstrapPackages{Commands: commands}
	if err := packages.Install(context.Background(), []string{"apache2", "php-fpm"}); err != nil {
		t.Fatal(err)
	}
	if len(commands.Calls) != 2 {
		t.Fatalf("calls = %#v", commands.Calls)
	}
	if got := commands.Calls[0]; got.Name != "/usr/bin/flock" || !strings.Contains(strings.Join(got.Args, " "), "/var/lib/dpkg/lock-frontend") {
		t.Errorf("lock call = %#v", got)
	}
	if got := commands.Calls[1]; got.Name != "/usr/bin/apt-get" || !strings.Contains(strings.Join(got.Args, " "), "install -y --no-install-recommends apache2 php-fpm") {
		t.Errorf("apt call = %#v", got)
	}
}

func TestBootstrapPackages_InstallRejectsUnapprovedPackage(t *testing.T) {
	err := (BootstrapPackages{Commands: &fake.Commander{}}).Install(context.Background(), []string{"curl"})
	if err == nil || !strings.Contains(err.Error(), "allowlist") {
		t.Fatalf("Install() error = %v, want allowlist error", err)
	}
}

func TestBootstrapPackages_InstallReportsLockFailure(t *testing.T) {
	commands := &fake.Commander{FailAt: 1, Err: errors.New("locked")}
	err := (BootstrapPackages{Commands: commands}).Install(context.Background(), []string{"apache2"})
	if err == nil || !strings.Contains(err.Error(), "dpkg lock") {
		t.Fatalf("Install() error = %v, want lock error", err)
	}
}
