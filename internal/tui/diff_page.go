package tui

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/bluekeyes/go-gitdiff/gitdiff"
	"github.com/charmbracelet/x/ansi"

	"github.com/jamesofficer/gith/internal/github"
)

// filePage is one file of the diff, rendered for the screen. The gutter is
// not in it, because the cursor and the marks in the gutter change often.
type filePage struct {
	path string

	// head holds the title of the file and any note above the code.
	head        string
	lines       []pageLine
	numberWidth int
	codeWidth   int
}

// pageLine is a hunk header or one line of code. A long line of code wraps
// onto more than one row. op and tokens let a selected line render again.
type pageLine struct {
	rows []string

	// code is false for a hunk header. hunk is the index of the hunk.
	code   bool
	hunk   int
	spot   github.DiffLine
	op     gitdiff.LineOp
	tokens []chroma.Token
}

// codeSpot is the place of a code comment that can get a mark.
type codeSpot struct {
	path string
	spot github.DiffLine
}

// gutterSpace is the width of the selection bar, the space after the line
// number, and the comment mark.
const gutterSpace = 4

// diffContent is the shown file with the gutter, and the first and last row
// of the cursor line. It renders a file only when it is first shown.
func (m Model) diffContent(loaded *detailLoaded, diff *diffLoaded, width int) (string, int, int) {
	if len(diff.diff.Files) == 0 {
		return lipgloss.NewStyle().Foreground(m.tone().muted).Render("This pull request changes no files."), 0, 0
	}

	if diff.pages == nil || diff.pagesW != width || diff.pagesDark != m.dark {
		diff.pages = make([]*filePage, len(diff.diff.Files))
		diff.pagesW, diff.pagesDark = width, m.dark
		diff.threads = nil
	}

	page := m.pageAt(diff, diff.index)

	if diff.cursor < 0 {
		diff.cursor = moveCursor(page.lines, -1, 1, -1)
	}

	threads := codeThreads(loaded.detail.Thread)
	colors := m.tone()
	bar := lipgloss.NewStyle().Foreground(colors.accent).Render("▌")
	number := lipgloss.NewStyle().Foreground(colors.muted)
	current := lipgloss.NewStyle().Foreground(colors.accent).Bold(true)
	first, last := diff.cursor, diff.cursor

	if diff.selecting {
		first, last = min(diff.anchor, diff.cursor), max(diff.anchor, diff.cursor)
	}

	rows := strings.Split(page.head, "\n")
	top, bottom := 0, 0

	for i, line := range page.lines {
		if !line.code {
			rows = append(rows, line.rows...)

			continue
		}

		lead, style, code := " ", number, line.rows

		if i >= first && i <= last {
			lead = bar
		}

		if diff.selecting && i >= first && i <= last {
			code = m.diffLine(line.op, line.tokens, page.codeWidth, shadeSelected)
		}

		if i == diff.cursor {
			style = current
			top, bottom = len(rows), len(rows)+len(line.rows)-1
		}

		mark := "  "
		here := threads[codeSpot{path: page.path, spot: line.spot}]

		if len(here) > 0 {
			mark = "💬"
		}

		gutter := lead + style.Render(padLeft(strconv.Itoa(line.spot.Line), page.numberWidth)) + " " + mark
		blank := lead + strings.Repeat(" ", page.numberWidth+1) + "  "

		for j, row := range code {
			if j > 0 {
				gutter = blank
			}

			rows = append(rows, gutter+row)
		}

		if len(here) > 0 {
			open := diff.threadsOpen(codeSpot{path: page.path, spot: line.spot})
			rows = append(rows, m.inlineThreads(loaded, diff, here, open, page.numberWidth, width)...)
		}
	}

	return strings.Join(rows, "\n"), top, bottom
}

// pageAt is file i of the diff. It renders the file when it is first needed.
func (m Model) pageAt(diff *diffLoaded, i int) *filePage {
	if diff.pages[i] == nil {
		file := diff.diff.Files[i]
		diff.pages[i] = m.renderFile(file, diff.diff.Viewed[fileName(file)], diff.pagesW)
	}

	return diff.pages[i]
}

// inlineThreads is the code comments under a line, at the column of the +
// and - marks. A closed comment is one row with the start of its text. An
// open comment shows in full in a frame, with its replies.
func (m Model) inlineThreads(loaded *detailLoaded, diff *diffLoaded, indexes []int, open bool, numberWidth, width int) []string {
	muted := lipgloss.NewStyle().Foreground(m.tone().muted)
	indent := strings.Repeat(" ", numberWidth+4)
	frame := m.panelStyle().BorderForeground(m.tone().frame).Width(max(1, width-len(indent)))
	textWidth := max(1, width-len(indent)-frame.GetHorizontalFrameSize()-2)
	var rows []string

	for _, i := range indexes {
		post := loaded.detail.Thread[i]

		if !open {
			rows = append(rows, indent+fitLine(m.threadPreview(post), width-len(indent)))

			continue
		}

		rendered, err := diff.renderedThread(loaded.detail.Thread, i, textWidth, m.dark)

		if err != nil {
			rows = append(rows, indent+lipgloss.NewStyle().Foreground(m.tone().bad).Render("Could not show this comment. "+err.Error()))

			continue
		}

		lines := strings.Split(m.postWithReplies(post, rendered, textWidth), "\n")

		for j, line := range lines {
			lead := "  "

			if j == 0 {
				lead = muted.Render("▾ ")
			}

			lines[j] = lead + line
		}

		for _, line := range strings.Split(frame.Render(strings.Join(lines, "\n")), "\n") {
			rows = append(rows, indent+line)
		}
	}

	return rows
}

// threadPreview is one row for a closed code comment: the author, the
// first line of the text, and the number of replies.
func (m Model) threadPreview(post github.Post) string {
	muted := lipgloss.NewStyle().Foreground(m.tone().muted)
	text := ""

	for _, line := range strings.Split(post.Body, "\n") {
		if text = strings.TrimSpace(line); text != "" {
			break
		}
	}

	preview := muted.Render("▸ ") + lipgloss.NewStyle().Bold(true).Render(post.Author) + "  " + text

	replies := len(post.Code.Replies) + post.Code.MoreReplies

	if replies == 1 {
		preview += muted.Render("  ·  1 reply")
	} else if replies > 1 {
		preview += muted.Render(fmt.Sprintf("  ·  %d replies", replies))
	}

	return preview
}

// renderedThread is the markdown of one code comment. It renders all code
// comments again when the width changes or a new comment is not rendered.
func (d *diffLoaded) renderedThread(thread []github.Post, index, width int, dark bool) (renderedPost, error) {
	if rendered, ok := d.threads[index]; ok && d.threadsW == width {
		return rendered, nil
	}

	threads, err := renderCodeThreads(thread, width, dark)

	if err != nil {
		return renderedPost{}, err
	}

	d.threads, d.threadsW = threads, width

	return threads[index], nil
}

// moveCursor moves from line from by delta lines of code, and skips hunk
// headers. When hunk is 0 or more, it stops at the edge of that hunk. It
// returns -1 when no line of code is found.
func moveCursor(lines []pageLine, from, delta, hunk int) int {
	dir := 1

	if delta < 0 {
		dir, delta = -1, -delta
	}

	at := from

	for ; delta > 0; delta-- {
		next := at + dir

		for next >= 0 && next < len(lines) && !lines[next].code {
			next += dir
		}

		if next < 0 || next >= len(lines) || (hunk >= 0 && lines[next].hunk != hunk) {
			break
		}

		at = next
	}

	if at >= 0 && at < len(lines) && lines[at].code {
		return at
	}

	return -1
}

// codeThreads finds the code comments on each line, by their index in the
// thread. An outdated comment is on a line of an older diff, so it is left
// out.
func codeThreads(thread []github.Post) map[codeSpot][]int {
	threads := map[codeSpot][]int{}

	for i, post := range thread {
		if code := post.Code; code != nil && !code.Outdated && code.Line > 0 {
			spot := codeSpot{path: code.Path, spot: github.DiffLine{Side: code.Side, Line: code.Line}}
			threads[spot] = append(threads[spot], i)
		}
	}

	return threads
}

// renderFile is the name of a changed file, then each hunk of its diff.
func (m Model) renderFile(file *gitdiff.File, state github.ViewState, width int) *filePage {
	muted := lipgloss.NewStyle().Foreground(m.tone().muted)
	page := &filePage{path: fileName(file), head: m.fileTitle(file, state, width), numberWidth: lineNumberWidth(file)}
	look := shadeNormal

	if state == github.Viewed {
		look = shadeFaded
	}

	if file.IsBinary {
		page.head += "\n\n" + muted.Render("Binary file. gith cannot show it.")

		return page
	}

	if len(file.TextFragments) == 0 {
		page.head += "\n\n" + muted.Render("The content of this file did not change.")

		return page
	}

	page.codeWidth = max(1, width-gutterSpace-page.numberWidth-1)

	for hunk, fragment := range file.TextFragments {
		header := fitLine(muted.Render(strings.TrimSpace(fragment.Header())), width)
		page.lines = append(page.lines, pageLine{rows: []string{"", header}, hunk: hunk})
		var oldText, newText []string

		for _, line := range fragment.Lines {
			text := strings.ReplaceAll(strings.TrimSuffix(line.Line, "\n"), "\t", "    ")

			if line.Old() {
				oldText = append(oldText, text)
			}

			if line.New() {
				newText = append(newText, text)
			}
		}

		oldRows := highlightLines(file.OldName, oldText)
		newRows := highlightLines(file.NewName, newText)
		oldNumber, newNumber := fragment.OldPosition, fragment.NewPosition

		for _, line := range fragment.Lines {
			var tokens []chroma.Token
			spot := github.DiffLine{Side: github.SideNew, Line: int(newNumber)}

			if line.New() {
				tokens, newRows = newRows[0], newRows[1:]
				newNumber++
			}

			if line.Old() {
				if line.Op == gitdiff.OpDelete {
					tokens = oldRows[0]
					spot = github.DiffLine{Side: github.SideOld, Line: int(oldNumber)}
				}

				oldRows = oldRows[1:]
				oldNumber++
			}

			page.lines = append(page.lines, pageLine{
				rows:   m.diffLine(line.Op, tokens, page.codeWidth, look),
				code:   true,
				hunk:   hunk,
				spot:   spot,
				op:     line.Op,
				tokens: tokens,
			})
		}
	}

	return page
}

// fileName is the path of the file in the pull request. A deleted file
// keeps its old path.
func fileName(file *gitdiff.File) string {
	if file.IsDelete {
		return file.OldName
	}

	return file.NewName
}

// lineNumberWidth is the number of digits in the largest line number of the
// file, on the old side or the new side.
func lineNumberWidth(file *gitdiff.File) int {
	var largest int64

	for _, fragment := range file.TextFragments {
		largest = max(largest, fragment.OldPosition+fragment.OldLines, fragment.NewPosition+fragment.NewLines)
	}

	return len(strconv.FormatInt(largest, 10))
}

// codeStyle is the chroma theme that suits the background of the terminal.
func (m Model) codeStyle() *chroma.Style {
	if m.dark {
		return styles.Get("tokyonight-night")
	}

	return styles.Get("github")
}

// fileTitle is the file name, what happened to the file, and its counts.
func (m Model) fileTitle(file *gitdiff.File, state github.ViewState, width int) string {
	colors := m.tone()
	muted := lipgloss.NewStyle().Foreground(colors.muted)
	var added, deleted int64

	for _, fragment := range file.TextFragments {
		added += fragment.LinesAdded
		deleted += fragment.LinesDeleted
	}

	parts := []string{lipgloss.NewStyle().Bold(true).Render(fileName(file))}

	switch {
	case file.IsNew:
		parts = append(parts, muted.Render("new file"))

	case file.IsDelete:
		parts = append(parts, muted.Render("deleted"))

	case file.IsRename:
		parts = append(parts, muted.Render("renamed from "+file.OldName))
	}

	parts = append(parts,
		lipgloss.NewStyle().Foreground(colors.good).Render(fmt.Sprintf("+%d", added)),
		lipgloss.NewStyle().Foreground(colors.bad).Render(fmt.Sprintf("−%d", deleted)),
	)

	title := strings.Join(parts, muted.Render("  ·  "))

	switch state {
	case github.Viewed:
		return spread(title, lipgloss.NewStyle().Foreground(colors.good).Render("✓ Viewed"), width)

	case github.ChangedSinceViewed:
		return spread(title, lipgloss.NewStyle().Foreground(colors.accent).Render("Changed since viewed"), width)
	}

	return fitLine(title, width)
}

// shade is how a line of code is drawn.
type shade int

const (
	shadeNormal shade = iota

	// shadeFaded draws the code grey, because the file is viewed.
	shadeFaded

	// shadeSelected marks the lines that a comment is for.
	shadeSelected
)

// diffLine colors one line of code and wraps it onto more screen rows. Each
// row keeps the + or - mark. A changed or selected line fills the width
// with its background color.
func (m Model) diffLine(op gitdiff.LineOp, tokens []chroma.Token, codeWidth int, look shade) []string {
	colors := m.tone()
	code := m.codeStyle()
	var background color.Color
	mark := lipgloss.NewStyle().Foreground(colors.muted)

	switch op {
	case gitdiff.OpAdd:
		background = colors.added
		mark = mark.Foreground(colors.good)

		if look == shadeFaded {
			background = colors.fadedAdded
		}

	case gitdiff.OpDelete:
		background = colors.removed
		mark = mark.Foreground(colors.bad)

		if look == shadeFaded {
			background = colors.fadedRemoved
		}
	}

	if look == shadeSelected {
		background = colors.selected
	}

	base := lipgloss.NewStyle()

	if background != nil {
		base = base.Background(background)
		mark = mark.Background(background)
	}

	var text strings.Builder

	for _, token := range tokens {
		if look == shadeFaded {
			text.WriteString(base.Foreground(colors.faded).Render(token.Value))
		} else {
			text.WriteString(tokenStyle(base, code.Get(token.Type)).Render(token.Value))
		}
	}

	rows := strings.Split(hardwrap(text.String(), codeWidth), "\n")

	for i, row := range rows {
		fill := base.Render(strings.Repeat(" ", max(0, codeWidth-ansi.StringWidth(row))))
		rows[i] = mark.Render(op.String()) + row + fill
	}

	return rows
}

// hardwrap breaks styled text at the width and keeps each space. A word
// wrap removes spaces in code. The colors continue on the next line.
func hardwrap(text string, width int) string {
	var out strings.Builder
	writer := lipgloss.NewWrapWriter(&out)
	_, _ = writer.Write([]byte(ansi.Hardwrap(text, width, true)))
	_ = writer.Close()

	return out.String()
}

func tokenStyle(base lipgloss.Style, entry chroma.StyleEntry) lipgloss.Style {
	style := base

	if entry.Colour.IsSet() {
		style = style.Foreground(lipgloss.Color(entry.Colour.String()))
	}

	return style.Bold(entry.Bold == chroma.Yes).Italic(entry.Italic == chroma.Yes)
}
