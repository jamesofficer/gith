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
}

// Post is one comment or review in the thread of an issue or pull request.
type Post struct {
	Author    string
	Body      string
	CreatedAt time.Time

	// Review is the review state, such as APPROVED. It is empty for a comment.
	Review string
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
		args := c.apiArgs("graphql",
			"-f", "query="+query,
			"-f", "owner="+c.repo.Owner,
			"-f", "name="+c.repo.Name,
			"-F", "first="+strconv.Itoa(min(c.limit-len(items), maxPage)),
		)

		if after != "" {
			args = append(args, "-f", "after="+after)
		}

		out, err := c.run(ctx, nil, args...)

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
	issueDetailFields = "number,title,author,state,updatedAt,labels,body,comments"
	pullDetailFields  = issueDetailFields + ",isDraft,reviews,headRefName,baseRefName,reviewDecision,additions,deletions,changedFiles"
)

// Detail returns one issue or pull request with its comments and reviews.
func (c CLI) Detail(ctx context.Context, kind Kind, number int) (Detail, error) {
	var command, fields string

	switch kind {
	case KindIssue:
		command, fields = "issue", issueDetailFields

	case KindPull:
		command, fields = "pr", pullDetailFields

	default:
		return Detail{}, fmt.Errorf("unknown kind %q", kind)
	}

	out, err := c.run(ctx, nil, command, "view", strconv.Itoa(number), "-R", c.repo.flag(), "--json", fields)

	if err != nil {
		return Detail{}, err
	}

	return parseDetail(out)
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
