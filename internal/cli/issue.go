package cli

import (
	"encoding/json"

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
			issue, err := client.New(cfg).CreateIssue(cmd.Context(), subject, body)
			if err != nil {
				return err
			}
			return printJSON(cmd, issue)
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
			issue, err := client.New(cfg).GetIssue(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return printJSON(cmd, issue)
		},
	}
}

func printJSON(cmd *cobra.Command, v any) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
