package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"provctl/internal/config"
	"provctl/internal/meta"
	"provctl/internal/service"
)

func newSSHCommand() *cobra.Command {
	command := &cobra.Command{Use: "ssh", Short: "manage subscription SSH keys"}
	keys := &cobra.Command{Use: "key", Short: "manage public SSH keys"}
	keys.AddCommand(newSSHKeyAddCommand(), newSSHKeyListCommand(), newSSHKeyRemoveCommand())
	command.AddCommand(keys)
	return command
}

func newSSHKeyAddCommand() *cobra.Command {
	var configPath, file string
	var stdin bool
	command := &cobra.Command{Use: "add <subscription>", Short: "validate and add a public SSH key", Args: cobra.ExactArgs(1), RunE: func(command *cobra.Command, args []string) error {
		if (file == "") == !stdin {
			return fmt.Errorf("provide exactly one of --file or --stdin")
		}
		var contents []byte
		var err error
		if stdin {
			contents, err = io.ReadAll(command.InOrStdin())
			if err != nil {
				return fmt.Errorf("read SSH public key: %w", err)
			}
		}
		cfg, err := config.Load(configPath)
		if err != nil {
			return fmt.Errorf("load configuration: %w", err)
		}
		runtime, err := service.NewProductionSSHRuntime(context.Background())
		if err != nil {
			return fmt.Errorf("open SSH state: %w", err)
		}
		defer runtime.Close()
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Limits.LockTimeoutSeconds)*time.Second)
		defer cancel()
		var operationID int64
		if stdin {
			operationID, err = runtime.Service.Add(ctx, args[0], string(contents))
		} else {
			operationID, err = runtime.Service.AddFromFile(ctx, args[0], file)
		}
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(command.OutOrStdout(), "Added SSH key for subscription %q (operation %d).\n", args[0], operationID)
		return err
	}}
	command.Flags().StringVar(&configPath, "config", meta.ConfigFile, "path to config.toml")
	command.Flags().StringVar(&file, "file", "", "path to a public key file")
	command.Flags().BoolVar(&stdin, "stdin", false, "read one public key from stdin")
	return command
}

func newSSHKeyListCommand() *cobra.Command {
	command := &cobra.Command{Use: "list <subscription>", Short: "list a subscription's SSH keys", Args: cobra.ExactArgs(1), RunE: func(command *cobra.Command, args []string) error {
		runtime, err := service.NewReadOnlySSHRuntime(context.Background())
		if err != nil {
			return fmt.Errorf("open SSH state: %w", err)
		}
		defer runtime.Close()
		keys, err := runtime.Service.List(context.Background(), args[0])
		if err != nil {
			return err
		}
		for _, key := range keys {
			if _, err := fmt.Fprintf(command.OutOrStdout(), "%s\t%s\n", key.Fingerprint, key.Comment); err != nil {
				return err
			}
		}
		return nil
	}}
	return command
}

func newSSHKeyRemoveCommand() *cobra.Command {
	var configPath string
	command := &cobra.Command{Use: "remove <subscription> <fingerprint>", Short: "remove a public SSH key", Args: cobra.ExactArgs(2), RunE: func(command *cobra.Command, args []string) error {
		cfg, err := config.Load(configPath)
		if err != nil {
			return fmt.Errorf("load configuration: %w", err)
		}
		runtime, err := service.NewProductionSSHRuntime(context.Background())
		if err != nil {
			return fmt.Errorf("open SSH state: %w", err)
		}
		defer runtime.Close()
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Limits.LockTimeoutSeconds)*time.Second)
		defer cancel()
		operationID, err := runtime.Service.Remove(ctx, args[0], args[1])
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(command.OutOrStdout(), "Removed SSH key for subscription %q (operation %d).\n", args[0], operationID)
		return err
	}}
	command.Flags().StringVar(&configPath, "config", meta.ConfigFile, "path to config.toml")
	return command
}
