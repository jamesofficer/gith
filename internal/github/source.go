// Package github loads issues and pull requests through the GitHub CLI.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
	"golang.org/x/sync/errgroup"
)

// DefaultLimit is the number of open items to load when no limit is set.
const DefaultLimit = 30

// maxPage is the largest page that the GitHub API returns.
const maxPage = 100

// Kind selects issues or pull requests.
type Kind string

const (
	KindIssue Kind = "issue"
	KindPull  Kind = "pull"
)

// Label is a GitHub label name and its hex color, without a leading #.
type Label struct {
	Name  string
	Color string
}

// Item is one row in an issue or pull request list.
type Item struct {
	Number    int
	Title     string
	Author    string
	State     string
	UpdatedAt time.Time
	Labels    []Label
	Draft     bool
	Comments  int
	Assignees []string
}

// Post is one comment or review in the thread of an issue or pull request.
type Post struct {
	Author    string
	Body      string
	CreatedAt time.Time

	// Review is the review state, such as APPROVED. It is empty for a comment.
	Review string

	// Code is set when the post is a comment on a line of a pull request.
	Code *CodeThread
}

// CodeThread is the place of a code comment in the diff and its replies.
type CodeThread struct {
	Path string
	Line int

	// Hunk is the part of the diff that the comment is on.
	Hunk     string
	Resolved bool
	Outdated bool
	Replies  []Reply

	// MoreReplies is the number of replies that gith did not load.
	MoreReplies int
}

// Reply is an answer to a code comment.
type Reply struct {
	Author    string
	Body      string
	CreatedAt time.Time
}

// Detail is the full issue or pull request.
type Detail struct {
	Item
	Body string

	// Thread holds the comments and reviews, oldest first.
	Thread       []Post
	Head         string
	Base         string
	Review       string
	Additions    int
	Deletions    int
	ChangedFiles int
}

// Source loads repository data, posts comments, and opens items in a browser.
type Source interface {
	List(ctx context.Context, kind Kind) ([]Item, error)
	Detail(ctx context.Context, kind Kind, number int) (Detail, error)
	Diff(ctx context.Context, number int) ([]*gitdiff.File, error)
	Comment(ctx context.Context, number int, body string) (Post, error)
	Browse(ctx context.Context, number int) error
}

type runner func(ctx context.Context, stdin []byte, args ...string) ([]byte, error)

// CLI reads and writes data with the gh command.
type CLI struct {
	repo  Repo
	limit int
	run   runner
}

// NewCLI makes a CLI. An empty repo means the repository in this directory.
func NewCLI(ctx context.Context, repo string, limit int) (CLI, error) {
	return newCLI(ctx, repo, limit, runGH)
}

func newCLI(ctx context.Context, repo string, limit int, run runner) (CLI, error) {
	if limit < 1 {
		return CLI{}, fmt.Errorf("limit must be 1 or more, not %d", limit)
	}

	resolved, err := resolveRepo(ctx, repo, run)

	if err != nil {
		return CLI{}, err
	}

	return CLI{repo: resolved, limit: limit, run: run}, nil
}

func resolveRepo(ctx context.Context, repo string, run runner) (Repo, error) {
	if repo != "" {
		return ParseRepo(repo)
	}

	out, err := run(ctx, nil, "repo", "view", "--json", "owner,name,url")

	if err != nil {
		return Repo{}, fmt.Errorf("could not find a GitHub repository in this directory. Use -R OWNER/NAME.\n%w", err)
	}

	return parseRepoView(out)
}

// Repo is the repository that the CLI reads.
func (c CLI) Repo() Repo {
	return c.repo
}

const listQuery = `query($owner: String!, $name: String!, $first: Int!, $after: String) {
  repository(owner: $owner, name: $name) {
    items: %s(states: OPEN, first: $first, after: $after, orderBy: {field: CREATED_AT, direction: DESC}) {
      nodes {
        number title state updatedAt %s
        author { login }
        labels(first: 20) { nodes { name color } }
        comments { totalCount }
        assignees(first: 10) { nodes { login } }
      }
      pageInfo { hasNextPage endCursor }
    }
  }
}`

var (
	issueListQuery = fmt.Sprintf(listQuery, "issues", "")
	pullListQuery  = fmt.Sprintf(listQuery, "pullRequests", "isDraft")
)

// List returns open issues or pull requests, newest first.
func (c CLI) List(ctx context.Context, kind Kind) ([]Item, error) {
	query, err := listQueryFor(kind)

	if err != nil {
		return nil, err
	}

	var items []Item
	after := ""

	for len(items) < c.limit {
		out, err := c.graphql(ctx, query, after, "-F", "first="+strconv.Itoa(min(c.limit-len(items), maxPage)))

		if err != nil {
			return nil, err
		}

		page, err := parseListPage(out)

		if err != nil {
			return nil, err
		}

		items = append(items, page.items...)

		if page.next == "" {
			break
		}

		after = page.next
	}

	return items, nil
}

// graphql runs a query with the owner and name of the repository. An empty
// after means the first page.
func (c CLI) graphql(ctx context.Context, query, after string, fields ...string) ([]byte, error) {
	args := []string{"graphql", "-f", "query=" + query, "-f", "owner=" + c.repo.Owner, "-f", "name=" + c.repo.Name}

	if after != "" {
		args = append(args, "-f", "after="+after)
	}

	return c.run(ctx, nil, c.apiArgs(append(args, fields...)...)...)
}

func listQueryFor(kind Kind) (string, error) {
	switch kind {
	case KindIssue:
		return issueListQuery, nil

	case KindPull:
		return pullListQuery, nil

	default:
		return "", fmt.Errorf("unknown kind %q", kind)
	}
}

const (
	issueDetailFields = "number,title,author,assignees,state,updatedAt,labels,body,comments"
	pullDetailFields  = issueDetailFields + ",isDraft,reviews,headRefName,baseRefName,reviewDecision,additions,deletions,changedFiles"
)

// Detail returns one issue or pull request with its comments and reviews.
// For a pull request, it also returns the comments on lines of code.
func (c CLI) Detail(ctx context.Context, kind Kind, number int) (Detail, error) {
	switch kind {
	case KindIssue:
		return c.view(ctx, "issue", issueDetailFields, number)

	case KindPull:
		return c.pullDetail(ctx, number)

	default:
		return Detail{}, fmt.Errorf("unknown kind %q", kind)
	}
}

func (c CLI) view(ctx context.Context, command, fields string, number int) (Detail, error) {
	out, err := c.run(ctx, nil, command, "view", strconv.Itoa(number), "-R", c.repo.flag(), "--json", fields)

	if err != nil {
		return Detail{}, err
	}

	return parseDetail(out)
}

func (c CLI) pullDetail(ctx context.Context, number int) (Detail, error) {
	var detail Detail
	var code []Post
	group, ctx := errgroup.WithContext(ctx)

	group.Go(func() (err error) {
		detail, err = c.view(ctx, "pr", pullDetailFields, number)

		return err
	})

	group.Go(func() (err error) {
		code, err = c.codeThreads(ctx, number)

		return err
	})

	if err := group.Wait(); err != nil {
		return Detail{}, err
	}

	detail.Thread = append(detail.Thread, code...)
	sortThread(detail.Thread)

	return detail, nil
}

const threadQuery = `query($owner: String!, $name: String!, $number: Int!, $after: String) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      reviewThreads(first: 50, after: $after) {
        nodes {
          path line originalLine isResolved isOutdated
          comments(first: 100) {
            totalCount
            nodes { author { login } body createdAt state diffHunk }
          }
        }
        pageInfo { hasNextPage endCursor }
      }
    }
  }
}`

// codeThreads returns each comment on a line of code, with its replies.
func (c CLI) codeThreads(ctx context.Context, number int) ([]Post, error) {
	var posts []Post
	after := ""

	for {
		out, err := c.graphql(ctx, threadQuery, after, "-F", "number="+strconv.Itoa(number))

		if err != nil {
			return nil, err
		}

		page, next, err := parseThreadPage(out)

		if err != nil {
			return nil, err
		}

		posts = append(posts, page...)

		if next == "" {
			return posts, nil
		}

		after = next
	}
}

// Comment posts a comment on an issue or a pull request.
func (c CLI) Comment(ctx context.Context, number int, body string) (Post, error) {
	body = strings.TrimSpace(body)

	if body == "" {
		return Post{}, errors.New("comment is empty")
	}

	payload, err := json.Marshal(struct {
		Body string `json:"body"`
	}{Body: body})

	if err != nil {
		return Post{}, err
	}

	path := fmt.Sprintf("repos/%s/issues/%d/comments", c.repo.FullName(), number)
	args := c.apiArgs("--method", "POST", "--input", "-", "-H", "Content-Type:application/json", path)
	out, err := c.run(ctx, payload, args...)

	if err != nil {
		return Post{}, err
	}

	return parseCreatedPost(out)
}

// Diff returns the changed files of a pull request, in the order of the diff.
func (c CLI) Diff(ctx context.Context, number int) ([]*gitdiff.File, error) {
	out, err := c.run(ctx, nil, "pr", "diff", strconv.Itoa(number), "-R", c.repo.flag(), "--color", "never")

	if err != nil {
		return nil, err
	}

	files, _, err := gitdiff.Parse(bytes.NewReader(out))

	if err != nil {
		return nil, fmt.Errorf("could not read the diff: %w", err)
	}

	return files, nil
}

// Browse opens an issue or a pull request in the web browser.
func (c CLI) Browse(ctx context.Context, number int) error {
	_, err := c.run(ctx, nil, "browse", strconv.Itoa(number), "-R", c.repo.flag())

	return err
}

func (c CLI) apiArgs(args ...string) []string {
	out := append([]string{"api"}, args...)

	if c.repo.Host != "" {
		out = append(out, "--hostname", c.repo.Host)
	}

	return out
}

func runGH(ctx context.Context, stdin []byte, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Env = append(cmd.Environ(), "GH_PROMPT_DISABLED=1")

	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}

	out, err := cmd.Output()

	if err == nil {
		return out, nil
	}

	if ctx.Err() != nil {
		return nil, fmt.Errorf("gh did not answer in time: %w", ctx.Err())
	}

	var exit *exec.ExitError

	if errors.As(err, &exit) {
		if msg := strings.TrimSpace(string(exit.Stderr)); msg != "" {
			return nil, errors.New(msg)
		}
	}

	return nil, err
}
