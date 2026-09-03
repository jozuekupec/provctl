package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"provctl/internal/service"
)

func newMigrateCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "migrate",
		Short: "apply pending SQLite schema migrations",
		RunE: func(command *cobra.Command, _ []string) error {
			runtime, err := service.NewProductionMigrationRuntime(context.Background())
			if err != nil {
				return fmt.Errorf("migrate state database: %w", err)
			}
			defer runtime.Close()
			quiet, err := command.Flags().GetBool("quiet")
			if err != nil {
				return err
			}
			if !quiet {
				_, err = fmt.Fprintln(command.OutOrStdout(), "SQLite migrations completed.")
			}
			return err
		},
	}
	command.Flags().Bool("quiet", false, "suppress successful output")
	return command
}
