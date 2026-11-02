package tui

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jamesofficer/gith/internal/github"
)

type tone struct {
	accent color.Color
	muted  color.Color
	rule   color.Color
	frame  color.Color
	good   color.Color
	bad    color.Color

	// added and removed are the backgrounds of changed lines in a diff.
	// selected is the background of the lines that a comment is for.
	added    color.Color
	removed  color.Color
	selected color.Color
}

func (m Model) tone() tone {
	pick := lipgloss.LightDark(m.dark)

	return tone{
		accent: pick(lipgloss.Color("#6D28D9"), lipgloss.Color("#C4B5FD")),
		muted:  pick(lipgloss.Color("#6B7280"), lipgloss.Color("#A1A1AA")),
		rule:   pick(lipgloss.Color("#D4D4D8"), lipgloss.Color("#3F3F46")),
		frame:  pick(lipgloss.Color("#A1A1AA"), lipgloss.Color("#71717A")),
		good:   pick(lipgloss.Color("#15803D"), lipgloss.Color("#4ADE80")),
		bad:    pick(lipgloss.Color("#B91C1C"), lipgloss.Color("#F87171")),

		added:    pick(lipgloss.Color("#E6FFEC"), lipgloss.Color("#1B3326")),
		removed:  pick(lipgloss.Color("#FFEBE9"), lipgloss.Color("#3B1D22")),
		selected: pick(lipgloss.Color("#EDE9FE"), lipgloss.Color("#342A5E")),
	}
}

// View draws the list or the open item.
func (m Model) View() tea.View {
	content := "Loading…"

	if m.width > 0 && m.height > 0 {
		if m.open != nil {
			content = m.detailView()
		} else {
			content = m.listView()
		}
	}

	view := tea.NewView(content)
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion

	return view
}

func (m Model) listView() string {
	header := m.listHeader()
	footer := m.footer()
	bodyH := max(1, m.height-lipgloss.Height(header)-lipgloss.Height(footer))
	body := lipgloss.NewStyle().
		Width(m.width).
		Height(bodyH).
		MaxHeight(bodyH).
		Render(m.listBody(bodyH))

	return header + "\n" + body + "\n" + footer
}

func (m Model) listHeader() string {
	colors := m.tone()
	brand := lipgloss.NewStyle().Bold(true).Foreground(colors.accent).Render("gith")
	repo := lipgloss.NewStyle().Foreground(colors.muted).Render(m.repo)
	issues := m.tab("Issues", m.issues, m.kind == github.KindIssue)
	pulls := m.tab("Pull requests", m.pulls, m.kind == github.KindPull)
	tabs := lipgloss.JoinHorizontal(lipgloss.Bottom, issues.label, "    ", pulls.label)
	under := lipgloss.JoinHorizontal(lipgloss.Bottom, issues.under, "    ", pulls.under)

	return fitLine(spread(brand, repo, m.width), m.width) + "\n\n" +
		fitLine(tabs, m.width) + "\n" +
		fitLine(under, m.width)
}

type tab struct {
	label string
	under string
}

func (m Model) tab(name string, items list, active bool) tab {
	colors := m.tone()
	text := name

	if !items.loading && items.err == nil {
		text = fmt.Sprintf("%s  %d", name, len(items.items))
	}

	width := lipgloss.Width(text)
	under := strings.Repeat(" ", width)
	style := lipgloss.NewStyle().Foreground(colors.muted)

	if active {
		style = lipgloss.NewStyle().Bold(true).Foreground(colors.accent)
		under = lipgloss.NewStyle().Foreground(colors.accent).Render(strings.Repeat("━", width))
	}

	return tab{label: style.Render(text), under: under}
}

func (m Model) listBody(height int) string {
	colors := m.tone()
	active := m.listOf(m.kind)
	name := kindName(m.kind)

	if active.loading {
		return m.spin.View() + "  Loading " + name
	}

	if active.err != nil {
		return lipgloss.NewStyle().Foreground(colors.bad).Width(m.width).Render(
			"Could not load " + name + ".\n\n" + active.err.Error(),
		)
	}

	if len(active.items) == 0 {
		return lipgloss.NewStyle().Foreground(colors.muted).Render("No open " + name + ".")
	}

	start, end := window(active.cursor, len(active.items), height)
	numW := numberWidth(active.items)
	lines := make([]string, 0, end-start)
	now := time.Now()

	for i := start; i < end; i++ {
		lines = append(lines, m.row(active.items[i], i == active.cursor, numW, m.width, now))
	}

	return strings.Join(lines, "\n")
}

func (m Model) row(item github.Item, selected bool, numW, width int, now time.Time) string {
	colors := m.tone()
	muted := lipgloss.NewStyle().Foreground(colors.muted)
	bar := "  "

	if selected {
		bar = lipgloss.NewStyle().Foreground(colors.accent).Render("❯ ")
	}

	num := padRight(lipgloss.NewStyle().Foreground(colors.accent).Bold(selected).Render(
		fmt.Sprintf("#%d", item.Number),
	), numW)
	var tail []string

	for _, part := range []string{m.assigneeBadge(item.Assignees), commentChip(item.Comments)} {
		if part != "" {
			tail = append(tail, part)
		}
	}

	tail = append(tail, muted.Render(padLeft(ago(item.UpdatedAt, now), 4)))
	end := strings.Join(tail, "  ")
	middle := width - lipgloss.Width(bar+num+"  "+"  "+end)

	if middle < 8 {
		return fitLine(bar+num+"  "+item.Title+"  "+end, width)
	}

	labels := fitLabels(item.Labels, item.Draft, middle/2, colors.muted)
	titleW := middle - lipgloss.Width(labels) - 2

	if labels == "" || titleW < 8 {
		labels = ""
		titleW = middle
	}

	title := padRight(ansi.Truncate(item.Title, titleW, "…"), titleW)

	if selected {
		title = lipgloss.NewStyle().Bold(true).Render(title)
	}

	line := bar + num + "  " + title

	if labels != "" {
		line += "  " + labels
	}

	return fitLine(line+"  "+end, width)
}

// assigneeBadge names the first assignee and counts the others.
func (m Model) assigneeBadge(assignees []string) string {
	if len(assignees) == 0 {
		return ""
	}

	badge := "@" + ansi.Truncate(assignees[0], 12, "…")

	if len(assignees) > 1 {
		badge += fmt.Sprintf(" +%d", len(assignees)-1)
	}

	return lipgloss.NewStyle().Foreground(m.tone().accent).Render(badge)
}

func numberWidth(items []github.Item) int {
	width := 2

	for _, item := range items {
		width = max(width, len(fmt.Sprintf("#%d", item.Number)))
	}

	return width
}

func (m Model) footer() string {
	if c := m.compose(); c != nil {
		return m.composeFooter(c)
	}

	colors := m.tone()
	dim := lipgloss.NewStyle().Foreground(colors.muted)
	hint, right := m.hints()
	line := spread(dim.Render(hint), dim.Render(right), m.width)

	if m.problem != "" {
		line = fitLine(lipgloss.NewStyle().Foreground(colors.bad).Render(m.problem), m.width) + "\n" + line
	}

	if m.open != nil {
		return line
	}

	rule := lipgloss.NewStyle().Foreground(colors.rule).Render(strings.Repeat("─", max(m.width, 0)))

	return rule + "\n" + line
}

func (m Model) hints() (string, string) {
	if m.open == nil {
		active := m.listOf(m.kind)
		right := ""

		if !active.loading && active.err == nil && len(active.items) > 0 {
			right = fmt.Sprintf("%d of %d", active.cursor+1, len(active.items))
		}

		return "↑↓ move   ←→ switch   enter open   b browser   r refresh   q quit", right
	}

	switch state := m.open.state.(type) {
	case *detailLoaded:
		switch diff := state.diff.(type) {
		case *diffLoaded:
			if page := diff.page(); diff.selecting && page != nil {
				first, last := min(diff.anchor, diff.cursor), max(diff.anchor, diff.cursor)
				lines := linesName(github.CodeComment{Start: page.lines[first].spot, End: page.lines[last].spot})

				return "↑↓ jk pick lines   c comment on " + lines + "   esc cancel", ""
			}

			hint := "↑↓ jk line   "

			if threads := codeThreads(state.detail.Thread); len(threads) > 0 {
				if spot, ok := diff.cursorSpot(); ok && len(threads[spot]) > 0 {
					hint += "enter comments   "
				}

				if diff.collapsed {
					hint += "e expand   "
				} else {
					hint += "e collapse   "
				}
			}

			return hint + "c select   ←→ file   esc back", ""

		case *diffFailed:
			return "r retry   b browser   esc back", ""

		case *diffLoading:
			return "esc back", ""
		}

		if m.open.key.kind == github.KindPull {
			return "↑↓ thread   d diff   c comment   b browser   r refresh   esc back", ""
		}

		return "↑↓ thread   c comment   b browser   r refresh   esc back", ""

	case *detailFailed:
		return "r retry   esc back", ""

	default:
		return "esc back", ""
	}
}
