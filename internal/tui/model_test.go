package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/bluekeyes/go-gitdiff/gitdiff"
	"github.com/charmbracelet/x/ansi"

	"github.com/jamesofficer/gith/internal/github"
)

var when = time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)

type posted struct {
	number int
	body   string
}

// fakeSource answers like gh and records what the model asked for.
type fakeSource struct {
	issues     []github.Item
	pulls      []github.Item
	details    map[int]github.Detail
	listErr    error
	detailErr  error
	commentErr error
	browseErr  error
	diff       string
	diffErr    error

	listCalls   int
	detailCalls int
	diffCalls   int

	codeComments []github.CodeComment
	posted       []posted
	browsed      []int
}

func (f *fakeSource) List(_ context.Context, kind github.Kind) ([]github.Item, error) {
	f.listCalls++

	if f.listErr != nil {
		return nil, f.listErr
	}

	if kind == github.KindPull {
		return f.pulls, nil
	}

	return f.issues, nil
}

func (f *fakeSource) Detail(_ context.Context, _ github.Kind, number int) (github.Detail, error) {
	f.detailCalls++

	if f.detailErr != nil {
		return github.Detail{}, f.detailErr
	}

	detail, ok := f.details[number]

	if !ok {
		return github.Detail{}, fmt.Errorf("no item %d", number)
	}

	return detail, nil
}

func (f *fakeSource) Diff(_ context.Context, _ int) (github.PullDiff, error) {
	f.diffCalls++

	if f.diffErr != nil {
		return github.PullDiff{}, f.diffErr
	}

	files, _, err := gitdiff.Parse(strings.NewReader(f.diff))

	return github.PullDiff{Commit: "head-sha", Files: files}, err
}

func (f *fakeSource) CommentOnCode(_ context.Context, _ int, comment github.CodeComment) (github.Post, error) {
	f.codeComments = append(f.codeComments, comment)

	if f.commentErr != nil {
		return github.Post{}, f.commentErr
	}

	code := &github.CodeThread{Path: comment.Path, Line: comment.End.Line, Side: comment.End.Side}

	return github.Post{Author: "ada", Body: comment.Body, CreatedAt: when, Code: code}, nil
}

func (f *fakeSource) Comment(_ context.Context, number int, body string) (github.Post, error) {
	f.posted = append(f.posted, posted{number: number, body: body})

	if f.commentErr != nil {
		return github.Post{}, f.commentErr
	}

	return github.Post{Author: "ada", Body: body, CreatedAt: when}, nil
}

func (f *fakeSource) Browse(_ context.Context, number int) error {
	f.browsed = append(f.browsed, number)

	return f.browseErr
}

func issue(number int, title string, comments int) github.Item {
	return github.Item{Number: number, Title: title, Author: "grace", State: "OPEN", UpdatedAt: when, Comments: comments}
}

// cacheIssue is issue 9 with a description and one comment.
func cacheIssue() github.Detail {
	return github.Detail{
		Item: issue(9, "Document the cache", 1),
		Body: "Write down how the cache works.",
		Thread: []github.Post{
			{Author: "ada", Body: "Please do.", CreatedAt: when},
		},
	}
}

func newSource() *fakeSource {
	return &fakeSource{
		issues: []github.Item{issue(12, "Fix the timer", 0), issue(9, "Document the cache", 1)},
		pulls: []github.Item{
			{Number: 4, Title: "Add a setting", Author: "ada", State: "OPEN", UpdatedAt: when, Draft: true},
		},
		details: map[int]github.Detail{9: cacheIssue()},
	}
}

// start is the model at 80x24 with both lists loaded.
func start(t *testing.T, src *fakeSource) Model {
	t.Helper()
	m, _ := update(t, New(src, "acme/widgets"), tea.WindowSizeMsg{Width: 80, Height: 24})

	return settle(t, m, m.Init())
}

// openCacheIssue opens issue 9, the second issue in the list.
func openCacheIssue(t *testing.T, src *fakeSource) Model {
	t.Helper()

	return press(t, press(t, start(t, src), key(tea.KeyDown)), key(tea.KeyEnter))
}

func update(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	model, ok := next.(Model)

	if !ok {
		t.Fatalf("type %T", next)
	}

	return model, cmd
}

// settle runs cmd and gives each message that it makes to the model.
func settle(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()

	for _, msg := range run(cmd) {
		if _, ok := msg.(tea.QuitMsg); ok {
			continue
		}

		m, _ = update(t, m, msg)
	}

	return m
}

func run(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}

	msg := cmd()

	if batch, ok := msg.(tea.BatchMsg); ok {
		var msgs []tea.Msg

		for _, item := range batch {
			msgs = append(msgs, run(item)...)
		}

		return msgs
	}

	return []tea.Msg{msg}
}

func press(t *testing.T, m Model, msg tea.KeyPressMsg) Model {
	t.Helper()
	m, cmd := update(t, m, msg)

	return settle(t, m, cmd)
}

func typeText(t *testing.T, m Model, text string) Model {
	t.Helper()

	for _, r := range text {
		m = press(t, m, char(r))
	}

	return m
}

func key(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

func char(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

func ctrl(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Mod: tea.ModCtrl}
}

func screen(m Model) string {
	return ansi.Strip(m.View().Content)
}

func quits(cmd tea.Cmd) bool {
	for _, msg := range run(cmd) {
		if _, ok := msg.(tea.QuitMsg); ok {
			return true
		}
	}

	return false
}

func lineWith(content, text string) string {
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, text) {
			return line
		}
	}

	return ""
}

// postTop is the top border of the post frame, the second frame on screen.
func postTop(view string) string {
	opens := 0

	for _, line := range strings.Split(view, "\n") {
		if strings.HasPrefix(line, "╭") {
			opens++
		}

		if opens == 2 {
			return line
		}
	}

	return ""
}

func assertFillsScreen(t *testing.T, m Model) {
	t.Helper()
	content := m.View().Content

	if lipgloss.Width(content) != m.width || lipgloss.Height(content) != m.height {
		t.Fatalf("screen = %dx%d, want %dx%d\n%s",
			lipgloss.Width(content), lipgloss.Height(content), m.width, m.height, ansi.Strip(content))
	}
}

func TestListShowsTheRepoAndSelectsTheFirstItem(t *testing.T) {
	m := start(t, newSource())
	view := screen(m)

	if !strings.Contains(view, "acme/widgets") || !strings.Contains(view, "Fix the timer") {
		t.Fatalf("list =\n%s", view)
	}

	if !strings.Contains(lineWith(view, "Fix the timer"), "❯") {
		t.Fatal("first issue is not selected")
	}

	assertFillsScreen(t, m)
}

func TestEachTabKeepsItsOwnCursor(t *testing.T) {
	m := start(t, newSource())
	m = press(t, m, key(tea.KeyDown))
	m = press(t, m, key(tea.KeyDown))

	if !strings.Contains(lineWith(screen(m), "Document the cache"), "❯") {
		t.Fatal("down did not select the second issue")
	}

	m = press(t, m, key(tea.KeyRight))
	view := screen(m)

	if !strings.Contains(view, "Add a setting") || !strings.Contains(view, "draft") {
		t.Fatalf("pull request tab =\n%s", view)
	}

	m = press(t, m, key(tea.KeyLeft))

	if !strings.Contains(lineWith(screen(m), "Document the cache"), "❯") {
		t.Fatal("the issue cursor was lost")
	}
}

func TestOpenItemShowsOnePostAtATime(t *testing.T) {
	m := openCacheIssue(t, newSource())
	description := screen(m)

	if !strings.Contains(description, "Write down how the cache works.") || strings.Contains(description, "Please do.") {
		t.Fatalf("description =\n%s", description)
	}

	if !strings.HasPrefix(postTop(description), "╭─ 1 of 2 ─") || !strings.Contains(description, "1 comment") {
		t.Fatalf("thread position or count missing:\n%s", description)
	}

	assertFillsScreen(t, m)
	m = press(t, m, key(tea.KeyDown))
	comment := screen(m)

	if !strings.Contains(comment, "Please do.") || !strings.HasPrefix(postTop(comment), "╭─ 2 of 2 ─") || strings.Contains(comment, "cache works") {
		t.Fatalf("comment =\n%s", comment)
	}

	m = press(t, m, key(tea.KeyUp))

	if !strings.Contains(screen(m), "cache works") {
		t.Fatal("up did not return to the description")
	}

	m = press(t, m, key(tea.KeyEsc))

	if m.open != nil || !strings.Contains(screen(m), "Fix the timer") {
		t.Fatal("esc did not return to the list")
	}
}

func TestReviewShowsItsVerdict(t *testing.T) {
	src := newSource()
	detail := cacheIssue()
	detail.Thread = []github.Post{{Author: "lin", Review: "APPROVED", CreatedAt: when}}
	src.details[9] = detail
	m := press(t, openCacheIssue(t, src), key(tea.KeyDown))
	header := lineWith(screen(m), "lin")

	if !strings.Contains(header, "approved") {
		t.Fatalf("review header = %q", header)
	}
}

func TestLongPostScrollsInsideItsFrame(t *testing.T) {
	var body strings.Builder

	for i := 1; i <= 40; i++ {
		fmt.Fprintf(&body, "marker-%02d\n\n", i)
	}

	src := newSource()
	src.details[9] = github.Detail{Item: issue(9, "Document the cache", 0), Body: body.String()}
	m := openCacheIssue(t, src)
	top := screen(m)

	if !strings.Contains(top, "marker-01") || strings.Contains(top, "marker-40") {
		t.Fatal("long post did not stay inside its frame")
	}

	assertFillsScreen(t, m)
	m = press(t, m, key(tea.KeyEnd))
	bottom := screen(m)

	if !strings.Contains(bottom, "marker-40") || strings.Contains(bottom, "marker-01") {
		t.Fatalf("end did not scroll inside the frame:\n%s", bottom)
	}

	lines := strings.Split(bottom, "\n")

	if frame := strings.TrimSpace(lines[len(lines)-3]); !strings.HasPrefix(frame, "╰") {
		t.Fatalf("bottom of the frame is not on screen:\n%s", bottom)
	}
}

func TestQuitKeys(t *testing.T) {
	m := start(t, newSource())

	if _, cmd := update(t, m, char('q')); !quits(cmd) {
		t.Fatal("q did not quit")
	}

	if _, cmd := update(t, m, ctrl('c')); !quits(cmd) {
		t.Fatal("ctrl+c did not quit")
	}

	if _, cmd := update(t, m, key(tea.KeyEsc)); quits(cmd) {
		t.Fatal("esc on the list quit the app")
	}
}

func TestItemLoadedAfterLeavingIsKept(t *testing.T) {
	src := newSource()
	m := press(t, start(t, src), key(tea.KeyDown))
	m, load := update(t, m, key(tea.KeyEnter))
	m = press(t, m, key(tea.KeyEsc))
	m = settle(t, m, load)

	if _, cmd := update(t, m, spinner.TickMsg{}); cmd != nil {
		t.Fatal("spinner still runs on the list")
	}

	m = press(t, m, key(tea.KeyEnter))

	if src.detailCalls != 1 || !strings.Contains(screen(m), "cache works") {
		t.Fatalf("item was loaded %d times:\n%s", src.detailCalls, screen(m))
	}
}

func TestDetailErrorCanBeRetried(t *testing.T) {
	src := newSource()
	src.detailErr = errors.New("HTTP 502")
	m := openCacheIssue(t, src)

	if view := screen(m); !strings.Contains(view, "Could not open this item.") || !strings.Contains(view, "HTTP 502") {
		t.Fatalf("error view =\n%s", view)
	}

	src.detailErr = nil
	m = press(t, m, char('r'))

	if !strings.Contains(screen(m), "cache works") {
		t.Fatalf("retry did not load the item:\n%s", screen(m))
	}
}

func TestRefreshLoadsNewData(t *testing.T) {
	src := newSource()
	m := start(t, src)
	src.issues = append(src.issues, issue(30, "A new issue", 0))
	m = press(t, m, char('r'))

	if !strings.Contains(screen(m), "A new issue") || src.listCalls != 4 {
		t.Fatalf("refresh made %d list calls:\n%s", src.listCalls, screen(m))
	}

	m = press(t, m, key(tea.KeyDown))
	m = press(t, m, key(tea.KeyEnter))
	detail := cacheIssue()
	detail.Body = "New text."
	src.details[9] = detail
	m = press(t, m, char('r'))

	if !strings.Contains(screen(m), "New text.") || src.detailCalls != 2 {
		t.Fatalf("item refresh made %d calls:\n%s", src.detailCalls, screen(m))
	}
}

func TestOpenInBrowser(t *testing.T) {
	src := newSource()
	m := press(t, start(t, src), char('o'))
	m = press(t, press(t, m, key(tea.KeyDown)), key(tea.KeyEnter))
	src.browseErr = errors.New("no browser")
	m = press(t, m, char('o'))

	if fmt.Sprint(src.browsed) != "[12 9]" {
		t.Fatalf("browsed = %v", src.browsed)
	}

	if !strings.Contains(screen(m), "Could not open the browser. no browser") {
		t.Fatalf("browser error missing:\n%s", screen(m))
	}

	assertFillsScreen(t, m)
}

func TestCommentBoxTakesAllTypedKeys(t *testing.T) {
	m := press(t, openCacheIssue(t, newSource()), char('c'))

	if view := screen(m); !strings.Contains(view, "Write a comment") || !strings.Contains(view, "ctrl+s send") {
		t.Fatalf("comment box missing:\n%s", view)
	}

	assertFillsScreen(t, m)
	m = press(t, m, key(tea.KeyLeft))
	m = typeText(t, m, "Hi")
	m = press(t, m, key(tea.KeyEnter))
	m = press(t, m, char('q'))
	m, cmd := update(t, m, tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})

	if quits(cmd) || m.open == nil || m.compose().box.Value() != "Hi\nq\n" {
		t.Fatalf("keys left the comment box: %q\n%s", m.compose().box.Value(), screen(m))
	}
}

func TestSendComment(t *testing.T) {
	for _, send := range []tea.KeyPressMsg{ctrl('s'), ctrl(tea.KeyEnter)} {
		t.Run(send.String(), func(t *testing.T) {
			src := newSource()
			m := press(t, openCacheIssue(t, src), char('c'))
			m = typeText(t, m, "Ship it.")
			m = press(t, m, send)
			view := screen(m)

			if len(src.posted) != 1 || src.posted[0] != (posted{number: 9, body: "Ship it."}) {
				t.Fatalf("posted = %+v", src.posted)
			}

			if m.compose() != nil || !strings.Contains(view, "Ship it.") || !strings.HasPrefix(postTop(view), "╭─ 3 of 3 ─") || !strings.Contains(view, "2 comments") {
				t.Fatalf("sent view =\n%s", view)
			}

			m = press(t, m, key(tea.KeyEsc))

			if row := screen(m); !strings.Contains(lineWith(row, "Document the cache"), "💬 2") || strings.Contains(lineWith(row, "Fix the timer"), "💬") {
				t.Fatalf("list counts =\n%s", row)
			}
		})
	}
}

func TestEmptyCommentIsNotSent(t *testing.T) {
	src := newSource()
	m := press(t, openCacheIssue(t, src), char('c'))
	m = typeText(t, m, "  ")
	m = press(t, m, ctrl('s'))

	if len(src.posted) != 0 || !strings.Contains(screen(m), "Write a comment first.") {
		t.Fatalf("posted = %+v\n%s", src.posted, screen(m))
	}

	assertFillsScreen(t, m)
}

func TestFailedCommentKeepsItsText(t *testing.T) {
	src := newSource()
	src.commentErr = errors.New("no permission")
	m := press(t, openCacheIssue(t, src), char('c'))
	m = typeText(t, m, "Ship it.")
	m = press(t, m, ctrl('s'))
	view := screen(m)

	if m.compose() == nil || !strings.Contains(view, "Could not send this comment. no permission") || !strings.Contains(view, "Ship it.") {
		t.Fatalf("error view =\n%s", view)
	}

	assertFillsScreen(t, m)
}

func TestClosedCommentBoxKeepsTheDraft(t *testing.T) {
	m := press(t, openCacheIssue(t, newSource()), char('c'))
	m = typeText(t, m, "Half done")
	m = press(t, m, key(tea.KeyEsc))

	if m.compose() != nil || strings.Contains(screen(m), "Half done") {
		t.Fatal("esc did not close the comment box")
	}

	m = press(t, m, char('c'))

	if m.compose().box.Value() != "Half done" {
		t.Fatalf("draft = %q", m.compose().box.Value())
	}
}

func TestCommentKeyOnListDoesNothing(t *testing.T) {
	m := press(t, start(t, newSource()), char('c'))

	if m.open != nil {
		t.Fatal("c opened an item from the list")
	}
}

func TestEnterOnEmptyListDoesNothing(t *testing.T) {
	m := press(t, start(t, &fakeSource{}), key(tea.KeyEnter))

	if m.open != nil || !strings.Contains(screen(m), "No open issues.") {
		t.Fatalf("empty list =\n%s", screen(m))
	}
}

func TestListErrorIsShown(t *testing.T) {
	m := start(t, &fakeSource{listErr: errors.New("HTTP 401")})

	if view := screen(m); !strings.Contains(view, "Could not load issues.") || !strings.Contains(view, "HTTP 401") {
		t.Fatalf("error view =\n%s", view)
	}
}

func TestTinyTerminalDoesNotBreak(t *testing.T) {
	m := openCacheIssue(t, newSource())
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 12, Height: 5})
	m = press(t, m, char('c'))
	m = typeText(t, m, "Hi")
	m = press(t, m, key(tea.KeyDown))

	if screen(m) == "" {
		t.Fatal("nothing was drawn")
	}
}

func TestRowStaysOnOneScreenLine(t *testing.T) {
	m := New(&fakeSource{}, "acme/widgets")
	item := github.Item{
		Number:    14874,
		Title:     strings.Repeat("Very long title ", 20),
		UpdatedAt: when,
		Labels:    []github.Label{{Name: "bug", Color: "ff5451"}, {Name: "enhancement", Color: "0dd8ac"}},
		Draft:     true,
		Comments:  9,
	}
	line := m.row(item, true, 6, 80, when.Add(2*time.Hour))
	plain := ansi.Strip(line)

	if lipgloss.Width(line) != 80 || strings.Contains(line, "\n") {
		t.Fatalf("row = %q", plain)
	}

	if !strings.Contains(plain, "…") || !strings.Contains(plain, "💬 9") || !strings.Contains(plain, "2h") {
		t.Fatalf("row = %q", plain)
	}
}

func TestArrowsInItemDoNotSwitchTab(t *testing.T) {
	m := openCacheIssue(t, newSource())
	m = press(t, m, key(tea.KeyRight))
	m = press(t, m, key(tea.KeyLeft))

	if m.open == nil || m.kind != github.KindIssue || !strings.Contains(screen(m), "cache works") {
		t.Fatalf("arrows left the item:\n%s", screen(m))
	}
}

func TestCodeCommentShowsItsPlaceAndIndentedReplies(t *testing.T) {
	src := newSource()
	detail := cacheIssue()
	detail.Thread = []github.Post{{
		Author:    "sam",
		Body:      "Use a constant.",
		CreatedAt: when,
		Code: &github.CodeThread{
			Path:        "internal/timer.go",
			Line:        85,
			Hunk:        "@@ -80,6 +80,6 @@\n context\n-old := 1\n+next := 2",
			Resolved:    true,
			Replies:     []github.Reply{{Author: "ada", Body: "Done.", CreatedAt: when}},
			MoreReplies: 2,
		},
	}}
	src.details[9] = detail
	m := press(t, openCacheIssue(t, src), key(tea.KeyDown))
	view := screen(m)

	if !strings.Contains(lineWith(view, "internal/timer.go:85"), "resolved") {
		t.Fatalf("place of the comment missing:\n%s", view)
	}

	if !strings.Contains(view, "+next := 2") || strings.Contains(view, "@@") {
		t.Fatalf("diff missing:\n%s", view)
	}

	comment, reply := lineWith(view, "Use a constant."), lineWith(view, "Done.")

	if indentOf(reply) <= indentOf(comment) || !strings.Contains(reply, "│") {
		t.Fatalf("reply is not indented under the comment:\n%s", view)
	}

	assertFillsScreen(t, m)
	m = press(t, m, key(tea.KeyEnd))

	if !strings.Contains(screen(m), "2 more replies on GitHub") {
		t.Fatalf("note about hidden replies missing:\n%s", screen(m))
	}
}

// indentOf is the column where the text of a post line starts, inside the frame.
func indentOf(line string) int {
	inner := strings.TrimPrefix(strings.TrimSpace(line), "│")

	return len(inner) - len(strings.TrimLeft(inner, " │"))
}

func TestAssigneesAreShown(t *testing.T) {
	src := newSource()
	src.issues[0].Assignees = []string{"lin", "sam"}
	detail := cacheIssue()
	detail.Assignees = []string{"lin"}
	src.details[9] = detail
	m := start(t, src)

	if row := lineWith(screen(m), "Fix the timer"); !strings.Contains(row, "@lin +1") {
		t.Fatalf("row = %q", row)
	}

	if row := lineWith(screen(m), "Document the cache"); strings.Contains(row, "@") {
		t.Fatalf("row without assignees = %q", row)
	}

	m = press(t, press(t, m, key(tea.KeyDown)), key(tea.KeyEnter))

	if !strings.Contains(screen(m), "assigned to @lin") {
		t.Fatalf("header missing assignee:\n%s", screen(m))
	}
}
