package tui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func newSearchBox() textinput.Model {
	box := textinput.New()
	box.Prompt = "/ "
	box.Placeholder = "title, author, assignee, or #number"

	return box
}

// startSearch puts the keys into the search line.
func (m Model) startSearch() (Model, tea.Cmd) {
	colors := m.tone()
	styles := textinput.DefaultStyles(m.dark)
	styles.Focused.Prompt = lipgloss.NewStyle().Foreground(colors.accent)
	styles.Blurred.Prompt = lipgloss.NewStyle().Foreground(colors.muted)
	styles.Focused.Placeholder = lipgloss.NewStyle().Foreground(colors.muted)
	styles.Cursor.Color = colors.accent
	styles.Cursor.Blink = false
	m.search.SetStyles(styles)

	return m, m.search.Focus()
}

// onSearchKey handles a key while you type in the search line. The arrows
// up and down still move in the list, and enter opens the selected item.
func (m Model) onSearchKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit

	case "esc":
		return m.clearSearch(), nil

	case "enter":
		m.search.Blur()

		return m.openSelected()

	case "up", "down":
		return m.onListKey(msg.String())
	}

	before := m.search.Value()
	var cmd tea.Cmd
	m.search, cmd = m.search.Update(msg)

	if m.search.Value() != before {
		words := strings.Fields(strings.ToLower(m.search.Value()))
		m.issues.filter(words)
		m.pulls.filter(words)
	}

	return m, cmd
}

// clearSearch closes the search line. The selected items stay selected.
func (m Model) clearSearch() Model {
	m.search.SetValue("")
	m.search.Blur()
	m.issues.filter(nil)
	m.pulls.filter(nil)

	return m
}

// searching tells if the search line shows.
func (m Model) searching() bool {
	return m.search.Focused() || m.search.Value() != ""
}
