package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/jamesofficer/gith/internal/github"
)

func (m Model) detailView() string {
	colors := m.tone()
	layout := m.detailLayout()
	var post string

	switch state := m.open.state.(type) {
	case *detailLoading:
		post = m.frameBox(m.spin.View()+"  Loading…", layout.postHeight)

	case *detailFailed:
		post = m.frameBox(lipgloss.NewStyle().Foreground(colors.bad).Width(m.contentWidth()).Render(
			"Could not open this item.\n\n"+state.err.Error(),
		), layout.postHeight)

	case *detailLoaded:
		border := colors.accent

		if state.compose != nil {
			border = colors.frame
		}

		framed := m.panelStyle().
			BorderForeground(border).
			Width(m.width).
			Height(m.viewport.Height() + m.frameHeight()).
			Render(m.viewport.View())
		post = lipgloss.NewStyle().
			Width(m.width).
			Height(layout.postHeight).
			MaxHeight(layout.postHeight).
			Render(framed)
	}

	parts := []string{layout.header, post}

	if c := m.compose(); c != nil {
		parts = append(parts, m.commentBox(c))
	}

	return strings.Join(append(parts, layout.footer), "\n\n")
}

func (m Model) detailHeader() string {
	colors := m.tone()
	width := m.contentWidth()
	muted := lipgloss.NewStyle().Foreground(colors.muted)
	sep := muted.Render("  ·  ")
	loaded := m.loaded()
	item := m.open.summary

	if loaded != nil {
		item = loaded.detail.Item
	}

	crumb := muted.Render(kindTitle(m.open.key.kind)+"  /  ") +
		lipgloss.NewStyle().Bold(true).Foreground(colors.accent).Render(fmt.Sprintf("#%d", item.Number))
	title := lipgloss.NewStyle().Bold(true).Width(width).MaxHeight(3).Render(item.Title)
	meta := []string{
		lipgloss.NewStyle().Bold(true).Render(item.Author),
		stateStyle(item.State, colors).Render(strings.ToLower(item.State)),
		muted.Render("updated " + agoLong(item.UpdatedAt, time.Now())),
		muted.Render(count(item.Comments, "comment")),
	}
	lines := []string{fitLine(crumb, width), "", title, "", fitLine(strings.Join(meta, sep), width)}

	if loaded != nil && m.open.key.kind == github.KindPull {
		lines = append(lines, fitLine(m.pullLine(loaded.detail), width))
	}

	if labels := fitLabels(item.Labels, item.Draft, width, colors.muted); labels != "" {
		lines = append(lines, labels)
	}

	return strings.Join(lines, "\n")
}

func (m Model) pullLine(detail github.Detail) string {
	colors := m.tone()
	var parts []string

	if detail.Head != "" && detail.Base != "" {
		parts = append(parts, detail.Head+" → "+detail.Base)
	}

	parts = append(parts,
		lipgloss.NewStyle().Foreground(colors.good).Render(fmt.Sprintf("+%d", detail.Additions)),
		lipgloss.NewStyle().Foreground(colors.bad).Render(fmt.Sprintf("−%d", detail.Deletions)),
		count(detail.ChangedFiles, "file"),
	)

	if decision := decisionName(detail.Review); decision != "" {
		parts = append(parts, decision)
	}

	return strings.Join(parts, lipgloss.NewStyle().Foreground(colors.muted).Render("  ·  "))
}

// postContent is the shown post. Each post after the description starts
// with a line that names its author, review verdict, and age.
func (m Model) postContent(loaded *detailLoaded) string {
	text := loaded.posts[loaded.index]

	if loaded.index == 0 {
		return text
	}

	post := loaded.detail.Thread[loaded.index-1]
	muted := lipgloss.NewStyle().Foreground(m.tone().muted)
	parts := []string{lipgloss.NewStyle().Bold(true).Render(post.Author)}

	if verdict := verdictName(post.Review); verdict != "" {
		parts = append(parts, lipgloss.NewStyle().Foreground(m.tone().accent).Render(verdict))
	}

	parts = append(parts, muted.Render(agoLong(post.CreatedAt, time.Now())))

	return fitLine(strings.Join(parts, muted.Render("  ·  ")), m.contentWidth()) + "\n\n" + text
}
