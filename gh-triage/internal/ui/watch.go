package ui

import (
	"context"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/trepidity/gh-triage/internal/github"
)

// Watching reads the current query's update-time feed while gh triage is open and moves
// issues with new activity above the created-time queue. It is a change feed, not a
// snapshot: only activity after the query opened counts, the user's own changes are
// skipped, and a copy older than the one the queue holds never replaces it.

const (
	// indexLag is how far back each poll re-reads the feed: GitHub's search index can list
	// an update after it has already listed a later one.
	indexLag = 2 * time.Minute
	// clockSkew allows for the local clock trailing GitHub's when deciding whether an
	// update is one of ours.
	clockSkew = 10 * time.Second
	// maxBackoff caps how many times failed polls double the interval (16×).
	maxBackoff = 4

	watchFailed = "watch failed: "
)

type (
	watchTickMsg   struct{}
	watchLoadedMsg struct {
		gen     int
		since   time.Time
		issues  []github.Issue
		hasMore bool
		err     error
	}
)

// scheduleWatch starts the wait for the next poll. Exactly one tick or poll is ever
// pending, so polls never overlap.
func (m *Model) scheduleWatch() tea.Cmd {
	if m.opts.WatchInterval <= 0 {
		return nil
	}
	tick := m.opts.Tick
	if tick == nil {
		tick = tea.Tick
	}
	d := m.opts.WatchInterval << min(m.watchFailures, maxBackoff)
	return tick(d, func(time.Time) tea.Msg { return watchTickMsg{} })
}

func (m *Model) onWatchTick() tea.Cmd {
	if m.query == "" { // the repo picker is up: nothing to watch yet
		return m.scheduleWatch()
	}
	return m.poll(m.watchSince.Add(-indexLag))
}

func (m *Model) poll(since time.Time) tea.Cmd {
	client, gen, query := m.client, m.gen, m.query
	return m.fetch(func(ctx context.Context) tea.Msg {
		issues, hasMore, err := client.SearchUpdatedIssues(ctx, query, since)
		return watchLoadedMsg{gen: gen, since: since, issues: issues, hasMore: hasMore, err: err}
	})
}

func (m *Model) onWatchLoaded(msg watchLoadedMsg) tea.Cmd {
	if msg.gen != m.gen { // the query changed while this poll was out
		return m.scheduleWatch()
	}
	if msg.err != nil {
		m.watchFailures++
		if m.status == "" || strings.HasPrefix(m.status, watchFailed) {
			m.status = watchFailed + firstLine(msg.err.Error())
		}
		return m.scheduleWatch()
	}
	m.watchFailures = 0
	if strings.HasPrefix(m.status, watchFailed) {
		m.status = ""
	}
	surfaced := false
	for _, fresh := range msg.issues {
		if fresh.UpdatedAt.After(m.watchSince) {
			m.watchSince = fresh.UpdatedAt
		}
		surfaced = m.surface(fresh) || surfaced
	}
	var cmd tea.Cmd
	if surfaced {
		slices.SortStableFunc(m.issueOrder, func(a, b int) int { // queue order holds among the rest
			return m.activity[m.issues[b].Key()].Compare(m.activity[m.issues[a].Key()])
		})
		m.applyFilter()
		if is, ok := m.current(); ok && m.screen == screenIssue {
			m.refreshIssue()
			cmd = m.loadComments(is) // a no-op unless surface dropped its thread
		}
	}
	if n := len(msg.issues); msg.hasMore && n > 0 && msg.issues[n-1].UpdatedAt.After(msg.since) {
		return tea.Batch(cmd, m.poll(msg.issues[n-1].UpdatedAt)) // the rest of a burst larger than a page
	}
	return tea.Batch(cmd, m.scheduleWatch())
}

// surface applies one feed entry and reports whether it is activity to show.
func (m *Model) surface(fresh github.Issue) bool {
	key := fresh.Key()
	switch {
	case !fresh.UpdatedAt.After(m.watchFrom): // before the query opened
		return false
	case !fresh.UpdatedAt.After(m.touched[key].Add(clockSkew)): // our own change
		return false
	case !fresh.UpdatedAt.After(m.activity[key]): // already surfaced; polls overlap by indexLag
		return false
	}
	m.activity[key] = fresh.UpdatedAt
	if is, ok := m.current(); !ok || m.screen != screenIssue || is.Key() != key {
		m.unseen[key] = true
	}
	i, ok := m.byKey[key]
	switch {
	case !ok:
		m.appendIssue(fresh)
	case fresh.UpdatedAt.After(m.issues[i].UpdatedAt): // otherwise paging already loaded this copy or a newer one
		m.issues[i] = fresh
		for name, c := range fresh.LabelColors {
			m.labelColors[strings.ToLower(name)] = c
		}
		delete(m.comments, key)
		delete(m.commentsErr, key)
	}
	return true
}
