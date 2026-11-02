package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jamesofficer/gith/internal/github"
)

const twoFiles = `diff --git a/timer.go b/timer.go
index 1111111..2222222 100644
--- a/timer.go
+++ b/timer.go
@@ -1,2 +1,2 @@ package timer
 context
-old := 1
+next := 2
diff --git a/logo.png b/logo.png
new file mode 100644
index 0000000..3333333
Binary files /dev/null and b/logo.png differ
`

// openSettingPull opens pull request 4, the only item on the second tab.
func openSettingPull(t *testing.T, src *fakeSource) Model {
	t.Helper()
	src.details[4] = github.Detail{Item: src.pulls[0], Body: "Adds a setting."}

	return press(t, press(t, start(t, src), key(tea.KeyRight)), key(tea.KeyEnter))
}

func TestDiffShowsOneFileAtATime(t *testing.T) {
	src := newSource()
	src.diff = twoFiles
	m := press(t, openSettingPull(t, src), char('d'))
	first := screen(m)

	if !strings.HasPrefix(first, "╭─ file 1 of 2 ─") || !strings.Contains(first, "+next := 2") || strings.Contains(first, "logo.png") {
		t.Fatalf("first file =\n%s", first)
	}

	if strings.Contains(first, "Add a setting") {
		t.Fatalf("the header shows over the diff:\n%s", first)
	}

	assertFillsScreen(t, m)
	m = press(t, m, key(tea.KeyRight))
	second := screen(m)

	if !strings.Contains(lineWith(second, "logo.png"), "new file") || !strings.Contains(second, "Binary file") || strings.Contains(second, "next := 2") {
		t.Fatalf("second file =\n%s", second)
	}

	m = press(t, m, key(tea.KeyEsc))

	if m.open == nil || !strings.Contains(screen(m), "Adds a setting.") {
		t.Fatalf("esc did not return to the thread:\n%s", screen(m))
	}

	m = press(t, m, char('d'))

	if src.diffCalls != 1 || !strings.Contains(screen(m), "+next := 2") {
		t.Fatalf("diff was loaded %d times:\n%s", src.diffCalls, screen(m))
	}
}

func TestLongDiffLineWrapsAndKeepsItsMark(t *testing.T) {
	src := newSource()
	long := strings.Repeat("abc := de(f, g)   ", 8) + "end"
	src.diff = "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -0,0 +1 @@\n+" + long + "\n"
	m := press(t, openSettingPull(t, src), char('d'))
	view := screen(m)
	_, panel, _ := strings.Cut(view, postTop(view))
	var shown string

	// Each code row is the gutter, the + mark, the code with its fill,
	// then the padding and the frame.
	for _, line := range strings.Split(panel, "\n") {
		if gutter, code, ok := strings.Cut(line, "+"); ok && strings.Trim(gutter, "│▌ 0123456789") == "" {
			shown += strings.TrimSuffix(code, " │")
		}
	}

	if shown = strings.TrimRight(shown, " "); shown != long {
		t.Fatalf("wrapped line = %q\n%s", shown, screen(m))
	}

	assertFillsScreen(t, m)
}

func TestDiffErrorCanBeRetried(t *testing.T) {
	src := newSource()
	src.diffErr = errors.New("HTTP 406: the diff is too large")
	m := press(t, openSettingPull(t, src), char('d'))

	if view := screen(m); !strings.Contains(view, "Could not load the diff.") || !strings.Contains(view, "too large") {
		t.Fatalf("error view =\n%s", view)
	}

	src.diffErr = nil
	src.diff = twoFiles
	m = press(t, m, char('r'))

	if !strings.Contains(screen(m), "+next := 2") {
		t.Fatalf("retry did not load the diff:\n%s", screen(m))
	}
}

func TestDiffKeyOnIssueDoesNothing(t *testing.T) {
	src := newSource()
	m := press(t, openCacheIssue(t, src), char('d'))

	if src.diffCalls != 0 || !strings.Contains(screen(m), "cache works") {
		t.Fatalf("d on an issue made %d diff calls:\n%s", src.diffCalls, screen(m))
	}
}

func TestDiffShowsTheLineNumberOfEachSide(t *testing.T) {
	src := newSource()
	src.diff = "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -9,2 +9,3 @@\n context\n-old\n+first\n+second\n"
	m := press(t, openSettingPull(t, src), char('d'))
	view := screen(m)

	for _, want := range []string{"│ ▌ 9    context", "│  10   -old", "│  10   +first", "│  11   +second"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q:\n%s", want, view)
		}
	}
}

// fortyLines is one new file with the lines line-01 to line-40.
func fortyLines() string {
	var body strings.Builder

	for i := 1; i <= 40; i++ {
		fmt.Fprintf(&body, "+line-%02d\n", i)
	}

	return "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -0,0 +1,40 @@\n" + body.String()
}

// cursorLine is the screen line of the cursor. The bar only marks the
// cursor when no range is open.
func cursorLine(m Model) string {
	return lineWith(screen(m), "▌")
}

func TestArrowsMoveTheCursorFasterThanJK(t *testing.T) {
	src := newSource()
	src.diff = fortyLines()
	m := press(t, openSettingPull(t, src), char('d'))

	if !strings.Contains(cursorLine(m), "line-01") {
		t.Fatalf("cursor does not start on the first line:\n%s", screen(m))
	}

	steps := []struct {
		key  tea.KeyPressMsg
		want string
	}{
		{key(tea.KeyDown), "line-07"},
		{char('j'), "line-08"},
		{key(tea.KeyUp), "line-02"},
		{char('k'), "line-01"},
		{key(tea.KeyEnd), "line-40"},
	}

	for _, s := range steps {
		m = press(t, m, s.key)

		if !strings.Contains(cursorLine(m), s.want) {
			t.Fatalf("after %s, cursor is not on %s:\n%s", s.key, s.want, screen(m))
		}
	}

	assertFillsScreen(t, m)
}

func TestRangeCommentIsSentForTheSelectedLines(t *testing.T) {
	src := newSource()
	src.diff = "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -9,2 +9,3 @@\n context\n-old\n+first\n+second\n"
	m := press(t, openSettingPull(t, src), char('d'))
	m = press(t, m, char('j'))
	m = press(t, m, char('c'))
	m = press(t, m, char('j'))
	m = press(t, m, char('j'))

	if footer := lineWith(screen(m), "esc cancel"); !strings.Contains(footer, "c comment on lines 10–11") {
		t.Fatalf("footer = %q", footer)
	}

	if bars := strings.Count(screen(m), "▌"); bars != 3 {
		t.Fatalf("%d lines are marked, want 3:\n%s", bars, screen(m))
	}

	m = press(t, m, char('c'))

	if !strings.Contains(screen(m), "Write a comment on a.go lines 10–11") {
		t.Fatalf("comment box missing:\n%s", screen(m))
	}

	assertFillsScreen(t, m)
	m = typeText(t, m, "Merge these.")
	m = press(t, m, ctrl('s'))
	want := github.CodeComment{
		Commit: "head-sha",
		Path:   "a.go",
		Start:  github.DiffLine{Side: github.SideOld, Line: 10},
		End:    github.DiffLine{Side: github.SideNew, Line: 11},
		Body:   "Merge these.",
	}

	if len(src.codeComments) != 1 || src.codeComments[0] != want {
		t.Fatalf("sent %+v", src.codeComments)
	}

	if line := lineWith(screen(m), "+second"); !strings.Contains(line, "💬") || strings.Count(screen(m), "▌") != 1 {
		t.Fatalf("sent comment has no mark, or the range is still open:\n%s", screen(m))
	}

	if m = press(t, m, key(tea.KeyEsc)); !strings.Contains(screen(m), "Adds a setting.") {
		t.Fatalf("esc did not return to the thread:\n%s", screen(m))
	}
}

func TestSelectedLinesStayHighlightedWhileYouWrite(t *testing.T) {
	src := newSource()
	src.diff = "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -9,2 +9,3 @@\n context\n-old\n+first\n+second\n"
	m := press(t, openSettingPull(t, src), char('d'))
	m = press(t, press(t, m, char('j')), char('c'))
	m = press(t, press(t, m, char('j')), char('c'))
	r, g, b, _ := m.tone().selected.RGBA()
	highlight := fmt.Sprintf("48;2;%d;%d;%d", r>>8, g>>8, b>>8)

	for text, want := range map[string]bool{"context": false, "old": true, "first": true, "second": false} {
		if row := lineWith(m.View().Content, text); strings.Contains(row, highlight) != want {
			t.Errorf("row %q highlighted = %t, want %t", text, !want, want)
		}
	}
}

func TestDoubleCCommentsOnOneLine(t *testing.T) {
	src := newSource()
	src.diff = twoFiles
	m := press(t, openSettingPull(t, src), char('d'))
	m = press(t, press(t, m, char('c')), char('c'))
	m = press(t, typeText(t, m, "Why?"), ctrl('s'))
	line := github.DiffLine{Side: github.SideNew, Line: 1}

	if len(src.codeComments) != 1 || src.codeComments[0].Start != line || src.codeComments[0].End != line {
		t.Fatalf("sent %+v", src.codeComments)
	}
}

func TestEscCancelsTheRangeBeforeItLeavesTheDiff(t *testing.T) {
	src := newSource()
	src.diff = twoFiles
	m := press(t, openSettingPull(t, src), char('d'))
	m = press(t, press(t, m, char('c')), key(tea.KeyEsc))

	if strings.Contains(screen(m), "esc cancel") || !strings.Contains(screen(m), "+next := 2") {
		t.Fatalf("esc did not cancel the range:\n%s", screen(m))
	}

	if m = press(t, m, key(tea.KeyEsc)); !strings.Contains(screen(m), "Adds a setting.") {
		t.Fatalf("second esc did not leave the diff:\n%s", screen(m))
	}
}

func TestRangeStaysInsideOneHunk(t *testing.T) {
	src := newSource()
	src.diff = "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1 +1 @@\n-a\n+b\n@@ -20 +20 @@\n-c\n+d\n"
	m := press(t, openSettingPull(t, src), char('d'))
	m = press(t, press(t, m, char('c')), key(tea.KeyDown))

	if footer := lineWith(screen(m), "esc cancel"); !strings.Contains(footer, "lines 1–1") {
		t.Fatalf("range left its hunk: %q\n%s", footer, screen(m))
	}

	m = press(t, press(t, m, key(tea.KeyEsc)), key(tea.KeyDown))

	if !strings.Contains(cursorLine(m), "+d") {
		t.Fatalf("without a range, the cursor did not move to the next hunk:\n%s", screen(m))
	}
}

func TestExistingCodeCommentsAreMarked(t *testing.T) {
	src := newSource()
	src.diff = twoFiles
	src.pulls[0].Comments = 0
	m := openSettingPull(t, src)
	detail := src.details[4]
	detail.Thread = []github.Post{
		{Author: "sam", Body: "Hm.", CreatedAt: when, Code: &github.CodeThread{Path: "timer.go", Line: 2, Side: github.SideOld}},
		{Author: "sam", Body: "Old.", CreatedAt: when, Code: &github.CodeThread{Path: "timer.go", Line: 2, Side: github.SideNew, Outdated: true}},
	}
	src.details[4] = detail
	m = press(t, press(t, m, char('r')), char('d'))
	view := screen(m)

	if !strings.Contains(lineWith(view, "-old := 1"), "💬") || strings.Contains(lineWith(view, "+next := 2"), "💬") {
		t.Fatalf("marks are wrong:\n%s", view)
	}
}

// pullWithCodeComment is pull request 4 with one comment and two replies on
// the line "+next := 2" of twoFiles.
func pullWithCodeComment(t *testing.T) Model {
	t.Helper()
	src := newSource()
	src.diff = twoFiles
	src.details[4] = github.Detail{
		Item: src.pulls[0],
		Body: "Adds a setting.",
		Thread: []github.Post{{
			Author:    "sam",
			Body:      "\nUse a constant here.\n\nIt is clearer.",
			CreatedAt: when,
			Code: &github.CodeThread{
				Path:        "timer.go",
				Line:        2,
				Side:        github.SideNew,
				Replies:     []github.Reply{{Author: "ada", Body: "Done in the next commit.", CreatedAt: when}},
				MoreReplies: 1,
			},
		}},
	}

	return press(t, press(t, start(t, src), key(tea.KeyRight)), key(tea.KeyEnter))
}

func TestCodeCommentShowsAPreviewUnderItsLine(t *testing.T) {
	m := press(t, pullWithCodeComment(t), char('d'))
	view := screen(m)
	rows := strings.Split(view, "\n")

	for i, row := range rows {
		if strings.Contains(row, "+next := 2") {
			preview := rows[i+1]

			if !strings.Contains(preview, "▸ sam  Use a constant here.") || !strings.Contains(preview, "2 replies") {
				t.Fatalf("preview = %q\n%s", preview, view)
			}
		}
	}

	if strings.Contains(view, "It is clearer.") || strings.Contains(view, "Done in the next commit.") {
		t.Fatalf("closed comment shows in full:\n%s", view)
	}

	assertFillsScreen(t, m)
}

func TestEnterOpensAndClosesTheCommentsOfALine(t *testing.T) {
	m := press(t, pullWithCodeComment(t), char('d'))

	if strings.Contains(screen(m), "enter comments") {
		t.Fatal("hint shows on a line without comments")
	}

	m = press(t, m, key(tea.KeyEnter))

	if strings.Contains(screen(m), "It is clearer.") {
		t.Fatal("enter on a line without comments opened a comment")
	}

	m = press(t, m, char('j'))
	m = press(t, m, char('j'))

	if !strings.Contains(screen(m), "enter comments") {
		t.Fatalf("hint missing on a line with comments:\n%s", screen(m))
	}

	m = press(t, m, key(tea.KeyEnter))
	view := screen(m)

	for _, want := range []string{"▾ sam", "It is clearer.", "ada", "Done in the next commit.", "1 more reply on GitHub"} {
		if !strings.Contains(view, want) {
			t.Fatalf("open comment misses %q:\n%s", want, view)
		}
	}

	if !strings.Contains(lineWith(view, "▾ sam"), "│ ▾ sam") || strings.Count(view, "╭") != 2 {
		t.Fatalf("open comment has no frame:\n%s", view)
	}

	assertFillsScreen(t, m)
	m = press(t, m, key(tea.KeyEnter))

	if strings.Contains(screen(m), "It is clearer.") || !strings.Contains(screen(m), "▸ sam") {
		t.Fatalf("enter did not close the comment:\n%s", screen(m))
	}
}

func TestCursorSkipsCommentRows(t *testing.T) {
	m := press(t, pullWithCodeComment(t), char('d'))
	m = press(t, m, char('j'))
	m = press(t, m, char('j'))
	m = press(t, m, key(tea.KeyEnter))
	m = press(t, m, char('k'))

	if !strings.Contains(cursorLine(m), "-old := 1") {
		t.Fatalf("cursor is not on the line above:\n%s", screen(m))
	}

	m = press(t, press(t, m, char('j')), char('j'))

	if !strings.Contains(cursorLine(m), "+next := 2") {
		t.Fatalf("cursor left the last line of code:\n%s", screen(m))
	}
}
