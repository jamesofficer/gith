// Package tui is the full-screen issue and pull request browser.
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jamesofficer/gith/internal/github"
)

type screen int

const (
	screenList screen = iota
	screenDetail
)

type list struct {
	items   []github.Item
	cursor  int
	loading bool
	err     error
}

// Model is the Bubble Tea model for gith.
type Model struct {
	source github.Source
	name   string
	width  int
	height int
	dark   bool
	kind   github.Kind
	issues list
	pulls  list
	screen screen

	opened        github.Item
	detail        *github.Detail
	detailLoading bool
	detailErr     error
	detailReq     int
	cache         map[string]github.Detail

	spin     spinner.Model
	viewport viewport.Model
	posts    []string
	postTop  []int
	postHigh []int
	thread   int
	renderW  int
	renderD  bool
	renderR  int
}

type nameMsg struct {
	name string
	err  error
}

type listMsg struct {
	kind  github.Kind
	items []github.Item
	err   error
}

type detailMsg struct {
	req    int
	detail github.Detail
	err    error
}

// New starts on the open issue list and loads both lists.
func New(source github.Source) Model {
	m := Model{
		source:   source,
		dark:     true,
		kind:     github.KindIssue,
		issues:   list{loading: true},
		pulls:    list{loading: true},
		cache:    map[string]github.Detail{},
		spin:     spinner.New(spinner.WithSpinner(spinner.MiniDot)),
		viewport: viewport.New(),
	}
	m.spin.Style = lipgloss.NewStyle().Foreground(m.tone().accent)
	m.viewport.SoftWrap = true
	m.viewport.MouseWheelEnabled = true

	return m
}

// Init loads the repository name and both lists.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		tea.RequestBackgroundColor,
		m.spin.Tick,
		m.loadName(),
		m.loadList(github.KindIssue),
		m.loadList(github.KindPull),
	)
}

// Update handles keys, window size, and loaded data.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		if m.screen == screenDetail && m.detail != nil {
			m = m.prepareDetail(false)
		}

		return m, nil

	case tea.BackgroundColorMsg:
		if msg.IsDark() == m.dark {
			return m, nil
		}

		m.dark = msg.IsDark()
		m.spin.Style = lipgloss.NewStyle().Foreground(m.tone().accent)

		if m.screen == screenDetail && m.detail != nil {
			m = m.prepareDetail(false)
		}

		return m, nil

	case tea.KeyPressMsg:
		return m.onKey(msg)

	case tea.MouseWheelMsg:
		if m.screen == screenDetail && m.detail != nil {
			m.viewport, _ = m.viewport.Update(msg)
		}

		return m, nil

	case nameMsg:
		if msg.err == nil {
			m.name = msg.name
		}

		return m, nil

	case listMsg:
		return m.applyList(msg), nil

	case detailMsg:
		return m.applyDetail(msg), nil

	case spinner.TickMsg:
		if !m.busy() {
			return m, nil
		}

		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)

		return m, cmd
	}

	return m, nil
}

func (m Model) onKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "esc":
		if m.screen == screenDetail {
			m.screen = screenList

			return m, nil
		}

		return m, tea.Quit

	case "left", "right":
		if m.screen == screenDetail {
			m.screen = screenList
		}

		m.kind = other(m.kind)

		return m, nil

	case "up":
		return m.move(-1)

	case "down":
		return m.move(1)

	case "pgup":
		if m.screen == screenDetail {
			m.viewport.PageUp()
		}

		return m, nil

	case "pgdown":
		if m.screen == screenDetail {
			m.viewport.PageDown()
		}

		return m, nil

	case "home":
		if m.screen == screenDetail {
			m.viewport.GotoTop()
		}

		return m, nil

	case "end":
		if m.screen == screenDetail {
			m.viewport.GotoBottom()
		}

		return m, nil

	case "enter":
		if m.screen == screenList {
			return m.open()
		}

		return m, nil

	case "backspace":
		if m.screen == screenDetail {
			m.screen = screenList
		}

		return m, nil
	}

	return m, nil
}

func (m Model) move(delta int) (tea.Model, tea.Cmd) {
	if m.screen == screenDetail {
		if len(m.posts) == 0 {
			return m, nil
		}

		next := step(m.thread, delta, len(m.posts))

		if next == m.thread {
			return m, nil
		}

		m.thread = next

		return m.paintThread(false), nil
	}

	active := m.active()
	active.cursor = step(active.cursor, delta, len(active.items))

	return m.setActive(active), nil
}

func (m Model) open() (tea.Model, tea.Cmd) {
	item, ok := m.active().current()

	if !ok {
		return m, nil
	}

	m.screen = screenDetail
	m.opened = item
	m.detail = nil
	m.detailErr = nil
	m.posts = nil
	m.thread = 0
	m.detailReq++
	req := m.detailReq
	key := cacheKey(m.kind, item.Number)

	if cached, ok := m.cache[key]; ok {
		m.detail = &cached
		m.detailLoading = false
		m = m.prepareDetail(true)

		return m, nil
	}

	m.detailLoading = true
	kind := m.kind
	number := item.Number

	return m, tea.Batch(m.spin.Tick, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		detail, err := m.source.Detail(ctx, kind, number)

		return detailMsg{req: req, detail: detail, err: err}
	})
}

func (m Model) applyList(msg listMsg) Model {
	active := m.issues

	if msg.kind == github.KindPull {
		active = m.pulls
	}

	active.loading = false
	active.err = msg.err
	active.items = msg.items
	active.cursor = clamp(active.cursor, len(active.items))

	if msg.kind == github.KindPull {
		m.pulls = active

		return m
	}

	m.issues = active

	return m
}

func (m Model) applyDetail(msg detailMsg) Model {
	if msg.req != m.detailReq || m.screen != screenDetail {
		return m
	}

	m.detailLoading = false

	if msg.err != nil {
		m.detailErr = msg.err
		m.detail = nil
		m.posts = nil

		return m
	}

	m.detailErr = nil
	m.cache[cacheKey(m.kind, msg.detail.Number)] = msg.detail
	detail := msg.detail
	m.detail = &detail

	return m.prepareDetail(true)
}

func (m Model) prepareDetail(reset bool) Model {
	if m.width <= 0 || m.height <= 0 || m.detail == nil {
		return m
	}

	contentW := m.detailContentWidth()
	m.viewport.SetWidth(m.width)
	m.viewport.SetHeight(m.threadViewportHeight())

	if m.posts == nil || m.renderW != contentW || m.renderD != m.dark || m.renderR != m.detailReq {
		m.posts = renderPosts(*m.detail, contentW, m.dark)
		m.renderW = contentW
		m.renderD = m.dark
		m.renderR = m.detailReq
	}

	if reset {
		m.thread = 0
	}

	return m.paintThread(reset)
}

func (m Model) paintThread(reset bool) Model {
	if len(m.posts) == 0 {
		m.viewport.SetContent("")

		return m
	}

	m.thread = step(m.thread, 0, len(m.posts))
	blocks := make([]string, len(m.posts))
	m.postTop = make([]int, len(m.posts))
	m.postHigh = make([]int, len(m.posts))
	line := 0

	for i := range m.posts {
		m.postTop[i] = line
		blocks[i] = m.postBox(m.postContent(i), i == m.thread)
		m.postHigh[i] = lipgloss.Height(blocks[i])
		line += m.postHigh[i]

		if i < len(m.posts)-1 {
			line++
		}
	}

	m.viewport.SetContent(strings.Join(blocks, "\n\n"))

	if reset {
		m.viewport.GotoTop()

		return m
	}

	return m.revealThread()
}

func (m Model) revealThread() Model {
	if m.thread < 0 || m.thread >= len(m.postTop) {
		return m
	}

	top := m.postTop[m.thread]
	height := m.postHigh[m.thread]
	bottom := top + height
	viewH := m.viewport.Height()
	y := m.viewport.YOffset()

	if top < y {
		m.viewport.SetYOffset(top)

		return m
	}

	if bottom > y+viewH {
		if height >= viewH {
			m.viewport.SetYOffset(top)

			return m
		}

		m.viewport.SetYOffset(bottom - viewH)
	}

	return m
}

func (m Model) loadName() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		name, err := m.source.Name(ctx)

		return nameMsg{name: name, err: err}
	}
}

func (m Model) loadList(kind github.Kind) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		items, err := m.source.List(ctx, kind)

		return listMsg{kind: kind, items: items, err: err}
	}
}

func (m Model) busy() bool {
	return m.issues.loading || m.pulls.loading || m.detailLoading
}

func (m Model) active() list {
	if m.kind == github.KindPull {
		return m.pulls
	}

	return m.issues
}

func (m Model) setActive(active list) Model {
	if m.kind == github.KindPull {
		m.pulls = active

		return m
	}

	m.issues = active

	return m
}

func (l list) current() (github.Item, bool) {
	if l.cursor < 0 || l.cursor >= len(l.items) {
		return github.Item{}, false
	}

	return l.items[l.cursor], true
}

func (m Model) shown() github.Item {
	if m.detail != nil {
		return m.detail.Item
	}

	return m.opened
}

func other(kind github.Kind) github.Kind {
	if kind == github.KindPull {
		return github.KindIssue
	}

	return github.KindPull
}

func cacheKey(kind github.Kind, number int) string {
	return fmt.Sprintf("%s/%d", kind, number)
}

func step(cursor, delta, n int) int {
	if n == 0 {
		return 0
	}

	next := cursor + delta

	if next < 0 {
		return 0
	}

	if next >= n {
		return n - 1
	}

	return next
}

func clamp(cursor, n int) int {
	return step(cursor, 0, n)
}

// window is the visible slice that keeps the cursor on screen.
func window(cursor, count, height int) (int, int) {
	if height < 1 {
		height = 1
	}

	if count <= height {
		return 0, count
	}

	start := cursor - height/2

	if start < 0 {
		start = 0
	}

	if start+height > count {
		start = count - height
	}

	return start, start + height
}
