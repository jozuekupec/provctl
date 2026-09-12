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
		sslRuntime, err := service.NewProductionSSLRuntime(context.Background(), cfg)
		if err != nil {
			return fmt.Errorf("open SSL mutation state: %w", err)
		}
		defer sslRuntime.Close()
		healthRuntime, err := service.NewProductionHealthRuntime(context.Background(), cfg)
		if err != nil {
			return fmt.Errorf("open health state: %w", err)
		}
		defer healthRuntime.Close()
		databaseRuntime, err := service.NewReadOnlyDatabaseRuntime(context.Background(), cfg)
		if err != nil {
			return fmt.Errorf("open database TUI state: %w", err)
		}
		defer databaseRuntime.Close()
		phpReadRuntime, err := service.NewReadOnlyPHPRuntime(context.Background(), cfg)
		if err != nil {
			return fmt.Errorf("open PHP TUI state: %w", err)
		}
		defer phpReadRuntime.Close()
		phpWriteRuntime, err := service.NewProductionPHPRuntime(context.Background(), cfg)
		if err != nil {
			return fmt.Errorf("open PHP mutation state: %w", err)
		}
		defer phpWriteRuntime.Close()
		setWebsiteTLS := func(ctx context.Context, subscription, domain string, enabled bool) error {
			if enabled {
				return sslRuntime.Service.Enable(ctx, subscription, domain, false, true, true)
			}
			return sslRuntime.Service.Disable(ctx, subscription, domain)
		}
		_, err = ui.Program(ui.Deps{LoadSubscriptions: runtime.Service.List, LoadWebsites: websiteRuntime.Service.List, LoadDatabases: databaseRuntime.Service.ListForSubscription, ReadWebsiteLogs: websiteRuntime.Service.ReadLogs, SetWebsiteEnabled: websiteWriteRuntime.Service.SetEnabled, SetWebsiteTLS: setWebsiteTLS, SetSubscriptionStatus: subscriptionWriteRuntime.Service.SetStatus, DeleteSubscription: subscriptionWriteRuntime.Service.Delete, LoadPHPVersions: phpReadRuntime.Service.ListVersions, SetWebsitePHP: phpWriteRuntime.Service.Set, RunHealth: healthRuntime.Service.Run}).Run()
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
	root.AddCommand(newMigrateCommand())
	return root
}
