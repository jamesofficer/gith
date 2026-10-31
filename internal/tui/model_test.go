package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jamesofficer/gith/internal/github"
)

type fakeSource struct {
	name    string
	issues  []github.Item
	pulls   []github.Item
	detail  github.Detail
	listErr error
}

func (f fakeSource) Name(context.Context) (string, error) {
	return f.name, nil
}

func (f fakeSource) List(_ context.Context, kind github.Kind) ([]github.Item, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}

	if kind == github.KindPull {
		return f.pulls, nil
	}

	return f.issues, nil
}

func (f fakeSource) Detail(context.Context, github.Kind, int) (github.Detail, error) {
	return f.detail, nil
}

func TestKeysMatch(t *testing.T) {
	cases := []struct {
		key  tea.KeyPressMsg
		want string
	}{
		{tea.KeyPressMsg{Code: tea.KeyUp}, "up"},
		{tea.KeyPressMsg{Code: tea.KeyDown}, "down"},
		{tea.KeyPressMsg{Code: tea.KeyLeft}, "left"},
		{tea.KeyPressMsg{Code: tea.KeyRight}, "right"},
		{tea.KeyPressMsg{Code: tea.KeyEnter}, "enter"},
		{tea.KeyPressMsg{Code: tea.KeyEsc}, "esc"},
		{tea.KeyPressMsg{Code: 'q', Text: "q"}, "q"},
	}

	for _, tc := range cases {
		if got := tc.key.String(); got != tc.want {
			t.Errorf("%q key string = %q", tc.want, got)
		}
	}
}

func TestBrowseListAndOpenDetail(t *testing.T) {
	when := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	src := fakeSource{
		name: "acme/widgets",
		issues: []github.Item{
			{Number: 12, Title: "Fix the timer", Author: "ada", State: "OPEN", UpdatedAt: when},
			{Number: 9, Title: "Document the cache", Author: "grace", State: "OPEN", UpdatedAt: when},
		},
		pulls: []github.Item{
			{Number: 4, Title: "Add a setting", Author: "ada", State: "OPEN", UpdatedAt: when, Draft: true},
		},
		detail: github.Detail{
			Item: github.Item{
				Number: 9, Title: "Document the cache", Author: "grace", State: "OPEN", UpdatedAt: when,
			},
			Body: "Write down how the cache works.",
			Comments: []github.Comment{
				{Author: "ada", Body: "Please do.", CreatedAt: when},
			},
		},
	}

	m := sized(t, New(src))
	m = apply(t, m, run(t, m.Init()))

	view := m.View().Content

	if !strings.Contains(view, "Fix the timer") || !strings.Contains(view, "acme/widgets") {
		t.Fatalf("list view missing issue or repo:\n%s", view)
	}

	if lipgloss.Height(view) != 24 {
		t.Fatalf("list height = %d", lipgloss.Height(view))
	}

	if !strings.Contains(lineWith(view, "Fix the timer"), "❯") {
		t.Fatal("first issue is not selected")
	}

	m = press(t, m, tea.KeyDown)
	m = press(t, m, tea.KeyDown)

	if m.issues.cursor != 1 {
		t.Fatalf("cursor = %d", m.issues.cursor)
	}

	if strings.Contains(lineWith(m.View().Content, "Fix the timer"), "❯") {
		t.Fatal("first issue stayed selected")
	}

	m = press(t, m, tea.KeyRight)

	if m.kind != github.KindPull {
		t.Fatal("right did not open pull requests")
	}

	if !strings.Contains(m.View().Content, "Add a setting") {
		t.Fatal("pull request missing")
	}

	if !strings.Contains(m.View().Content, "draft") {
		t.Fatal("draft marker missing")
	}

	m = press(t, m, tea.KeyLeft)

	if m.kind != github.KindIssue || m.issues.cursor != 1 {
		t.Fatalf("left lost the issue cursor: kind=%s cursor=%d", m.kind, m.issues.cursor)
	}

	m = press(t, m, tea.KeyEnter)

	if m.screen != screenDetail {
		t.Fatal("enter did not open the item")
	}

	detail := ansi.Strip(m.View().Content)

	if !strings.Contains(detail, "Write down how the cache works.") {
		t.Fatalf("detail missing body:\n%s", detail)
	}

	if !strings.Contains(detail, "Please do.") {
		t.Fatal("detail missing comment")
	}

	if strings.Count(detail, "╭") != 2 {
		t.Fatalf("detail frames = %d\n%s", strings.Count(detail, "╭"), detail)
	}

	if lipgloss.Width(m.View().Content) != 80 {
		t.Fatalf("detail width = %d", lipgloss.Width(m.View().Content))
	}

	if lipgloss.Height(m.View().Content) != 24 {
		t.Fatalf("detail height = %d", lipgloss.Height(m.View().Content))
	}

	m = press(t, m, tea.KeyEsc)

	if m.screen != screenList {
		t.Fatal("esc did not return to the list")
	}

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})

	if cmd == nil {
		t.Fatal("q did not quit")
	}

	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("q did not return quit")
	}
}

func TestEnterOnEmptyListDoesNothing(t *testing.T) {
	m := sized(t, New(fakeSource{name: "acme/widgets"}))
	m = apply(t, m, run(t, m.Init()))
	next := press(t, m, tea.KeyEnter)

	if next.screen != screenList {
		t.Fatal("enter opened an empty list")
	}
}

func TestListErrorIsShown(t *testing.T) {
	m := sized(t, New(fakeSource{listErr: errors.New("no git remotes found")}))
	m = apply(t, m, run(t, m.Init()))
	view := m.View().Content

	if !strings.Contains(view, "Could not load issues.") || !strings.Contains(view, "no git remotes found") {
		t.Fatalf("error view = %s", view)
	}
}

func TestStaleDetailIsIgnored(t *testing.T) {
	m := sized(t, New(fakeSource{}))
	m.screen = screenList
	m.detailReq = 2
	m = m.applyDetail(detailMsg{
		req:    1,
		detail: github.Detail{Item: github.Item{Number: 1, Title: "Old"}, Body: "stale"},
	})

	if m.detail != nil {
		t.Fatal("stale detail was applied")
	}
}

func TestRowStaysOnOneScreenLine(t *testing.T) {
	m := New(fakeSource{})
	item := github.Item{
		Number:    14874,
		Title:     strings.Repeat("Very long title ", 20),
		UpdatedAt: time.Now().Add(-2 * time.Hour),
		Labels:    []github.Label{{Name: "bug", Color: "ff5451"}, {Name: "enhancement", Color: "0dd8ac"}},
		Draft:     true,
	}
	line := m.row(item, true, 6, 80)

	if lipgloss.Width(line) != 80 {
		t.Fatalf("width = %d", lipgloss.Width(line))
	}

	if strings.Contains(line, "\n") {
		t.Fatal("row wrapped")
	}

	if !strings.Contains(line, "…") {
		t.Fatal("long title was not cut")
	}
}

func TestWindowKeepsCursorVisible(t *testing.T) {
	start, end := window(0, 10, 3)

	if start != 0 || end != 3 {
		t.Fatalf("top window = %d,%d", start, end)
	}

	start, end = window(9, 10, 3)

	if start != 7 || end != 10 {
		t.Fatalf("bottom window = %d,%d", start, end)
	}
}

func sized(t *testing.T, m Model) Model {
	t.Helper()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model, ok := next.(Model)

	if !ok {
		t.Fatalf("type %T", next)
	}

	return model
}

func press(t *testing.T, m Model, code rune) Model {
	t.Helper()
	msg := tea.KeyPressMsg{Code: code}

	if code == 'q' {
		msg.Text = "q"
	}

	next, cmd := m.Update(msg)
	model, ok := next.(Model)

	if !ok {
		t.Fatalf("type %T", next)
	}

	if cmd == nil {
		return model
	}

	return apply(t, model, []tea.Msg{cmd()})
}

func run(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()

	if cmd == nil {
		return nil
	}

	msg := cmd()

	if batch, ok := msg.(tea.BatchMsg); ok {
		var msgs []tea.Msg

		for _, item := range batch {
			msgs = append(msgs, run(t, item)...)
		}

		return msgs
	}

	return []tea.Msg{msg}
}

func apply(t *testing.T, m Model, msgs []tea.Msg) Model {
	t.Helper()

	for _, msg := range msgs {
		switch msg.(type) {
		case tea.BatchMsg:
			m = apply(t, m, run(t, func() tea.Msg { return msg }))
			continue
		case tea.QuitMsg:
			continue
		}

		next, _ := m.Update(msg)
		model, ok := next.(Model)

		if !ok {
			t.Fatalf("type %T", next)
		}

		m = model
	}

	return m
}

func lineWith(content, text string) string {
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, text) {
			return line
		}
	}

	return ""
}
