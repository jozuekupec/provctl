package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"provctl/internal/config"
	"provctl/internal/domain"
	"provctl/internal/meta"
	"provctl/internal/service"
)

func newDatabaseCommand() *cobra.Command {
	command := &cobra.Command{Use: "database", Short: "manage MariaDB databases"}
	command.AddCommand(newDatabaseCreateCommand(), newDatabaseListCommand(), newDatabasePasswordCommand(), newDatabaseDeleteCommand())
	return command
}

func newDatabaseCreateCommand() *cobra.Command {
	var configPath, credentialsPath string
	var dryRun bool
	command := &cobra.Command{Use: "create <subscription> <name>", Short: "create a prefixed MariaDB database", Args: cobra.ExactArgs(2), RunE: func(command *cobra.Command, args []string) error {
		cfg, err := config.Load(configPath)
		if err != nil {
			return fmt.Errorf("load configuration: %w", err)
		}
		ctx := context.Background()
		if dryRun {
			runtime, err := service.NewReadOnlyDatabaseRuntime(ctx, cfg)
			if err != nil {
				return fmt.Errorf("open database state: %w", err)
			}
			defer runtime.Close()
			operation, _, err := runtime.Service.PrepareCreateWithCredentials(ctx, args[0], args[1], credentialsPath)
			if err != nil {
				return err
			}
			return writePlan(command, operation)
		}
		runtime, err := service.NewProductionDatabaseRuntime(ctx, cfg)
		if err != nil {
			return fmt.Errorf("open database state: %w", err)
		}
		defer runtime.Close()
		lockCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.Limits.LockTimeoutSeconds)*time.Second)
		defer cancel()
		password, operationID, err := runtime.Service.CreateWithCredentials(lockCtx, args[0], args[1], credentialsPath)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(command.OutOrStdout(), "Created database %q for subscription %q (operation %d). Save this password now; it will not be shown again:\n%s\n", args[0]+"_"+args[1], args[0], operationID, password)
		return err
	}}
	command.Flags().StringVar(&configPath, "config", meta.ConfigFile, "path to config.toml")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "show the operation plan without changing the system")
	command.Flags().StringVar(&credentialsPath, "write-credentials", "", "write one subscription-owned client credentials file")
	return command
}

func newDatabaseListCommand() *cobra.Command {
	var configPath string
	command := &cobra.Command{Use: "list <subscription>", Short: "list a subscription's databases", Args: cobra.ExactArgs(1), RunE: func(command *cobra.Command, args []string) error {
		cfg, err := config.Load(configPath)
		if err != nil {
			return fmt.Errorf("load configuration: %w", err)
		}
		runtime, err := service.NewReadOnlyDatabaseRuntime(context.Background(), cfg)
		if err != nil {
			return fmt.Errorf("open database state: %w", err)
		}
		defer runtime.Close()
		databases, err := runtime.Service.ListForSubscription(context.Background(), args[0])
		if err != nil {
			return err
		}
		return writeDatabaseList(command, databases)
	}}
	command.Flags().StringVar(&configPath, "config", meta.ConfigFile, "path to config.toml")
	return command
}

func newDatabasePasswordCommand() *cobra.Command {
	var configPath string
	command := &cobra.Command{Use: "password <subscription> <name>", Short: "replace a database password", Args: cobra.ExactArgs(2), RunE: func(command *cobra.Command, args []string) error {
		cfg, err := config.Load(configPath)
		if err != nil {
			return fmt.Errorf("load configuration: %w", err)
		}
		runtime, err := service.NewProductionDatabaseRuntime(context.Background(), cfg)
		if err != nil {
			return fmt.Errorf("open database state: %w", err)
		}
		defer runtime.Close()
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Limits.LockTimeoutSeconds)*time.Second)
		defer cancel()
		password, operationID, err := runtime.Service.ChangePassword(ctx, args[0], args[1])
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(command.OutOrStdout(), "Changed password for database %q (operation %d). Save it now; it will not be shown again:\n%s\n", args[0]+"_"+args[1], operationID, password)
		return err
	}}
	command.Flags().StringVar(&configPath, "config", meta.ConfigFile, "path to config.toml")
	return command
}

func newDatabaseDeleteCommand() *cobra.Command {
	var configPath string
	var yes bool
	command := &cobra.Command{Use: "delete <subscription> <name>", Short: "permanently remove a database and its local user", Args: cobra.ExactArgs(2), RunE: func(command *cobra.Command, args []string) error {
		if !yes {
			return fmt.Errorf("refusing database deletion without --yes")
		}
		cfg, err := config.Load(configPath)
		if err != nil {
			return fmt.Errorf("load configuration: %w", err)
		}
		runtime, err := service.NewProductionDatabaseRuntime(context.Background(), cfg)
		if err != nil {
			return fmt.Errorf("open database state: %w", err)
		}
		defer runtime.Close()
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Limits.LockTimeoutSeconds)*time.Second)
		defer cancel()
		operationID, err := runtime.Service.Delete(ctx, args[0], args[1])
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(command.OutOrStdout(), "Deleted database %q for subscription %q (operation %d).\n", args[0]+"_"+args[1], args[0], operationID)
		return err
	}}
	command.Flags().StringVar(&configPath, "config", meta.ConfigFile, "path to config.toml")
	command.Flags().BoolVar(&yes, "yes", false, "confirm permanent database deletion")
	return command
}

func writeDatabaseList(command *cobra.Command, databases []domain.Database) error {
	if _, err := fmt.Fprintln(command.OutOrStdout(), "NAME\tUSER\tHOST\tCHARSET"); err != nil {
		return err
	}
	for _, database := range databases {
		if _, err := fmt.Fprintf(command.OutOrStdout(), "%s\t%s\t%s\t%s\n", database.Name, database.User, database.Host, database.Charset); err != nil {
			return err
		}
	}
	return nil
}
