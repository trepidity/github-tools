package ui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/trepidity/gh-triage/internal/github"
)

func (m *Model) startDuplicate() tea.Cmd {
	is, ok := m.writable()
	if !ok {
		return nil
	}
	if m.isClosed(is) {
		m.status = "already closed"
		return nil
	}
	if ref, done := m.dupCommented[is.Key()]; done {
		return m.closeAsDuplicate(is, ref) // the comment already posted; only close
	}
	m.mode, m.status = modeDupRef, ""
	m.ref.SetValue("")
	return m.ref.Focus()
}

func (m *Model) keyDupRef(k tea.KeyMsg) tea.Cmd {
	switch k.String() {
	case "esc":
		m.mode = modeNone
		m.ref.Blur()
		return nil
	case "enter":
		is, _ := m.current()
		repo, n, err := github.ParseIssueRef(m.ref.Value(), is.Repo)
		if err != nil {
			m.status = err.Error()
			return nil
		}
		if repo == is.Repo && n == is.Number {
			m.status = is.Key() + " can't be a duplicate of itself"
			return nil
		}
		m.mode = modeNone
		m.ref.Blur()
		ref := fmt.Sprintf("#%d", n)
		if repo != is.Repo {
			ref = fmt.Sprintf("%s#%d", repo, n)
		}
		return m.duplicate(is, ref)
	}
	var cmd tea.Cmd
	m.ref, cmd = m.ref.Update(k)
	return cmd
}

// duplicate comments "Duplicate of <ref>" then closes as duplicate. If only the comment
// lands, that part is applied and remembered so d retries only the close.
func (m *Model) duplicate(is github.Issue, ref string) tea.Cmd {
	client := m.client
	return m.run("duplicate", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
		c, err := client.AddComment(ctx, is.Repo, is.Number, "Duplicate of "+ref)
		if err != nil {
			return nil, err
		}
		if err := client.CloseIssue(ctx, is.Repo, is.Number, github.Duplicate); err != nil {
			return func(m *Model) tea.Cmd {
				m.recordComment(is.Key(), c)
				m.dupCommented[is.Key()] = ref
				m.status = "commented"
				return nil
			}, err
		}
		return func(m *Model) tea.Cmd {
			m.recordComment(is.Key(), c)
			return m.markClosed(is)
		}, nil
	})
}

func (m *Model) closeAsDuplicate(is github.Issue, ref string) tea.Cmd {
	client := m.client
	return m.run("close", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
		if err := client.CloseIssue(ctx, is.Repo, is.Number, github.Duplicate); err != nil {
			return nil, err
		}
		return func(m *Model) tea.Cmd {
			delete(m.dupCommented, is.Key())
			return m.markClosed(is)
		}, nil
	})
}

// undoClose reopens the most recent close this session. Comments posted with it stay.
func (m *Model) undoClose() tea.Cmd {
	if m.lastClose == nil {
		m.status = "nothing to undo"
		return nil
	}
	is := *m.lastClose
	client := m.client
	return m.run("reopen", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
		if err := client.ReopenIssue(ctx, is.Repo, is.Number); err != nil {
			return nil, err // stays closed; lastClose kept so u retries
		}
		return func(m *Model) tea.Cmd {
			m.stateOverride[is.Key()] = "open"
			m.lastClose = nil
			m.tally[actClosed]--
			m.status = "reopened " + is.Key()
			m.refreshIssue()
			return nil
		}, nil
	})
}
