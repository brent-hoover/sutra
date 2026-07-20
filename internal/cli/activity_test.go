package cli

import (
	"math"
	"strconv"
	"testing"
	"time"
)

func TestParseWindowDuration(t *testing.T) {
	cases := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{"72h", 72 * time.Hour, false},
		{"3d", 72 * time.Hour, false},
		{"90m", 90 * time.Minute, false},
		{"-24h", 0, true}, // finding 3: negative duration rejected
		{"-1d", 0, true},  // negative days rejected
		{"bogus", 0, true},
		// finding: exercise the day-overflow boundary
		{maxWindowDays() + "d", 0, true},
	}
	for _, tc := range cases {
		got, err := parseWindowDuration(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseWindowDuration(%q) = %v, want error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseWindowDuration(%q): unexpected error %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("parseWindowDuration(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestParseSinceRejectsNegative(t *testing.T) {
	if _, err := parseSince("-24h"); err == nil {
		t.Error("parseSince(-24h) = nil error, want rejection of a future window start")
	}
}

func TestParseSinceDefaultsToWindow(t *testing.T) {
	before := time.Now().Add(-defaultActivityWindow)
	got, err := parseSince("")
	if err != nil {
		t.Fatalf("parseSince: %v", err)
	}
	if got.Before(before.Add(-time.Minute)) || got.After(time.Now()) {
		t.Errorf("parseSince(\"\") = %v, want ~now-%v", got, defaultActivityWindow)
	}
}

// maxWindowDays returns the largest day count that overflows the days→duration
// multiplication, as a string (one past the safe maximum).
func maxWindowDays() string {
	return strconv.Itoa(int(math.MaxInt64/int64(24*time.Hour)) + 1)
}

func TestParseWindowDurationOverflowBoundary(t *testing.T) {
	maxDays := int(math.MaxInt64 / int64(24*time.Hour))
	if _, err := parseWindowDuration(strconv.Itoa(maxDays) + "d"); err != nil {
		t.Errorf("parseWindowDuration(max days) unexpected error: %v", err)
	}
	if _, err := parseWindowDuration(strconv.Itoa(maxDays+1) + "d"); err == nil {
		t.Error("parseWindowDuration(max days + 1) = nil error, want overflow rejection")
	}
}
