package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/brent-hoover/sutra/internal/client"
	"github.com/brent-hoover/sutra/internal/config"
	"github.com/spf13/cobra"
)

func docCmd(cfg config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doc",
		Short: "Manage documents attached to issues",
	}
	cmd.AddCommand(
		docAttachCmd(cfg),
		docReadCmd(cfg),
		docListCmd(cfg),
		docUpdateCmd(cfg),
		docRemoveCmd(cfg),
	)
	return cmd
}

func docAttachCmd(cfg config.Config) *cobra.Command {
	var kind, title, content string
	cmd := &cobra.Command{
		Use:   "attach <issue-id>",
		Short: "Attach a document to an issue",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := client.New(cfg).AttachDocument(cmd.Context(), args[0], kind, title, content)
			if err != nil {
				return err
			}
			return docOutput(cmd, res)
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "", "document kind: problem|design|plan|scenarios (required)")
	cmd.Flags().StringVar(&title, "title", "", "document title")
	cmd.Flags().StringVar(&content, "content", "", "markdown content (required)")
	_ = cmd.MarkFlagRequired("kind")
	_ = cmd.MarkFlagRequired("content")
	return cmd
}

func docReadCmd(cfg config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "read <doc-id>",
		Short: "Read a document by id",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := client.New(cfg).GetDocument(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return docDetail(cmd, res)
		},
	}
}

func docListCmd(cfg config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "list <issue-id>",
		Short: "List an issue's documents",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := client.New(cfg).ListDocuments(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return docList(cmd, res)
		},
	}
}

func docUpdateCmd(cfg config.Config) *cobra.Command {
	var content string
	cmd := &cobra.Command{
		Use:   "update <doc-id>",
		Short: "Update a document's content",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := client.New(cfg).UpdateDocument(cmd.Context(), args[0], content)
			if err != nil {
				return err
			}
			return docOutput(cmd, res)
		},
	}
	cmd.Flags().StringVar(&content, "content", "", "new markdown content (required)")
	_ = cmd.MarkFlagRequired("content")
	return cmd
}

func docRemoveCmd(cfg config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <doc-id>",
		Short: "Remove a document",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := client.New(cfg).RemoveDocument(cmd.Context(), args[0]); err != nil {
				return err
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "removed %s\n", args[0])
			return err
		},
	}
}

// docWantJSON prints the raw response when --json is set.
func docWantJSON(cmd *cobra.Command, raw []byte) (bool, error) {
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), strings.TrimSpace(string(raw)))
		return true, err
	}
	return false, nil
}

// docOutput prints a compact one-line confirmation (used by attach/update).
func docOutput(cmd *cobra.Command, res client.DocumentResult) error {
	if handled, err := docWantJSON(cmd, res.Raw); handled {
		return err
	}
	d := res.Document
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t[%s]\t%s\n", d.ID, d.Kind, d.Title)
	return err
}

// docDetail prints a document's kind, title, and content (used by read).
func docDetail(cmd *cobra.Command, res client.DocumentResult) error {
	if handled, err := docWantJSON(cmd, res.Raw); handled {
		return err
	}
	d := res.Document
	var b strings.Builder
	fmt.Fprintf(&b, "id:      %s\n", d.ID)
	fmt.Fprintf(&b, "kind:    %s\n", d.Kind)
	fmt.Fprintf(&b, "title:   %s\n", d.Title)
	fmt.Fprintf(&b, "\n%s\n", d.Content)
	_, err := io.WriteString(cmd.OutOrStdout(), b.String())
	return err
}

// docList prints each document's kind and title (used by list).
func docList(cmd *cobra.Command, res client.DocumentListResult) error {
	if handled, err := docWantJSON(cmd, res.Raw); handled {
		return err
	}
	var b strings.Builder
	for _, d := range res.Documents {
		fmt.Fprintf(&b, "%s\t[%s]\t%s\n", d.ID, d.Kind, d.Title)
	}
	_, err := io.WriteString(cmd.OutOrStdout(), b.String())
	return err
}
