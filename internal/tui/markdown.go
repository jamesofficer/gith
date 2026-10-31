package tui

import (
	"strings"

	"charm.land/glamour/v2"
	gansi "charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"

	"github.com/jamesofficer/gith/internal/github"
)

// renderPosts renders the description, then each post of the thread.
func renderPosts(detail github.Detail, width int, dark bool) ([]string, error) {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStyles(markdownStyle(dark, width)),
		glamour.WithWordWrap(width),
	)

	if err != nil {
		return nil, err
	}

	texts := []string{orElse(detail.Body, "_No description._")}

	for _, post := range detail.Thread {
		texts = append(texts, orElse(post.Body, "_No comment._"))
	}

	posts := make([]string, 0, len(texts))

	for _, text := range texts {
		out, err := renderer.Render(text)

		if err != nil {
			return nil, err
		}

		posts = append(posts, strings.TrimSpace(out))
	}

	return posts, nil
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
