package tui

import (
	"errors"
	"fmt"
	"image/color"
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
	highlight := ansiBackground(m.tone().selected)

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
	press(t, typeText(t, m, "Why?"), ctrl('s'))
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
func pullWithCodeComment(t *testing.T, src *fakeSource) Model {
	t.Helper()
	src.diff = twoFiles
	src.details[4] = github.Detail{
		Item: src.pulls[0],
		Body: "Adds a setting.",
		Thread: []github.Post{{
			Author:    "sam",
			Body:      "\nUse a constant here.\n\nIt is clearer.",
			CreatedAt: when,
			Code: &github.CodeThread{
				ID:          501,
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

func TestCodeCommentShowsInFullWithItsReplies(t *testing.T) {
	m := press(t, pullWithCodeComment(t, newSource()), char('d'))
	view := screen(m)

	for _, want := range []string{"▾ sam", "It is clearer.", "ada", "Done in the next commit.", "1 more reply on GitHub"} {
		if !strings.Contains(view, want) {
			t.Fatalf("comment misses %q:\n%s", want, view)
		}
	}

	if !strings.Contains(lineWith(view, "▾ sam"), "│ ▾ sam") || strings.Count(view, "╭") != 2 {
		t.Fatalf("comment has no frame:\n%s", view)
	}

	assertFillsScreen(t, m)
}

func TestECollapsesAllCommentsToAPreview(t *testing.T) {
	m := press(t, press(t, pullWithCodeComment(t, newSource()), char('d')), char('e'))
	view := screen(m)

	if preview := lineWith(view, "▸ sam"); !strings.Contains(preview, "▸ sam  Use a constant here.") || !strings.Contains(preview, "2 replies") {
		t.Fatalf("no preview:\n%s", view)
	}

	if strings.Contains(view, "It is clearer.") || strings.Contains(view, "Done in the next commit.") {
		t.Fatalf("collapsed comment shows in full:\n%s", view)
	}

	assertFillsScreen(t, m)

	if m = press(t, m, char('e')); !strings.Contains(screen(m), "It is clearer.") {
		t.Fatalf("second e did not open the comments:\n%s", screen(m))
	}
}

func TestEnterClosesAndOpensTheCommentsOfALine(t *testing.T) {
	m := press(t, pullWithCodeComment(t, newSource()), char('d'))

	if strings.Contains(screen(m), "enter comments") {
		t.Fatal("hint shows on a line without comments")
	}

	if m = press(t, m, key(tea.KeyEnter)); !strings.Contains(screen(m), "It is clearer.") {
		t.Fatal("enter on a line without comments closed a comment")
	}

	m = press(t, press(t, m, char('j')), char('j'))

	if !strings.Contains(screen(m), "enter comments") {
		t.Fatalf("hint missing on a line with comments:\n%s", screen(m))
	}

	if m = press(t, m, key(tea.KeyEnter)); strings.Contains(screen(m), "It is clearer.") || !strings.Contains(screen(m), "▸ sam") {
		t.Fatalf("enter did not close the comment:\n%s", screen(m))
	}

	if m = press(t, m, key(tea.KeyEnter)); !strings.Contains(screen(m), "It is clearer.") {
		t.Fatalf("enter did not open the comment:\n%s", screen(m))
	}
}

func TestShiftRRepliesToTheCommentOnTheLine(t *testing.T) {
	src := newSource()
	m := press(t, pullWithCodeComment(t, src), char('d'))
	m = press(t, press(t, m, char('e')), char('R'))

	if strings.Contains(screen(m), "Write a reply") {
		t.Fatal("R opened a reply on a line without comments")
	}

	m = press(t, press(t, press(t, m, char('j')), char('j')), char('R'))

	if !strings.Contains(screen(m), "Write a reply to sam on timer.go line 2") || !strings.Contains(screen(m), "It is clearer.") {
		t.Fatalf("no reply box, or the comment is not open:\n%s", screen(m))
	}

	assertFillsScreen(t, m)
	m = press(t, typeText(t, m, "Will do."), ctrl('s'))

	if len(src.replies) != 1 || src.replies[0] != (posted{number: 501, body: "Will do."}) {
		t.Fatalf("sent %+v", src.replies)
	}

	view := screen(m)

	if strings.Contains(view, "Write a reply") || !strings.Contains(view, "Will do.") || !strings.Contains(view, "+next := 2") {
		t.Fatalf("reply does not show in the diff:\n%s", view)
	}

	if m = press(t, m, key(tea.KeyEnter)); !strings.Contains(lineWith(screen(m), "▸ sam"), "3 replies") {
		t.Fatalf("reply count did not change:\n%s", screen(m))
	}
}

// twoFilesWithBlocks has two blocks of changes in a.go, after 30 lines that
// did not change, and one block in b.go. Each file has a comment.
func twoFilesWithBlocks(t *testing.T) Model {
	t.Helper()
	var diff strings.Builder
	diff.WriteString("diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1,34 +1,34 @@\n")

	for i := 1; i <= 30; i++ {
		fmt.Fprintf(&diff, " same-%02d\n", i)
	}

	diff.WriteString("-two\n+TWO\n three\n-four\n+FOUR\n five\n")
	diff.WriteString("diff --git a/b.go b/b.go\n--- a/b.go\n+++ b/b.go\n@@ -1,2 +1,2 @@\n-x\n+X\n y\n")
	src := newSource()
	src.diff = diff.String()
	src.details[4] = github.Detail{
		Item: src.pulls[0],
		Body: "Adds a setting.",
		Thread: []github.Post{
			{Author: "sam", Body: "On three.", CreatedAt: when, Code: &github.CodeThread{Path: "a.go", Line: 32, Side: github.SideNew}},
			{Author: "sam", Body: "On y.", CreatedAt: when, Code: &github.CodeThread{Path: "b.go", Line: 2, Side: github.SideNew}},
		},
	}

	return press(t, press(t, start(t, src), key(tea.KeyRight)), key(tea.KeyEnter))
}

func TestNAndPMoveBetweenBlocksOfChanges(t *testing.T) {
	m := press(t, press(t, twoFilesWithBlocks(t), char('d')), char('n'))

	if !strings.Contains(cursorLine(m), "-two") || !strings.Contains(screen(m), "+TWO") {
		t.Fatalf("n did not show the first change:\n%s", screen(m))
	}

	steps := []struct {
		key  tea.KeyPressMsg
		want string
	}{
		{char('n'), "-four"},
		{char('n'), "-x"},
		{char('n'), "-x"},
		{char('p'), "-four"},
		{char('p'), "-two"},
	}

	for _, s := range steps {
		m = press(t, m, s.key)

		if !strings.Contains(cursorLine(m), s.want) {
			t.Fatalf("after %s, cursor is not on %s:\n%s", s.key, s.want, screen(m))
		}
	}
}

func TestShiftNAndPMoveBetweenComments(t *testing.T) {
	m := press(t, twoFilesWithBlocks(t), char('d'))
	steps := []struct {
		key  tea.KeyPressMsg
		want string
	}{
		{char('N'), "three"},
		{char('N'), " y"},
		{char('P'), "three"},
	}

	for _, s := range steps {
		m = press(t, m, s.key)

		if !strings.Contains(cursorLine(m), s.want) {
			t.Fatalf("after %s, cursor is not on %s:\n%s", s.key, s.want, screen(m))
		}
	}

	if !strings.Contains(screen(m), "On three.") {
		t.Fatalf("the comment under the cursor is not on screen:\n%s", screen(m))
	}
}

// threeFiles is a diff of a.go, b.go, and c.go. b.go is viewed, and c.go
// changed after it was viewed.
func threeFiles(t *testing.T, src *fakeSource) Model {
	t.Helper()
	var diff strings.Builder

	for _, name := range []string{"a", "b", "c"} {
		fmt.Fprintf(&diff, "diff --git a/%[1]s.go b/%[1]s.go\n--- a/%[1]s.go\n+++ b/%[1]s.go\n@@ -1 +1 @@\n-old_%[1]s\n+new_%[1]s\n", name)
	}

	src.diff = diff.String()
	src.viewed = map[string]github.ViewState{"b.go": github.Viewed, "c.go": github.ChangedSinceViewed}

	return press(t, openSettingPull(t, src), char('d'))
}

func TestVMarksTheFileViewedAndMovesToTheNextFile(t *testing.T) {
	src := newSource()
	m := threeFiles(t, src)

	if title := lineWith(screen(m), "file 1 of 3"); !strings.Contains(title, "file 1 of 3 · 1 viewed") {
		t.Fatalf("title = %q", title)
	}

	m = press(t, m, char('v'))

	if len(src.viewedCalls) != 1 || src.viewedCalls[0] != (viewedCall{pull: "PR_4", path: "a.go", viewed: true}) {
		t.Fatalf("sent %+v", src.viewedCalls)
	}

	view := screen(m)

	if !strings.Contains(view, "file 3 of 3 · 2 viewed") || !strings.Contains(lineWith(view, "c.go"), "Changed since viewed") {
		t.Fatalf("v did not move past the viewed file to c.go:\n%s", view)
	}

	m = press(t, press(t, m, key(tea.KeyLeft)), key(tea.KeyLeft))

	if !strings.Contains(lineWith(screen(m), "a.go"), "✓ Viewed") {
		t.Fatalf("a.go is not marked:\n%s", screen(m))
	}

	m = press(t, m, char('v'))

	if last := src.viewedCalls[len(src.viewedCalls)-1]; last.viewed || !strings.Contains(screen(m), "file 1 of 3 · 1 viewed") || strings.Contains(screen(m), "✓ Viewed") {
		t.Fatalf("second v did not remove the mark (%+v):\n%s", last, screen(m))
	}
}

func TestDiffOpensAtTheFirstFileThatIsNotViewed(t *testing.T) {
	src := newSource()
	m := threeFiles(t, src)
	m = press(t, press(t, m, char('v')), key(tea.KeyEsc))

	if m = press(t, m, char('d')); !strings.Contains(screen(m), "file 3 of 3") {
		t.Fatalf("diff did not open at c.go:\n%s", screen(m))
	}

	src.viewed = map[string]github.ViewState{"a.go": github.Viewed, "b.go": github.Viewed, "c.go": github.Viewed}

	if m = press(t, m, char('r')); !strings.Contains(screen(m), "file 1 of 3 · 3 viewed") {
		t.Fatalf("diff with all files viewed did not open at the first file:\n%s", screen(m))
	}
}

func TestViewedFileIsGrey(t *testing.T) {
	m := threeFiles(t, newSource())
	m = press(t, m, key(tea.KeyRight))
	colors := m.tone()
	faded, added := ansiBackground(colors.fadedAdded), ansiBackground(colors.added)

	if row := lineWith(m.View().Content, "new_b"); !strings.Contains(row, faded) || strings.Contains(row, added) {
		t.Fatalf("viewed row is not faded: %q", row)
	}

	m = press(t, m, key(tea.KeyRight))

	if row := lineWith(m.View().Content, "new_c"); strings.Contains(row, faded) || !strings.Contains(row, added) {
		t.Fatalf("row of a file that changed since viewed is faded: %q", row)
	}
}

func TestViewedMarkGoesBackWhenGitHubRefusesIt(t *testing.T) {
	src := newSource()
	src.viewedErr = errors.New("forbidden")
	m := threeFiles(t, src)
	m = press(t, m, char('v'))

	if !strings.Contains(screen(m), "Could not mark a.go as viewed. forbidden") || !strings.Contains(screen(m), "· 1 viewed") {
		t.Fatalf("no error, or the count did not go back:\n%s", screen(m))
	}

	m = press(t, press(t, m, key(tea.KeyLeft)), key(tea.KeyLeft))

	if strings.Contains(lineWith(screen(m), "a.go"), "✓ Viewed") {
		t.Fatalf("a.go is still marked:\n%s", screen(m))
	}
}

// ansiBackground is the escape code that sets c as the background.
func ansiBackground(c color.Color) string {
	r, g, b, _ := c.RGBA()

	return fmt.Sprintf("48;2;%d;%d;%d", r>>8, g>>8, b>>8)
}

func TestCursorSkipsCommentRows(t *testing.T) {
	m := press(t, pullWithCodeComment(t, newSource()), char('d'))
	m = press(t, m, char('j'))
	m = press(t, m, char('j'))
	m = press(t, m, char('k'))

	if !strings.Contains(cursorLine(m), "-old := 1") {
		t.Fatalf("cursor is not on the line above:\n%s", screen(m))
	}

	m = press(t, press(t, m, char('j')), char('j'))

	if !strings.Contains(cursorLine(m), "+next := 2") {
		t.Fatalf("cursor left the last line of code:\n%s", screen(m))
	}
}
