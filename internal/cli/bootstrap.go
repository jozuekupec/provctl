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
	command := &cobra.Command{Use: "bootstrap", Short: "prepare Apache integration", RunE: func(command *cobra.Command, _ []string) error {
		cfg, err := config.Load(configPath)
		if err != nil {
			return fmt.Errorf("load configuration: %w", err)
		}
		runtime, err := service.NewProductionBootstrapRuntime(context.Background(), cfg)
		if err != nil {
			return fmt.Errorf("initialize bootstrap: %w", err)
		}
		defer runtime.Close()
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Limits.LockTimeoutSeconds)*time.Second)
		defer cancel()
		id, err := runtime.Service.Run(ctx)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(command.OutOrStdout(), "Bootstrap completed (operation %d).\n", id)
		return err
	}}
	command.Flags().StringVar(&configPath, "config", meta.ConfigFile, "path to config.toml")
	return command
}
