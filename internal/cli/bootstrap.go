package cli

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"provctl/internal/config"
	"provctl/internal/meta"
	"provctl/internal/service"
	"time"
)

func newBootstrapCommand() *cobra.Command {
	var configPath string
	var dryRun bool
	command := &cobra.Command{Use: "bootstrap", Short: "prepare Apache integration", RunE: func(command *cobra.Command, _ []string) error {
		cfg, err := config.Load(configPath)
		if err != nil {
			return fmt.Errorf("load configuration: %w", err)
		}
		if dryRun {
			preview, err := service.NewBootstrapPreview(cfg)
			if err != nil {
				return err
			}
			operation, err := preview.Prepare(context.Background())
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
		id, unchanged, err := runtime.Service.Run(ctx)
		if err != nil {
			return err
		}
		if unchanged {
			_, err = fmt.Fprintln(command.OutOrStdout(), "Bootstrap: nothing to do.")
			return err
		}
		_, err = fmt.Fprintf(command.OutOrStdout(), "Bootstrap completed (operation %d).\n", id)
		return err
	}}
	command.Flags().StringVar(&configPath, "config", meta.ConfigFile, "path to config.toml")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "show the operation plan without changing the system")
	return command
}
