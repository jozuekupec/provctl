package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"provctl/internal/config"
	"provctl/internal/meta"
	"provctl/internal/service"
)

func newHealthCommand() *cobra.Command {
	var configPath string
	var jsonOutput bool
	command := &cobra.Command{Use: "health [subscription [domain]]", Short: "inspect service health without changing the server", Args: cobra.RangeArgs(0, 2), RunE: func(command *cobra.Command, args []string) error {
		cfg, err := config.Load(configPath)
		if err != nil {
			return fmt.Errorf("load configuration: %w", err)
		}
		ctx := context.Background()
		runtime, err := service.NewProductionHealthRuntime(ctx, cfg)
		if err != nil {
			return fmt.Errorf("open health state: %w", err)
		}
		defer runtime.Close()
		var subscriptionName, primaryDomain string
		if len(args) > 0 {
			subscriptionName = args[0]
		}
		if len(args) > 1 {
			primaryDomain = args[1]
		}
		checks, err := runtime.Service.Run(ctx, subscriptionName, primaryDomain)
		if err != nil {
			return err
		}
		if err := writeChecks(command, checks, jsonOutput); err != nil {
			return err
		}
		if service.HasFailure(checks) {
			return fmt.Errorf("health checks failed")
		}
		return nil
	}}
	command.Flags().StringVar(&configPath, "config", meta.ConfigFile, "path to config.toml")
	command.Flags().BoolVar(&jsonOutput, "json", false, "write checks as JSON")
	return command
}
