package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/bluekeyes/go-gitdiff/gitdiff"
	"github.com/charmbracelet/x/ansi"

	"github.com/jamesofficer/gith/internal/github"
)

// diffState is *diffLoading, *diffFailed, or *diffLoaded.
type diffState interface{ isDiffState() }

type diffLoading struct{ req int }

type diffFailed struct{ err error }

type diffLoaded struct {
	files []*gitdiff.File

	// pages holds each file after it is rendered for the screen, or "".
	pages     []string
	pagesW    int
	pagesDark bool
	index     int
}

func (*diffLoading) isDiffState() {}
func (*diffFailed) isDiffState()  {}
func (*diffLoaded) isDiffState()  {}

type diffMsg struct {
	req   int
	key   itemKey
	files []*gitdiff.File
	err   error
}

func (m Model) openDiff(loaded *detailLoaded) (Model, tea.Cmd) {
	if m.open.key.kind != github.KindPull {
		return m, nil
	}

	m.viewport.SetYOffset(0)

	if files, ok := m.diffs[m.open.key]; ok {
		loaded.diff = &diffLoaded{files: files}

		return m, nil
	}

	return m.loadDiff(loaded)
}

func (m Model) loadDiff(loaded *detailLoaded) (Model, tea.Cmd) {
	m.req++
	loaded.diff = &diffLoading{req: m.req}
	source, key, req := m.source, m.open.key, m.req

	return m, tea.Batch(m.spin.Tick, request(func(ctx context.Context) tea.Msg {
		files, err := source.Diff(ctx, key.number)

		return diffMsg{req: req, key: key, files: files, err: err}
	}))
}

// applyDiff caches a loaded diff even when the user has left it.
func (m Model) applyDiff(msg diffMsg) Model {
	if msg.err == nil {
		m.diffs[msg.key] = msg.files
	}

	loaded := m.loaded()

	if loaded == nil {
		return m
	}

	if waiting, ok := loaded.diff.(*diffLoading); !ok || waiting.req != msg.req {
		return m
	}

	if msg.err != nil {
		loaded.diff = &diffFailed{err: msg.err}

		return m
	}

	loaded.diff = &diffLoaded{files: msg.files}

	return m
}

// arrowScroll is the number of lines that an arrow key scrolls the diff.
const arrowScroll = 6

func (m Model) onDiffKey(loaded *detailLoaded, key string) (Model, tea.Cmd) {
	switch key {
	case "esc", "backspace", "d":
		loaded.diff = nil
		m.viewport.SetYOffset(0)

	case "o":
		return m, m.browse(m.open.key.number)

	case "r":
		if _, loading := loaded.diff.(*diffLoading); !loading {
			delete(m.diffs, m.open.key)

			return m.loadDiff(loaded)
		}

	case "left":
		return m.moveFile(loaded, -1), nil

	case "right":
		return m.moveFile(loaded, 1), nil

	case "up":
		m.viewport.ScrollUp(arrowScroll)

	case "down":
		m.viewport.ScrollDown(arrowScroll)

	case "k":
		m.viewport.ScrollUp(1)

	case "j":
		m.viewport.ScrollDown(1)

	case "pgup":
		m.viewport.PageUp()

	case "pgdown":
		m.viewport.PageDown()

	case "home":
		m.viewport.GotoTop()

	case "end":
		m.viewport.GotoBottom()
	}

	return m, nil
}

func (m Model) moveFile(loaded *detailLoaded, delta int) Model {
	diff, ok := loaded.diff.(*diffLoaded)

	if !ok {
		return m
	}

	next := step(diff.index, delta, len(diff.files))

	if next != diff.index {
		diff.index = next
		m.viewport.SetYOffset(0)
	}

	return m
}

// diffPage is the shown file. It renders a file only when it is first shown,
// and forgets all pages when the width or the colors change.
func (m Model) diffPage(diff *diffLoaded, width int) string {
	if len(diff.files) == 0 {
		return lipgloss.NewStyle().Foreground(m.tone().muted).Render("This pull request changes no files.")
	}

	if diff.pages == nil || diff.pagesW != width || diff.pagesDark != m.dark {
		diff.pages = make([]string, len(diff.files))
		diff.pagesW, diff.pagesDark = width, m.dark
	}

	if diff.pages[diff.index] == "" {
		diff.pages[diff.index] = m.renderFile(diff.files[diff.index], width)
	}

	return diff.pages[diff.index]
}

func diffTitle(diff *diffLoaded) string {
	if len(diff.files) == 0 {
		return "no files"
	}

	return fmt.Sprintf("file %d of %d", diff.index+1, len(diff.files))
}

// renderFile is the name of a changed file, then each hunk of its diff.
func (m Model) renderFile(file *gitdiff.File, width int) string {
	muted := lipgloss.NewStyle().Foreground(m.tone().muted)
	lines := []string{m.fileTitle(file, width)}

	if file.IsBinary {
		return lines[0] + "\n\n" + muted.Render("Binary file. gith cannot show it.")
	}

	if len(file.TextFragments) == 0 {
		return lines[0] + "\n\n" + muted.Render("The content of this file did not change.")
	}

	code := m.codeStyle()
	numberWidth := lineNumberWidth(file)

	for _, fragment := range file.TextFragments {
		lines = append(lines, "", fitLine(muted.Render(strings.TrimSpace(fragment.Header())), width))
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
			number := newNumber

			if line.New() {
				tokens, newRows = newRows[0], newRows[1:]
				newNumber++
			}

			if line.Old() {
				if line.Op == gitdiff.OpDelete {
					tokens, number = oldRows[0], oldNumber
				}

				oldRows = oldRows[1:]
				oldNumber++
			}

			gutter := padLeft(strconv.FormatInt(number, 10), numberWidth)
			lines = append(lines, m.diffLine(gutter, line.Op, tokens, code, width)...)
		}
	}

	return strings.Join(lines, "\n")
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
func (m Model) fileTitle(file *gitdiff.File, width int) string {
	colors := m.tone()
	muted := lipgloss.NewStyle().Foreground(colors.muted)
	name := file.NewName

	if file.IsDelete {
		name = file.OldName
	}

	var added, deleted int64

	for _, fragment := range file.TextFragments {
		added += fragment.LinesAdded
		deleted += fragment.LinesDeleted
	}

	parts := []string{lipgloss.NewStyle().Bold(true).Render(name)}

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

	return fitLine(strings.Join(parts, muted.Render("  ·  ")), width)
}

// diffLine colors one line of code and wraps it onto more screen lines.
// The gutter holds the line number, on the first part only. Each part keeps
// the + or - mark, and a changed line fills the width with its background.
func (m Model) diffLine(gutter string, op gitdiff.LineOp, tokens []chroma.Token, code *chroma.Style, width int) []string {
	colors := m.tone()
	base := lipgloss.NewStyle()
	mark := lipgloss.NewStyle().Foreground(colors.muted)

	switch op {
	case gitdiff.OpAdd:
		base = base.Background(colors.added)
		mark = mark.Foreground(colors.good).Background(colors.added)

	case gitdiff.OpDelete:
		base = base.Background(colors.removed)
		mark = mark.Foreground(colors.bad).Background(colors.removed)
	}

	var text strings.Builder

	for _, token := range tokens {
		text.WriteString(tokenStyle(base, code.Get(token.Type)).Render(token.Value))
	}

	number := lipgloss.NewStyle().Foreground(colors.muted)
	blank := strings.Repeat(" ", lipgloss.Width(gutter))
	codeWidth := max(1, width-lipgloss.Width(gutter)-2)
	parts := strings.Split(hardwrap(text.String(), codeWidth), "\n")

	for i, part := range parts {
		fill := base.Render(strings.Repeat(" ", max(0, codeWidth-ansi.StringWidth(part))))
		parts[i] = number.Render(gutter) + " " + mark.Render(op.String()) + part + fill
		gutter = blank
	}

	return parts
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
