package cli

import (
	"fmt"
	"strings"

	"github.com/brent-hoover/sutra/internal/client"
	"github.com/brent-hoover/sutra/internal/config"
	"github.com/spf13/cobra"
)

func createCmd(cfg config.Config) *cobra.Command {
	var subject, body string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an issue",
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := client.New(cfg).CreateIssue(cmd.Context(), subject, body)
			if err != nil {
				return err
			}
			return output(cmd, res)
		},
	}
	cmd.Flags().StringVar(&subject, "subject", "", "issue subject (required)")
	cmd.Flags().StringVar(&body, "body", "", "issue body (required)")
	_ = cmd.MarkFlagRequired("subject")
	_ = cmd.MarkFlagRequired("body")
	return cmd
}

func viewCmd(cfg config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "view <id>",
		Short: "View an issue by id",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := client.New(cfg).GetIssue(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return output(cmd, res)
		},
	}
}

// output prints the raw JSON response when --json is set, otherwise a compact
// human-readable line.
func output(cmd *cobra.Command, res client.IssueResult) error {
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), strings.TrimSpace(string(res.Raw)))
		return err
	}
	i := res.Issue
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t[%s/%s/%s]\t%s\n", i.ID, i.Type, i.Status, i.Priority, i.Subject)
	return err
}
