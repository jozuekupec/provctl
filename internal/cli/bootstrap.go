package cli

import (
	"bufio"
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"provctl/internal/config"
	"provctl/internal/meta"
	"provctl/internal/service"
	"strings"
	"time"
)

func newBootstrapCommand() *cobra.Command {
	var configPath string
	var dryRun, yes, installMissing bool
	var skip []string
	command := &cobra.Command{Use: "bootstrap", Short: "prepare Apache integration", RunE: func(command *cobra.Command, _ []string) error {
		cfg, err := config.Load(configPath)
		if err != nil {
			return fmt.Errorf("load configuration: %w", err)
		}
		packages := service.NewProductionBootstrapPackages()
		missing, err := packages.Missing(context.Background())
		if err != nil {
			return fmt.Errorf("inspect bootstrap packages: %w", err)
		}
		if len(missing) > 0 {
			if !installMissing {
				return fmt.Errorf("missing required Debian packages: %s; install them with: %s", strings.Join(missing, ", "), service.BootstrapInstallHint(missing))
			}
			if !yes {
				if _, err := fmt.Fprintf(command.OutOrStdout(), "Install Debian packages: %s? [y/N] ", strings.Join(missing, " ")); err != nil {
					return err
				}
				answer, err := bufio.NewReader(command.InOrStdin()).ReadString('\n')
				if err != nil && len(answer) == 0 {
					return err
				}
				answer = strings.ToLower(strings.TrimSpace(answer))
				if answer != "y" && answer != "yes" {
					return fmt.Errorf("package installation cancelled")
				}
			}
			if dryRun {
				_, err := fmt.Fprintf(command.OutOrStdout(), "Would install Debian packages: %s\n", strings.Join(missing, " "))
				return err
			}
			if err := packages.Install(context.Background(), missing); err != nil {
				return err
			}
		}
		if !dryRun && !yes {
			if _, err := fmt.Fprint(command.OutOrStdout(), "Apply bootstrap changes? [y/N] "); err != nil {
				return err
			}
			answer, err := bufio.NewReader(command.InOrStdin()).ReadString('\n')
			if err != nil && len(answer) == 0 {
				return err
			}
			answer = strings.ToLower(strings.TrimSpace(answer))
			if answer != "y" && answer != "yes" {
				return fmt.Errorf("bootstrap cancelled")
			}
		}
		if dryRun {
			preview, err := service.NewBootstrapPreview(cfg)
			if err != nil {
				return err
			}
			operation, err := preview.PrepareWithSkip(context.Background(), skip)
			if err != nil {
				return err
			}
			return writePlan(command, operation)
		}
		runtime, err := service.NewProductionBootstrapRuntime(context.Background(), cfg)
		if err != nil {
			return fmt.Errorf("initialize bootstrap: %w", err)
		}
		defer runtime.Close()
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Limits.LockTimeoutSeconds)*time.Second)
		defer cancel()
		id, unchanged, err := runtime.Service.RunWithSkip(ctx, skip)
		if err != nil {
			return err
		}
		if unchanged {
			_, err = fmt.Fprintln(command.OutOrStdout(), "Bootstrap: nothing to do.")
			if err != nil {
				return err
			}
		} else if _, err := fmt.Fprintf(command.OutOrStdout(), "Bootstrap completed (operation %d).\n", id); err != nil {
			return err
		}
		checks := service.NewProductionDoctor().Run(context.Background(), cfg)
		if err := writeChecks(command, checks, false); err != nil {
			return err
		}
		if service.HasFailure(checks) {
			return fmt.Errorf("bootstrap completed but environment checks failed")
		}
		return nil
	}}
	command.Flags().StringVar(&configPath, "config", meta.ConfigFile, "path to config.toml")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "show the operation plan without changing the system")
	command.Flags().BoolVar(&installMissing, "install-missing", false, "install missing official Debian prerequisites")
	command.Flags().BoolVar(&yes, "yes", false, "confirm package installation without a prompt")
	command.Flags().StringSliceVar(&skip, "skip", nil, "skip optional bootstrap artifact: modules, certificate, vhost, deploy-hook, logrotate, audit-log")
	return command
}
