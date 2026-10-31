// Package github loads issues and pull requests through the GitHub CLI.
package github

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

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
	URL       string
	UpdatedAt time.Time
	Labels    []Label
	Draft     bool
}

// Comment is one comment on an issue or pull request.
type Comment struct {
	Author    string
	Body      string
	CreatedAt time.Time
}

// Detail is the full issue or pull request.
type Detail struct {
	Item
	Body         string
	Comments     []Comment
	Head         string
	Base         string
	Review       string
	Additions    int
	Deletions    int
	ChangedFiles int
}

// Source loads repository data.
type Source interface {
	Name(ctx context.Context) (string, error)
	List(ctx context.Context, kind Kind) ([]Item, error)
	Detail(ctx context.Context, kind Kind, number int) (Detail, error)
}

// CLI reads data with the gh command.
type CLI struct {
	Repo  string
	Limit int
}

const (
	issueListFields = "number,title,author,state,updatedAt,labels,url"
	pullListFields  = issueListFields + ",isDraft"

	issueDetailFields = issueListFields + ",body,comments"
	pullDetailFields  = pullListFields + ",body,comments,headRefName,baseRefName,reviewDecision,additions,deletions,changedFiles"
)

// Name returns owner/name. A set repo skips the gh call.
func (c CLI) Name(ctx context.Context) (string, error) {
	if c.Repo != "" {
		return c.Repo, nil
	}

	out, err := runGH(ctx, c.nameArgs()...)

	if err != nil {
		return "", err
	}

	return parseName(out)
}

// List returns open issues or pull requests.
func (c CLI) List(ctx context.Context, kind Kind) ([]Item, error) {
	args, err := c.listArgs(kind)

	if err != nil {
		return nil, err
	}

	out, err := runGH(ctx, args...)

	if err != nil {
		return nil, err
	}

	return parseList(out)
}

// Detail returns one issue or pull request, including its comments.
func (c CLI) Detail(ctx context.Context, kind Kind, number int) (Detail, error) {
	args, err := c.detailArgs(kind, number)

	if err != nil {
		return Detail{}, err
	}

	out, err := runGH(ctx, args...)

	if err != nil {
		return Detail{}, err
	}

	return parseDetail(out)
}

func (c CLI) limit() int {
	if c.Limit <= 0 {
		return 30
	}

	return c.Limit
}

func (c CLI) nameArgs() []string {
	return c.withRepo([]string{"repo", "view", "--json", "nameWithOwner"})
}

func (c CLI) listArgs(kind Kind) ([]string, error) {
	command, fields, err := c.fields(kind, false)

	if err != nil {
		return nil, err
	}

	args := []string{command, "list", "--limit", strconv.Itoa(c.limit()), "--json", fields}

	return c.withRepo(args), nil
}

func (c CLI) detailArgs(kind Kind, number int) ([]string, error) {
	command, fields, err := c.fields(kind, true)

	if err != nil {
		return nil, err
	}

	args := []string{command, "view", strconv.Itoa(number), "--json", fields}

	return c.withRepo(args), nil
}

func (c CLI) fields(kind Kind, detail bool) (string, string, error) {
	switch kind {
	case KindIssue:
		if detail {
			return "issue", issueDetailFields, nil
		}

		return "issue", issueListFields, nil

	case KindPull:
		if detail {
			return "pr", pullDetailFields, nil
		}

		return "pr", pullListFields, nil

	default:
		return "", "", fmt.Errorf("unknown kind %q", kind)
	}
}

func (c CLI) withRepo(args []string) []string {
	if c.Repo == "" {
		return args
	}

	out := make([]string, 0, len(args)+2)
	out = append(out, args...)
	out = append(out, "-R", c.Repo)

	return out
}

func runGH(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	out, err := cmd.Output()

	if err != nil {
		var exit *exec.ExitError

		if errors.As(err, &exit) {
			msg := strings.TrimSpace(string(exit.Stderr))

			if msg != "" {
				return nil, errors.New(msg)
			}
		}

		return nil, err
	}

	return out, nil
}
