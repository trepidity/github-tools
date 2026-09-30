package ui

import (
	"context"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/trepidity/gh-triage/internal/github"
)

// optionsLoadedMsg carries a repo's labels or assignable users.
type optionsLoadedMsg struct {
	kind pickerKind // pickLabels or pickAssignees
	repo github.Repo
	opts []string
	err  error
}

// writable is the current issue, if actions may change it.
func (m *Model) writable() (github.Issue, bool) {
	return m.current()
}

// updateIssue edits the loaded copy of an issue, if it is still in the queue.
func (m *Model) updateIssue(key string, edit func(*github.Issue)) {
	if i := m.indexOf(key); i >= 0 {
		edit(&m.issues[i])
	}
	m.refreshIssue()
}

// openOptions opens the label or assignee picker with the issue's current set checked.
func (m *Model) openOptions(kind pickerKind) tea.Cmd {
	is, ok := m.writable()
	if !ok {
		return nil
	}
	current, cache, noun := is.Labels, m.labelOpts, "labels"
	if kind == pickAssignees {
		current, cache, noun = is.Assignees, m.assigneeOpts, "assignees"
	}
	opts, cached := cache[is.Repo]
	p := newPicker(kind, noun+" · "+is.Key(), "filter: ", optionItems(opts, current))
	p.multi = true
	for _, v := range current {
		p.checked[v] = true
	}
	p.loading = !cached
	cmd := m.openPicker(p)
	if cached {
		return cmd
	}
	client, repo := m.client, is.Repo
	return tea.Batch(cmd, m.fetch(func(ctx context.Context) tea.Msg {
		var opts []string
		var err error
		if kind == pickLabels {
			opts, err = client.ListLabels(ctx, repo)
		} else {
			opts, err = client.ListAssignees(ctx, repo)
		}
		return optionsLoadedMsg{kind: kind, repo: repo, opts: opts, err: err}
	}))
}

// optionItems lists the repo's options, then any value the issue carries that the repo
// no longer lists, so applying the picker never strips it silently.
func optionItems(opts, current []string) []pickerItem {
	var items []pickerItem
	for _, v := range opts {
		items = append(items, pickerItem{label: v, value: v})
	}
	for _, v := range current {
		if !slices.Contains(opts, v) {
			items = append(items, pickerItem{label: v, value: v})
		}
	}
	return items
}

func (m *Model) onOptionsLoaded(msg optionsLoadedMsg) {
	open := m.mode == modePicker && m.pk.kind == msg.kind
	if open {
		m.pk.loading = false
	}
	if msg.err != nil {
		m.status = "loading options failed: " + firstLine(msg.err.Error())
		return
	}
	if msg.kind == pickLabels {
		m.labelOpts[msg.repo] = msg.opts
	} else {
		m.assigneeOpts[msg.repo] = msg.opts
	}
	if is, ok := m.current(); open && ok && is.Repo == msg.repo {
		current := is.Labels
		if msg.kind == pickAssignees {
			current = is.Assignees
		}
		m.pk.setItems(optionItems(msg.opts, current))
	}
}

func (m *Model) setLabels(want []string) tea.Cmd {
	is, _ := m.current()
	client := m.client
	return m.run("label", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
		got, err := client.SetLabels(ctx, is.Repo, is.Number, want)
		if err != nil {
			return nil, err
		}
		return func(m *Model) tea.Cmd {
			m.updateIssue(is.Key(), func(i *github.Issue) { i.Labels = got })
			status, changed := setResult("labels", is.Labels, want, got)
			if changed {
				m.count(actLabeled)
			}
			m.status = status
			return nil
		}, nil
	})
}

func (m *Model) setAssignees(want []string) tea.Cmd {
	is, _ := m.current()
	client := m.client
	return m.run("assign", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
		got, err := client.SetAssignees(ctx, is.Repo, is.Number, want)
		if err != nil {
			return nil, err
		}
		return func(m *Model) tea.Cmd { m.assigned(is, want, got); return nil }, nil
	})
}

// toggleSelf assigns or unassigns the signed-in user, learning who that is on first use.
func (m *Model) toggleSelf() tea.Cmd {
	is, ok := m.writable()
	if !ok {
		return nil
	}
	client, me := m.client, m.me
	return m.run("assign", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
		if me == "" {
			var err error
			if me, err = client.CurrentUser(ctx); err != nil {
				return nil, err
			}
		}
		want := slices.Clone(is.Assignees)
		if i := slices.Index(want, me); i >= 0 {
			want = slices.Delete(want, i, i+1)
		} else {
			want = append(want, me)
		}
		got, err := client.SetAssignees(ctx, is.Repo, is.Number, want)
		if err != nil {
			return func(m *Model) tea.Cmd { m.me = me; return nil }, err
		}
		return func(m *Model) tea.Cmd { m.me = me; m.assigned(is, want, got); return nil }, nil
	})
}

func (m *Model) assigned(is github.Issue, want, got []string) {
	m.updateIssue(is.Key(), func(i *github.Issue) { i.Assignees = got })
	status, changed := setResult("assigned", is.Assignees, want, got)
	if changed {
		m.count(actAssigned)
	}
	m.status = status
}

// setResult describes the set GitHub returned, naming any requested value it ignored, and
// reports whether the set differs from before. Names compare case-insensitively.
func setResult(verb string, before, want, got []string) (string, bool) {
	status := verb + ": " + strings.Join(got, ", ")
	if len(got) == 0 {
		status = verb + ": none"
	}
	if ignored := without(want, got); len(ignored) > 0 {
		status += " · ignored: " + strings.Join(ignored, ", ")
	}
	return status, len(without(before, got)) > 0 || len(without(got, before)) > 0
}

// without is every value in a that is not in b.
func without(a, b []string) []string {
	var out []string
	for _, x := range a {
		if !slices.ContainsFunc(b, func(y string) bool { return strings.EqualFold(x, y) }) {
			out = append(out, x)
		}
	}
	return out
}

func (m *Model) openReactions() tea.Cmd {
	is, ok := m.writable()
	if !ok {
		return nil
	}
	var items []pickerItem
	for _, r := range github.AllReactions() {
		items = append(items, pickerItem{label: r.Emoji() + " " + r.String(), value: r.String()})
	}
	return m.openPicker(newPicker(pickReaction, "react to "+is.Key(), "reaction: ", items))
}

func (m *Model) react(r github.Reaction) tea.Cmd {
	is, _ := m.current()
	client := m.client
	return m.run("react", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
		if err := client.AddReaction(ctx, is.Repo, is.Number, r); err != nil {
			return nil, err
		}
		return func(m *Model) tea.Cmd {
			m.count(actReacted)
			m.status = "reacted " + r.Emoji()
			return nil
		}, nil
	})
}
