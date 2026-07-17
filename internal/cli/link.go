package cli

import (
	"fmt"

	"github.com/brent-hoover/sutra/internal/client"
	"github.com/brent-hoover/sutra/internal/config"
	"github.com/spf13/cobra"
)

// linkCmd groups the issue-linking subcommands: parent/child, related, blocking.
func linkCmd(cfg config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "link",
		Short: "Link issues by hierarchy, relation, or blocking",
	}
	cmd.AddCommand(
		linkParentCmd(cfg),
		linkRelatedCmd(cfg),
		linkBlockedCmd(cfg),
	)
	return cmd
}

func linkParentCmd(cfg config.Config) *cobra.Command {
	var parent string
	cmd := &cobra.Command{
		Use:   "parent <id>",
		Short: "Set an issue's parent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := client.New(cfg).SetParent(cmd.Context(), args[0], parent)
			if err != nil {
				return err
			}
			return outputLink(cmd, res)
		},
	}
	cmd.Flags().StringVar(&parent, "parent", "", "parent issue id (required)")
	_ = cmd.MarkFlagRequired("parent")
	return cmd
}

func linkRelatedCmd(cfg config.Config) *cobra.Command {
	var to string
	cmd := &cobra.Command{
		Use:   "related <id>",
		Short: "Relate an issue to another (symmetric)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := client.New(cfg).RelateIssue(cmd.Context(), args[0], to)
			if err != nil {
				return err
			}
			return outputLink(cmd, res)
		},
	}
	cmd.Flags().StringVar(&to, "to", "", "issue id to relate to (required)")
	_ = cmd.MarkFlagRequired("to")
	return cmd
}

func linkBlockedCmd(cfg config.Config) *cobra.Command {
	var by string
	cmd := &cobra.Command{
		Use:   "blocked <id>",
		Short: "Mark an issue as blocked by another",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := client.New(cfg).BlockIssue(cmd.Context(), args[0], by)
			if err != nil {
				return err
			}
			return outputLink(cmd, res)
		},
	}
	cmd.Flags().StringVar(&by, "by", "", "issue id that blocks this one (required)")
	_ = cmd.MarkFlagRequired("by")
	return cmd
}

// outputLink prints a one-line confirmation (or raw JSON with --json).
func outputLink(cmd *cobra.Command, res client.IssueResult) error {
	if handled, err := wantJSON(cmd, res); handled {
		return err
	}
	i := res.Issue
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t[%s/%s/%s]\t%s\n", i.ID, i.Type, i.Status, i.Priority, i.Subject)
	return err
}
