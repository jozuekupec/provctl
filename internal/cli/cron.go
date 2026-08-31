package cli

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"provctl/internal/config"
	"provctl/internal/meta"
	"provctl/internal/service"
)

func newCronCommand() *cobra.Command {
	command := &cobra.Command{Use: "cron", Short: "manage generated subscription crontabs"}
	command.AddCommand(newCronListCommand(), newCronAddCommand(), newCronRemoveCommand())
	return command
}

func newCronListCommand() *cobra.Command {
	return &cobra.Command{Use: "list <subscription>", Short: "list a subscription's cron jobs", Args: cobra.ExactArgs(1), RunE: func(command *cobra.Command, args []string) error {
		runtime, err := service.NewReadOnlyCronRuntime(context.Background())
		if err != nil {
			return fmt.Errorf("open cron state: %w", err)
		}
		defer runtime.Close()
		jobs, err := runtime.Service.List(context.Background(), args[0])
		if err != nil {
			return err
		}
		for _, job := range jobs {
			if _, err := fmt.Fprintf(command.OutOrStdout(), "%d\t%s\t%s\t%s\n", job.ID, job.Schedule, job.Command, job.Comment); err != nil {
				return err
			}
		}
		return nil
	}}
}

func newCronAddCommand() *cobra.Command {
	var configPath, comment string
	command := &cobra.Command{Use: "add <subscription> <schedule> <command>", Short: "add a generated cron job", Args: cobra.ExactArgs(3), RunE: func(command *cobra.Command, args []string) error {
		cfg, err := config.Load(configPath)
		if err != nil {
			return fmt.Errorf("load configuration: %w", err)
		}
		runtime, err := service.NewProductionCronRuntime(context.Background())
		if err != nil {
			return fmt.Errorf("open cron state: %w", err)
		}
		defer runtime.Close()
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Limits.LockTimeoutSeconds)*time.Second)
		defer cancel()
		operationID, err := runtime.Service.Add(ctx, args[0], args[1], args[2], comment)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(command.OutOrStdout(), "Added cron job for subscription %q (operation %d).\n", args[0], operationID)
		return err
	}}
	command.Flags().StringVar(&configPath, "config", meta.ConfigFile, "path to config.toml")
	command.Flags().StringVar(&comment, "comment", "", "optional generated crontab comment")
	return command
}

func newCronRemoveCommand() *cobra.Command {
	var configPath string
	command := &cobra.Command{Use: "remove <subscription> <job-id>", Short: "remove a generated cron job", Args: cobra.ExactArgs(2), RunE: func(command *cobra.Command, args []string) error {
		jobID, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			return fmt.Errorf("parse cron job ID: %w", err)
		}
		cfg, err := config.Load(configPath)
		if err != nil {
			return fmt.Errorf("load configuration: %w", err)
		}
		runtime, err := service.NewProductionCronRuntime(context.Background())
		if err != nil {
			return fmt.Errorf("open cron state: %w", err)
		}
		defer runtime.Close()
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Limits.LockTimeoutSeconds)*time.Second)
		defer cancel()
		operationID, err := runtime.Service.Remove(ctx, args[0], jobID)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(command.OutOrStdout(), "Removed cron job %d for subscription %q (operation %d).\n", jobID, args[0], operationID)
		return err
	}}
	command.Flags().StringVar(&configPath, "config", meta.ConfigFile, "path to config.toml")
	return command
}
