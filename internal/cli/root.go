// Package cli exposes the command-line frontend without business logic.
package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"provctl/internal/config"
	"provctl/internal/meta"
	"provctl/internal/service"
	"provctl/internal/ui"
)

func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           meta.Name,
		Short:         "Provisioning control for Debian web hosting",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.Flags().Bool("version", false, "print version and exit")
	root.RunE = func(command *cobra.Command, _ []string) error {
		version, err := command.Flags().GetBool("version")
		if err != nil {
			return err
		}
		if version {
			_, err := fmt.Fprintln(command.OutOrStdout(), meta.Version)
			return err
		}
		cfg, err := config.Load(meta.ConfigFile)
		if err != nil {
			return fmt.Errorf("load configuration: %w", err)
		}
		runtime, err := service.NewReadOnlySubscriptionRuntime(context.Background(), cfg)
		if err != nil {
			return fmt.Errorf("open TUI state: %w", err)
		}
		defer runtime.Close()
		websiteRuntime, err := service.NewReadOnlyWebsiteRuntime(context.Background(), cfg)
		if err != nil {
			return fmt.Errorf("open website TUI state: %w", err)
		}
		defer websiteRuntime.Close()
		_, err = ui.Program(ui.Deps{LoadSubscriptions: runtime.Service.List, LoadWebsites: websiteRuntime.Service.List}).Run()
		return err
	}
	root.AddCommand(newDoctorCommand())
	root.AddCommand(newBootstrapCommand())
	root.AddCommand(newSubscriptionCommand())
	root.AddCommand(newWebsiteCommand())
	root.AddCommand(newPHPCommand())
	root.AddCommand(newDatabaseCommand())
	root.AddCommand(newSSHCommand())
	root.AddCommand(newCronCommand())
	root.AddCommand(newReconcileCommand())
	return root
}
