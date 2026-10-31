package github

import (
	"testing"
	"time"
)

func TestParseListAndDetail(t *testing.T) {
	listJSON := []byte(`[
		{
			"number": 12,
			"title": "Fix the timer",
			"state": "OPEN",
			"url": "https://github.com/acme/widgets/issues/12",
			"updatedAt": "2026-10-30T15:18:41Z",
			"author": {"login": "ada"},
			"labels": [{"name": "bug", "color": "ff5451"}],
			"isDraft": false
		}
	]`)

	items, err := parseList(listJSON)

	if err != nil {
		t.Fatal(err)
	}

	if len(items) != 1 {
		t.Fatalf("got %d items", len(items))
	}

	item := items[0]

	if item.Number != 12 || item.Title != "Fix the timer" || item.Author != "ada" {
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
	if _, err := parseList([]byte(`not json`)); err == nil {
		t.Fatal("expected an error")
	}

	if _, err := parseDetail([]byte(`[]`)); err == nil {
		t.Fatal("expected an error")
	}
}

func TestListArgs(t *testing.T) {
	c := CLI{Repo: "acme/widgets", Limit: 15}
	args, err := c.listArgs(KindIssue)

	if err != nil {
		t.Fatal(err)
	}

	want := []string{"issue", "list", "--limit", "15", "--json", issueListFields, "-R", "acme/widgets"}

	if !same(args, want) {
		t.Fatalf("args = %#v", args)
	}

	args, err = c.listArgs(KindPull)

	if err != nil {
		t.Fatal(err)
	}

	if args[0] != "pr" || args[len(args)-1] != "acme/widgets" {
		t.Fatalf("args = %#v", args)
	}

	plain := CLI{}
	args, err = plain.listArgs(KindIssue)

	if err != nil {
		t.Fatal(err)
	}

	if len(args) != 6 {
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
