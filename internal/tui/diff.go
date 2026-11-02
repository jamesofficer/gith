package tui

import (
	"context"
	"fmt"
	"math"

	tea "charm.land/bubbletea/v2"
	"github.com/bluekeyes/go-gitdiff/gitdiff"

	"github.com/jamesofficer/gith/internal/github"
)

// diffState is *diffLoading, *diffFailed, or *diffLoaded.
type diffState interface{ isDiffState() }

type diffLoading struct{ req int }

type diffFailed struct{ err error }

type diffLoaded struct {
	diff github.PullDiff

	// pages holds each file after it is rendered for the screen, or nil.
	pages     []*filePage
	pagesW    int
	pagesDark bool
	index     int

	// cursor is the selected line of the shown file, or -1 before the page
	// is rendered. While selecting is true, anchor is the first line of a
	// range of lines.
	cursor    int
	anchor    int
	selecting bool

	// follow scrolls the view to the cursor at the next sync. jumped also
	// puts the cursor near the top, so the lines under it show.
	follow bool
	jumped bool

	// collapsed shows only the start of each code comment. lineOpen holds
	// the lines that enter opened or closed against that rule.
	collapsed bool
	lineOpen  map[codeSpot]bool

	// threads holds the markdown of each code comment, rendered at threadsW.
	threads  map[int]renderedPost
	threadsW int
}

func (*diffLoading) isDiffState() {}
func (*diffFailed) isDiffState()  {}
func (*diffLoaded) isDiffState()  {}

type diffMsg struct {
	req  int
	key  itemKey
	diff github.PullDiff
	err  error
}

func newDiffLoaded(diff github.PullDiff) *diffLoaded {
	return &diffLoaded{diff: diff, cursor: -1, lineOpen: map[codeSpot]bool{}}
}

// threadsOpen tells if the code comments on a line show in full.
func (d *diffLoaded) threadsOpen(spot codeSpot) bool {
	if open, ok := d.lineOpen[spot]; ok {
		return open
	}

	return !d.collapsed
}

func (m Model) openDiff(loaded *detailLoaded) (Model, tea.Cmd) {
	if m.open.key.kind != github.KindPull {
		return m, nil
	}

	m.viewport.SetYOffset(0)

	if diff, ok := m.diffs[m.open.key]; ok {
		loaded.diff = newDiffLoaded(diff)

		return m, nil
	}

	return m.loadDiff(loaded)
}

func (m Model) loadDiff(loaded *detailLoaded) (Model, tea.Cmd) {
	m.req++
	loaded.diff = &diffLoading{req: m.req}
	source, key, req := m.source, m.open.key, m.req

	return m, tea.Batch(m.spin.Tick, request(func(ctx context.Context) tea.Msg {
		diff, err := source.Diff(ctx, key.number)

		return diffMsg{req: req, key: key, diff: diff, err: err}
	}))
}

// applyDiff caches a loaded diff even when the user has left it.
func (m Model) applyDiff(msg diffMsg) Model {
	if msg.err == nil {
		m.diffs[msg.key] = msg.diff
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

	loaded.diff = newDiffLoaded(msg.diff)

	return m
}

// arrowLines is the number of lines that an arrow key moves the cursor.
const arrowLines = 6

// jumpContext is the number of rows that show above the cursor after a jump.
const jumpContext = 3

func (m Model) onDiffKey(loaded *detailLoaded, key string) (Model, tea.Cmd) {
	diff, _ := loaded.diff.(*diffLoaded)

	switch key {
	case "esc":
		if diff != nil && diff.selecting {
			diff.selecting = false

			return m, nil
		}

		return m.closeDiff(loaded), nil

	case "backspace", "d":
		return m.closeDiff(loaded), nil

	case "b":
		return m, m.browse(m.open.key.number)

	case "r":
		if _, loading := loaded.diff.(*diffLoading); !loading {
			delete(m.diffs, m.open.key)

			return m.loadDiff(loaded)
		}
	}

	if diff == nil {
		return m, nil
	}

	switch key {
	case "left":
		return m.moveFile(diff, -1), nil

	case "right":
		return m.moveFile(diff, 1), nil

	case "up":
		diff.moveCursor(-arrowLines)

	case "down":
		diff.moveCursor(arrowLines)

	case "k":
		diff.moveCursor(-1)

	case "j":
		diff.moveCursor(1)

	case "pgup":
		diff.moveCursor(-m.viewport.Height())

	case "pgdown":
		diff.moveCursor(m.viewport.Height())

	case "home":
		diff.moveCursor(-math.MaxInt)

	case "end":
		diff.moveCursor(math.MaxInt)

	case "c":
		return m.selectLines(loaded, diff)

	case "enter":
		if spot, ok := diff.cursorSpot(); ok && len(codeThreads(loaded.detail.Thread)[spot]) > 0 {
			diff.lineOpen[spot] = !diff.threadsOpen(spot)
			diff.follow = true
		}

	case "e":
		diff.collapsed = !diff.collapsed
		clear(diff.lineOpen)
		diff.follow = true

	case "n":
		return m.jump(diff, 1, changeStart), nil

	case "p":
		return m.jump(diff, -1, changeStart), nil

	case "N":
		return m.jump(diff, 1, hasThreads(loaded.detail.Thread)), nil

	case "P":
		return m.jump(diff, -1, hasThreads(loaded.detail.Thread)), nil
	}

	return m, nil
}

// cursorSpot is the place of the cursor line, used to find its comments.
func (d *diffLoaded) cursorSpot() (codeSpot, bool) {
	page := d.page()

	if page == nil || d.cursor < 0 {
		return codeSpot{}, false
	}

	return codeSpot{path: page.path, spot: page.lines[d.cursor].spot}, true
}

func (m Model) closeDiff(loaded *detailLoaded) Model {
	loaded.diff = nil
	m.viewport.SetYOffset(0)

	return m
}

func (m Model) moveFile(diff *diffLoaded, delta int) Model {
	next := step(diff.index, delta, len(diff.diff.Files))

	if next != diff.index {
		diff.index = next
		diff.cursor = -1
		diff.selecting = false
		m.viewport.SetYOffset(0)
	}

	return m
}

// moveCursor moves the cursor by delta lines of code. During a selection,
// the cursor stays in the hunk of the anchor, because GitHub only accepts
// a range of lines in one hunk.
func (d *diffLoaded) moveCursor(delta int) {
	page := d.page()

	if page == nil || d.cursor < 0 {
		return
	}

	hunk := -1

	if d.selecting {
		hunk = page.lines[d.anchor].hunk
	}

	d.cursor = moveCursor(page.lines, d.cursor, delta, hunk)
	d.follow = true
}

// selectLines starts a range of lines at the cursor. When a range is open,
// it opens the comment box for the range.
func (m Model) selectLines(loaded *detailLoaded, diff *diffLoaded) (Model, tea.Cmd) {
	if diff.cursor < 0 {
		return m, nil
	}

	if !diff.selecting {
		diff.selecting = true
		diff.anchor = diff.cursor

		return m, nil
	}

	page := diff.page()
	first, last := min(diff.anchor, diff.cursor), max(diff.anchor, diff.cursor)
	target := &github.CodeComment{
		Commit: diff.diff.Commit,
		Path:   page.path,
		Start:  page.lines[first].spot,
		End:    page.lines[last].spot,
	}

	return m.startCompose(target)
}

// jump moves the cursor in the direction dir to the next line where stop is
// true. When the shown file has no such line, it looks in the next files.
func (m Model) jump(diff *diffLoaded, dir int, stop func(*filePage, int) bool) Model {
	if diff.page() == nil || diff.cursor < 0 || diff.selecting {
		return m
	}

	from := diff.cursor

	for index := diff.index; index >= 0 && index < len(diff.diff.Files); index += dir {
		page := m.pageAt(diff, index)

		if index != diff.index {
			from = -1

			if dir < 0 {
				from = len(page.lines)
			}
		}

		for i := from + dir; i >= 0 && i < len(page.lines); i += dir {
			if !stop(page, i) {
				continue
			}

			m = m.moveFile(diff, index-diff.index)
			diff.cursor = i
			diff.follow, diff.jumped = true, true

			return m
		}
	}

	return m
}

// changeStart tells if line i is the first added or removed line of a block.
func changeStart(page *filePage, i int) bool {
	line := page.lines[i]

	if !line.code || line.op == gitdiff.OpContext {
		return false
	}

	before := page.lines[i-1]

	return !before.code || before.op == gitdiff.OpContext
}

// hasThreads tells if a line has code comments.
func hasThreads(thread []github.Post) func(*filePage, int) bool {
	threads := codeThreads(thread)

	return func(page *filePage, i int) bool {
		line := page.lines[i]

		return line.code && len(threads[codeSpot{path: page.path, spot: line.spot}]) > 0
	}
}

// page is the rendered page of the shown file, or nil before the first sync.
func (d *diffLoaded) page() *filePage {
	if d.index >= len(d.pages) {
		return nil
	}

	return d.pages[d.index]
}

func diffTitle(diff *diffLoaded) string {
	if len(diff.diff.Files) == 0 {
		return "no files"
	}

	return fmt.Sprintf("file %d of %d", diff.index+1, len(diff.diff.Files))
}

// linesName names the lines of a code comment, such as "lines 10–12".
func linesName(target github.CodeComment) string {
	if target.Start == target.End {
		return fmt.Sprintf("line %d", target.End.Line)
	}

	return fmt.Sprintf("lines %d–%d", target.Start.Line, target.End.Line)
}
