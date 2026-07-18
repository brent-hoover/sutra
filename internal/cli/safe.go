package cli

import (
	"io"
	"strings"
)

// printRawJSON writes a raw JSON response byte-for-byte (no trimming). The
// daemon's bytes are preserved verbatim so `--json` is a faithful passthrough;
// the only transform is appending a single trailing newline when the response
// does not already end in one (existing trailing newlines are left as-is).
func printRawJSON(w io.Writer, raw []byte) error {
	if _, err := w.Write(raw); err != nil {
		return err
	}
	if n := len(raw); n == 0 || raw[n-1] != '\n' {
		_, err := io.WriteString(w, "\n")
		return err
	}
	return nil
}

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
