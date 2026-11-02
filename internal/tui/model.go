// Package tui is the full-screen issue and pull request browser.
package tui

import (
	"context"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jamesofficer/gith/internal/github"
)

const requestTimeout = 30 * time.Second

type list struct {
	items   []github.Item
	cursor  int
	loading bool
	err     error
}

type itemKey struct {
	kind   github.Kind
	number int
}

// openItem is the issue or pull request on the detail screen.
type openItem struct {
	key     itemKey
	summary github.Item
	req     int
	state   detailState
}

// detailState is *detailLoading, *detailFailed, or *detailLoaded.
type detailState interface{ isDetailState() }

type detailLoading struct{}

type detailFailed struct{ err error }

type detailLoaded struct {
	detail github.Detail

	// posts holds the rendered description, then each post of the thread.
	posts     []renderedPost
	postsW    int
	postsDark bool
	index     int
	compose   *compose

	// diff is nil when the thread shows.
	diff diffState
}

func (*detailLoading) isDetailState() {}
func (*detailFailed) isDetailState()  {}
func (*detailLoaded) isDetailState()  {}

// Model is the Bubble Tea model for gith.
type Model struct {
	source   github.Source
	repo     string
	width    int
	height   int
	dark     bool
	spin     spinner.Model
	viewport viewport.Model

	kind   github.Kind
	issues list
	pulls  list

	// open is nil on the list screen.
	open    *openItem
	problem string
	cache   map[itemKey]github.Detail
	diffs   map[itemKey]github.PullDiff
	drafts  map[draftKey]string
	req     int
}

type listMsg struct {
	kind  github.Kind
	items []github.Item
	err   error
}

type detailMsg struct {
	req    int
	key    itemKey
	detail github.Detail
	err    error
}

type commentMsg struct {
	req  int
	key  itemKey
	post github.Post
	err  error
}

type browseMsg struct{ err error }

// New starts on the issue list of the named repository.
func New(source github.Source, repo string) Model {
	m := Model{
		source:   source,
		repo:     repo,
		dark:     true,
		kind:     github.KindIssue,
		issues:   list{loading: true},
		pulls:    list{loading: true},
		cache:    map[itemKey]github.Detail{},
		diffs:    map[itemKey]github.PullDiff{},
		drafts:   map[draftKey]string{},
		spin:     spinner.New(spinner.WithSpinner(spinner.MiniDot)),
		viewport: viewport.New(),
	}
	m.spin.Style = lipgloss.NewStyle().Foreground(m.tone().accent)
	m.viewport.SoftWrap = true
	m.viewport.MouseWheelEnabled = true

	return m
}

// Init loads both lists.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		tea.RequestBackgroundColor,
		m.spin.Tick,
		m.loadList(github.KindIssue),
		m.loadList(github.KindPull),
	)
}

// Update handles a message, then fits the open item to the screen.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m, cmd := m.update(msg)

	return m.sync(), cmd
}

func (m Model) update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		return m, nil

	case tea.BackgroundColorMsg:
		m.dark = msg.IsDark()
		m.spin.Style = lipgloss.NewStyle().Foreground(m.tone().accent)

		return m, nil

	case tea.KeyPressMsg:
		return m.onKey(msg)

	case tea.MouseWheelMsg:
		if m.loaded() != nil {
			m.viewport, _ = m.viewport.Update(msg)
		}

		return m, nil

	case listMsg:
		return m.applyList(msg), nil

	case detailMsg:
		return m.applyDetail(msg), nil

	case diffMsg:
		return m.applyDiff(msg), nil

	case commentMsg:
		return m.applyComment(msg)

	case browseMsg:
		if msg.err != nil {
			m.problem = "Could not open the browser. " + msg.err.Error()
		}

		return m, nil

	case spinner.TickMsg:
		if !m.busy() {
			return m, nil
		}

		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)

		return m, cmd
	}

	if c := m.compose(); c != nil && !c.sending {
		var cmd tea.Cmd
		c.box, cmd = c.box.Update(msg)

		return m, cmd
	}

	return m, nil
}

func (m Model) onKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if c := m.compose(); c != nil {
		return m.onComposeKey(c, msg)
	}

	m.problem = ""
	key := msg.String()

	if key == "ctrl+c" || (key == "q" && m.open == nil) {
		return m, tea.Quit
	}

	if m.open != nil {
		return m.onDetailKey(key)
	}

	return m.onListKey(key)
}

func (m Model) onListKey(key string) (Model, tea.Cmd) {
	active := m.listOf(m.kind)

	switch key {
	case "up":
		active.cursor = step(active.cursor, -1, len(active.items))

	case "down":
		active.cursor = step(active.cursor, 1, len(active.items))

	case "left", "right":
		m.kind = other(m.kind)

	case "enter":
		return m.openSelected()

	case "b":
		if item, ok := active.current(); ok {
			return m, m.browse(item.Number)
		}

	case "r":
		return m.refreshLists()
	}

	return m, nil
}

func (m Model) onDetailKey(key string) (Model, tea.Cmd) {
	loaded := m.loaded()

	if loaded != nil && loaded.diff != nil {
		return m.onDiffKey(loaded, key)
	}

	switch key {
	case "esc", "backspace":
		m.open = nil

	case "b":
		return m, m.browse(m.open.key.number)

	case "r":
		return m.reloadDetail()

	case "c":
		return m.startCompose(nil)

	case "d":
		if loaded != nil {
			return m.openDiff(loaded)
		}

	case "up":
		return m.moveThread(-1), nil

	case "down":
		return m.moveThread(1), nil

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

func (m Model) moveThread(delta int) Model {
	loaded := m.loaded()

	if loaded == nil {
		return m
	}

	next := step(loaded.index, delta, len(loaded.posts))

	if next != loaded.index {
		loaded.index = next
		m.viewport.SetYOffset(0)
	}

	return m
}

func (m Model) openSelected() (Model, tea.Cmd) {
	item, ok := m.listOf(m.kind).current()

	if !ok {
		return m, nil
	}

	key := itemKey{kind: m.kind, number: item.Number}
	m.req++
	m.open = &openItem{key: key, summary: item, req: m.req}
	m.viewport.SetYOffset(0)

	if cached, ok := m.cache[key]; ok {
		m.open.state = &detailLoaded{detail: cached}

		return m, nil
	}

	return m.loadDetail()
}

func (m Model) reloadDetail() (Model, tea.Cmd) {
	if _, loading := m.open.state.(*detailLoading); loading {
		return m, nil
	}

	delete(m.cache, m.open.key)
	m.req++
	m.open.req = m.req
	m.viewport.SetYOffset(0)

	return m.loadDetail()
}

func (m Model) loadDetail() (Model, tea.Cmd) {
	m.open.state = &detailLoading{}
	source, key, req := m.source, m.open.key, m.open.req

	return m, tea.Batch(m.spin.Tick, request(func(ctx context.Context) tea.Msg {
		detail, err := source.Detail(ctx, key.kind, key.number)

		return detailMsg{req: req, key: key, detail: detail, err: err}
	}))
}

// applyDetail caches a loaded item even when the user has left it.
func (m Model) applyDetail(msg detailMsg) Model {
	if msg.err == nil {
		m.cache[msg.key] = msg.detail
		m = m.setCommentCount(msg.key, msg.detail.Comments)
	}

	if m.open == nil || m.open.req != msg.req {
		return m
	}

	if msg.err != nil {
		m.open.state = &detailFailed{err: msg.err}

		return m
	}

	m.open.state = &detailLoaded{detail: msg.detail}

	return m
}

func (m Model) refreshLists() (Model, tea.Cmd) {
	if m.issues.loading || m.pulls.loading {
		return m, nil
	}

	m.issues.loading = true
	m.pulls.loading = true
	clear(m.cache)
	clear(m.diffs)

	return m, tea.Batch(m.spin.Tick, m.loadList(github.KindIssue), m.loadList(github.KindPull))
}

func (m Model) loadList(kind github.Kind) tea.Cmd {
	source := m.source

	return request(func(ctx context.Context) tea.Msg {
		items, err := source.List(ctx, kind)

		return listMsg{kind: kind, items: items, err: err}
	})
}

func (m Model) applyList(msg listMsg) Model {
	l := m.listOf(msg.kind)
	l.loading = false
	l.err = msg.err
	l.items = msg.items
	l.cursor = step(l.cursor, 0, len(l.items))

	return m
}

func (m Model) setCommentCount(key itemKey, count int) Model {
	l := m.listOf(key.kind)

	for i := range l.items {
		if l.items[i].Number == key.number {
			l.items[i].Comments = count
		}
	}

	if m.open != nil && m.open.key == key {
		m.open.summary.Comments = count
	}

	return m
}

func (m Model) browse(number int) tea.Cmd {
	source := m.source

	return request(func(ctx context.Context) tea.Msg {
		return browseMsg{err: source.Browse(ctx, number)}
	})
}

func request(fn func(ctx context.Context) tea.Msg) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()

		return fn(ctx)
	}
}

// sync renders the open item for the screen size and colors, then sizes the
// post viewport and the comment box to the layout.
func (m Model) sync() Model {
	loaded := m.loaded()

	if loaded == nil || m.width <= 0 || m.height <= 0 {
		return m
	}

	width := m.contentWidth()

	if loaded.posts == nil || loaded.postsW != width || loaded.postsDark != m.dark {
		posts, err := renderPosts(loaded.detail, width, m.dark)

		if err != nil {
			m.open.state = &detailFailed{err: err}

			return m
		}

		loaded.posts, loaded.postsW, loaded.postsDark = posts, width, m.dark
		loaded.index = step(loaded.index, 0, len(posts))
	}

	layout := m.detailLayout()
	var content string
	var cursorTop, cursorBottom int
	diff, showDiff := loaded.diff.(*diffLoaded)

	if showDiff {
		content, cursorTop, cursorBottom = m.diffContent(loaded, diff, width)
	} else {
		content = m.postContent(loaded)
	}

	if c := loaded.compose; c != nil {
		c.box.SetStyles(m.commentStyles())
		c.box.SetWidth(width)
		c.box.SetHeight(layout.composeRows)
	}

	m.viewport.SetWidth(width)
	m.viewport.SetHeight(min(lipgloss.Height(content), layout.postHeight-m.frameHeight()))
	m.viewport.SetContent(content)

	if showDiff && diff.follow {
		offset := rowsInView(m.viewport.YOffset(), m.viewport.Height(), cursorTop, cursorBottom)

		if diff.jumped {
			offset = max(0, cursorTop-jumpContext)
		}

		diff.follow, diff.jumped = false, false
		m.viewport.SetYOffset(offset)
	}

	return m
}

func (m Model) busy() bool {
	if m.issues.loading || m.pulls.loading {
		return true
	}

	if m.open == nil {
		return false
	}

	switch state := m.open.state.(type) {
	case *detailLoading:
		return true

	case *detailLoaded:
		_, diffLoading := state.diff.(*diffLoading)

		return diffLoading || (state.compose != nil && state.compose.sending)
	}

	return false
}

func (m Model) loaded() *detailLoaded {
	if m.open == nil {
		return nil
	}

	loaded, _ := m.open.state.(*detailLoaded)

	return loaded
}

func (m Model) compose() *compose {
	if loaded := m.loaded(); loaded != nil {
		return loaded.compose
	}

	return nil
}

func (m *Model) listOf(kind github.Kind) *list {
	if kind == github.KindPull {
		return &m.pulls
	}

	return &m.issues
}

func (l list) current() (github.Item, bool) {
	if l.cursor < 0 || l.cursor >= len(l.items) {
		return github.Item{}, false
	}

	return l.items[l.cursor], true
}

func other(kind github.Kind) github.Kind {
	if kind == github.KindPull {
		return github.KindIssue
	}

	return github.KindPull
}
