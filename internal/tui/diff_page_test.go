package tui

import (
	"math"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// twoHunks is a hunk header and two lines of code, then the same again.
var twoHunks = []pageLine{
	{hunk: 0}, {code: true, hunk: 0}, {code: true, hunk: 0},
	{hunk: 1}, {code: true, hunk: 1}, {code: true, hunk: 1},
}

func TestMoveCursor(t *testing.T) {
	tests := []struct {
		name              string
		from, delta, hunk int
		want              int
	}{
		{"first line of code", -1, 1, -1, 1},
		{"skips a hunk header", 2, 1, -1, 4},
		{"stops at the last line", 1, math.MaxInt, -1, 5},
		{"stops at the first line", 5, -math.MaxInt, -1, 1},
		{"stays in the hunk of a range", 1, 6, 0, 2},
		{"stays in the hunk going up", 5, -6, 1, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := moveCursor(twoHunks, tt.from, tt.delta, tt.hunk); got != tt.want {
				t.Fatalf("moveCursor = %d, want %d", got, tt.want)
			}
		})
	}

	if got := moveCursor([]pageLine{{hunk: 0}}, -1, 1, -1); got != -1 {
		t.Fatalf("no code gave %d, want -1", got)
	}
}

func TestDiffWithNoFilesTakesKeys(t *testing.T) {
	m := press(t, openSettingPull(t, newSource()), char('d'))

	for _, k := range []tea.KeyPressMsg{key(tea.KeyHome), key(tea.KeyEnd), char('j'), char('c'), char('c')} {
		m = press(t, m, k)
	}

	if m.compose() != nil || lineWith(screen(m), "changes no files") == "" {
		t.Fatalf("empty diff =\n%s", screen(m))
	}
}
