package tui

// White-box test: constructs a Model directly and asserts every rendered list
// line fits within the terminal width, including narrow widths, wide Unicode,
// and the ANSI-styled selected row.

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/brent-hoover/sutra/internal/domain"
)

func TestListViewRowsFitWidth(t *testing.T) {
	parent := "p0000000"
	issues := []domain.Issue{
		{ID: "aaaaaaaa", Subject: "short", Type: domain.TypeTask, Status: domain.StatusOpen, Priority: domain.P2},
		{ID: "bbbbbbbb", Subject: strings.Repeat("日本語ワイド", 8), Type: domain.TypeBug, Status: domain.StatusInProgress, Priority: domain.P1},
		{ID: "cccccccc", Subject: strings.Repeat("x", 200), Type: domain.TypeChore, Status: domain.StatusClosed, Priority: domain.P3, ParentID: &parent},
	}

	for _, width := range []int{1, 2, 3, 8, 20, 80} {
		for cursor := range issues {
			m := &Model{issues: issues, width: width, height: 24, cursor: cursor}
			out := m.listView()
			for _, line := range strings.Split(out, "\n") {
				if w := ansi.StringWidth(line); w > width {
					t.Errorf("width=%d cursor=%d: line width %d exceeds terminal width %d: %q",
						width, cursor, w, width, line)
				}
			}
		}
	}
}
