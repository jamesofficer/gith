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

	if !strings.HasPrefix(postTop(first), "╭─ file 1 of 2 ─") || !strings.Contains(first, "+next := 2") || strings.Contains(first, "logo.png") {
		t.Fatalf("first file =\n%s", first)
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

	for _, line := range strings.Split(panel, "\n") {
		inner := strings.TrimSpace(strings.Trim(strings.TrimSpace(line), "│"))

		if inner = strings.TrimLeft(inner, "0123456789 "); strings.HasPrefix(inner, "+") {
			shown += strings.TrimPrefix(inner, "+")
		}
	}

	if shown != long {
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

	for _, want := range []string{"│  9  context", "│ 10 -old", "│ 10 +first", "│ 11 +second"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q:\n%s", want, view)
		}
	}
}

func TestArrowsScrollTheDiffFasterThanJK(t *testing.T) {
	var body strings.Builder

	for i := 1; i <= 40; i++ {
		fmt.Fprintf(&body, "+line-%02d\n", i)
	}

	src := newSource()
	src.diff = "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -0,0 +1,40 @@\n" + body.String()
	m := press(t, openSettingPull(t, src), char('d'))
	m = press(t, m, key(tea.KeyDown))
	view := screen(m)

	if strings.Contains(view, "line-03") || !strings.Contains(view, "line-04") {
		t.Fatalf("down did not scroll 6 lines:\n%s", view)
	}

	m = press(t, m, char('j'))
	view = screen(m)

	if strings.Contains(view, "line-04") || !strings.Contains(view, "line-05") {
		t.Fatalf("j did not scroll 1 line:\n%s", view)
	}

	m = press(t, m, key(tea.KeyUp))

	if !strings.Contains(screen(m), "line-01") {
		t.Fatalf("up did not scroll back 6 lines:\n%s", screen(m))
	}
}
