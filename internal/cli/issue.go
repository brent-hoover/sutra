package cli

import (
	"fmt"
	"io"
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
			return outputDetail(cmd, res)
		},
	}
}

// wantJSON reports whether --json was set and, if so, prints the raw response.
func wantJSON(cmd *cobra.Command, res client.IssueResult) (bool, error) {
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), strings.TrimSpace(string(res.Raw)))
		return true, err
	}
	return false, nil
}

// output prints a compact one-line confirmation (used by create).
func output(cmd *cobra.Command, res client.IssueResult) error {
	if handled, err := wantJSON(cmd, res); handled {
		return err
	}
	i := res.Issue
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t[%s/%s/%s]\t%s\n", i.ID, i.Type, i.Status, i.Priority, i.Subject)
	return err
}

// outputDetail prints all core fields including the body (used by view).
func outputDetail(cmd *cobra.Command, res client.IssueResult) error {
	if handled, err := wantJSON(cmd, res); handled {
		return err
	}
	i := res.Issue
	var b strings.Builder
	fmt.Fprintf(&b, "id:       %s\n", i.ID)
	fmt.Fprintf(&b, "subject:  %s\n", i.Subject)
	fmt.Fprintf(&b, "type:     %s\n", i.Type)
	fmt.Fprintf(&b, "status:   %s\n", i.Status)
	fmt.Fprintf(&b, "priority: %s\n", i.Priority)
	if i.Owner != "" {
		fmt.Fprintf(&b, "owner:    %s\n", i.Owner)
	}
	fmt.Fprintf(&b, "\n%s\n", i.Body)
	_, err := io.WriteString(cmd.OutOrStdout(), b.String())
	return err
}
