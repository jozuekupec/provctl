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
		websiteWriteRuntime, err := service.NewProductionWebsiteRuntime(context.Background(), cfg)
		if err != nil {
			return fmt.Errorf("open website mutation state: %w", err)
		}
		defer websiteWriteRuntime.Close()
		subscriptionWriteRuntime, err := service.NewProductionSubscriptionRuntime(context.Background(), cfg)
		if err != nil {
			return fmt.Errorf("open subscription mutation state: %w", err)
		}
		defer subscriptionWriteRuntime.Close()
		healthRuntime, err := service.NewProductionHealthRuntime(context.Background(), cfg)
		if err != nil {
			return fmt.Errorf("open health state: %w", err)
		}
		defer healthRuntime.Close()
		_, err = ui.Program(ui.Deps{LoadSubscriptions: runtime.Service.List, LoadWebsites: websiteRuntime.Service.List, SetWebsiteEnabled: websiteWriteRuntime.Service.SetEnabled, SetSubscriptionStatus: subscriptionWriteRuntime.Service.SetStatus, RunHealth: healthRuntime.Service.Run}).Run()
		return err
	}
	root.AddCommand(newDoctorCommand())
	root.AddCommand(newHealthCommand())
	root.AddCommand(newBootstrapCommand())
	root.AddCommand(newSubscriptionCommand())
	root.AddCommand(newWebsiteCommand())
	root.AddCommand(newPHPCommand())
	root.AddCommand(newDatabaseCommand())
	root.AddCommand(newSSHCommand())
	root.AddCommand(newCronCommand())
	root.AddCommand(newBackupCommand())
	root.AddCommand(newSSLCommand())
	root.AddCommand(newReconcileCommand())
	return root
}
