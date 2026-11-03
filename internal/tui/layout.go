package tui

import "charm.land/lipgloss/v2"

const (
	// commentRows is the most text rows that the comment box shows.
	commentRows = 4

	// minPostHeight is a frame around one line of text.
	minPostHeight = 3
)

// detailLayout places the header, post, comment box, and footer on the
// detail screen. One blank line goes between each two blocks.
type detailLayout struct {
	// header is empty when the diff shows, to give the code more rows.
	header string
	footer string

	// postHeight is the outer height of the post area, frame included.
	postHeight  int
	composeRows int
}

func (m Model) detailLayout() detailLayout {
	footer := m.footer()
	frame := m.frameHeight()
	c := m.compose()
	blocks := 2
	free := m.height - lipgloss.Height(footer)
	header := ""

	if loaded := m.loaded(); loaded == nil || loaded.diff == nil {
		header = m.frameBox(m.detailHeader(), 0)
		free -= lipgloss.Height(header)
		blocks++
	}

	if c != nil {
		blocks++
	}

	free -= blocks - 1
	rows := 0

	if c != nil {
		rows = max(1, min(commentRows, free-minPostHeight-frame))
		free -= rows + frame
	}

	return detailLayout{
		header:      header,
		footer:      footer,
		postHeight:  max(free, minPostHeight),
		composeRows: rows,
	}
}

func (m Model) panelStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.tone().frame).
		Padding(0, 1)
}

func (m Model) frameHeight() int {
	return m.panelStyle().GetVerticalFrameSize()
}

func (m Model) contentWidth() int {
	return max(1, m.width-m.panelStyle().GetHorizontalFrameSize())
}

func (m Model) frameBox(content string, height int) string {
	style := m.panelStyle().Width(m.width)

	if height > 0 {
		style = style.Height(height)
	}

	return style.Render(content)
}

// window is the visible slice that keeps the cursor on screen.
func window(cursor, count, height int) (int, int) {
	height = max(height, 1)

	if count <= height {
		return 0, count
	}

	start := max(cursor-height/2, 0)
	start = min(start, count-height)

	return start, start + height
}

// step moves the cursor by delta and keeps it in 0 to n-1.
func step(cursor, delta, n int) int {
	if n == 0 {
		return 0
	}

	return max(0, min(cursor+delta, n-1))
}
