package github

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type rawAuthor struct {
	Login string `json:"login"`
}

type rawLabel struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

type rawComment struct {
	Author    rawAuthor `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
}

type rawItem struct {
	Number         int          `json:"number"`
	Title          string       `json:"title"`
	State          string       `json:"state"`
	URL            string       `json:"url"`
	UpdatedAt      time.Time    `json:"updatedAt"`
	Author         rawAuthor    `json:"author"`
	Labels         []rawLabel   `json:"labels"`
	IsDraft        bool         `json:"isDraft"`
	Body           string       `json:"body"`
	Comments       []rawComment `json:"comments"`
	HeadRefName    string       `json:"headRefName"`
	BaseRefName    string       `json:"baseRefName"`
	ReviewDecision string       `json:"reviewDecision"`
	Additions      int          `json:"additions"`
	Deletions      int          `json:"deletions"`
	ChangedFiles   int          `json:"changedFiles"`
}

func parseName(data []byte) (string, error) {
	var raw struct {
		NameWithOwner string `json:"nameWithOwner"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return "", err
	}

	if raw.NameWithOwner == "" {
		return "", errors.New("gh did not return a repository name")
	}

	return raw.NameWithOwner, nil
}

func parseList(data []byte) ([]Item, error) {
	var raw []rawItem

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	items := make([]Item, 0, len(raw))

	for _, item := range raw {
		items = append(items, item.item())
	}

	return items, nil
}

func parseDetail(data []byte) (Detail, error) {
	var raw rawItem

	if err := json.Unmarshal(data, &raw); err != nil {
		return Detail{}, err
	}

	return raw.detail(), nil
}

func (r rawItem) item() Item {
	labels := make([]Label, 0, len(r.Labels))

	for _, label := range r.Labels {
		labels = append(labels, Label{Name: label.Name, Color: label.Color})
	}

	return Item{
		Number:    r.Number,
		Title:     r.Title,
		Author:    authorName(r.Author.Login),
		State:     r.State,
		URL:       r.URL,
		UpdatedAt: r.UpdatedAt,
		Labels:    labels,
		Draft:     r.IsDraft,
	}
}

func (r rawItem) detail() Detail {
	comments := make([]Comment, 0, len(r.Comments))

	for _, comment := range r.Comments {
		comments = append(comments, Comment{
			Author:    authorName(comment.Author.Login),
			Body:      comment.Body,
			CreatedAt: comment.CreatedAt,
		})
	}

	return Detail{
		Item:         r.item(),
		Body:         r.Body,
		Comments:     comments,
		Head:         r.HeadRefName,
		Base:         r.BaseRefName,
		Review:       r.ReviewDecision,
		Additions:    r.Additions,
		Deletions:    r.Deletions,
		ChangedFiles: r.ChangedFiles,
	}
}

func authorName(login string) string {
	if login == "" {
		return "unknown"
	}

	return login
}

// Ago is a short age such as now, 5m, 3h, 2d, 4mo, or 1y.
func Ago(t, now time.Time) string {
	if t.IsZero() {
		return ""
	}

	d := now.Sub(t)

	if d < time.Minute {
		return "now"
	}

	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}

	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}

	if d < 30*24*time.Hour {
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}

	if d < 365*24*time.Hour {
		return fmt.Sprintf("%dmo", int(d.Hours()/(24*30)))
	}

	return fmt.Sprintf("%dy", int(d.Hours()/(24*365)))
}

// AgoLong is Ago plus "ago", for a detail header.
func AgoLong(t, now time.Time) string {
	short := Ago(t, now)

	if short == "" || short == "now" {
		return "just now"
	}

	return short + " ago"
}
