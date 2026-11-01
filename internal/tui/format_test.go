package tui

import (
	"testing"
	"time"

	"charm.land/lipgloss/v2"
)

func TestAgo(t *testing.T) {
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		at   time.Time
		want string
	}{
		{now.Add(-10 * time.Second), "now"},
		{now.Add(-5 * time.Minute), "5m"},
		{now.Add(-3 * time.Hour), "3h"},
		{now.Add(-48 * time.Hour), "2d"},
		{now.Add(-70 * 24 * time.Hour), "2mo"},
		{now.Add(-400 * 24 * time.Hour), "1y"},
		{time.Time{}, ""},
	}

	for _, tc := range cases {
		if got := ago(tc.at, now); got != tc.want {
			t.Errorf("ago(%s) = %q, want %q", tc.at, got, tc.want)
		}
	}
}

func TestSpreadKeepsTheRightSideOnANarrowLine(t *testing.T) {
	line := spread("↑↓ thread   c comment   b browser", "3/7", 20)

	if lipgloss.Width(line) != 20 || line[len(line)-3:] != "3/7" {
		t.Fatalf("line = %q", line)
	}
}

func TestWindowKeepsCursorVisible(t *testing.T) {
	if start, end := window(0, 10, 3); start != 0 || end != 3 {
		t.Fatalf("top window = %d,%d", start, end)
	}

	if start, end := window(9, 10, 3); start != 7 || end != 10 {
		t.Fatalf("bottom window = %d,%d", start, end)
	}
}
