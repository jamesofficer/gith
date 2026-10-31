package tui

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	gansi "charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
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
	}
}

// View draws the list or the open item.
func (m Model) View() tea.View {
	content := "Loading…"

	if m.width > 0 && m.height > 0 {
		if m.screen == screenDetail {
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
	bodyH := m.bodyHeight(header, footer)
	body := lipgloss.NewStyle().
		Width(m.width).
		Height(bodyH).
		MaxHeight(bodyH).
		Render(m.listBody(bodyH))

	return header + "\n" + body + "\n" + footer
}

func (m Model) detailView() string {
	header := m.framedHeader()
	footer := m.footer()
	body := m.frameBox(m.detailBody(), m.detailBodyBoxHeight())

	return header + "\n\n" + body + "\n\n" + footer
}

func (m Model) panelStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.tone().frame).
		Padding(0, 1)
}

func (m Model) frameBox(content string, height int) string {
	style := m.panelStyle().Width(m.width)

	if height > 0 {
		style = style.Height(height)
	}

	return style.Render(content)
}

func (m Model) framedHeader() string {
	return m.frameBox(m.detailHeader(), 0)
}

func (m Model) detailContentWidth() int {
	width := m.width - m.panelStyle().GetHorizontalFrameSize()

	if width < 1 {
		return 1
	}

	return width
}

func (m Model) detailBodyBoxHeight() int {
	frameY := m.panelStyle().GetVerticalFrameSize()
	used := lipgloss.Height(m.framedHeader()) + 2 + lipgloss.Height(m.footer())
	height := m.height - used

	if height < frameY+1 {
		return frameY + 1
	}

	return height
}

func (m Model) detailContentHeight() int {
	inner := m.detailBodyBoxHeight() - m.panelStyle().GetVerticalFrameSize()

	if inner < 1 {
		return 1
	}

	return inner
}

func (m Model) bodyHeight(header, footer string) int {
	h := m.height - lipgloss.Height(header) - lipgloss.Height(footer)

	if h < 1 {
		return 1
	}

	return h
}

func (m Model) listHeader() string {
	colors := m.tone()
	brand := lipgloss.NewStyle().Bold(true).Foreground(colors.accent).Render("gith")
	var repo string

	if m.name != "" {
		repo = lipgloss.NewStyle().Foreground(colors.muted).Render(m.name)
	}

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
	active := m.active()
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

	for i := start; i < end; i++ {
		lines = append(lines, m.row(active.items[i], i == active.cursor, numW, m.width))
	}

	return strings.Join(lines, "\n")
}

func (m Model) row(item github.Item, selected bool, numW, width int) string {
	colors := m.tone()

	if width <= 0 {
		return ""
	}

	bar := "  "

	if selected {
		bar = lipgloss.NewStyle().Foreground(colors.accent).Render("❯ ")
	}

	num := padRight(lipgloss.NewStyle().Foreground(colors.accent).Bold(selected).Render(
		fmt.Sprintf("#%d", item.Number),
	), numW)
	age := padLeft(github.Ago(item.UpdatedAt, time.Now()), 4)
	fixed := lipgloss.Width(bar) + numW + 2 + 2 + lipgloss.Width(age)
	middle := width - fixed

	if middle < 8 {
		plain := bar + num + "  " + item.Title + "  " + age

		return ansi.Truncate(plain, width, "…")
	}

	labels := fitLabels(item.Labels, item.Draft, middle/2, colors.muted)
	labelW := lipgloss.Width(labels)
	gap := 0

	if labelW > 0 {
		gap = 2
	}

	titleW := middle - labelW - gap

	if titleW < 8 {
		labels = ""
		labelW = 0
		gap = 0
		titleW = middle
	}

	title := padRight(ansi.Truncate(item.Title, titleW, "…"), titleW)

	if selected {
		title = lipgloss.NewStyle().Bold(true).Render(title)
	}

	line := bar + num + "  " + title

	if labelW > 0 {
		line += "  " + labels
	}

	line += "  " + lipgloss.NewStyle().Foreground(colors.muted).Render(age)

	if lipgloss.Width(line) > width {
		return ansi.Truncate(line, width, "")
	}

	return padRight(line, width)
}

func (m Model) detailHeader() string {
	colors := m.tone()
	width := m.detailContentWidth()
	item := m.shown()
	crumbLeft := lipgloss.NewStyle().Foreground(colors.muted).Render(kindTitle(m.kind))
	crumbSep := lipgloss.NewStyle().Foreground(colors.muted).Render("  /  ")
	crumbNum := lipgloss.NewStyle().Bold(true).Foreground(colors.accent).Render(fmt.Sprintf("#%d", item.Number))
	crumb := crumbLeft + crumbSep + crumbNum
	title := lipgloss.NewStyle().Bold(true).Width(width).MaxHeight(3).Render(item.Title)

	var metaParts []string
	metaParts = append(metaParts, lipgloss.NewStyle().Bold(true).Render(item.Author))
	metaParts = append(metaParts, stateStyle(item.State, colors).Render(strings.ToLower(item.State)))
	metaParts = append(metaParts, lipgloss.NewStyle().Foreground(colors.muted).Render(
		"updated "+github.AgoLong(item.UpdatedAt, time.Now()),
	))

	if m.detail != nil {
		metaParts = append(metaParts, lipgloss.NewStyle().Foreground(colors.muted).Render(
			commentCount(len(m.detail.Comments)),
		))
	}

	meta := strings.Join(metaParts, lipgloss.NewStyle().Foreground(colors.muted).Render("  ·  "))
	lines := []string{
		fitLine(crumb, width),
		"",
		title,
		"",
		fitLine(meta, width),
	}

	if m.detail != nil && m.kind == github.KindPull {
		lines = append(lines, fitLine(m.pullLine(*m.detail), width))
	}

	if len(item.Labels) > 0 || item.Draft {
		labels := fitLabels(item.Labels, item.Draft, width, colors.muted)

		if labels != "" {
			lines = append(lines, labels)
		}
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
		fileCount(detail.ChangedFiles),
	)

	if review := reviewName(detail.Review); review != "" {
		parts = append(parts, review)
	}

	return strings.Join(parts, lipgloss.NewStyle().Foreground(colors.muted).Render("  ·  "))
}

func (m Model) detailBody() string {
	colors := m.tone()

	if m.detailLoading {
		return m.spin.View() + "  Loading…"
	}

	if m.detailErr != nil {
		return lipgloss.NewStyle().Foreground(colors.bad).Width(m.width).Render(
			"Could not open this item.\n\n" + m.detailErr.Error(),
		)
	}

	return m.viewport.View()
}

func (m Model) footer() string {
	colors := m.tone()
	hint := "↑↓ move    ←→ switch    enter open    q quit"
	right := ""

	if m.screen == screenDetail {
		hint = "↑↓ scroll    esc back    q quit"

		if m.detail != nil && !(m.viewport.AtTop() && m.viewport.AtBottom()) {
			right = fmt.Sprintf("%d%%", int(m.viewport.ScrollPercent()*100))
		}
	} else {
		active := m.active()

		if !active.loading && active.err == nil && len(active.items) > 0 {
			right = fmt.Sprintf("%d of %d", active.cursor+1, len(active.items))
		}
	}

	dim := lipgloss.NewStyle().Foreground(colors.muted)
	hintLine := spread(dim.Render(hint), dim.Render(right), m.width)

	if m.screen == screenDetail {
		return hintLine
	}

	ruleLine := lipgloss.NewStyle().Foreground(colors.rule).Render(strings.Repeat("─", max(m.width, 0)))

	return ruleLine + "\n" + hintLine
}

func kindTitle(kind github.Kind) string {
	if kind == github.KindPull {
		return "Pull requests"
	}

	return "Issues"
}

func kindName(kind github.Kind) string {
	if kind == github.KindPull {
		return "pull requests"
	}

	return "issues"
}

func stateStyle(state string, colors tone) lipgloss.Style {
	switch strings.ToUpper(state) {
	case "OPEN":
		return lipgloss.NewStyle().Foreground(colors.good)
	case "MERGED":
		return lipgloss.NewStyle().Foreground(colors.accent)
	default:
		return lipgloss.NewStyle().Foreground(colors.muted)
	}
}

func reviewName(raw string) string {
	switch raw {
	case "APPROVED":
		return "approved"
	case "CHANGES_REQUESTED":
		return "changes requested"
	case "REVIEW_REQUIRED":
		return "review required"
	default:
		return ""
	}
}

func commentCount(n int) string {
	if n == 1 {
		return "1 comment"
	}

	return fmt.Sprintf("%d comments", n)
}

func fileCount(n int) string {
	if n == 1 {
		return "1 file"
	}

	return fmt.Sprintf("%d files", n)
}

func numberWidth(items []github.Item) int {
	width := 2

	for _, item := range items {
		n := len(fmt.Sprintf("#%d", item.Number))

		if n > width {
			width = n
		}
	}

	return width
}

func fitLabels(labels []github.Label, draft bool, maxWidth int, fade color.Color) string {
	if maxWidth <= 0 {
		return ""
	}

	var chips []string

	if draft {
		chips = append(chips, chip("draft", "6B7280"))
	}

	for _, label := range labels {
		chips = append(chips, chip(label.Name, label.Color))
	}

	if len(chips) == 0 {
		return ""
	}

	more := lipgloss.NewStyle().Foreground(fade)

	for n := len(chips); n >= 0; n-- {
		parts := append([]string{}, chips[:n]...)
		hidden := len(chips) - n

		if hidden > 0 {
			parts = append(parts, more.Render(fmt.Sprintf("+%d", hidden)))
		}

		if len(parts) == 0 {
			continue
		}

		line := strings.Join(parts, " ")

		if lipgloss.Width(line) <= maxWidth {
			return line
		}
	}

	return ""
}

func chip(text, color string) string {
	hex := normalizeHex(color)

	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(textOn(hex))).
		Background(lipgloss.Color("#"+hex)).
		Padding(0, 1).
		Render(text)
}

func normalizeHex(color string) string {
	color = strings.TrimPrefix(color, "#")

	if len(color) == 3 {
		color = string([]byte{color[0], color[0], color[1], color[1], color[2], color[2]})
	}

	if len(color) != 6 {
		return "6B7280"
	}

	return color
}

func textOn(hex string) string {
	r := hexValue(hex[0:2])
	g := hexValue(hex[2:4])
	b := hexValue(hex[4:6])
	lum := (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) / 255

	if lum > 0.62 {
		return "#1C1917"
	}

	return "#FAFAF9"
}

func hexValue(pair string) int64 {
	var n int64

	for _, r := range pair {
		n *= 16

		switch {
		case r >= '0' && r <= '9':
			n += int64(r - '0')
		case r >= 'a' && r <= 'f':
			n += int64(r-'a') + 10
		case r >= 'A' && r <= 'F':
			n += int64(r-'A') + 10
		}
	}

	return n
}

func detailMarkdown(detail github.Detail, now time.Time) string {
	body := strings.TrimSpace(detail.Body)

	if body == "" {
		body = "_No description._"
	}

	var b strings.Builder
	b.WriteString(body)

	if len(detail.Comments) == 0 {
		return b.String()
	}

	b.WriteString("\n\n---\n\n")

	for i, comment := range detail.Comments {
		if i > 0 {
			b.WriteString("\n\n")
		}

		fmt.Fprintf(
			&b,
			"**%s** · %s\n\n%s\n",
			comment.Author,
			github.AgoLong(comment.CreatedAt, now),
			strings.TrimSpace(comment.Body),
		)
	}

	return b.String()
}

func renderMarkdown(text string, width int, dark bool) string {
	if width < 1 {
		width = 1
	}

	renderer, err := glamour.NewTermRenderer(
		glamour.WithStyles(markdownStyle(dark, width)),
		glamour.WithWordWrap(width),
	)

	if err != nil {
		return text
	}

	out, err := renderer.Render(text)

	if err != nil {
		return text
	}

	return strings.TrimSpace(out)
}

func markdownStyle(dark bool, width int) gansi.StyleConfig {
	cfg := styles.LightStyleConfig

	if dark {
		cfg = styles.TokyoNightStyleConfig
	}

	margin := uint(0)
	cfg.Document.Margin = &margin
	cfg.CodeBlock.Margin = &margin

	if width < 1 {
		width = 1
	}

	cfg.HorizontalRule.Format = "\n" + strings.Repeat("─", width) + "\n"
	dropHashPrefix(&cfg.H1)
	dropHashPrefix(&cfg.H2)
	dropHashPrefix(&cfg.H3)
	dropHashPrefix(&cfg.H4)
	dropHashPrefix(&cfg.H5)
	dropHashPrefix(&cfg.H6)

	if cfg.Code.BackgroundColor == nil {
		bg := "#24283b"
		pad := "\u00a0"
		cfg.Code.BackgroundColor = &bg
		cfg.Code.Prefix = pad
		cfg.Code.Suffix = pad
	}

	return cfg
}

func dropHashPrefix(block *gansi.StyleBlock) {
	if strings.HasPrefix(block.Prefix, "#") {
		block.Prefix = ""
	}
}

func spread(left, right string, width int) string {
	if width <= 0 {
		return ""
	}

	rightW := lipgloss.Width(right)

	if rightW == 0 {
		return fitLine(left, width)
	}

	leftW := lipgloss.Width(left)

	if leftW+rightW+1 > width {
		return fitLine(left, width)
	}

	return left + strings.Repeat(" ", width-leftW-rightW) + right
}

func fitLine(text string, width int) string {
	if width <= 0 {
		return ""
	}

	if lipgloss.Width(text) <= width {
		return padRight(text, width)
	}

	return ansi.Truncate(text, width, "…")
}

func padRight(text string, width int) string {
	gap := width - lipgloss.Width(text)

	if gap <= 0 {
		return text
	}

	return text + strings.Repeat(" ", gap)
}

func padLeft(text string, width int) string {
	gap := width - lipgloss.Width(text)

	if gap <= 0 {
		return text
	}

	return strings.Repeat(" ", gap) + text
}
