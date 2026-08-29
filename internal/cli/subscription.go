package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"provctl/internal/config"
	"provctl/internal/meta"
	"provctl/internal/plan"
	"provctl/internal/service"
)

func newSubscriptionCommand() *cobra.Command {
	command := &cobra.Command{Use: "subscription", Short: "manage hosting subscriptions"}
	command.AddCommand(newSubscriptionCreateCommand())
	return command
}

func newSubscriptionCreateCommand() *cobra.Command {
	var configPath string
	var dryRun bool
	command := &cobra.Command{
		Use:   "create <name>",
		Short: "create an isolated hosting subscription",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("load configuration: %w", err)
			}
			ctx := context.Background()
			if dryRun {
				runtime, err := service.NewReadOnlySubscriptionRuntime(ctx, cfg)
				if err != nil {
					return fmt.Errorf("open subscription state: %w", err)
				}
				defer runtime.Close()
				operation, err := runtime.Service.PrepareCreate(ctx, args[0])
				if err != nil {
					return err
				}
				return writePlan(command, operation)
			}
			runtime, err := service.NewProductionSubscriptionRuntime(ctx, cfg)
			if err != nil {
				return fmt.Errorf("open subscription state: %w", err)
			}
			defer runtime.Close()
			lockCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.Limits.LockTimeoutSeconds)*time.Second)
			defer cancel()
			operationID, err := runtime.Service.Create(lockCtx, args[0])
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(command.OutOrStdout(), "Created subscription %q (operation %d).\n", args[0], operationID)
			return err
		},
	}
	command.Flags().StringVar(&configPath, "config", meta.ConfigFile, "path to config.toml")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "show the operation plan without changing the system")
	return command
}

func writePlan(command *cobra.Command, operation plan.Plan) error {
	if _, err := fmt.Fprintf(command.OutOrStdout(), "Plan: %s %s\n", operation.Action, operation.Target); err != nil {
		return err
	}
	for index, step := range operation.Steps {
		if _, err := fmt.Fprintf(command.OutOrStdout(), "%d. %s\n   %s\n", index+1, step.Name, step.Preview); err != nil {
			return err
		}
	}
	return nil
}
