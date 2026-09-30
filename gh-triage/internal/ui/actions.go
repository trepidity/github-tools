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

func newEditor() textarea.Model {
	ta := textarea.New()
	ta.Placeholder = "Leave a comment (ctrl+s send · esc cancel)"
	ta.ShowLineNumbers = false
	ta.CharLimit = 65536 // GitHub's comment limit
	ta.SetHeight(editorHeight)
	ta.Cursor.SetMode(cursor.CursorStatic)
	return ta
}

// actionDoneMsg carries a finished action back to the UI goroutine.
type actionDoneMsg struct {
	label string
	apply func(*Model) tea.Cmd // what GitHub confirmed; nil if nothing
	err   error                // what failed; nil if nothing
}

// run starts an action: it sets busy, applies the request timeout and calls do off the UI
// goroutine. do returns apply for whatever GitHub confirmed and err for whatever failed —
// both may be set (partial success). apply is the only place an action changes state.
func (m *Model) run(label string, do func(ctx context.Context) (func(*Model) tea.Cmd, error)) tea.Cmd {
	m.busy, m.status = true, label+"…"
	timeout := m.timeout()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		apply, err := do(ctx)
		return actionDoneMsg{label: label, apply: apply, err: err}
	}
}

func (m *Model) onActionDone(msg actionDoneMsg) tea.Cmd {
	m.busy, m.status = false, ""
	var cmd tea.Cmd
	if msg.apply != nil {
		cmd = msg.apply(m)
	}
	if msg.err != nil {
		failed := msg.label + " failed: " + firstLine(msg.err.Error())
		if m.status != "" {
			failed = m.status + "; " + failed
		}
		m.status = failed
	}
	return cmd
}

func (m *Model) startComment(thenClose bool) tea.Cmd {
	is, ok := m.current()
	if !ok {
		return nil
	}
	if thenClose && m.isClosed(is) {
		m.status = "already closed"
		return nil
	}
	m.mode = modeComment
	m.closeAfterComment = thenClose
	m.editor.Reset()
	m.editor.SetValue(m.drafts[is.Key()])
	return m.editor.Focus()
}

func (m *Model) keyComment(k tea.KeyMsg) tea.Cmd {
	switch k.String() {
	case "esc":
		is, _ := m.current()
		if v := m.editor.Value(); strings.TrimSpace(v) != "" {
			m.drafts[is.Key()] = v
		} else {
			delete(m.drafts, is.Key())
		}
		m.mode = modeNone
		m.editor.Blur()
		return nil
	case "ctrl+t":
		return m.openTemplates()
	case "ctrl+e":
		return m.openEditor()
	case "ctrl+s":
		body := strings.TrimSpace(m.editor.Value())
		if body == "" {
			m.status = "comment is empty"
			return nil
		}
		is, _ := m.current()
		client := m.client
		return m.run("comment", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
			c, err := client.AddComment(ctx, is.Repo, is.Number, body)
			if err != nil {
				return nil, err // the draft stays in the editor
			}
			return func(m *Model) tea.Cmd { return m.commentPosted(is.Key(), c) }, nil
		})
	}
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(k)
	return cmd
}

func (m *Model) commentPosted(key string, c github.Comment) tea.Cmd {
	m.recordComment(key, c)
	delete(m.drafts, key)
	m.editor.Reset()
	m.editor.Blur()
	m.status = "commented"
	m.viewport.GotoBottom()
	if m.closeAfterComment {
		m.mode = modeCloseReason
		return nil
	}
	m.mode = modeNone
	return nil
}

// recordComment adds a comment GitHub accepted to the thread and the row's count.
func (m *Model) recordComment(key string, c github.Comment) {
	if cs, loaded := m.comments[key]; loaded {
		m.comments[key] = append(cs, c)
	}
	if i := m.indexOf(key); i >= 0 {
		m.issues[i].Comments++
	}
	m.count(actCommented)
	m.refreshIssue()
}

func (m *Model) startClose() tea.Cmd {
	is, ok := m.current()
	if !ok {
		return nil
	}
	if m.isClosed(is) {
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
	m.mode = modeNone
	is, _ := m.current()
	client := m.client
	return m.run("close", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
		if err := client.CloseIssue(ctx, is.Repo, is.Number, reason); err != nil {
			return nil, err
		}
		return func(m *Model) tea.Cmd { return m.markClosed(is) }, nil
	})
}

// markClosed records a close GitHub confirmed and advances to the next issue.
func (m *Model) markClosed(is github.Issue) tea.Cmd {
	m.stateOverride[is.Key()] = "closed"
	m.count(actClosed)
	var cmd tea.Cmd
	if m.cursor+1 < len(m.visible) {
		cmd = m.openIssue(m.cursor + 1)
	}
	m.status = "closed " + is.Key()
	return cmd
}
