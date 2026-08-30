package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"provctl/internal/config"
	"provctl/internal/meta"
	"provctl/internal/service"
)

func newPHPCommand() *cobra.Command {
	command := &cobra.Command{Use: "php", Short: "manage PHP-FPM versions and pools"}
	command.AddCommand(newPHPListVersionsCommand(), newPHPSetCommand())
	return command
}

func newPHPListVersionsCommand() *cobra.Command {
	var configPath string
	command := &cobra.Command{Use: "list-versions", Short: "list installed PHP-FPM versions", Args: cobra.NoArgs, RunE: func(command *cobra.Command, _ []string) error {
		cfg, err := config.Load(configPath)
		if err != nil {
			return fmt.Errorf("load configuration: %w", err)
		}
		runtime, err := service.NewReadOnlyPHPRuntime(context.Background(), cfg)
		if err != nil {
			return fmt.Errorf("open PHP state: %w", err)
		}
		defer runtime.Close()
		versions, err := runtime.Service.ListVersions(context.Background())
		if err != nil {
			return err
		}
		if len(versions) == 0 {
			_, err = fmt.Fprintln(command.OutOrStdout(), "No PHP-FPM versions found.")
			return err
		}
		for _, version := range versions {
			state := "inactive"
			if version.Active {
				state = "active"
			}
			if _, err := fmt.Fprintf(command.OutOrStdout(), "%s\t%s\n", version.Version, state); err != nil {
				return err
			}
		}
		return nil
	}}
	command.Flags().StringVar(&configPath, "config", meta.ConfigFile, "path to config.toml")
	return command
}

func newPHPSetCommand() *cobra.Command {
	var configPath, version, memoryLimit string
	var maxChildren int
	var dryRun bool
	command := &cobra.Command{Use: "set <subscription>", Short: "change a subscription PHP-FPM version", Args: cobra.ExactArgs(1), RunE: func(command *cobra.Command, args []string) error {
		if strings.TrimSpace(version) == "" {
			return fmt.Errorf("--version is required")
		}
		cfg, err := config.Load(configPath)
		if err != nil {
			return fmt.Errorf("load configuration: %w", err)
		}
		ctx := context.Background()
		options := service.PHPSetOptions{Version: version, MaxChildren: maxChildren, MemoryLimit: memoryLimit}
		if dryRun {
			runtime, err := service.NewReadOnlyPHPRuntime(ctx, cfg)
			if err != nil {
				return fmt.Errorf("open PHP state: %w", err)
			}
			defer runtime.Close()
			operation, err := runtime.Service.PrepareSet(ctx, args[0], options)
			if err != nil {
				return err
			}
			return writePlan(command, operation)
		}
		runtime, err := service.NewProductionPHPRuntime(ctx, cfg)
		if err != nil {
			return fmt.Errorf("open PHP state: %w", err)
		}
		defer runtime.Close()
		lockCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.Limits.LockTimeoutSeconds)*time.Second)
		defer cancel()
		operationID, err := runtime.Service.Set(lockCtx, args[0], options)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(command.OutOrStdout(), "Changed PHP-FPM version for subscription %q to %s (operation %d).\n", args[0], version, operationID)
		return err
	}}
	command.Flags().StringVar(&configPath, "config", meta.ConfigFile, "path to config.toml")
	command.Flags().StringVar(&version, "version", "", "installed PHP-FPM version in major.minor form")
	command.Flags().IntVar(&maxChildren, "max-children", 0, "override the pool pm.max_children limit")
	command.Flags().StringVar(&memoryLimit, "memory-limit", "", "override the pool PHP memory limit")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "show the operation plan without changing the system")
	return command
}
