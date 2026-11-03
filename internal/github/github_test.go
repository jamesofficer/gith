package github

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
)

type call struct {
	args  []string
	stdin string
}

// fakeGH answers each type of gh call with its replies in order, and
// records each call. Calls can come from more than one goroutine.
type fakeGH struct {
	mu      sync.Mutex
	replies map[string][]string
	calls   []call
}

// callType is "list", "threads", "files", or "viewed" for GraphQL calls,
// "rest" for other API calls, and the gh command, such as "pr", for all
// other calls.
func callType(args []string) string {
	if args[0] != "api" {
		return args[0]
	}

	if args[1] != "graphql" {
		return "rest"
	}

	joined := strings.Join(args, " ")

	switch {
	case strings.Contains(joined, "reviewThreads"):
		return "threads"

	case strings.Contains(joined, "viewerViewedState"):
		return "files"

	case strings.Contains(joined, "FileAsViewed"):
		return "viewed"
	}

	return "list"
}

func (f *fakeGH) run(_ context.Context, stdin []byte, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call{args: args, stdin: string(stdin)})
	kind := callType(args)
	replies := f.replies[kind]

	if len(replies) == 0 {
		return nil, fmt.Errorf("no reply for gh %v", args)
	}

	f.replies[kind] = replies[1:]

	return []byte(replies[0]), nil
}

func (f *fakeGH) callsOf(kind string) []call {
	var out []call

	for _, c := range f.calls {
		if callType(c.args) == kind {
			out = append(out, c)
		}
	}

	return out
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
	gh := &fakeGH{replies: map[string][]string{
		"repo":    {`{"name": "widgets", "owner": {"login": "acme"}, "url": "https://ghe.example.com/acme/widgets"}`},
		"list":    {page(1, 1, false)},
		"pr":      {`{"number": 1}`},
		"threads": {threads(false)},
	}}
	c := testCLI(t, "", 30, gh)

	if c.Repo().FullName() != "acme/widgets" {
		t.Fatalf("repo = %+v", c.Repo())
	}

	if _, err := c.List(context.Background(), KindIssue); err != nil {
		t.Fatal(err)
	}

	if _, err := c.Detail(context.Background(), KindPull, 1); err != nil {
		t.Fatal(err)
	}

	for _, kind := range []string{"list", "threads"} {
		if args := strings.Join(gh.callsOf(kind)[0].args, " "); !strings.Contains(args, "--hostname ghe.example.com") {
			t.Fatalf("%s call did not use the host: %s", kind, args)
		}
	}

	if args := strings.Join(gh.callsOf("pr")[0].args, " "); !strings.Contains(args, "-R ghe.example.com/acme/widgets") {
		t.Fatalf("pr call did not use the host: %s", args)
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
	gh := &fakeGH{replies: map[string][]string{"list": {page(500, 100, true), page(400, 50, true)}}}
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
	gh := &fakeGH{replies: map[string][]string{"list": {page(5, 5, false)}}}
	items, err := testCLI(t, "acme/widgets", 30, gh).List(context.Background(), KindIssue)

	if err != nil {
		t.Fatal(err)
	}

	if len(items) != 5 || len(gh.calls) != 1 {
		t.Fatalf("got %d items from %d calls", len(items), len(gh.calls))
	}
}

func TestListReadsCountsDraftsAndLabels(t *testing.T) {
	gh := &fakeGH{replies: map[string][]string{"list": {`{"data": {"repository": {"items": {
		"nodes": [{
			"number": 8, "title": "Add a setting", "state": "OPEN", "isDraft": true,
			"updatedAt": "2026-10-30T15:18:41Z", "author": null,
			"labels": {"nodes": [{"name": "bug", "color": "ff5451"}]},
			"comments": {"totalCount": 4},
			"assignees": {"nodes": [{"login": "lin"}, {"login": "sam"}]}
		}],
		"pageInfo": {"hasNextPage": false, "endCursor": null}
	}}}}`}}}
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

	if !slices.Equal(item.Assignees, []string{"lin", "sam"}) {
		t.Fatalf("assignees = %v", item.Assignees)
	}
}

func TestListOfMissingRepoFails(t *testing.T) {
	gh := &fakeGH{replies: map[string][]string{"list": {`{"data": {"repository": null}}`}}}

	if _, err := testCLI(t, "acme/gone", 30, gh).List(context.Background(), KindIssue); err == nil {
		t.Fatal("expected an error")
	}
}

func TestDetailPutsReviewsInTheThread(t *testing.T) {
	gh := &fakeGH{replies: map[string][]string{"threads": {threads(false)}, "pr": {`{
		"number": 12, "title": "Fix the timer", "state": "OPEN",
		"author": {"login": "ada"}, "body": "The last set is skipped.",
		"assignees": [{"login": "lin"}],
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
	}`}}}
	detail, err := testCLI(t, "acme/widgets", 30, gh).Detail(context.Background(), KindPull, 12)

	if err != nil {
		t.Fatal(err)
	}

	var got []string

	for _, post := range detail.Thread {
		got = append(got, post.Author+":"+post.Body+post.Review)
	}

	if want := []string{"grace:First", "lin:APPROVED", "sam:Use a constant.", "grace:Third"}; !slices.Equal(got, want) {
		t.Fatalf("thread = %v", got)
	}

	if detail.Comments != 2 || detail.Head != "fix-timer" || detail.Review != "APPROVED" || detail.ChangedFiles != 2 {
		t.Fatalf("detail = %+v", detail)
	}

	if !slices.Equal(detail.Assignees, []string{"lin"}) {
		t.Fatalf("assignees = %v", detail.Assignees)
	}
}

func TestCommentPostsTheBodyToTheItem(t *testing.T) {
	gh := &fakeGH{replies: map[string][]string{"rest": {`{"body": "Ship it.", "user": {"login": "ada"}, "created_at": "2026-10-31T03:04:05Z"}`}}}
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

// threads is one review thread page. The thread on timer.go has one reply,
// one pending reply, and one reply that was not loaded.
func threads(more bool) string {
	return fmt.Sprintf(`{"data": {"repository": {"pullRequest": {"reviewThreads": {
		"nodes": [{
			"path": "timer.go", "line": null, "originalLine": 85, "diffSide": "LEFT", "isResolved": true, "isOutdated": true,
			"comments": {"totalCount": 4, "nodes": [
				{"databaseId": 501, "author": {"login": "sam"}, "body": "Use a constant.", "createdAt": "2026-10-30T11:10:00Z", "state": "SUBMITTED", "diffHunk": "@@ -1 +1 @@\n-a\n+b"},
				{"author": {"login": "ada"}, "body": "Done.", "createdAt": "2026-10-30T13:00:00Z", "state": "SUBMITTED"},
				{"author": {"login": "sam"}, "body": "Not sent", "createdAt": "2026-10-30T14:00:00Z", "state": "PENDING"}
			]}
		}],
		"pageInfo": {"hasNextPage": %t, "endCursor": "threads-2"}
	}}}}}`, more)
}

func TestCodeCommentKeepsItsPlaceAndReplies(t *testing.T) {
	gh := &fakeGH{replies: map[string][]string{
		"pr":      {`{"number": 12, "comments": [], "reviews": []}`},
		"threads": {threads(true), threads(false)},
	}}
	detail, err := testCLI(t, "acme/widgets", 30, gh).Detail(context.Background(), KindPull, 12)

	if err != nil {
		t.Fatal(err)
	}

	if len(detail.Thread) != 2 {
		t.Fatalf("thread = %+v", detail.Thread)
	}

	post := detail.Thread[0]
	code := post.Code

	if post.Author != "sam" || post.Body != "Use a constant." || code == nil {
		t.Fatalf("post = %+v", post)
	}

	if code.ID != 501 || code.Path != "timer.go" || code.Line != 85 || code.Side != SideOld || !code.Resolved || !code.Outdated || code.Hunk != "@@ -1 +1 @@\n-a\n+b" {
		t.Fatalf("code = %+v", code)
	}

	if len(code.Replies) != 1 || code.Replies[0].Author != "ada" || code.Replies[0].Body != "Done." || code.MoreReplies != 1 {
		t.Fatalf("replies = %+v, more = %d", code.Replies, code.MoreReplies)
	}

	if after := gh.callsOf("threads")[1].value("-f", "after"); after != "threads-2" {
		t.Fatalf("second thread page after = %q", after)
	}
}

func TestIssueDetailDoesNotLoadCodeComments(t *testing.T) {
	gh := &fakeGH{replies: map[string][]string{"issue": {`{"number": 3}`}}}

	if _, err := testCLI(t, "acme/widgets", 30, gh).Detail(context.Background(), KindIssue, 3); err != nil {
		t.Fatal(err)
	}

	if len(gh.calls) != 1 {
		t.Fatalf("calls = %+v", gh.calls)
	}
}

// files is one page of the files of pull request 12.
func files(nodes string, more bool) string {
	return fmt.Sprintf(`{"data": {"repository": {"pullRequest": {"id": "PR_12", "headRefOid": "abc123", "files": {
		"nodes": [%s], "pageInfo": {"hasNextPage": %t, "endCursor": "files-2"}
	}}}}}`, nodes, more)
}

func TestDiffReadsEachFileOfThePullRequest(t *testing.T) {
	gh := &fakeGH{replies: map[string][]string{"files": {
		files(`{"path": "timer.go", "viewerViewedState": "VIEWED"}`, true),
		files(`{"path": "logo.png", "viewerViewedState": "DISMISSED"}`, false),
	}, "pr": {`diff --git a/timer.go b/timer.go
--- a/timer.go
+++ b/timer.go
@@ -1 +1 @@
-old := 1
+next := 2
diff --git a/logo.png b/logo.png
Binary files a/logo.png and b/logo.png differ
`}}}
	diff, err := testCLI(t, "acme/widgets", 30, gh).Diff(context.Background(), 12)

	if err != nil {
		t.Fatal(err)
	}

	files := diff.Files

	if diff.ID != "PR_12" || diff.Commit != "abc123" || len(files) != 2 || files[0].NewName != "timer.go" || !files[1].IsBinary {
		t.Fatalf("diff = %+v", diff)
	}

	if diff.Viewed["timer.go"] != Viewed || diff.Viewed["logo.png"] != ChangedSinceViewed {
		t.Fatalf("viewed = %v", diff.Viewed)
	}

	if after := gh.callsOf("files")[1].value("-f", "after"); after != "files-2" {
		t.Fatalf("second file page after = %q", after)
	}

	if args := strings.Join(gh.callsOf("pr")[0].args, " "); !strings.HasPrefix(args, "pr diff 12 -R acme/widgets") {
		t.Fatalf("gh %s", args)
	}

	if first, diffCall := gh.calls[0], gh.callsOf("pr")[0]; callType(first.args) != "files" || callType(diffCall.args) != "pr" {
		t.Fatal("the head commit was not read before the diff")
	}
}

func TestSetViewedMarksAndUnmarksTheFile(t *testing.T) {
	gh := &fakeGH{replies: map[string][]string{"viewed": {"{}", "{}"}}}
	cli := testCLI(t, "acme/widgets", 30, gh)

	for _, viewed := range []bool{true, false} {
		if err := cli.SetViewed(context.Background(), "PR_12", "timer.go", viewed); err != nil {
			t.Fatal(err)
		}
	}

	calls := gh.callsOf("viewed")

	for i, want := range []string{"markFileAsViewed", "unmarkFileAsViewed"} {
		query := calls[i].value("-f", "query")

		if !strings.Contains(query, " "+want+"(") || calls[i].value("-f", "id") != "PR_12" || calls[i].value("-f", "path") != "timer.go" {
			t.Fatalf("call %d: %v", i, calls[i].args)
		}
	}
}

func TestCommentOnCodeSendsTheRangeOfLines(t *testing.T) {
	reply := `{"id": 77, "body": "Merge these.", "user": {"login": "ada"}, "created_at": "2026-11-02T03:04:05Z",
		"path": "timer.go", "line": 12, "side": "RIGHT", "diff_hunk": "@@ -9 +9,4 @@"}`
	gh := &fakeGH{replies: map[string][]string{"rest": {reply, reply}}}
	cli := testCLI(t, "acme/widgets", 30, gh)
	comment := CodeComment{
		Commit: "abc123",
		Path:   "timer.go",
		Start:  DiffLine{Side: SideOld, Line: 10},
		End:    DiffLine{Side: SideNew, Line: 12},
		Body:   " Merge these.\n",
	}
	post, err := cli.CommentOnCode(context.Background(), 4, comment)

	if err != nil {
		t.Fatal(err)
	}

	if post.Author != "ada" || post.Code == nil || post.Code.ID != 77 || post.Code.Path != "timer.go" || post.Code.Line != 12 || post.Code.Side != SideNew {
		t.Fatalf("post = %+v, code = %+v", post, post.Code)
	}

	sent := gh.callsOf("rest")[0]
	want := `{"body":"Merge these.","commit_id":"abc123","line":12,"path":"timer.go","side":"RIGHT","start_line":10,"start_side":"LEFT"}`

	if sent.stdin != want || !slices.Contains(sent.args, "repos/acme/widgets/pulls/4/comments") {
		t.Fatalf("sent %s to %v", sent.stdin, sent.args)
	}

	comment.Start = comment.End

	if _, err := cli.CommentOnCode(context.Background(), 4, comment); err != nil {
		t.Fatal(err)
	}

	if single := gh.callsOf("rest")[1].stdin; strings.Contains(single, "start_line") {
		t.Fatalf("a comment on one line sent a range: %s", single)
	}
}

func TestReplyToCodeAnswersTheComment(t *testing.T) {
	gh := &fakeGH{replies: map[string][]string{"rest": {`{"id": 78, "body": "Fixed.", "user": {"login": "ada"}, "created_at": "2026-11-02T03:04:05Z"}`}}}
	reply, err := testCLI(t, "acme/widgets", 30, gh).ReplyToCode(context.Background(), 4, 501, " Fixed.\n")

	if err != nil {
		t.Fatal(err)
	}

	if reply.Author != "ada" || reply.Body != "Fixed." {
		t.Fatalf("reply = %+v", reply)
	}

	sent := gh.callsOf("rest")[0]

	if sent.stdin != `{"body":"Fixed."}` || !slices.Contains(sent.args, "repos/acme/widgets/pulls/4/comments/501/replies") {
		t.Fatalf("sent %s to %v", sent.stdin, sent.args)
	}
}
