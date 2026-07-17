package cli

import (
	"github.com/brent-hoover/sutra/internal/client"
	"github.com/brent-hoover/sutra/internal/config"
	"github.com/spf13/cobra"
)

// labelCmd groups the add/remove label subcommands.
func labelCmd(cfg config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "label",
		Short: "Add or remove an issue's labels",
	}
	cmd.AddCommand(
		labelAddCmd(cfg),
		labelRemoveCmd(cfg),
	)
	return cmd
}

func labelAddCmd(cfg config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "add <id> <label>",
		Short: "Add a label to an issue",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := client.New(cfg).AddLabel(cmd.Context(), args[0], args[1])
			if err != nil {
				return err
			}
			return outputLink(cmd, res)
		},
	}
}

func labelRemoveCmd(cfg config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <id> <label>",
		Short: "Remove a label from an issue",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := client.New(cfg).RemoveLabel(cmd.Context(), args[0], args[1])
			if err != nil {
				return err
			}
			return outputLink(cmd, res)
		},
	}
}
