package tui

import (
	"strings"

	"charm.land/glamour/v2"
	gansi "charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"

	"github.com/jamesofficer/gith/internal/github"
)

// renderedPost is the markdown of one post, rendered for the terminal.
type renderedPost struct {
	body string

	// replies holds the replies to a code comment, rendered narrower to fit
	// the indent.
	replies []string
}

// renderPosts renders the description, then each post of the thread.
func renderPosts(detail github.Detail, width int, dark bool) ([]renderedPost, error) {
	post, err := markdownRenderer(width, dark)

	if err != nil {
		return nil, err
	}

	reply, err := markdownRenderer(max(1, width-replyIndent), dark)

	if err != nil {
		return nil, err
	}

	description, err := render(post, detail.Body, "_No description._")

	if err != nil {
		return nil, err
	}

	posts := []renderedPost{{body: description}}

	for _, p := range detail.Thread {
		rendered, err := renderPost(p, post, reply)

		if err != nil {
			return nil, err
		}

		posts = append(posts, rendered)
	}

	return posts, nil
}

// renderCodeThreads renders each code comment of the thread, by its index in
// the thread, for the inline view in the diff.
func renderCodeThreads(thread []github.Post, width int, dark bool) (map[int]renderedPost, error) {
	post, err := markdownRenderer(width, dark)

	if err != nil {
		return nil, err
	}

	reply, err := markdownRenderer(max(1, width-replyIndent), dark)

	if err != nil {
		return nil, err
	}

	threads := map[int]renderedPost{}

	for i, p := range thread {
		if p.Code == nil {
			continue
		}

		if threads[i], err = renderPost(p, post, reply); err != nil {
			return nil, err
		}
	}

	return threads, nil
}

// renderPost renders the body of a post and the replies to a code comment.
func renderPost(p github.Post, post, reply *glamour.TermRenderer) (renderedPost, error) {
	body, err := render(post, p.Body, "_No comment._")

	if err != nil {
		return renderedPost{}, err
	}

	rendered := renderedPost{body: body}

	if p.Code == nil {
		return rendered, nil
	}

	for _, r := range p.Code.Replies {
		text, err := render(reply, r.Body, "_No comment._")

		if err != nil {
			return renderedPost{}, err
		}

		rendered.replies = append(rendered.replies, text)
	}

	return rendered, nil
}

func markdownRenderer(width int, dark bool) (*glamour.TermRenderer, error) {
	return glamour.NewTermRenderer(
		glamour.WithStyles(markdownStyle(dark, width)),
		glamour.WithWordWrap(width),
	)
}

func render(renderer *glamour.TermRenderer, text, empty string) (string, error) {
	out, err := renderer.Render(orElse(text, empty))

	if err != nil {
		return "", err
	}

	return strings.TrimSpace(out), nil
}

func orElse(text, fallback string) string {
	if strings.TrimSpace(text) == "" {
		return fallback
	}

	return text
}

func markdownStyle(dark bool, width int) gansi.StyleConfig {
	cfg := styles.LightStyleConfig

	if dark {
		cfg = styles.TokyoNightStyleConfig
	}

	margin := uint(0)
	cfg.Document.Margin = &margin
	cfg.CodeBlock.Margin = &margin
	cfg.HorizontalRule.Format = "\n" + strings.Repeat("─", width) + "\n"

	for _, heading := range []*gansi.StyleBlock{&cfg.H1, &cfg.H2, &cfg.H3, &cfg.H4, &cfg.H5, &cfg.H6} {
		if strings.HasPrefix(heading.Prefix, "#") {
			heading.Prefix = ""
		}
	}

	if cfg.Code.BackgroundColor == nil {
		bg := "#24283b"
		pad := " "
		cfg.Code.BackgroundColor = &bg
		cfg.Code.Prefix = pad
		cfg.Code.Suffix = pad
	}

	return cfg
}
