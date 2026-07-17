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

// transcriptCmd groups the transcript-capture subcommands.
func transcriptCmd(cfg config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "transcript",
		Short: "Capture and read Claude session transcripts",
	}
	cmd.AddCommand(
		transcriptIngestCmd(cfg),
		transcriptViewCmd(cfg),
		transcriptLinkCmd(cfg),
		transcriptListCmd(cfg),
		transcriptDiscoverCmd(cfg),
	)
	return cmd
}

func transcriptIngestCmd(cfg config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "ingest <path>",
		Short: "Ingest a Claude .jsonl session file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := client.New(cfg).IngestTranscript(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return outputTranscript(cmd, res)
		},
	}
}

func transcriptViewCmd(cfg config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "view <id>",
		Short: "Read a transcript's messages in order",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := client.New(cfg).GetTranscript(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return outputTranscriptDetail(cmd, res)
		},
	}
}

func transcriptLinkCmd(cfg config.Config) *cobra.Command {
	var issueID string
	cmd := &cobra.Command{
		Use:   "link <transcript-id>",
		Short: "Link a transcript to an issue",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := client.New(cfg).LinkTranscript(cmd.Context(), args[0], issueID)
			if err != nil {
				return err
			}
			return outputTranscript(cmd, res)
		},
	}
	cmd.Flags().StringVar(&issueID, "issue", "", "issue id to link to (required)")
	_ = cmd.MarkFlagRequired("issue")
	return cmd
}

func transcriptListCmd(cfg config.Config) *cobra.Command {
	var issueID string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the transcripts linked to an issue",
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := client.New(cfg).TranscriptsForIssue(cmd.Context(), issueID)
			if err != nil {
				return err
			}
			if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), strings.TrimSpace(string(res.Raw)))
				return err
			}
			var b strings.Builder
			for _, t := range res.Transcripts {
				fmt.Fprintf(&b, "%s\t%s\t%s\n", t.ID, t.CapturedAt.Format("2006-01-02 15:04"), cleanLine(t.Title))
			}
			_, err = io.WriteString(cmd.OutOrStdout(), b.String())
			return err
		},
	}
	cmd.Flags().StringVar(&issueID, "issue", "", "issue id (required)")
	_ = cmd.MarkFlagRequired("issue")
	return cmd
}

func transcriptDiscoverCmd(cfg config.Config) *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "discover",
		Short: "List local Claude session files and their ingested state",
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := client.New(cfg).DiscoverTranscripts(cmd.Context(), dir)
			if err != nil {
				return err
			}
			if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), strings.TrimSpace(string(res.Raw)))
				return err
			}
			var b strings.Builder
			for _, d := range res.Found {
				state := "not-ingested"
				if d.Ingested {
					state = "ingested"
				}
				fmt.Fprintf(&b, "%s\t%s\t%s\n", d.SessionID, state, cleanLine(d.Path))
			}
			_, err = io.WriteString(cmd.OutOrStdout(), b.String())
			return err
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "project dir to scan (default ~/.claude/projects)")
	return cmd
}

// outputTranscript prints a compact one-line confirmation (ingest, link).
func outputTranscript(cmd *cobra.Command, res client.TranscriptResult) error {
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), strings.TrimSpace(string(res.Raw)))
		return err
	}
	t := res.Transcript
	link := "-"
	if t.IssueID != nil {
		link = *t.IssueID
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\tsession=%s\tissue=%s\t%s\n", t.ID, t.SessionID, link, cleanLine(t.Title))
	return err
}

// outputTranscriptDetail prints the transcript's messages in seq order.
func outputTranscriptDetail(cmd *cobra.Command, res client.TranscriptResult) error {
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), strings.TrimSpace(string(res.Raw)))
		return err
	}
	t := res.Transcript
	var b strings.Builder
	fmt.Fprintf(&b, "id:       %s\n", t.ID)
	fmt.Fprintf(&b, "session:  %s\n", t.SessionID)
	fmt.Fprintf(&b, "title:    %s\n", cleanLine(t.Title))
	fmt.Fprintf(&b, "captured: %s\n\n", t.CapturedAt.Format("2006-01-02 15:04"))
	for _, m := range t.Messages {
		fmt.Fprintf(&b, "[%d] %s: %s\n", m.Seq, m.Role, renderMessage(m))
	}
	_, err := io.WriteString(cmd.OutOrStdout(), b.String())
	return err
}

// renderMessage reconstructs a message's displayed text, preferring the
// extracted text and falling back to the raw JSON line.
func renderMessage(m domain.Message) string {
	// Stored message content is untrusted — strip terminal control sequences
	// before it reaches the terminal.
	if m.Text != "" {
		return clean(m.Text)
	}
	return clean(m.Raw)
}
