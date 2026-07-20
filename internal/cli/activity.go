package cli

import (
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/brent-hoover/sutra/internal/client"
	"github.com/brent-hoover/sutra/internal/config"
	"github.com/brent-hoover/sutra/internal/domain"
	"github.com/spf13/cobra"
)

// defaultActivityWindow is how far back `activity` looks when --since is omitted.
const defaultActivityWindow = 72 * time.Hour

func activityCmd(cfg config.Config) *cobra.Command {
	var since string
	cmd := &cobra.Command{
		Use:   "activity",
		Short: "Show recent activity across issues and captured transcripts",
		Long: "Show a reverse-chronological feed of what changed recently — issue\n" +
			"updates, comments, links, and captured Claude transcripts — so you can\n" +
			"pick up where you left off. Recent sessions are auto-ingested first.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			start, err := parseSince(since)
			if err != nil {
				return err
			}
			res, err := client.New(cfg).Activity(cmd.Context(), start)
			if err != nil {
				return err
			}
			if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
				return printRawJSON(cmd.OutOrStdout(), res.Raw)
			}
			return renderActivity(cmd, res.Feed)
		},
	}
	cmd.Flags().StringVar(&since, "since", "",
		"window start: a duration (72h, 3d) or a date (2006-01-02); default 3 days")
	return cmd
}

// parseSince resolves the --since flag to an absolute time. An empty value uses
// the default window; otherwise it accepts a Go duration (with a 'd' days
// extension) or a calendar date interpreted in local time.
func parseSince(s string) (time.Time, error) {
	now := time.Now()
	if strings.TrimSpace(s) == "" {
		return now.Add(-defaultActivityWindow), nil
	}
	if d, err := parseWindowDuration(s); err == nil {
		return now.Add(-d), nil
	}
	if t, err := time.ParseInLocation("2006-01-02", s, time.Local); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("invalid --since %q: want a duration (72h, 3d) or a date (2006-01-02)", s)
}

// parseWindowDuration extends time.ParseDuration with a trailing 'd' for days.
// A negative window is rejected regardless of syntax — the window looks back.
func parseWindowDuration(s string) (time.Duration, error) {
	if n, ok := strings.CutSuffix(s, "d"); ok {
		days, err := strconv.Atoi(n)
		if err != nil {
			return 0, err
		}
		if days < 0 {
			return 0, fmt.Errorf("negative days: %s", s)
		}
		// Guard the days→duration multiplication against int64 overflow, which
		// would otherwise wrap to a negative (future) window.
		if int64(days) > math.MaxInt64/int64(24*time.Hour) {
			return 0, fmt.Errorf("window too large: %s", s)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, err
	}
	if d < 0 {
		return 0, fmt.Errorf("negative duration: %s", s)
	}
	return d, nil
}

// renderActivity prints the feed newest-first, grouped by day. All stored text
// is sanitized before display.
func renderActivity(cmd *cobra.Command, feed domain.ActivityFeed) error {
	var b strings.Builder
	if len(feed.Events) == 0 {
		fmt.Fprintf(&b, "no activity since %s\n", feed.Since.Local().Format("Mon Jan 2 2006 15:04"))
		_, err := io.WriteString(cmd.OutOrStdout(), b.String())
		return err
	}

	var day string
	for _, e := range feed.Events {
		at := e.At.Local()
		if d := at.Format("Mon Jan 2 2006"); d != day {
			day = d
			fmt.Fprintf(&b, "\n%s\n", day)
		}
		fmt.Fprintf(&b, "  %s  %s\n", at.Format("15:04"), activityLine(e))
	}
	_, err := io.WriteString(cmd.OutOrStdout(), b.String())
	return err
}

// activityLine renders one event as a single sanitized line.
func activityLine(e domain.ActivityEvent) string {
	switch e.Type {
	case domain.ActivityTranscript:
		s := fmt.Sprintf("transcript  %q (%s)", cleanLine(e.Title), e.TranscriptID)
		if e.IssueID != "" {
			s += fmt.Sprintf("  → issue %s %q", e.IssueID, cleanLine(e.IssueSubject))
		}
		return s
	default: // ActivityLedger
		s := fmt.Sprintf("%s  %s", e.IssueID, e.LedgerKind)
		if e.Field != "" {
			s += fmt.Sprintf("  %s: %s → %s", cleanLine(e.Field), cleanLine(e.OldValue), cleanLine(e.NewValue))
		}
		if e.IssueSubject != "" {
			s += fmt.Sprintf("  %q", cleanLine(e.IssueSubject))
		}
		return s
	}
}
