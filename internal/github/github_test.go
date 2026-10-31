package github

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
)

type call struct {
	args  []string
	stdin string
}

// fakeGH answers gh calls with replies in order and records each call.
type fakeGH struct {
	replies []string
	calls   []call
}

func (f *fakeGH) run(_ context.Context, stdin []byte, args ...string) ([]byte, error) {
	f.calls = append(f.calls, call{args: args, stdin: string(stdin)})

	if len(f.replies) == 0 {
		return nil, fmt.Errorf("no reply for gh %v", args)
	}

	reply := f.replies[0]
	f.replies = f.replies[1:]

	return []byte(reply), nil
}

func (c call) value(flag, key string) string {
	for i := 0; i+1 < len(c.args); i++ {
		if c.args[i] == flag && strings.HasPrefix(c.args[i+1], key+"=") {
			return strings.TrimPrefix(c.args[i+1], key+"=")
		}
	}

	return ""
}

func testCLI(t *testing.T, repo string, limit int, gh *fakeGH) CLI {
	t.Helper()
	c, err := newCLI(context.Background(), repo, limit, gh.run)

	if err != nil {
		t.Fatal(err)
	}

	return c
}

func TestParseRepo(t *testing.T) {
	cases := []struct {
		in   string
		want Repo
		ok   bool
	}{
		{"acme/widgets", Repo{Owner: "acme", Name: "widgets"}, true},
		{"ghe.example.com/acme/widgets", Repo{Host: "ghe.example.com", Owner: "acme", Name: "widgets"}, true},
		{"widgets", Repo{}, false},
		{"acme/", Repo{}, false},
		{"a/b/c/d", Repo{}, false},
	}

	for _, tc := range cases {
		got, err := ParseRepo(tc.in)

		if (err == nil) != tc.ok || got != tc.want {
			t.Errorf("ParseRepo(%q) = %+v, %v", tc.in, got, err)
		}
	}
}

func TestNewCLIRejectsBadLimit(t *testing.T) {
	for _, limit := range []int{0, -5} {
		if _, err := newCLI(context.Background(), "acme/widgets", limit, (&fakeGH{}).run); err == nil {
			t.Errorf("limit %d was accepted", limit)
		}
	}
}

func TestEnterpriseRepoFromDirectoryUsesItsHost(t *testing.T) {
	gh := &fakeGH{replies: []string{
		`{"name": "widgets", "owner": {"login": "acme"}, "url": "https://ghe.example.com/acme/widgets"}`,
		page(1, 1, false),
		`{"number": 1}`,
	}}
	c := testCLI(t, "", 30, gh)

	if c.Repo().FullName() != "acme/widgets" {
		t.Fatalf("repo = %+v", c.Repo())
	}

	if _, err := c.List(context.Background(), KindIssue); err != nil {
		t.Fatal(err)
	}

	if _, err := c.Detail(context.Background(), KindIssue, 1); err != nil {
		t.Fatal(err)
	}

	list, detail := strings.Join(gh.calls[1].args, " "), strings.Join(gh.calls[2].args, " ")

	if !strings.Contains(list, "--hostname ghe.example.com") || !strings.Contains(detail, "-R ghe.example.com/acme/widgets") {
		t.Fatalf("calls did not use the host:\n%s\n%s", list, detail)
	}
}

// page is one GraphQL list page with numbers from first down to first-count+1.
func page(first, count int, more bool) string {
	nodes := make([]string, 0, count)

	for n := first; n > first-count; n-- {
		nodes = append(nodes, fmt.Sprintf(`{"number": %d, "title": "Item %d"}`, n, n))
	}

	return fmt.Sprintf(
		`{"data": {"repository": {"items": {"nodes": [%s], "pageInfo": {"hasNextPage": %t, "endCursor": "after-%d"}}}}}`,
		strings.Join(nodes, ","), more, first-count,
	)
}

func TestListPagesWithoutRepeats(t *testing.T) {
	gh := &fakeGH{replies: []string{page(500, 100, true), page(400, 50, true)}}
	items, err := testCLI(t, "acme/widgets", 150, gh).List(context.Background(), KindPull)

	if err != nil {
		t.Fatal(err)
	}

	seen := map[int]bool{}

	for _, item := range items {
		if seen[item.Number] {
			t.Fatalf("item %d is in the list two times", item.Number)
		}

		seen[item.Number] = true
	}

	if len(items) != 150 {
		t.Fatalf("got %d items", len(items))
	}

	if gh.calls[1].value("-f", "after") != "after-400" || gh.calls[1].value("-F", "first") != "50" {
		t.Fatalf("second page call = %v", gh.calls[1].args)
	}
}

func TestListStopsAtTheLastPage(t *testing.T) {
	gh := &fakeGH{replies: []string{page(5, 5, false)}}
	items, err := testCLI(t, "acme/widgets", 30, gh).List(context.Background(), KindIssue)

	if err != nil {
		t.Fatal(err)
	}

	if len(items) != 5 || len(gh.calls) != 1 {
		t.Fatalf("got %d items from %d calls", len(items), len(gh.calls))
	}
}

func TestListReadsCountsDraftsAndLabels(t *testing.T) {
	gh := &fakeGH{replies: []string{`{"data": {"repository": {"items": {
		"nodes": [{
			"number": 8, "title": "Add a setting", "state": "OPEN", "isDraft": true,
			"updatedAt": "2026-10-30T15:18:41Z", "author": null,
			"labels": {"nodes": [{"name": "bug", "color": "ff5451"}]},
			"comments": {"totalCount": 4}
		}],
		"pageInfo": {"hasNextPage": false, "endCursor": null}
	}}}}`}}
	items, err := testCLI(t, "acme/widgets", 30, gh).List(context.Background(), KindPull)

	if err != nil {
		t.Fatal(err)
	}

	item := items[0]

	if item.Comments != 4 || !item.Draft || item.Author != "unknown" || item.UpdatedAt.IsZero() {
		t.Fatalf("item = %+v", item)
	}

	if len(item.Labels) != 1 || item.Labels[0] != (Label{Name: "bug", Color: "ff5451"}) {
		t.Fatalf("labels = %+v", item.Labels)
	}
}

func TestListOfMissingRepoFails(t *testing.T) {
	gh := &fakeGH{replies: []string{`{"data": {"repository": null}}`}}

	if _, err := testCLI(t, "acme/gone", 30, gh).List(context.Background(), KindIssue); err == nil {
		t.Fatal("expected an error")
	}
}

func TestDetailPutsReviewsInTheThread(t *testing.T) {
	gh := &fakeGH{replies: []string{`{
		"number": 12, "title": "Fix the timer", "state": "OPEN",
		"author": {"login": "ada"}, "body": "The last set is skipped.",
		"headRefName": "fix-timer", "baseRefName": "main", "reviewDecision": "APPROVED",
		"additions": 4, "deletions": 1, "changedFiles": 2,
		"comments": [
			{"author": {"login": "grace"}, "body": "First", "createdAt": "2026-10-30T10:00:00Z"},
			{"author": {"login": "grace"}, "body": "Third", "createdAt": "2026-10-30T12:00:00Z"}
		],
		"reviews": [
			{"author": {"login": "lin"}, "body": "", "state": "APPROVED", "submittedAt": "2026-10-30T11:00:00Z"},
			{"author": {"login": "lin"}, "body": "", "state": "COMMENTED", "submittedAt": "2026-10-30T11:30:00Z"},
			{"author": {"login": "lin"}, "body": "Draft", "state": "PENDING"}
		]
	}`}}
	detail, err := testCLI(t, "acme/widgets", 30, gh).Detail(context.Background(), KindPull, 12)

	if err != nil {
		t.Fatal(err)
	}

	var got []string

	for _, post := range detail.Thread {
		got = append(got, post.Author+":"+post.Body+post.Review)
	}

	if want := []string{"grace:First", "lin:APPROVED", "grace:Third"}; !slices.Equal(got, want) {
		t.Fatalf("thread = %v", got)
	}

	if detail.Comments != 2 || detail.Head != "fix-timer" || detail.Review != "APPROVED" || detail.ChangedFiles != 2 {
		t.Fatalf("detail = %+v", detail)
	}
}

func TestCommentPostsTheBodyToTheItem(t *testing.T) {
	gh := &fakeGH{replies: []string{`{"body": "Ship it.", "user": {"login": "ada"}, "created_at": "2026-10-31T03:04:05Z"}`}}
	post, err := testCLI(t, "acme/widgets", 30, gh).Comment(context.Background(), 9, "  Ship it.\n")

	if err != nil {
		t.Fatal(err)
	}

	if post.Author != "ada" || post.Body != "Ship it." || post.CreatedAt.IsZero() {
		t.Fatalf("post = %+v", post)
	}

	sent := gh.calls[0]

	if sent.stdin != `{"body":"Ship it."}` || !slices.Contains(sent.args, "repos/acme/widgets/issues/9/comments") {
		t.Fatalf("call = %+v", sent)
	}
}

func TestCommentRejectsEmpty(t *testing.T) {
	gh := &fakeGH{}

	if _, err := testCLI(t, "acme/widgets", 30, gh).Comment(context.Background(), 1, "  \n"); err == nil {
		t.Fatal("expected an error")
	}

	if len(gh.calls) != 0 {
		t.Fatal("an empty comment was sent")
	}
}
