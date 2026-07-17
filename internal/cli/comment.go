package cli

import (
	"fmt"

	"github.com/brent-hoover/sutra/internal/client"
	"github.com/brent-hoover/sutra/internal/config"
	"github.com/spf13/cobra"
)

func commentCmd(cfg config.Config) *cobra.Command {
	var body, author string
	cmd := &cobra.Command{
		Use:   "comment <id>",
		Short: "Add a comment to an issue",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := client.New(cfg).AddComment(cmd.Context(), args[0], author, body)
			if err != nil {
				return err
			}
			return outputComment(cmd, res)
		},
	}
	cmd.Flags().StringVar(&body, "body", "", "comment body (required)")
	cmd.Flags().StringVar(&author, "author", "", "comment author")
	_ = cmd.MarkFlagRequired("body")
	return cmd
}

// outputComment prints a one-line confirmation (or raw JSON with --json).
func outputComment(cmd *cobra.Command, res client.CommentResult) error {
	if jsonRequested(cmd) {
		return printRaw(cmd, res.Raw)
	}
	c := res.Comment
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\tcomment on %s\n", c.ID, c.IssueID)
	return err
}
