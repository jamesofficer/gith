package tui

import (
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
)

func TestHighlightKeepsOneRowForEachLine(t *testing.T) {
	lines := []string{"package timer", "", "/* first", "second */", "var x = 1"}
	rows := highlightLines("timer.go", lines)

	if len(rows) != len(lines) {
		t.Fatalf("rows = %d, want %d", len(rows), len(lines))
	}

	for i, row := range rows {
		var text strings.Builder

		for _, token := range row {
			text.WriteString(token.Value)
		}

		if text.String() != lines[i] {
			t.Fatalf("row %d = %q, want %q", i, text.String(), lines[i])
		}
	}

	if rows[3][0].Type.Category() != chroma.Comment {
		t.Fatalf("second line of the comment is %v", rows[3][0].Type)
	}
}

func TestHighlightUnknownFileIsPlainText(t *testing.T) {
	rows := highlightLines("notes.unknown-type", []string{"just text"})

	if len(rows) != 1 || len(rows[0]) != 1 || rows[0][0].Value != "just text" {
		t.Fatalf("rows = %+v", rows)
	}
}
