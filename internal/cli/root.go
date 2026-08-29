// Package cli exposes the command-line frontend without business logic.
package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"provctl/internal/meta"
)

func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:          meta.Name,
		Short:        "Provisioning control for Debian web hosting",
		SilenceUsage: true,
	}
	root.Flags().Bool("version", false, "print version and exit")
	root.RunE = func(command *cobra.Command, _ []string) error {
		version, err := command.Flags().GetBool("version")
		if err != nil {
			return err
		}
		if version {
			_, err := fmt.Fprintln(command.OutOrStdout(), meta.Version)
			return err
		}
		return command.Help()
	}
	root.AddCommand(newDoctorCommand())
	root.AddCommand(newSubscriptionCommand())
	root.AddCommand(newWebsiteCommand())
	return root
}
