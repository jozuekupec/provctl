package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"provctl/internal/config"
	"provctl/internal/meta"
	"provctl/internal/plan"
	"provctl/internal/service"
)

func newWebsiteCommand() *cobra.Command {
	command := &cobra.Command{Use: "website", Short: "manage hosted websites"}
	command.AddCommand(newWebsiteCreateCommand())
	return command
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
	command.Flags().StringVar(&websiteType, "type", "php-fpm", "website type: php-fpm or static")
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
