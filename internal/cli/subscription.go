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

func newSubscriptionCommand() *cobra.Command {
	command := &cobra.Command{Use: "subscription", Short: "manage hosting subscriptions"}
	command.AddCommand(newSubscriptionCreateCommand())
	command.AddCommand(newSubscriptionListCommand())
	command.AddCommand(newSubscriptionShowCommand())
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
	var configPath string
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
			ctx := context.Background()
			if dryRun {
				runtime, err := service.NewReadOnlySubscriptionRuntime(ctx, cfg)
				if err != nil {
					return fmt.Errorf("open subscription state: %w", err)
				}
				defer runtime.Close()
				operation, err := runtime.Service.PrepareCreate(ctx, args[0])
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
			operationID, err := runtime.Service.Create(lockCtx, args[0])
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(command.OutOrStdout(), "Created subscription %q (operation %d).\n", args[0], operationID)
			return err
		},
	}
	command.Flags().StringVar(&configPath, "config", meta.ConfigFile, "path to config.toml")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "show the operation plan without changing the system")
	return command
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
	_, err := fmt.Fprintf(command.OutOrStdout(), "Name: %s\nUnix user: %s\nUID: %d\nHome: %s\nStatus: %s\nPHP version: %s\nSSH access: %s\n", subscription.Name, subscription.UnixUser, subscription.UnixUID, subscription.Home, subscription.Status, subscription.PHPVersion, subscription.SSHAccess)
	return err
}
