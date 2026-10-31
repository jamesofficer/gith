package github

import (
	"strings"
	"testing"
	"time"
)

func TestParseListAndDetail(t *testing.T) {
	listJSON := []byte(`[
		{
			"number": 12,
			"title": "Fix the timer",
			"state": "open",
			"html_url": "https://github.com/acme/widgets/issues/12",
			"updated_at": "2026-10-30T15:18:41Z",
			"user": {"login": "ada"},
			"labels": [{"name": "bug", "color": "ff5451"}],
			"draft": null,
			"comments": 4
		},
		{
			"number": 8,
			"title": "A pull request",
			"state": "open",
			"html_url": "https://github.com/acme/widgets/pull/8",
			"updated_at": "2026-10-30T15:18:41Z",
			"user": {"login": "grace"},
			"labels": [],
			"draft": true,
			"comments": 1,
			"pull_request": {"url": "https://api.github.com/repos/acme/widgets/pulls/8"}
		}
	]`)

	items, rawCount, err := parseList(listJSON, KindIssue)

	if err != nil {
		t.Fatal(err)
	}

	if len(items) != 1 || rawCount != 2 {
		t.Fatalf("got %d items from %d rows", len(items), rawCount)
	}

	item := items[0]

	if item.Number != 12 || item.Title != "Fix the timer" || item.Author != "ada" || item.Comments != 4 {
		t.Fatalf("item = %+v", item)
	}

	if item.State != "OPEN" || len(item.Labels) != 1 || item.Labels[0].Name != "bug" {
		t.Fatalf("item = %+v", item)
	}

	detailJSON := []byte(`{
		"number": 12,
		"title": "Fix the timer",
		"state": "OPEN",
		"author": {"login": ""},
		"updatedAt": "2026-10-30T15:18:41Z",
		"labels": [],
		"body": "The last set is skipped.",
		"isDraft": true,
		"headRefName": "fix-timer",
		"baseRefName": "main",
		"reviewDecision": "APPROVED",
		"additions": 4,
		"deletions": 1,
		"changedFiles": 2,
		"comments": [
			{"author": {"login": "grace"}, "body": "I can reproduce this.", "createdAt": "2026-10-30T16:00:00Z"}
		]
	}`)

	detail, err := parseDetail(detailJSON)

	if err != nil {
		t.Fatal(err)
	}

	if detail.Author != "unknown" || detail.Body != "The last set is skipped." {
		t.Fatalf("detail = %+v", detail)
	}

	if !detail.Draft || detail.Head != "fix-timer" || detail.Base != "main" {
		t.Fatalf("detail = %+v", detail)
	}

	if detail.Review != "APPROVED" || detail.Additions != 4 || detail.ChangedFiles != 2 {
		t.Fatalf("detail = %+v", detail)
	}

	if len(detail.Comments) != 1 || detail.Comments[0].Author != "grace" {
		t.Fatalf("comments = %+v", detail.Comments)
	}
}

func TestParseRejectsBadJSON(t *testing.T) {
	if _, _, err := parseList([]byte(`not json`), KindIssue); err == nil {
		t.Fatal("expected an error")
	}

	if _, err := parseDetail([]byte(`[]`)); err == nil {
		t.Fatal("expected an error")
	}
}

func TestListArgs(t *testing.T) {
	c := CLI{Repo: "acme/widgets", Limit: 15}
	args := c.listArgs(KindIssue, 1, 15)
	want := []string{"api", "repos/acme/widgets/issues?state=open&per_page=15&page=1"}

	if !same(args, want) {
		t.Fatalf("args = %#v", args)
	}

	args = c.listArgs(KindPull, 1, 15)

	if args[0] != "api" || !strings.Contains(args[1], "acme/widgets/pulls") {
		t.Fatalf("args = %#v", args)
	}

	plain := CLI{}
	args = plain.listArgs(KindIssue, 1, 30)

	if len(args) != 2 || !strings.Contains(args[1], "repos/{owner}/{repo}/issues") {
		t.Fatalf("args = %#v", args)
	}
}

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
		if got := Ago(tc.at, now); got != tc.want {
			t.Errorf("Ago(%s) = %q, want %q", tc.at, got, tc.want)
		}
	}

	if got := AgoLong(now.Add(-2*time.Hour), now); got != "2h ago" {
		t.Fatalf("AgoLong = %q", got)
	}
}

func same(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}
