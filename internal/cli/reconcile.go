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

func newReconcileCommand() *cobra.Command {
	var configPath, subscriptionName string
	var dryRun bool
	command := &cobra.Command{Use: "reconcile", Short: "restore generated Apache vhosts from SQLite", Args: cobra.NoArgs, RunE: func(command *cobra.Command, _ []string) error {
		cfg, err := config.Load(configPath)
		if err != nil {
			return fmt.Errorf("load configuration: %w", err)
		}
		ctx := context.Background()
		if dryRun {
			runtime, err := service.NewReadOnlyReconcileRuntime(ctx, cfg)
			if err != nil {
				return fmt.Errorf("open reconcile state: %w", err)
			}
			defer runtime.Close()
			drifts, err := runtime.Service.Inspect(ctx, subscriptionName)
			if err != nil {
				return err
			}
			if len(drifts) == 0 {
				_, err := fmt.Fprintln(command.OutOrStdout(), "No generated Apache vhost drift detected.")
				return err
			}
			for _, drift := range drifts {
				if _, err := fmt.Fprint(command.OutOrStdout(), service.UnifiedDiff(drift.Path, drift.Current, drift.Expected)); err != nil {
					return err
				}
			}
			return service.DriftError{}
		}
		runtime, err := service.NewProductionReconcileRuntime(ctx, cfg)
		if err != nil {
			return fmt.Errorf("open reconcile state: %w", err)
		}
		defer runtime.Close()
		lockCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.Limits.LockTimeoutSeconds)*time.Second)
		defer cancel()
		operationID, err := runtime.Service.Reconcile(lockCtx, subscriptionName)
		if err != nil {
			return err
		}
		if operationID == 0 {
			_, err = fmt.Fprintln(command.OutOrStdout(), "Nothing to reconcile.")
			return err
		}
		_, err = fmt.Fprintf(command.OutOrStdout(), "Reconciled generated Apache vhosts (operation %d).\n", operationID)
		return err
	}}
	command.Flags().StringVar(&configPath, "config", meta.ConfigFile, "path to config.toml")
	command.Flags().StringVar(&subscriptionName, "subscription", "", "limit reconciliation to one subscription")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "show generated configuration drift without changing the system")
	return command
}
