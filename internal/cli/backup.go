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
	command.AddCommand(newBackupListCommand())
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
