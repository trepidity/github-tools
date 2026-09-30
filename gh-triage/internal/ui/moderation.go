package ui

import (
	"context"
	"fmt"
	"strings"

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

// confirmation is a y/n question; yes runs on y.
type confirmation struct {
	question string
	yes      func(*Model) tea.Cmd
}

func (m *Model) ask(question string, yes func(*Model) tea.Cmd) tea.Cmd {
	m.confirm = confirmation{question: question, yes: yes}
	m.mode, m.status = modeConfirm, ""
	return nil
}

func (m *Model) keyConfirm(k tea.KeyMsg) tea.Cmd {
	switch k.String() {
	case "y":
		m.mode = modeNone
		return m.confirm.yes(m)
	case "n", "esc":
		m.mode = modeNone
	}
	return nil
}

func (m *Model) startLock() tea.Cmd {
	is, ok := m.writable()
	if !ok {
		return nil
	}
	if is.Locked {
		return m.ask("unlock "+is.Key()+"?", func(m *Model) tea.Cmd { return m.unlock(is) })
	}
	var items []pickerItem
	for _, r := range github.AllLockReasons() {
		items = append(items, pickerItem{label: r.String(), value: r.String()})
	}
	return m.openPicker(newPicker(pickLock, "lock "+is.Key()+" as", "reason: ", items))
}

func (m *Model) lock(is github.Issue, reason github.LockReason) tea.Cmd {
	client := m.client
	return m.run("lock", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
		if err := client.Lock(ctx, is.Repo, is.Number, reason); err != nil {
			return nil, err
		}
		return func(m *Model) tea.Cmd {
			m.updateIssue(is.Key(), func(i *github.Issue) { i.Locked = true })
			m.count(actLocked)
			m.status = "locked as " + reason.String()
			return nil
		}, nil
	})
}

func (m *Model) unlock(is github.Issue) tea.Cmd {
	client := m.client
	return m.run("unlock", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
		if err := client.Unlock(ctx, is.Repo, is.Number); err != nil {
			return nil, err
		}
		return func(m *Model) tea.Cmd {
			m.updateIssue(is.Key(), func(i *github.Issue) { i.Locked = false })
			m.count(actUnlocked)
			m.status = "unlocked"
			return nil
		}, nil
	})
}

func (m *Model) startTransfer() tea.Cmd {
	is, ok := m.writable()
	if !ok {
		return nil
	}
	return m.openRepoPicker(pickTransfer, "transfer "+is.Key()+" to")
}

func (m *Model) confirmTransfer(to github.Repo) tea.Cmd {
	is, _ := m.current()
	if strings.EqualFold(to.String(), is.Repo.String()) {
		m.status = is.Key() + " is already in " + to.String()
		return nil
	}
	return m.ask(fmt.Sprintf("transfer %s to %s? this cannot be undone", is.Key(), to), func(m *Model) tea.Cmd {
		return m.transfer(is, to)
	})
}

// transfer moves the issue; like a close, the row stays (marked moved) and the view advances.
func (m *Model) transfer(is github.Issue, to github.Repo) tea.Cmd {
	client := m.client
	return m.run("transfer", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
		url, err := client.TransferIssue(ctx, is, to)
		if err != nil {
			return nil, err
		}
		return func(m *Model) tea.Cmd {
			m.transferred[is.Key()] = url
			m.count(actTransferred)
			var cmd tea.Cmd
			if m.cursor+1 < len(m.visible) {
				cmd = m.openIssue(m.cursor + 1)
			}
			m.status = "moved " + is.Key() + " → " + url
			return cmd
		}, nil
	})
}
