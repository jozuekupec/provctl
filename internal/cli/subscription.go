package cli

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"provctl/internal/config"
	"provctl/internal/domain"
	"provctl/internal/meta"
	"provctl/internal/plan"
	"provctl/internal/service"
)

func newSubscriptionCommand() *cobra.Command {
	command := &cobra.Command{Use: "subscription", Short: "manage hosting subscriptions"}
	command.AddCommand(newSubscriptionCreateCommand())
	command.AddCommand(newSubscriptionListCommand())
	command.AddCommand(newSubscriptionShowCommand())
	command.AddCommand(newSubscriptionDeleteCommand())
	command.AddCommand(newSubscriptionStatusCommand("suspend", "suspended"))
	command.AddCommand(newSubscriptionStatusCommand("resume", "active"))
	return command
}

func newSubscriptionStatusCommand(action, status string) *cobra.Command {
	var configPath string
	command := &cobra.Command{Use: action + " <name>", Short: action + " a subscription", Args: cobra.ExactArgs(1), RunE: func(command *cobra.Command, args []string) error {
		cfg, err := config.Load(configPath)
		if err != nil {
			return fmt.Errorf("load configuration: %w", err)
		}
		runtime, err := service.NewProductionSubscriptionRuntime(context.Background(), cfg)
		if err != nil {
			return fmt.Errorf("open subscription state: %w", err)
		}
		defer runtime.Close()
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Limits.LockTimeoutSeconds)*time.Second)
		defer cancel()
		id, err := runtime.Service.SetStatus(ctx, args[0], status)
		if err != nil {
			return err
		}
		pastTense := map[string]string{"suspend": "Suspended", "resume": "Resumed"}[action]
		_, err = fmt.Fprintf(command.OutOrStdout(), "%s subscription %q (operation %d).\n", pastTense, args[0], id)
		return err
	}}
	command.Flags().StringVar(&configPath, "config", meta.ConfigFile, "path to config.toml")
	return command
}

func newSubscriptionDeleteCommand() *cobra.Command {
	var configPath, confirmName string
	var dryRun, force, sure bool
	command := &cobra.Command{
		Use:   "delete <name>",
		Short: "permanently delete an archived subscription",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if confirmName != args[0] {
				return fmt.Errorf("--confirm-name must exactly match %q", args[0])
			}
			if !sure {
				return fmt.Errorf("refusing destructive operation without --yes-i-am-sure")
			}
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("load configuration: %w", err)
			}
			ctx := context.Background()
			if dryRun {
				runtime, err := service.NewReadOnlySubscriptionRuntime(ctx, cfg)
				if err != nil {
					return fmt.Errorf("open subscription state: %w", err)
				}
				defer runtime.Close()
				operation, err := runtime.Service.PrepareDelete(ctx, args[0], force)
				if err != nil {
					return err
				}
				return writePlan(command, operation)
			}
			runtime, err := service.NewProductionSubscriptionRuntime(ctx, cfg)
			if err != nil {
				return fmt.Errorf("open subscription state: %w", err)
			}
			defer runtime.Close()
			lockCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.Limits.LockTimeoutSeconds)*time.Second)
			defer cancel()
			operationID, err := runtime.Service.Delete(lockCtx, args[0], force)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(command.OutOrStdout(), "Deleted subscription %q (operation %d).\n", args[0], operationID)
			return err
		},
	}
	command.Flags().StringVar(&configPath, "config", meta.ConfigFile, "path to config.toml")
	command.Flags().StringVar(&confirmName, "confirm-name", "", "repeat the subscription name")
	command.Flags().BoolVar(&sure, "yes-i-am-sure", false, "confirm permanent deletion")
	command.Flags().BoolVar(&force, "force", false, "delete an active subscription")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "show the operation plan without changing the system")
	return command
}

func newSubscriptionListCommand() *cobra.Command {
	var configPath string
	command := &cobra.Command{
		Use:   "list",
		Short: "list subscriptions",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			service, closeRuntime, err := openReadOnlySubscriptionService(configPath)
			if err != nil {
				return err
			}
			defer closeRuntime()
			subscriptions, err := service.List(context.Background())
			if err != nil {
				return err
			}
			return writeSubscriptionList(command, subscriptions)
		},
	}
	command.Flags().StringVar(&configPath, "config", meta.ConfigFile, "path to config.toml")
	return command
}

func newSubscriptionShowCommand() *cobra.Command {
	var configPath string
	command := &cobra.Command{
		Use:   "show <name>",
		Short: "show a subscription",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			service, closeRuntime, err := openReadOnlySubscriptionService(configPath)
			if err != nil {
				return err
			}
			defer closeRuntime()
			subscription, err := service.Show(context.Background(), args[0])
			if err != nil {
				return err
			}
			return writeSubscription(command, subscription)
		},
	}
	command.Flags().StringVar(&configPath, "config", meta.ConfigFile, "path to config.toml")
	return command
}

func openReadOnlySubscriptionService(configPath string) (service.SubscriptionService, func() error, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return service.SubscriptionService{}, nil, fmt.Errorf("load configuration: %w", err)
	}
	runtime, err := service.NewReadOnlySubscriptionRuntime(context.Background(), cfg)
	if err != nil {
		return service.SubscriptionService{}, nil, fmt.Errorf("open subscription state: %w", err)
	}
	return runtime.Service, runtime.Close, nil
}

func newSubscriptionCreateCommand() *cobra.Command {
	var configPath, quotaDisk string
	var quotaWebsites, quotaDatabases, quotaBackups int
	var dryRun bool
	command := &cobra.Command{
		Use:   "create <name>",
		Short: "create an isolated hosting subscription",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("load configuration: %w", err)
			}
			quotaBytes, err := parseByteSize(quotaDisk)
			if err != nil {
				return fmt.Errorf("parse --quota-disk: %w", err)
			}
			if quotaWebsites < 0 || quotaDatabases < 0 || quotaBackups < 0 {
				return fmt.Errorf("object quotas must not be negative")
			}
			options := service.SubscriptionCreateOptions{QuotaDiskBytes: quotaBytes, QuotaWebsites: quotaWebsites, QuotaDatabases: quotaDatabases, QuotaBackups: quotaBackups}
			ctx := context.Background()
			if dryRun {
				runtime, err := service.NewReadOnlySubscriptionRuntime(ctx, cfg)
				if err != nil {
					return fmt.Errorf("open subscription state: %w", err)
				}
				defer runtime.Close()
				operation, err := runtime.Service.PrepareCreateWithOptions(ctx, args[0], options)
				if err != nil {
					return err
				}
				return writePlan(command, operation)
			}
			runtime, err := service.NewProductionSubscriptionRuntime(ctx, cfg)
			if err != nil {
				return fmt.Errorf("open subscription state: %w", err)
			}
			defer runtime.Close()
			lockCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.Limits.LockTimeoutSeconds)*time.Second)
			defer cancel()
			operationID, err := runtime.Service.CreateWithOptions(lockCtx, args[0], options)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(command.OutOrStdout(), "Created subscription %q (operation %d).\n", args[0], operationID)
			return err
		},
	}
	command.Flags().StringVar(&configPath, "config", meta.ConfigFile, "path to config.toml")
	command.Flags().StringVar(&quotaDisk, "quota-disk", "", "measured disk quota (for example 20G)")
	command.Flags().IntVar(&quotaWebsites, "quota-websites", 0, "maximum websites (0 means unlimited)")
	command.Flags().IntVar(&quotaDatabases, "quota-databases", 0, "maximum databases (0 means unlimited)")
	command.Flags().IntVar(&quotaBackups, "quota-backups", 0, "maximum backups (0 means unlimited)")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "show the operation plan without changing the system")
	return command
}

var byteSize = regexp.MustCompile(`^([1-9][0-9]*)([KMGT]?)$`)

func parseByteSize(value string) (int64, error) {
	if value == "" {
		return 0, nil
	}
	match := byteSize.FindStringSubmatch(strings.ToUpper(value))
	if match == nil {
		return 0, fmt.Errorf("must be a positive size such as 20G")
	}
	amount, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil {
		return 0, err
	}
	for range match[2] {
		for range strings.Index("KMGT", match[2]) + 1 {
			if amount > (1<<63-1)/1024 {
				return 0, fmt.Errorf("size is too large")
			}
			amount *= 1024
		}
	}
	return amount, nil
}

func writePlan(command *cobra.Command, operation plan.Plan) error {
	if _, err := fmt.Fprintf(command.OutOrStdout(), "Plan: %s %s\n", operation.Action, operation.Target); err != nil {
		return err
	}
	for index, step := range operation.Steps {
		if _, err := fmt.Fprintf(command.OutOrStdout(), "%d. %s\n   %s\n", index+1, step.Name, step.Preview); err != nil {
			return err
		}
	}
	return nil
}

func writeSubscriptionList(command *cobra.Command, subscriptions []domain.Subscription) error {
	if _, err := fmt.Fprintln(command.OutOrStdout(), "NAME\tUID\tSTATUS\tHOME"); err != nil {
		return err
	}
	for _, subscription := range subscriptions {
		if _, err := fmt.Fprintf(command.OutOrStdout(), "%s\t%d\t%s\t%s\n", subscription.Name, subscription.UnixUID, subscription.Status, subscription.Home); err != nil {
			return err
		}
	}
	return nil
}

func writeSubscription(command *cobra.Command, subscription domain.Subscription) error {
	_, err := fmt.Fprintf(command.OutOrStdout(), "Name: %s\nUnix user: %s\nUID: %d\nHome: %s\nStatus: %s\nPHP version: %s\nSSH access: %s\nDisk quota: %d bytes\nWebsite quota: %d\nDatabase quota: %d\nBackup quota: %d\n", subscription.Name, subscription.UnixUser, subscription.UnixUID, subscription.Home, subscription.Status, subscription.PHPVersion, subscription.SSHAccess, subscription.QuotaDiskBytes, subscription.QuotaWebsites, subscription.QuotaDatabases, subscription.QuotaBackups)
	return err
}
