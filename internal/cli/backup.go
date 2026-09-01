package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"provctl/internal/config"
	"provctl/internal/domain"
	"provctl/internal/meta"
	"provctl/internal/service"
)

func newBackupCommand() *cobra.Command {
	command := &cobra.Command{Use: "backup", Short: "manage subscription backups"}
	command.AddCommand(newBackupCreateCommand(), newBackupListCommand(), newBackupInspectCommand(), newBackupRestoreCommand())
	return command
}

func newBackupRestoreCommand() *cobra.Command {
	var configPath string
	var dryRun bool
	command := &cobra.Command{Use: "restore <subscription> <id>", Short: "validate and restore a subscription backup", Args: cobra.ExactArgs(2), RunE: func(command *cobra.Command, args []string) error {
		if !dryRun {
			return fmt.Errorf("restore is not yet enabled; rerun with --dry-run to validate the archive")
		}
		cfg, err := config.Load(configPath)
		if err != nil {
			return fmt.Errorf("load configuration: %w", err)
		}
		var id int64
		if _, err := fmt.Sscan(args[1], &id); err != nil {
			return fmt.Errorf("parse backup ID: %w", err)
		}
		runtime, err := service.NewReadOnlyBackupRuntime(context.Background(), cfg)
		if err != nil {
			return fmt.Errorf("open backup state: %w", err)
		}
		defer runtime.Close()
		metadata, err := runtime.Service.PrepareRestore(context.Background(), args[0], id)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(command.OutOrStdout(), "Validated backup for subscription %q from %s; no changes made.\n", metadata.Subscription.Name, metadata.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"))
		return err
	}}
	command.Flags().StringVar(&configPath, "config", meta.ConfigFile, "path to config.toml")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "validate the backup without restoring")
	return command
}

func newBackupCreateCommand() *cobra.Command {
	var configPath string
	command := &cobra.Command{Use: "create <subscription>", Short: "create a subscription backup", Args: cobra.ExactArgs(1), RunE: func(command *cobra.Command, args []string) error {
		cfg, err := config.Load(configPath)
		if err != nil {
			return fmt.Errorf("load configuration: %w", err)
		}
		runtime, err := service.NewProductionBackupRuntime(context.Background(), cfg)
		if err != nil {
			return fmt.Errorf("open backup state: %w", err)
		}
		defer runtime.Close()
		id, err := runtime.Service.Create(context.Background(), args[0])
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(command.OutOrStdout(), "Created backup %d for subscription %q.\n", id, args[0])
		return err
	}}
	command.Flags().StringVar(&configPath, "config", meta.ConfigFile, "path to config.toml")
	return command
}

func newBackupInspectCommand() *cobra.Command {
	var configPath string
	command := &cobra.Command{Use: "inspect <subscription> <id>", Short: "inspect a backup manifest", Args: cobra.ExactArgs(2), RunE: func(command *cobra.Command, args []string) error {
		cfg, err := config.Load(configPath)
		if err != nil {
			return fmt.Errorf("load configuration: %w", err)
		}
		var id int64
		if _, err := fmt.Sscan(args[1], &id); err != nil {
			return fmt.Errorf("parse backup ID: %w", err)
		}
		runtime, err := service.NewReadOnlyBackupRuntime(context.Background(), cfg)
		if err != nil {
			return fmt.Errorf("open backup state: %w", err)
		}
		defer runtime.Close()
		metadata, err := runtime.Service.Inspect(context.Background(), args[0], id)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(command.OutOrStdout(), "Format: %d\nCreated: %s\nSubscription: %s\n", metadata.FormatVersion, metadata.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"), metadata.Subscription.Name)
		return err
	}}
	command.Flags().StringVar(&configPath, "config", meta.ConfigFile, "path to config.toml")
	return command
}

func newBackupListCommand() *cobra.Command {
	var configPath string
	command := &cobra.Command{Use: "list <subscription>", Short: "list a subscription's backups", Args: cobra.ExactArgs(1), RunE: func(command *cobra.Command, args []string) error {
		cfg, err := config.Load(configPath)
		if err != nil {
			return fmt.Errorf("load configuration: %w", err)
		}
		runtime, err := service.NewReadOnlyBackupRuntime(context.Background(), cfg)
		if err != nil {
			return fmt.Errorf("open backup state: %w", err)
		}
		defer runtime.Close()
		backups, err := runtime.Service.ListForSubscription(context.Background(), args[0])
		if err != nil {
			return err
		}
		return writeBackupList(command, backups)
	}}
	command.Flags().StringVar(&configPath, "config", meta.ConfigFile, "path to config.toml")
	return command
}

func writeBackupList(command *cobra.Command, backups []domain.Backup) error {
	if _, err := fmt.Fprintln(command.OutOrStdout(), "ID\tSTATUS\tSIZE\tSTARTED\tPATH"); err != nil {
		return err
	}
	for _, backup := range backups {
		if _, err := fmt.Fprintf(command.OutOrStdout(), "%d\t%s\t%d\t%s\t%s\n", backup.ID, backup.Status, backup.SizeBytes, backup.StartedAt.UTC().Format("2006-01-02T15:04:05Z"), backup.Path); err != nil {
			return err
		}
	}
	return nil
}
