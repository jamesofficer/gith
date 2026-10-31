package tui

import (
	"context"
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// compose is the comment box under the open item.
type compose struct {
	box     textarea.Model
	sending bool
	problem string
}

func (m Model) startCompose() (Model, tea.Cmd) {
	loaded := m.loaded()

	if loaded == nil {
		return m, nil
	}

	box := textarea.New()
	box.Prompt = ""
	box.ShowLineNumbers = false
	box.Placeholder = "Write a comment"
	box.EndOfBufferCharacter = ' '
	box.SetStyles(m.commentStyles())
	box.SetValue(m.drafts[m.open.key])
	loaded.compose = &compose{box: box}

	return m, loaded.compose.box.Focus()
}

func (m Model) onComposeKey(c *compose, msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit

	case "ctrl+s", "ctrl+enter":
		return m.sendComment(c)
	}

	if c.sending {
		return m, nil
	}

	if msg.String() == "esc" {
		m.saveDraft(c.box.Value())
		m.loaded().compose = nil

		return m, nil
	}

	c.problem = ""
	var cmd tea.Cmd
	c.box, cmd = c.box.Update(msg)

	return m, cmd
}

func (m Model) saveDraft(text string) {
	if strings.TrimSpace(text) == "" {
		delete(m.drafts, m.open.key)

		return
	}

	m.drafts[m.open.key] = text
}

func (m Model) sendComment(c *compose) (Model, tea.Cmd) {
	if c.sending {
		return m, nil
	}

	body := strings.TrimSpace(c.box.Value())

	if body == "" {
		c.problem = "Write a comment first."

		return m, nil
	}

	c.sending = true
	c.problem = ""
	c.box.Blur()
	source, key, req := m.source, m.open.key, m.open.req

	return m, tea.Batch(m.spin.Tick, request(func(ctx context.Context) tea.Msg {
		post, err := source.Comment(ctx, key.number, body)

		return commentMsg{req: req, key: key, post: post, err: err}
	}))
}

func (m Model) applyComment(msg commentMsg) (Model, tea.Cmd) {
	loaded := m.loaded()

	if loaded == nil || m.open.req != msg.req || loaded.compose == nil || !loaded.compose.sending {
		return m, nil
	}

	c := loaded.compose
	c.sending = false

	if msg.err != nil {
		c.problem = "Could not send this comment. " + msg.err.Error()

		return m, c.box.Focus()
	}

	loaded.compose = nil
	loaded.detail.Thread = append(loaded.detail.Thread, msg.post)
	loaded.detail.Comments++
	loaded.posts = nil
	loaded.index = len(loaded.detail.Thread)
	delete(m.drafts, msg.key)
	m.cache[msg.key] = loaded.detail
	m.viewport.SetYOffset(0)

	return m.setCommentCount(msg.key, loaded.detail.Comments), nil
}

func (m Model) commentStyles() textarea.Styles {
	styles := textarea.DefaultStyles(m.dark)
	plain := lipgloss.NewStyle()
	muted := lipgloss.NewStyle().Foreground(m.tone().muted)
	styles.Focused.Base = plain
	styles.Blurred.Base = plain
	styles.Focused.CursorLine = plain
	styles.Blurred.CursorLine = plain
	styles.Focused.Placeholder = muted
	styles.Blurred.Placeholder = muted
	styles.Cursor.Color = m.tone().accent
	styles.Cursor.Blink = false

	return styles
}

func (m Model) commentBox(c *compose) string {
	return m.panelStyle().
		BorderForeground(m.tone().accent).
		Width(m.width).
		Render(c.box.View())
}

func (m Model) composeFooter(c *compose) string {
	colors := m.tone()
	dim := lipgloss.NewStyle().Foreground(colors.muted)
	hint := dim.Render("enter new line   ctrl+s send   esc close")

	if c.sending {
		hint = m.spin.View() + dim.Render(" Sending…")
	}

	line := fitLine(hint, m.width)

	if c.problem == "" {
		return line
	}

	return fitLine(lipgloss.NewStyle().Foreground(colors.bad).Render(c.problem), m.width) + "\n" + line
}
