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
}

func parseListPage(data []byte) (listPage, error) {
	var raw struct {
		Data struct {
			Repository *struct {
				Items struct {
					Nodes    []rawListItem `json:"nodes"`
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
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

	info := repo.Items.PageInfo

	if info.HasNextPage && info.EndCursor == "" {
		return listPage{}, errors.New("gh did not return the cursor of the next page")
	}

	page := listPage{items: make([]Item, 0, len(repo.Items.Nodes))}

	if info.HasNextPage {
		page.next = info.EndCursor
	}

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

	slices.SortStableFunc(thread, func(a, b Post) int {
		return a.CreatedAt.Compare(b.CreatedAt)
	})

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
// comment review only holds code comments, which gith does not show.
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
