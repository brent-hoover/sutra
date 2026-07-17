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

func searchCmd(cfg config.Config) *cobra.Command {
	var kind, issue string
	var limit int
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Full-text search across issues, documents, and transcript messages",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := client.New(cfg).Search(cmd.Context(), args[0], kind, issue, limit)
			if err != nil {
				return err
			}
			if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), strings.TrimSpace(string(res.Raw)))
				return err
			}
			return renderSearch(cmd, res.Results)
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "", "restrict to a kind: issue|document|message")
	cmd.Flags().StringVar(&issue, "issue", "", "restrict to a single issue id")
	cmd.Flags().IntVar(&limit, "limit", 0, "maximum number of hits (0 = daemon default)")
	return cmd
}

// renderSearch prints ranked hits, one block per hit, each labelled with its
// source kind. Message hits also show their transcript, linked issue, and
// adjacent context.
func renderSearch(cmd *cobra.Command, results domain.SearchResults) error {
	var b strings.Builder
	if len(results.Hits) == 0 {
		fmt.Fprintf(&b, "no results for %q\n", results.Query)
		_, err := io.WriteString(cmd.OutOrStdout(), b.String())
		return err
	}
	for _, h := range results.Hits {
		switch h.Kind {
		case domain.KindIssue:
			fmt.Fprintf(&b, "[issue]    %s\t%s\n", h.Issue.ID, clean(h.Issue.Subject))
		case domain.KindDocument:
			fmt.Fprintf(&b, "[document] %s\t[%s]\t%s (issue %s)\n",
				h.Document.ID, clean(string(h.Document.Kind)), clean(h.Document.Title), h.Document.IssueID)
		case domain.KindMessage:
			renderMessageHit(&b, h)
		}
	}
	_, err := io.WriteString(cmd.OutOrStdout(), b.String())
	return err
}

func renderMessageHit(b *strings.Builder, h domain.SearchHit) {
	link := "-"
	if h.LinkedIssue != nil {
		link = h.LinkedIssue.ID
	}
	fmt.Fprintf(b, "[message]  transcript %s (issue %s)\n", h.Transcript.ID, link)
	fmt.Fprintf(b, "  [%d] %s: %s\n", h.Message.Seq, h.Message.Role, renderMessage(*h.Message))
	for _, c := range h.Context {
		fmt.Fprintf(b, "    ~[%d] %s: %s\n", c.Seq, c.Role, renderMessage(c))
	}
}
