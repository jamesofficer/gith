package tui

import (
	"fmt"
	"image/color"
	"slices"
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
		switch diff := state.diff.(type) {
		case nil:
			border := colors.accent

			if state.compose != nil {
				border = colors.frame
			}

			position := fmt.Sprintf("%d of %d", state.index+1, len(state.posts))
			post = m.viewportPanel(position, border, layout.postHeight)

		case *diffLoading:
			post = m.frameBox(m.spin.View()+"  Loading the diff…", layout.postHeight)

		case *diffFailed:
			post = m.frameBox(lipgloss.NewStyle().Foreground(colors.bad).Width(m.contentWidth()).Render(
				"Could not load the diff. Press o to see it on GitHub.\n\n"+diff.err.Error(),
			), layout.postHeight)

		case *diffLoaded:
			post = m.viewportPanel(diffTitle(diff), colors.accent, layout.postHeight)
		}
	}

	parts := []string{layout.header, post}

	if c := m.compose(); c != nil {
		parts = append(parts, m.commentBox(c))
	}

	return strings.Join(append(parts, layout.footer), "\n\n")
}

// viewportPanel frames the viewport, with title on the top border, and
// fills the given height.
func (m Model) viewportPanel(title string, border color.Color, height int) string {
	framed := m.panelStyle().
		BorderForeground(border).
		Width(m.width).
		Height(m.viewport.Height() + m.frameHeight()).
		Render(m.viewport.View())

	return lipgloss.NewStyle().
		Width(m.width).
		Height(height).
		MaxHeight(height).
		Render(withTitle(framed, title, border))
}

// withTitle puts title at the left end of the top border of a framed box.
// It keeps the plain border when the box is too narrow for the title.
func withTitle(framed, title string, border color.Color) string {
	edges := lipgloss.RoundedBorder()
	top, rest, _ := strings.Cut(framed, "\n")
	label := " " + title + " "
	fill := lipgloss.Width(top) - lipgloss.Width(label) - 3

	if fill < 1 {
		return framed
	}

	line := lipgloss.NewStyle().Foreground(border)
	top = line.Render(edges.TopLeft+edges.Top) +
		line.Bold(true).Render(label) +
		line.Render(strings.Repeat(edges.Top, fill)+edges.TopRight)

	return top + "\n" + rest
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

	if len(item.Assignees) > 0 {
		meta = append(meta, muted.Render("assigned to ")+
			lipgloss.NewStyle().Foreground(colors.accent).Render("@"+strings.Join(item.Assignees, ", @")))
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
// with a byline. A code comment also shows its place in the diff, and its
// replies go under it with an indent.
func (m Model) postContent(loaded *detailLoaded) string {
	rendered := loaded.posts[loaded.index]

	if loaded.index == 0 {
		return rendered.body
	}

	post := loaded.detail.Thread[loaded.index-1]
	width := m.contentWidth()
	text := m.byline(post.Author, verdictName(post.Review), post.CreatedAt, width) + "\n\n" + rendered.body

	if post.Code == nil {
		return text
	}

	sections := []string{m.codePlace(*post.Code, width), text}

	for i, reply := range post.Code.Replies {
		byline := m.byline(reply.Author, "", reply.CreatedAt, width-replyIndent)
		sections = append(sections, m.indent(byline+"\n\n"+rendered.replies[i]))
	}

	if more := post.Code.MoreReplies; more > 0 {
		note := fmt.Sprintf("%d more replies on GitHub. Press o to read them.", more)

		if more == 1 {
			note = "1 more reply on GitHub. Press o to read it."
		}

		sections = append(sections, m.indent(lipgloss.NewStyle().Foreground(m.tone().muted).Render(note)))
	}

	return strings.Join(sections, "\n\n")
}

// byline names the author of a post, the review verdict if any, and the age.
func (m Model) byline(author, verdict string, at time.Time, width int) string {
	muted := lipgloss.NewStyle().Foreground(m.tone().muted)
	parts := []string{lipgloss.NewStyle().Bold(true).Render(author)}

	if verdict != "" {
		parts = append(parts, lipgloss.NewStyle().Foreground(m.tone().accent).Render(verdict))
	}

	parts = append(parts, muted.Render(agoLong(at, time.Now())))

	return fitLine(strings.Join(parts, muted.Render("  ·  ")), width)
}

// hunkLines is the number of diff lines shown above a code comment.
const hunkLines = 4

// codePlace is the file and line of a code comment, then the last lines of
// the diff above the comment.
func (m Model) codePlace(code github.CodeThread, width int) string {
	colors := m.tone()
	muted := lipgloss.NewStyle().Foreground(colors.muted)
	place := code.Path

	if code.Line > 0 {
		place += fmt.Sprintf(":%d", code.Line)
	}

	parts := []string{lipgloss.NewStyle().Bold(true).Render(place)}

	if code.Outdated {
		parts = append(parts, muted.Render("outdated"))
	}

	if code.Resolved {
		parts = append(parts, muted.Render("resolved"))
	}

	lines := []string{fitLine(strings.Join(parts, muted.Render("  ·  ")), width)}
	hunk := slices.DeleteFunc(strings.Split(code.Hunk, "\n"), func(line string) bool {
		return strings.HasPrefix(line, "@@")
	})

	for _, line := range hunk[max(0, len(hunk)-hunkLines):] {
		style := muted

		switch {
		case strings.HasPrefix(line, "+"):
			style = lipgloss.NewStyle().Foreground(colors.good)

		case strings.HasPrefix(line, "-"):
			style = lipgloss.NewStyle().Foreground(colors.bad)
		}

		lines = append(lines, fitLine(style.Render(strings.ReplaceAll(line, "\t", "    ")), width))
	}

	return strings.Join(lines, "\n")
}

// replyIndent is the width of the bar and the space in front of a reply.
const replyIndent = 4

func (m Model) indent(text string) string {
	bar := lipgloss.NewStyle().Foreground(m.tone().rule).Render("  │ ")
	lines := strings.Split(text, "\n")

	for i, line := range lines {
		lines[i] = bar + line
	}

	return strings.Join(lines, "\n")
}
