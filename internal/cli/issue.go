package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

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

func listCmd(cfg config.Config) *cobra.Command {
	var status, typ, priority, owner string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List issues (soft-deleted excluded)",
		RunE: func(cmd *cobra.Command, args []string) error {
			filters := map[string]string{
				"status":   status,
				"type":     typ,
				"priority": priority,
				"owner":    owner,
			}
			res, err := client.New(cfg).ListIssues(cmd.Context(), filters)
			if err != nil {
				return err
			}
			return outputList(cmd, res)
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "filter by status")
	cmd.Flags().StringVar(&typ, "type", "", "filter by type")
	cmd.Flags().StringVar(&priority, "priority", "", "filter by priority")
	cmd.Flags().StringVar(&owner, "owner", "", "filter by owner")
	return cmd
}

func updateCmd(cfg config.Config) *cobra.Command {
	var typ, status, priority, owner string
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update an issue's type, status, priority, or owner",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Send only fields the user set, so an update changes exactly
			// what was asked (and an explicit --owner "" can clear owner).
			fields := map[string]string{}
			for name, val := range map[string]string{
				"type": typ, "status": status, "priority": priority, "owner": owner,
			} {
				if cmd.Flags().Changed(name) {
					fields[name] = val
				}
			}
			res, err := client.New(cfg).UpdateIssue(cmd.Context(), args[0], fields)
			if err != nil {
				return err
			}
			return outputDetail(cmd, res)
		},
	}
	cmd.Flags().StringVar(&typ, "type", "", "new type (feature|bug|task|chore)")
	cmd.Flags().StringVar(&status, "status", "", "new status (open|in_progress|closed)")
	cmd.Flags().StringVar(&priority, "priority", "", "new priority (p0|p1|p2|p3)")
	cmd.Flags().StringVar(&owner, "owner", "", "new owner (agent)")
	return cmd
}

func deleteCmd(cfg config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Soft-delete an issue",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := client.New(cfg).DeleteIssue(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return output(cmd, res)
		},
	}
}

func historyCmd(cfg config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "history <id>",
		Short: "View an issue's change history",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := client.New(cfg).History(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return outputHistory(cmd, res)
		},
	}
}

// jsonRequested reports whether --json was set.
func jsonRequested(cmd *cobra.Command) bool {
	b, _ := cmd.Flags().GetBool("json")
	return b
}

// printRaw writes the raw JSON response, trimmed.
func printRaw(cmd *cobra.Command, raw json.RawMessage) error {
	_, err := fmt.Fprintln(cmd.OutOrStdout(), strings.TrimSpace(string(raw)))
	return err
}

// outputList prints one issue per line (or raw JSON with --json).
func outputList(cmd *cobra.Command, res client.IssueListResult) error {
	if jsonRequested(cmd) {
		return printRaw(cmd, res.Raw)
	}
	var b strings.Builder
	for _, i := range res.Issues {
		fmt.Fprintf(&b, "%s\t[%s/%s/%s]\t%s\n", i.ID, i.Type, i.Status, i.Priority, i.Subject)
	}
	_, err := io.WriteString(cmd.OutOrStdout(), b.String())
	return err
}

// outputHistory prints ledger rows chronologically (or raw JSON with --json).
func outputHistory(cmd *cobra.Command, res client.HistoryResult) error {
	if jsonRequested(cmd) {
		return printRaw(cmd, res.Raw)
	}
	var b strings.Builder
	for _, e := range res.Entries {
		fmt.Fprintf(&b, "%s\t%s", e.At.Format(time.RFC3339), e.Kind)
		if e.Field != "" {
			fmt.Fprintf(&b, "\t%s: %q -> %q", e.Field, e.OldValue, e.NewValue)
		}
		b.WriteByte('\n')
	}
	_, err := io.WriteString(cmd.OutOrStdout(), b.String())
	return err
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
