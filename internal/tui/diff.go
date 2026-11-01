package tui

import (
	"context"
	"fmt"
	"math"

	tea "charm.land/bubbletea/v2"

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

	// follow scrolls the view to the cursor at the next sync.
	follow bool

	// expanded holds the lines whose code comments show in full. threads
	// holds the markdown of each code comment, rendered at threadsW.
	expanded map[codeSpot]bool
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
	return &diffLoaded{diff: diff, cursor: -1, expanded: map[codeSpot]bool{}}
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
			diff.expanded[spot] = !diff.expanded[spot]
			diff.follow = true
		}
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
