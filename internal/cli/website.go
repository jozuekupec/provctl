package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"provctl/internal/config"
	"provctl/internal/meta"
	"provctl/internal/service"
)

func newWebsiteCommand() *cobra.Command {
	command := &cobra.Command{Use: "website", Short: "manage hosted websites"}
	command.AddCommand(newWebsiteCreateCommand())
	return command
}

func newWebsiteCreateCommand() *cobra.Command {
	var configPath string
	var dryRun bool
	command := &cobra.Command{
		Use:   "create <subscription> <domain>",
		Short: "create a PHP-FPM website",
		Args:  cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("load configuration: %w", err)
			}
			ctx := context.Background()
			if dryRun {
				runtime, err := service.NewReadOnlyWebsiteRuntime(ctx, cfg)
				if err != nil {
					return fmt.Errorf("open website state: %w", err)
				}
				defer runtime.Close()
				operation, err := runtime.Service.PrepareCreatePHPFPM(ctx, args[0], args[1])
				if err != nil {
					return err
				}
				return writePlan(command, operation)
			}
			runtime, err := service.NewProductionWebsiteRuntime(ctx, cfg)
			if err != nil {
				return fmt.Errorf("open website state: %w", err)
			}
			defer runtime.Close()
			lockCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.Limits.LockTimeoutSeconds)*time.Second)
			defer cancel()
			operationID, err := runtime.Service.CreatePHPFPM(lockCtx, args[0], args[1])
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(command.OutOrStdout(), "Created website %q for subscription %q (operation %d).\n", args[1], args[0], operationID)
			return err
		},
	}
	command.Flags().StringVar(&configPath, "config", meta.ConfigFile, "path to config.toml")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "show the operation plan without changing the system")
	return command
}
