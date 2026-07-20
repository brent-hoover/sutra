package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/brent-hoover/sutra/internal/client"
	"github.com/brent-hoover/sutra/internal/config"
	"github.com/brent-hoover/sutra/internal/domain"
	"github.com/spf13/cobra"
)

func threadCmd(cfg config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "thread",
		Short: "Group issues, documents, transcripts, and comments into a thread",
	}
	cmd.AddCommand(
		threadCreateCmd(cfg),
		threadListCmd(cfg),
		threadViewCmd(cfg),
		threadUpdateCmd(cfg),
		threadDeleteCmd(cfg),
		threadAddCmd(cfg),
		threadRemoveCmd(cfg),
	)
	return cmd
}

func threadCreateCmd(cfg config.Config) *cobra.Command {
	var title, body, project string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a thread",
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := client.New(cfg).CreateThread(cmd.Context(), title, body, project)
			if err != nil {
				return err
			}
			if jsonRequested(cmd) {
				return printRawJSON(cmd.OutOrStdout(), res.Raw)
			}
			t := res.Thread
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\t[%s]\t%s\n", t.ID, t.Status, cleanLine(t.Title))
			return err
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "thread title (required)")
	cmd.Flags().StringVar(&body, "body", "", "thread notes")
	cmd.Flags().StringVar(&project, "project", "", "scope the thread to a project id")
	_ = cmd.MarkFlagRequired("title")
	return cmd
}

func threadListCmd(cfg config.Config) *cobra.Command {
	var project string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List threads",
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := client.New(cfg).ListThreads(cmd.Context(), project)
			if err != nil {
				return err
			}
			if jsonRequested(cmd) {
				return printRawJSON(cmd.OutOrStdout(), res.Raw)
			}
			var b strings.Builder
			for _, t := range res.Threads {
				fmt.Fprintf(&b, "%s\t[%s]\t%s\n", t.ID, t.Status, cleanLine(t.Title))
			}
			_, err = io.WriteString(cmd.OutOrStdout(), b.String())
			return err
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "filter by project id")
	return cmd
}

func threadViewCmd(cfg config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "view <id>",
		Short: "View a thread and its members grouped by kind",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := client.New(cfg).GetThread(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if jsonRequested(cmd) {
				return printRawJSON(cmd.OutOrStdout(), res.Raw)
			}
			return renderThread(cmd, res.View)
		},
	}
}

// renderThread prints a thread's fields and its members grouped by kind.
func renderThread(cmd *cobra.Command, v domain.ThreadView) error {
	var b strings.Builder
	fmt.Fprintf(&b, "id:     %s\n", v.ID)
	fmt.Fprintf(&b, "title:  %s\n", cleanLine(v.Title))
	fmt.Fprintf(&b, "status: %s\n", v.Status)
	if v.Body != "" {
		fmt.Fprintf(&b, "\n%s\n", clean(v.Body))
	}
	for _, kind := range []domain.ThreadItemKind{
		domain.ThreadItemIssue, domain.ThreadItemDocument, domain.ThreadItemTranscript, domain.ThreadItemComment,
	} {
		var ids []string
		for _, it := range v.Items {
			if it.Kind == kind {
				ids = append(ids, it.ItemID)
			}
		}
		if len(ids) > 0 {
			fmt.Fprintf(&b, "\n%ss:\n", kind)
			for _, id := range ids {
				fmt.Fprintf(&b, "  %s\n", id)
			}
		}
	}
	_, err := io.WriteString(cmd.OutOrStdout(), b.String())
	return err
}

func threadUpdateCmd(cfg config.Config) *cobra.Command {
	var title, body, status string
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a thread's title, body, or status (active|archived)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fields := map[string]string{}
			for name := range map[string]struct{}{"title": {}, "body": {}, "status": {}} {
				if cmd.Flags().Changed(name) {
					fields[name] = cmd.Flags().Lookup(name).Value.String()
				}
			}
			res, err := client.New(cfg).UpdateThread(cmd.Context(), args[0], fields)
			if err != nil {
				return err
			}
			if jsonRequested(cmd) {
				return printRawJSON(cmd.OutOrStdout(), res.Raw)
			}
			t := res.Thread
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\t[%s]\t%s\n", t.ID, t.Status, cleanLine(t.Title))
			return err
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "new title")
	cmd.Flags().StringVar(&body, "body", "", "new notes")
	cmd.Flags().StringVar(&status, "status", "", "new status: active|archived")
	return cmd
}

func threadDeleteCmd(cfg config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a thread (its members are left intact)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := client.New(cfg).DeleteThread(cmd.Context(), args[0]); err != nil {
				return err
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "deleted %s\n", args[0])
			return err
		},
	}
}

func threadAddCmd(cfg config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "add <thread-id> <kind> <item-id>",
		Short: "Attach an item (kind: issue|document|transcript|comment) to a thread",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := client.New(cfg).AddThreadItem(cmd.Context(), args[0], args[1], args[2])
			if err != nil {
				return err
			}
			if jsonRequested(cmd) {
				return printRawJSON(cmd.OutOrStdout(), res.Raw)
			}
			return renderThread(cmd, res.View)
		},
	}
}

func threadRemoveCmd(cfg config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <thread-id> <kind> <item-id>",
		Short: "Detach an item from a thread",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := client.New(cfg).RemoveThreadItem(cmd.Context(), args[0], args[1], args[2]); err != nil {
				return err
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "removed %s %s from %s\n", args[1], args[2], args[0])
			return err
		},
	}
}
