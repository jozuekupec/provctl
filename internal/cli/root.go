// Package cli exposes the command-line frontend without business logic.
package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"provctl/internal/config"
	"provctl/internal/domain"
	"provctl/internal/fsbrowse"
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
		reconcileRuntime, err := service.NewProductionReconcileRuntime(context.Background(), cfg)
		if err != nil {
			return fmt.Errorf("open reconcile TUI state: %w", err)
		}
		defer reconcileRuntime.Close()
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
		sshRuntime, err := service.NewReadOnlySSHRuntime(context.Background())
		if err != nil {
			return fmt.Errorf("open SSH TUI state: %w", err)
		}
		defer sshRuntime.Close()
		cronRuntime, err := service.NewReadOnlyCronRuntime(context.Background())
		if err != nil {
			return fmt.Errorf("open cron TUI state: %w", err)
		}
		defer cronRuntime.Close()
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
			// Settings can change the ACME e-mail or environment while this TUI is
			// running, so create the short-lived SSL runtime from the current file.
			current, err := config.Load(meta.ConfigFile)
			if err != nil {
				return err
			}
			sslRuntime, err := service.NewProductionSSLRuntime(ctx, current)
			if err != nil {
				return err
			}
			defer sslRuntime.Close()
			if enabled {
				return sslRuntime.Service.Enable(ctx, subscription, domain, false, true, true)
			}
			return sslRuntime.Service.Disable(ctx, subscription, domain)
		}
		createWebsiteFromUI := func(ctx context.Context, subscription, name string, kind domain.WebsiteType, target string, redirectCode int) (int64, error) {
			switch kind {
			case domain.WebsiteStatic:
				return websiteWriteRuntime.Service.CreateStatic(ctx, subscription, name)
			case domain.WebsiteProxy:
				return websiteWriteRuntime.Service.CreateProxy(ctx, subscription, name, target)
			case domain.WebsiteRedirect:
				return websiteWriteRuntime.Service.CreateRedirect(ctx, subscription, name, target, redirectCode)
			default:
				return websiteWriteRuntime.Service.CreatePHPFPM(ctx, subscription, name)
			}
		}
		setWebsiteAlias := func(ctx context.Context, subscription, primaryDomain, alias string, add bool) (int64, error) {
			websites, err := websiteWriteRuntime.Service.ListForSubscription(ctx, subscription)
			if err != nil {
				return 0, err
			}
			for _, website := range websites {
				if website.PrimaryDomain != primaryDomain {
					continue
				}
				if !website.SSLEnabled {
					if add {
						return websiteWriteRuntime.Service.AddAlias(ctx, subscription, primaryDomain, alias)
					}
					return websiteWriteRuntime.Service.RemoveAlias(ctx, subscription, primaryDomain, alias)
				}
				current, err := config.Load(meta.ConfigFile)
				if err != nil {
					return 0, err
				}
				sslRuntime, err := service.NewProductionSSLRuntime(ctx, current)
				if err != nil {
					return 0, err
				}
				defer sslRuntime.Close()
				return 0, sslRuntime.Service.ReconcileAliases(ctx, subscription, primaryDomain, alias, add, false)
			}
			return 0, fmt.Errorf("website %q not found in subscription %q", primaryDomain, subscription)
		}
		_, err = ui.Program(ui.Deps{LoadSubscriptions: runtime.Service.List, LoadWebsites: websiteRuntime.Service.List, LoadDatabases: databaseRuntime.Service.ListForSubscription, LoadSSHKeys: sshRuntime.Service.List, LoadCronJobs: cronRuntime.Service.List, ReadWebsiteLogs: websiteRuntime.Service.ReadLogs, SetWebsiteEnabled: websiteWriteRuntime.Service.SetEnabled, SetWebsiteTLS: setWebsiteTLS, SetWebsiteDocumentRoot: websiteWriteRuntime.Service.SetDocumentRoot, SetWebsiteTarget: websiteWriteRuntime.Service.SetTarget, DeleteWebsite: websiteWriteRuntime.Service.Delete, CreateWebsite: createWebsiteFromUI, SetWebsiteAlias: setWebsiteAlias, CreateSubscription: subscriptionWriteRuntime.Service.Create, SetSubscriptionStatus: subscriptionWriteRuntime.Service.SetStatus, DeleteSubscription: subscriptionWriteRuntime.Service.Delete, LoadPHPVersions: phpReadRuntime.Service.ListVersions, SetWebsitePHP: phpWriteRuntime.Service.Set, RunHealth: healthRuntime.Service.Run, Reconcile: reconcileRuntime.Service.Reconcile, SaveConfig: func(_ context.Context, updated config.Config) error {
			return config.Update(meta.ConfigFile, updated)
		}, BrowsePath: func(_ context.Context, path string, mode fsbrowse.Mode) (string, []fsbrowse.Entry, error) {
			return fsbrowse.Browse(path, mode)
		}, Config: cfg}).Run()
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
