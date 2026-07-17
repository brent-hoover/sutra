package cli

import "strings"

// clean strips terminal control characters (C0/C1 controls, ESC, DEL) from a
// daemon-provided string so crafted stored content (issue subjects, document
// titles, message text/raw, …) cannot inject ANSI/OSC escape sequences into the
// user's terminal. Newlines and tabs are preserved for multi-line content.
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

// cleanLine is clean for single-line fields: it also folds newlines/tabs into
// spaces so a stored value cannot break the row/label layout.
func cleanLine(s string) string {
	return strings.NewReplacer("\n", " ", "\t", " ", "\r", " ").Replace(clean(s))
}

// cleanLabels sanitizes each label for single-line display.
func cleanLabels(labels []string) []string {
	out := make([]string, len(labels))
	for i, l := range labels {
		out[i] = cleanLine(l)
	}
	return out
}
