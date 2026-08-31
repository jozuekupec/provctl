package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"provctl/internal/config"
	"provctl/internal/meta"
	"provctl/internal/service"
)

func newSSLCommand() *cobra.Command {
	command := &cobra.Command{Use: "ssl", Short: "manage Certbot certificates"}
	command.AddCommand(newSSLEnableCommand(), newSSLDisableCommand(), newSSLStatusCommand(), newSSLDeployHookCommand())
	return command
}

func newSSLEnableCommand() *cobra.Command {
	var configPath string
	var force, forceHTTPS, renewalCheck bool
	command := &cobra.Command{Use: "enable <subscription> <domain>", Short: "issue and enable a TLS certificate", Args: cobra.ExactArgs(2), RunE: func(command *cobra.Command, args []string) error {
		cfg, err := config.Load(configPath)
		if err != nil {
			return fmt.Errorf("load configuration: %w", err)
		}
		runtime, err := service.NewProductionSSLRuntime(context.Background(), cfg)
		if err != nil {
			return fmt.Errorf("open SSL state: %w", err)
		}
		defer runtime.Close()
		return runtime.Service.Enable(context.Background(), args[0], args[1], force, forceHTTPS, renewalCheck)
	}}
	command.Flags().StringVar(&configPath, "config", meta.ConfigFile, "path to config.toml")
	command.Flags().BoolVar(&force, "force", false, "continue when DNS does not match a local server IP")
	command.Flags().BoolVar(&forceHTTPS, "force-https", true, "redirect HTTP to HTTPS after issuance")
	command.Flags().BoolVar(&renewalCheck, "renewal-check", true, "verify Certbot renewal after issuance")
	command.Flags().Bool("no-renewal-check", false, "skip Certbot renewal verification")
	command.PreRunE = func(command *cobra.Command, _ []string) error {
		disabled, err := command.Flags().GetBool("no-renewal-check")
		renewalCheck = !disabled
		return err
	}
	return command
}

func newSSLDisableCommand() *cobra.Command {
	var configPath string
	command := &cobra.Command{Use: "disable <subscription> <domain>", Short: "disable TLS without deleting the certificate", Args: cobra.ExactArgs(2), RunE: func(command *cobra.Command, args []string) error {
		cfg, err := config.Load(configPath)
		if err != nil {
			return fmt.Errorf("load configuration: %w", err)
		}
		runtime, err := service.NewProductionSSLRuntime(context.Background(), cfg)
		if err != nil {
			return fmt.Errorf("open SSL state: %w", err)
		}
		defer runtime.Close()
		return runtime.Service.Disable(context.Background(), args[0], args[1])
	}}
	command.Flags().StringVar(&configPath, "config", meta.ConfigFile, "path to config.toml")
	return command
}

func newSSLStatusCommand() *cobra.Command {
	var configPath string
	command := &cobra.Command{Use: "status <subscription> <domain>", Short: "show the live certificate expiry", Args: cobra.ExactArgs(2), RunE: func(command *cobra.Command, args []string) error {
		if _, err := config.Load(configPath); err != nil {
			return fmt.Errorf("load configuration: %w", err)
		}
		certificate := service.NewCertificateStatusService()
		status, err := certificate.Status(context.Background(), args[0], args[1])
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(command.OutOrStdout(), "lineage: %s\nnot_after: %s\n", status.Lineage, status.NotAfter.Format(time.RFC3339))
		return err
	}}
	command.Flags().StringVar(&configPath, "config", meta.ConfigFile, "path to config.toml")
	return command
}

func newSSLDeployHookCommand() *cobra.Command {
	var configPath, lineage string
	command := &cobra.Command{Use: "deploy-hook", Short: "update certificate cache after Certbot renewal", Args: cobra.NoArgs, RunE: func(command *cobra.Command, _ []string) error {
		cfg, err := config.Load(configPath)
		if err != nil {
			return fmt.Errorf("load configuration: %w", err)
		}
		if lineage == "" {
			return fmt.Errorf("--lineage is required")
		}
		runtime, err := service.NewProductionCertificateRuntime(context.Background(), cfg.Apache.Service)
		if err != nil {
			return fmt.Errorf("open certificate state: %w", err)
		}
		defer runtime.Close()
		return runtime.Service.DeployHook(context.Background(), lineage)
	}}
	command.Flags().StringVar(&configPath, "config", meta.ConfigFile, "path to config.toml")
	command.Flags().StringVar(&lineage, "lineage", "", "Certbot renewed lineage directory")
	return command
}
