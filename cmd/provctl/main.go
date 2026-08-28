package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"provctl/internal/meta"
)

func main() {
	root := &cobra.Command{
		Use:          meta.Name,
		Short:        "Provisioning control for Debian web hosting",
		SilenceUsage: true,
	}
	root.Flags().Bool("version", false, "print version and exit")
	root.RunE = func(cmd *cobra.Command, _ []string) error {
		version, err := cmd.Flags().GetBool("version")
		if err != nil {
			return err
		}
		if version {
			fmt.Fprintln(cmd.OutOrStdout(), meta.Version)
			return nil
		}
		return cmd.Help()
	}

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
