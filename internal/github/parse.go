package github

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
)

type rawAuthor struct {
	Login string `json:"login"`
}

type rawLabel struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

func parseRepoView(data []byte) (Repo, error) {
	var raw struct {
		Name  string    `json:"name"`
		Owner rawAuthor `json:"owner"`
		URL   string    `json:"url"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return Repo{}, err
	}

	link, err := url.Parse(raw.URL)

	if err != nil || link.Host == "" || raw.Owner.Login == "" || raw.Name == "" {
		return Repo{}, errors.New("gh did not return a repository")
	}

	return Repo{Host: link.Host, Owner: raw.Owner.Login, Name: raw.Name}, nil
}

type pageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

// next is the cursor of the next page. It is empty on the last page.
func (p pageInfo) next() (string, error) {
	if !p.HasNextPage {
		return "", nil
	}

	if p.EndCursor == "" {
		return "", errors.New("gh did not return the cursor of the next page")
	}

	return p.EndCursor, nil
}

type listPage struct {
	items []Item

	// next is the cursor of the next page. It is empty on the last page.
	next string
}

type rawListItem struct {
	Number    int        `json:"number"`
	Title     string     `json:"title"`
	State     string     `json:"state"`
	UpdatedAt time.Time  `json:"updatedAt"`
	IsDraft   bool       `json:"isDraft"`
	Author    *rawAuthor `json:"author"`
	Labels    struct {
		Nodes []rawLabel `json:"nodes"`
	} `json:"labels"`
	Comments struct {
		TotalCount int `json:"totalCount"`
	} `json:"comments"`
	Assignees struct {
		Nodes []rawAuthor `json:"nodes"`
	} `json:"assignees"`
}

func parseListPage(data []byte) (listPage, error) {
	var raw struct {
		Data struct {
			Repository *struct {
				Items struct {
					Nodes    []rawListItem `json:"nodes"`
					PageInfo pageInfo      `json:"pageInfo"`
				} `json:"items"`
			} `json:"repository"`
		} `json:"data"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return listPage{}, err
	}

	repo := raw.Data.Repository

	if repo == nil {
		return listPage{}, errors.New("gh did not return the repository")
	}

	next, err := repo.Items.PageInfo.next()

	if err != nil {
		return listPage{}, err
	}

	page := listPage{items: make([]Item, 0, len(repo.Items.Nodes)), next: next}

	for _, node := range repo.Items.Nodes {
		login := ""

		if node.Author != nil {
			login = node.Author.Login
		}

		page.items = append(page.items, Item{
			Number:    node.Number,
			Title:     node.Title,
			Author:    authorName(login),
			State:     node.State,
			UpdatedAt: node.UpdatedAt,
			Labels:    labels(node.Labels.Nodes),
			Draft:     node.IsDraft,
			Comments:  node.Comments.TotalCount,
			Assignees: logins(node.Assignees.Nodes),
		})
	}

	return page, nil
}

type rawPost struct {
	Author    rawAuthor `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
}

type rawReview struct {
	Author      rawAuthor `json:"author"`
	Body        string    `json:"body"`
	State       string    `json:"state"`
	SubmittedAt time.Time `json:"submittedAt"`
}

type rawDetail struct {
	Number         int         `json:"number"`
	Title          string      `json:"title"`
	State          string      `json:"state"`
	UpdatedAt      time.Time   `json:"updatedAt"`
	Author         rawAuthor   `json:"author"`
	Assignees      []rawAuthor `json:"assignees"`
	Labels         []rawLabel  `json:"labels"`
	IsDraft        bool        `json:"isDraft"`
	Body           string      `json:"body"`
	Comments       []rawPost   `json:"comments"`
	Reviews        []rawReview `json:"reviews"`
	HeadRefName    string      `json:"headRefName"`
	BaseRefName    string      `json:"baseRefName"`
	ReviewDecision string      `json:"reviewDecision"`
	Additions      int         `json:"additions"`
	Deletions      int         `json:"deletions"`
	ChangedFiles   int         `json:"changedFiles"`
}

func parseDetail(data []byte) (Detail, error) {
	var raw rawDetail

	if err := json.Unmarshal(data, &raw); err != nil {
		return Detail{}, err
	}

	thread := make([]Post, 0, len(raw.Comments)+len(raw.Reviews))

	for _, comment := range raw.Comments {
		thread = append(thread, Post{
			Author:    authorName(comment.Author.Login),
			Body:      comment.Body,
			CreatedAt: comment.CreatedAt,
		})
	}

	for _, review := range raw.Reviews {
		if !shownReview(review) {
			continue
		}

		thread = append(thread, Post{
			Author:    authorName(review.Author.Login),
			Body:      review.Body,
			CreatedAt: review.SubmittedAt,
			Review:    review.State,
		})
	}

	sortThread(thread)

	return Detail{
		Item: Item{
			Number:    raw.Number,
			Title:     raw.Title,
			Author:    authorName(raw.Author.Login),
			State:     raw.State,
			UpdatedAt: raw.UpdatedAt,
			Labels:    labels(raw.Labels),
			Draft:     raw.IsDraft,
			Comments:  len(raw.Comments),
			Assignees: logins(raw.Assignees),
		},
		Body:         raw.Body,
		Thread:       thread,
		Head:         raw.HeadRefName,
		Base:         raw.BaseRefName,
		Review:       raw.ReviewDecision,
		Additions:    raw.Additions,
		Deletions:    raw.Deletions,
		ChangedFiles: raw.ChangedFiles,
	}, nil
}

// shownReview drops pending reviews and empty comment reviews. An empty
// comment review only holds code comments, which come from codeThreads.
func shownReview(review rawReview) bool {
	switch review.State {
	case "PENDING":
		return false

	case "APPROVED", "CHANGES_REQUESTED":
		return true

	default:
		return strings.TrimSpace(review.Body) != ""
	}
}

func parseCreatedPost(data []byte) (Post, error) {
	var raw struct {
		Body      string     `json:"body"`
		CreatedAt time.Time  `json:"created_at"`
		User      *rawAuthor `json:"user"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return Post{}, fmt.Errorf("gh did not return the comment: %w", err)
	}

	if strings.TrimSpace(raw.Body) == "" {
		return Post{}, errors.New("gh did not return the comment")
	}

	login := ""

	if raw.User != nil {
		login = raw.User.Login
	}

	return Post{Author: authorName(login), Body: raw.Body, CreatedAt: raw.CreatedAt}, nil
}

func sortThread(thread []Post) {
	slices.SortStableFunc(thread, func(a, b Post) int {
		return a.CreatedAt.Compare(b.CreatedAt)
	})
}

type rawCodeComment struct {
	Author    rawAuthor `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
	State     string    `json:"state"`
	DiffHunk  string    `json:"diffHunk"`
}

type rawThread struct {
	Path         string `json:"path"`
	Line         *int   `json:"line"`
	OriginalLine *int   `json:"originalLine"`
	IsResolved   bool   `json:"isResolved"`
	IsOutdated   bool   `json:"isOutdated"`
	Comments     struct {
		TotalCount int              `json:"totalCount"`
		Nodes      []rawCodeComment `json:"nodes"`
	} `json:"comments"`
}

// parseThreadPage returns one page of code comments and the cursor of the
// next page. Pending comments are not sent yet, so they are dropped.
func parseThreadPage(data []byte) ([]Post, string, error) {
	var raw struct {
		Data struct {
			Repository *struct {
				PullRequest *struct {
					ReviewThreads struct {
						Nodes    []rawThread `json:"nodes"`
						PageInfo pageInfo    `json:"pageInfo"`
					} `json:"reviewThreads"`
				} `json:"pullRequest"`
			} `json:"repository"`
		} `json:"data"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, "", err
	}

	if raw.Data.Repository == nil || raw.Data.Repository.PullRequest == nil {
		return nil, "", errors.New("gh did not return the pull request")
	}

	threads := raw.Data.Repository.PullRequest.ReviewThreads
	next, err := threads.PageInfo.next()

	if err != nil {
		return nil, "", err
	}

	var posts []Post

	for _, thread := range threads.Nodes {
		comments := slices.DeleteFunc(thread.Comments.Nodes, func(c rawCodeComment) bool {
			return c.State == "PENDING"
		})

		if len(comments) == 0 {
			continue
		}

		root := comments[0]
		replies := make([]Reply, 0, len(comments)-1)

		for _, reply := range comments[1:] {
			replies = append(replies, Reply{
				Author:    authorName(reply.Author.Login),
				Body:      reply.Body,
				CreatedAt: reply.CreatedAt,
			})
		}

		posts = append(posts, Post{
			Author:    authorName(root.Author.Login),
			Body:      root.Body,
			CreatedAt: root.CreatedAt,
			Code: &CodeThread{
				Path:        thread.Path,
				Line:        lineOf(thread),
				Hunk:        root.DiffHunk,
				Resolved:    thread.IsResolved,
				Outdated:    thread.IsOutdated,
				Replies:     replies,
				MoreReplies: max(0, thread.Comments.TotalCount-len(thread.Comments.Nodes)),
			},
		})
	}

	return posts, next, nil
}

// lineOf is the line of the comment. An outdated comment uses the line
// in the old diff. The line is 0 when GitHub does not know it.
func lineOf(thread rawThread) int {
	if thread.Line != nil {
		return *thread.Line
	}

	if thread.OriginalLine != nil {
		return *thread.OriginalLine
	}

	return 0
}

func logins(raw []rawAuthor) []string {
	out := make([]string, 0, len(raw))

	for _, author := range raw {
		out = append(out, author.Login)
	}

	return out
}

func labels(raw []rawLabel) []Label {
	out := make([]Label, 0, len(raw))

	for _, label := range raw {
		out = append(out, Label(label))
	}

	return out
}

func authorName(login string) string {
	if login == "" {
		return "unknown"
	}

	return login
}
