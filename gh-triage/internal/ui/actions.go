package ui

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/trepidity/gh-triage/internal/github"
)

const editorHeight = 6

type (
	commentPostedMsg struct {
		key     string
		comment github.Comment
		err     error
	}
	closedMsg struct {
		key string
		err error
	}
)

func newEditor() textarea.Model {
	ta := textarea.New()
	ta.Placeholder = "Leave a comment (ctrl+s send · esc cancel)"
	ta.ShowLineNumbers = false
	ta.CharLimit = 65536 // GitHub's comment limit
	ta.SetHeight(editorHeight)
	ta.Cursor.SetMode(cursor.CursorStatic)
	return ta
}

func (m *Model) startComment(thenClose bool) tea.Cmd {
	if _, ok := m.current(); !ok {
		return nil
	}
	m.mode = modeComment
	m.closeAfterComment = thenClose
	m.editor.Reset()
	return m.editor.Focus()
}

func (m *Model) keyComment(k tea.KeyMsg) tea.Cmd {
	switch k.String() {
	case "esc":
		m.mode = modeNone
		m.editor.Blur()
		return nil
	case "ctrl+s":
		body := strings.TrimSpace(m.editor.Value())
		if body == "" {
			m.status = "comment is empty"
			return nil
		}
		is, _ := m.current()
		m.busy, m.status = true, "posting comment…"
		client := m.client
		return func() tea.Msg {
			c, err := client.AddComment(context.Background(), is.Repo, is.Number, body)
			return commentPostedMsg{key: is.Key(), comment: c, err: err}
		}
	}
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(k)
	return cmd
}

func (m *Model) onCommentPosted(msg commentPostedMsg) {
	m.busy = false
	if msg.err != nil {
		m.status = "comment failed: " + msg.err.Error() // draft stays in the editor
		return
	}
	if cs, loaded := m.comments[msg.key]; loaded {
		m.comments[msg.key] = append(cs, msg.comment)
	}
	if i := m.indexOf(msg.key); i >= 0 {
		m.issues[i].Comments++
	}
	m.editor.Reset()
	m.editor.Blur()
	m.status = "commented"
	m.refreshIssue()
	m.viewport.GotoBottom()
	if m.closeAfterComment {
		m.mode = modeCloseReason
		return
	}
	m.mode = modeNone
}

func (m *Model) startClose() tea.Cmd {
	is, ok := m.current()
	if !ok {
		return nil
	}
	if m.closed[is.Key()] {
		m.status = "already closed"
		return nil
	}
	m.mode = modeCloseReason
	return nil
}

func (m *Model) keyCloseReason(k tea.KeyMsg) tea.Cmd {
	var reason github.CloseReason
	switch k.String() {
	case "c":
		reason = github.Completed
	case "n":
		reason = github.NotPlanned
	case "esc":
		m.mode = modeNone
		return nil
	default:
		return nil
	}
	is, _ := m.current()
	m.busy, m.status = true, "closing…"
	client := m.client
	return func() tea.Msg {
		return closedMsg{key: is.Key(), err: client.CloseIssue(context.Background(), is.Repo, is.Number, reason)}
	}
}

func (m *Model) onClosed(msg closedMsg) tea.Cmd {
	m.busy, m.mode = false, modeNone
	if msg.err != nil {
		m.status = "close failed: " + msg.err.Error()
		return nil
	}
	m.closed[msg.key] = true
	var cmd tea.Cmd
	if m.cursor+1 < len(m.visible) {
		cmd = m.openIssue(m.cursor + 1)
	}
	m.status = "closed " + msg.key
	return cmd
}
