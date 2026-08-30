package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"provctl/internal/config"
	"provctl/internal/domain"
	"provctl/internal/meta"
	"provctl/internal/plan"
	"provctl/internal/service"
)

func newWebsiteCommand() *cobra.Command {
	command := &cobra.Command{Use: "website", Short: "manage hosted websites"}
	command.AddCommand(newWebsiteCreateCommand())
	command.AddCommand(newWebsiteListCommand())
	command.AddCommand(newWebsiteShowCommand())
	command.AddCommand(newWebsiteSetEnabledCommand("enable", true))
	command.AddCommand(newWebsiteSetEnabledCommand("disable", false))
	command.AddCommand(newWebsiteDeleteCommand())
	command.AddCommand(newWebsiteLogsCommand())
	return command
}

func newWebsiteLogsCommand() *cobra.Command {
	var configPath string
	var errorLog bool
	var lines int
	command := &cobra.Command{Use: "logs <subscription> <domain>", Short: "show recent website logs", Args: cobra.ExactArgs(2), RunE: func(command *cobra.Command, args []string) error {
		service, closeRuntime, err := openReadOnlyWebsiteService(configPath)
		if err != nil {
			return err
		}
		defer closeRuntime()
		contents, err := service.ReadLogs(context.Background(), args[0], args[1], errorLog, lines)
		if err != nil {
			return err
		}
		_, err = fmt.Fprint(command.OutOrStdout(), contents)
		return err
	}}
	command.Flags().StringVar(&configPath, "config", meta.ConfigFile, "path to config.toml")
	command.Flags().BoolVar(&errorLog, "error", false, "show the error log instead of the access log")
	command.Flags().IntVar(&lines, "lines", 50, "number of lines to show (1-1000)")
	return command
}

func newWebsiteDeleteCommand() *cobra.Command {
	var configPath, confirmDomain string
	var dryRun, sure bool
	command := &cobra.Command{Use: "delete <subscription> <domain>", Short: "remove a website configuration while retaining site data", Args: cobra.ExactArgs(2), RunE: func(command *cobra.Command, args []string) error {
		if confirmDomain != args[1] {
			return fmt.Errorf("--confirm-domain must exactly match %q", args[1])
		}
		if !sure {
			return fmt.Errorf("refusing website deletion without --yes-i-am-sure")
		}
		cfg, err := config.Load(configPath)
		if err != nil {
			return fmt.Errorf("load configuration: %w", err)
		}
		ctx := context.Background()
		if dryRun {
			runtime, err := service.NewReadOnlyWebsiteRuntime(ctx, cfg)
			if err != nil {
				return fmt.Errorf("open website state: %w", err)
			}
			defer runtime.Close()
			operation, err := runtime.Service.PrepareDelete(ctx, args[0], args[1])
			if err != nil {
				return err
			}
			return writePlan(command, operation)
		}
		runtime, err := service.NewProductionWebsiteRuntime(ctx, cfg)
		if err != nil {
			return fmt.Errorf("open website state: %w", err)
		}
		defer runtime.Close()
		lockCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.Limits.LockTimeoutSeconds)*time.Second)
		defer cancel()
		operationID, err := runtime.Service.Delete(lockCtx, args[0], args[1])
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(command.OutOrStdout(), "Deleted website %q for subscription %q; site data and logs were retained (operation %d).\n", args[1], args[0], operationID)
		return err
	}}
	command.Flags().StringVar(&configPath, "config", meta.ConfigFile, "path to config.toml")
	command.Flags().StringVar(&confirmDomain, "confirm-domain", "", "repeat the website domain")
	command.Flags().BoolVar(&sure, "yes-i-am-sure", false, "confirm website configuration deletion")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "show the operation plan without changing the system")
	return command
}

func newWebsiteSetEnabledCommand(name string, enabled bool) *cobra.Command {
	var configPath string
	var dryRun bool
	command := &cobra.Command{
		Use:   name + " <subscription> <domain>",
		Short: name + " a website",
		Args:  cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("load configuration: %w", err)
			}
			ctx := context.Background()
			if dryRun {
				runtime, err := service.NewReadOnlyWebsiteRuntime(ctx, cfg)
				if err != nil {
					return fmt.Errorf("open website state: %w", err)
				}
				defer runtime.Close()
				operation, err := runtime.Service.PrepareSetEnabled(ctx, args[0], args[1], enabled)
				if err != nil {
					return err
				}
				return writePlan(command, operation)
			}
			runtime, err := service.NewProductionWebsiteRuntime(ctx, cfg)
			if err != nil {
				return fmt.Errorf("open website state: %w", err)
			}
			defer runtime.Close()
			lockCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.Limits.LockTimeoutSeconds)*time.Second)
			defer cancel()
			operationID, err := runtime.Service.SetEnabled(lockCtx, args[0], args[1], enabled)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(command.OutOrStdout(), "%s website %q for subscription %q (operation %d).\n", map[bool]string{true: "Enabled", false: "Disabled"}[enabled], args[1], args[0], operationID)
			return err
		},
	}
	command.Flags().StringVar(&configPath, "config", meta.ConfigFile, "path to config.toml")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "show the operation plan without changing the system")
	return command
}

func newWebsiteListCommand() *cobra.Command {
	var configPath string
	command := &cobra.Command{
		Use:   "list <subscription>",
		Short: "list websites in a subscription",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			service, closeRuntime, err := openReadOnlyWebsiteService(configPath)
			if err != nil {
				return err
			}
			defer closeRuntime()
			websites, err := service.ListForSubscription(context.Background(), args[0])
			if err != nil {
				return err
			}
			return writeWebsiteList(command, websites)
		},
	}
	command.Flags().StringVar(&configPath, "config", meta.ConfigFile, "path to config.toml")
	return command
}

func newWebsiteShowCommand() *cobra.Command {
	var configPath string
	command := &cobra.Command{
		Use:   "show <subscription> <domain>",
		Short: "show a website",
		Args:  cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			service, closeRuntime, err := openReadOnlyWebsiteService(configPath)
			if err != nil {
				return err
			}
			defer closeRuntime()
			websites, err := service.ListForSubscription(context.Background(), args[0])
			if err != nil {
				return err
			}
			for _, website := range websites {
				if website.PrimaryDomain == args[1] {
					return writeWebsite(command, website)
				}
			}
			return fmt.Errorf("website %q not found in subscription %q", args[1], args[0])
		},
	}
	command.Flags().StringVar(&configPath, "config", meta.ConfigFile, "path to config.toml")
	return command
}

func openReadOnlyWebsiteService(configPath string) (service.WebsiteService, func() error, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return service.WebsiteService{}, nil, fmt.Errorf("load configuration: %w", err)
	}
	runtime, err := service.NewReadOnlyWebsiteRuntime(context.Background(), cfg)
	if err != nil {
		return service.WebsiteService{}, nil, fmt.Errorf("open website state: %w", err)
	}
	return runtime.Service, runtime.Close, nil
}

func newWebsiteCreateCommand() *cobra.Command {
	var configPath string
	var dryRun bool
	var websiteType string
	var target string
	var redirectCode int
	command := &cobra.Command{
		Use:   "create <subscription> <domain>",
		Short: "create a PHP-FPM website",
		Args:  cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("load configuration: %w", err)
			}
			ctx := context.Background()
			if websiteType != "php-fpm" && websiteType != "static" && websiteType != "proxy" && websiteType != "redirect" {
				return fmt.Errorf("website type %q is not implemented", websiteType)
			}
			if dryRun {
				runtime, err := service.NewReadOnlyWebsiteRuntime(ctx, cfg)
				if err != nil {
					return fmt.Errorf("open website state: %w", err)
				}
				defer runtime.Close()
				operation, err := prepareWebsite(runtime.Service, websiteType, ctx, args[0], args[1], target, redirectCode)
				if err != nil {
					return err
				}
				return writePlan(command, operation)
			}
			runtime, err := service.NewProductionWebsiteRuntime(ctx, cfg)
			if err != nil {
				return fmt.Errorf("open website state: %w", err)
			}
			defer runtime.Close()
			lockCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.Limits.LockTimeoutSeconds)*time.Second)
			defer cancel()
			operationID, err := createWebsite(runtime.Service, websiteType, lockCtx, args[0], args[1], target, redirectCode)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(command.OutOrStdout(), "Created website %q for subscription %q (operation %d).\n", args[1], args[0], operationID)
			return err
		},
	}
	command.Flags().StringVar(&configPath, "config", meta.ConfigFile, "path to config.toml")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "show the operation plan without changing the system")
	command.Flags().StringVar(&websiteType, "type", "php-fpm", "website type: php-fpm, static, proxy, or redirect")
	command.Flags().StringVar(&target, "target", "", "proxy or redirect target URL")
	command.Flags().IntVar(&redirectCode, "redirect-code", 301, "redirect status code: 301 or 302")
	return command
}

func prepareWebsite(service service.WebsiteService, websiteType string, ctx context.Context, subscription, domain, target string, redirectCode int) (plan.Plan, error) {
	if websiteType == "static" {
		return service.PrepareCreateStatic(ctx, subscription, domain)
	}
	if websiteType == "proxy" {
		return service.PrepareCreateProxy(ctx, subscription, domain, target)
	}
	if websiteType == "redirect" {
		return service.PrepareCreateRedirect(ctx, subscription, domain, target, redirectCode)
	}
	return service.PrepareCreatePHPFPM(ctx, subscription, domain)
}

func createWebsite(service service.WebsiteService, websiteType string, ctx context.Context, subscription, domain, target string, redirectCode int) (int64, error) {
	if websiteType == "static" {
		return service.CreateStatic(ctx, subscription, domain)
	}
	if websiteType == "proxy" {
		return service.CreateProxy(ctx, subscription, domain, target)
	}
	if websiteType == "redirect" {
		return service.CreateRedirect(ctx, subscription, domain, target, redirectCode)
	}
	return service.CreatePHPFPM(ctx, subscription, domain)
}

func writeWebsiteList(command *cobra.Command, websites []domain.Website) error {
	if _, err := fmt.Fprintln(command.OutOrStdout(), "DOMAIN\tTYPE\tENABLED\tSSL"); err != nil {
		return err
	}
	for _, website := range websites {
		if _, err := fmt.Fprintf(command.OutOrStdout(), "%s\t%s\t%t\t%t\n", website.PrimaryDomain, website.Type, website.Enabled, website.SSLEnabled); err != nil {
			return err
		}
	}
	return nil
}

func writeWebsite(command *cobra.Command, website domain.Website) error {
	_, err := fmt.Fprintf(command.OutOrStdout(), "Domain: %s\nType: %s\nEnabled: %t\nSSL enabled: %t\nDocument root: %s\nTarget: %s\nRedirect code: %d\nPHP version: %s\n", website.PrimaryDomain, website.Type, website.Enabled, website.SSLEnabled, website.DocumentRoot, website.Target, website.RedirectCode, website.PHPVersion)
	return err
}
