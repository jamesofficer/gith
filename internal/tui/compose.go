package tui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jamesofficer/gith/internal/github"
)

// compose is the comment box under the open item.
type compose struct {
	box     textarea.Model
	sending bool
	problem string

	// target is where the comment goes, or nil for the whole item.
	target target
}

// target is *codeTarget or *replyTarget.
type target interface{ isTarget() }

// codeTarget is a new comment on lines of the diff.
type codeTarget struct{ comment github.CodeComment }

// replyTarget is a reply to a code comment. A sent reply goes into thread.
type replyTarget struct {
	author string
	thread *github.CodeThread
}

func (*codeTarget) isTarget()  {}
func (*replyTarget) isTarget() {}

// draftKey finds the unsent text of a comment box. place is empty for a
// comment on the whole item.
type draftKey struct {
	item  itemKey
	place string
}

func (c *compose) draftKey(item itemKey) draftKey {
	switch t := c.target.(type) {
	case *codeTarget:
		return draftKey{item: item, place: fmt.Sprintf("%s:%v:%v", t.comment.Path, t.comment.Start, t.comment.End)}

	case *replyTarget:
		return draftKey{item: item, place: fmt.Sprintf("reply:%d", t.thread.ID)}
	}

	return draftKey{item: item}
}

func (m Model) startCompose(target target) (Model, tea.Cmd) {
	loaded := m.loaded()

	if loaded == nil {
		return m, nil
	}

	box := textarea.New()
	box.Prompt = ""
	box.ShowLineNumbers = false
	box.Placeholder = "Write a comment"
	box.EndOfBufferCharacter = ' '
	box.KeyMap.InsertNewline.SetKeys(append(box.KeyMap.InsertNewline.Keys(), "shift+enter")...)
	box.SetStyles(m.commentStyles())

	switch t := target.(type) {
	case *codeTarget:
		box.Placeholder = "Write a comment on " + t.comment.Path + " " + linesName(t.comment)

	case *replyTarget:
		box.Placeholder = fmt.Sprintf("Write a reply to %s on %s line %d", t.author, t.thread.Path, t.thread.Line)
	}

	c := &compose{box: box, target: target}
	c.box.SetValue(m.drafts[c.draftKey(m.open.key)])
	loaded.compose = c

	return m, c.box.Focus()
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
		m.saveDraft(c)
		m.loaded().compose = nil

		return m, nil
	}

	c.problem = ""
	var cmd tea.Cmd
	c.box, cmd = c.box.Update(msg)

	return m, cmd
}

func (m Model) saveDraft(c *compose) {
	key := c.draftKey(m.open.key)

	if strings.TrimSpace(c.box.Value()) == "" {
		delete(m.drafts, key)

		return
	}

	m.drafts[key] = c.box.Value()
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
	source, key, req, target := m.source, m.open.key, m.open.req, c.target

	return m, tea.Batch(m.spin.Tick, request(func(ctx context.Context) tea.Msg {
		msg := commentMsg{req: req, key: key}

		switch t := target.(type) {
		case *codeTarget:
			code := t.comment
			code.Body = body
			msg.post, msg.err = source.CommentOnCode(ctx, key.number, code)

		case *replyTarget:
			msg.reply, msg.err = source.ReplyToCode(ctx, key.number, t.thread.ID, body)

		default:
			msg.post, msg.err = source.Comment(ctx, key.number, body)
		}

		return msg
	}))
}

// applyComment adds a sent comment to the thread. A comment on the whole
// item shows at once. A comment on code keeps the diff on screen.
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

	delete(m.drafts, c.draftKey(msg.key))
	loaded.compose = nil
	loaded.posts = nil

	if reply, ok := c.target.(*replyTarget); ok {
		reply.thread.Replies = append(reply.thread.Replies, msg.reply)
		m.cache[msg.key] = loaded.detail

		if diff, ok := loaded.diff.(*diffLoaded); ok {
			diff.threads = nil
		}

		return m, nil
	}

	loaded.detail.Thread = append(loaded.detail.Thread, msg.post)

	if diff, ok := loaded.diff.(*diffLoaded); ok {
		diff.selecting = false
		m.cache[msg.key] = loaded.detail

		return m, nil
	}

	loaded.detail.Comments++
	loaded.index = len(loaded.detail.Thread)
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
	hint := dim.Render("enter or shift+enter new line   ctrl+s send   esc close")

	if c.sending {
		hint = m.spin.View() + dim.Render(" Sending…")
	}

	line := fitLine(hint, m.width)

	if c.problem == "" {
		return line
	}

	return fitLine(lipgloss.NewStyle().Foreground(colors.bad).Render(c.problem), m.width) + "\n" + line
}
