package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"provctl/internal/config"
	"provctl/internal/meta"
	"provctl/internal/service"
)

func newDoctorCommand() *cobra.Command {
	var configPath string
	var jsonOutput bool
	command := &cobra.Command{
		Use:   "doctor",
		Short: "inspect the server without changing it",
		RunE: func(command *cobra.Command, _ []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("load configuration: %w", err)
			}
			checks := service.NewProductionDoctor().Run(context.Background(), cfg)
			if err := writeChecks(command, checks, jsonOutput); err != nil {
				return err
			}
			if service.HasFailure(checks) {
				return fmt.Errorf("environment checks failed")
			}
			return nil
		},
	}
	command.Flags().StringVar(&configPath, "config", meta.ConfigFile, "path to config.toml")
	command.Flags().BoolVar(&jsonOutput, "json", false, "write checks as JSON")
	return command
}

func writeChecks(command *cobra.Command, checks []service.Check, jsonOutput bool) error {
	if jsonOutput {
		return json.NewEncoder(command.OutOrStdout()).Encode(checks)
	}
	for _, check := range checks {
		if _, err := fmt.Fprintf(command.OutOrStdout(), "%s %s: %s", check.Status, check.Name, check.Detail); err != nil {
			return err
		}
		if check.Hint != "" {
			if _, err := fmt.Fprintf(command.OutOrStdout(), " (hint: %s)", check.Hint); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(command.OutOrStdout()); err != nil {
			return err
		}
	}
	return nil
}
