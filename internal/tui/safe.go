package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// clean strips terminal control characters (C0/C1 controls, ESC, DEL) from a
// daemon-provided string so a crafted subject, body, title, or comment cannot
// inject ANSI/OSC escape sequences into the user's terminal. Newlines and tabs
// are preserved for multi-line content.
func clean(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\n' || r == '\t':
			b.WriteRune(r)
		case r == 0x7f: // DEL
			continue
		case r < 0x20: // C0 controls, incl. ESC (0x1b)
			continue
		case r >= 0x80 && r <= 0x9f: // C1 controls
			continue
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// cleanLine is clean for single-line contexts: it also folds any newlines and
// tabs into spaces so a value cannot break the row layout.
func cleanLine(s string) string {
	return strings.NewReplacer("\n", " ", "\t", " ", "\r", " ").Replace(clean(s))
}

// truncate shortens s to at most w terminal display cells (handling wide and
// zero-width runes), appending an ellipsis when it overflows. A non-positive w
// means the width is not yet known and the string is returned unchanged.
// Callers must fold tabs first (tab display width is ambiguous); this operates
// on already-tab-free text.
func truncate(s string, w int) string {
	if w <= 0 || ansi.StringWidth(s) <= w {
		return s
	}
	return ansi.Truncate(s, w, "…")
}
