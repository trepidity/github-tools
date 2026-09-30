package ui_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/trepidity/gh-triage/internal/github"
	"github.com/trepidity/gh-triage/internal/ui"
)

// fakeClient stands in for GitHub. Commands run synchronously in tests, so no locking.
type fakeClient struct {
	open       []github.Issue // GitHub's search index for the queue, newest first
	staleIndex bool           // closed issues stay in search results (GitHub's index lags)
	searches   int            // SearchIssues calls
	comments   []string       // "key body" for each AddComment call
	closes     []string       // "key reason" for each CloseIssue call
	commentErr error
	closeErr   error
	repos      []github.Repo
	hang       bool // CloseIssue blocks until the request's context ends, like a hung connection
}

func (f *fakeClient) SearchIssues(_ context.Context, _ string, before time.Time) ([]github.Issue, bool, error) {
	f.searches++
	var out []github.Issue
	for _, is := range f.open {
		if before.IsZero() || !is.CreatedAt.After(before) {
			out = append(out, is)
		}
	}
	if len(out) > 100 {
		return out[:100], true, nil
	}
	return out, false, nil
}

func (f *fakeClient) GetComments(context.Context, github.Repo, int) ([]github.Comment, error) {
	return nil, nil
}

func (f *fakeClient) AddComment(_ context.Context, r github.Repo, n int, body string) (github.Comment, error) {
	f.comments = append(f.comments, fmt.Sprintf("%s#%d %s", r, n, body))
	if f.commentErr != nil {
		return github.Comment{}, f.commentErr
	}
	return github.Comment{Author: "me", Body: body, CreatedAt: time.Now()}, nil
}

func (f *fakeClient) CloseIssue(ctx context.Context, r github.Repo, n int, reason github.CloseReason) error {
	f.closes = append(f.closes, fmt.Sprintf("%s#%d %s", r, n, reason))
	if f.hang {
		<-ctx.Done()
		return ctx.Err()
	}
	if f.closeErr != nil {
		return f.closeErr
	}
	if !f.staleIndex {
		f.open = slices.DeleteFunc(f.open, func(is github.Issue) bool { return is.Repo == r && is.Number == n })
	}
	return nil
}

func (f *fakeClient) ListRepos(context.Context) ([]github.Repo, error) { return f.repos, nil }

func mustRepo(t *testing.T, s string) github.Repo {
	t.Helper()
	r, err := github.ParseRepo(s)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// issues returns open issues o/r#from … o/r#to, newest first like GitHub's search order.
func issues(t *testing.T, from, to int) []github.Issue {
	repo := mustRepo(t, "o/r")
	newest := time.Now().Truncate(time.Second)
	var out []github.Issue
	for n := from; n <= to; n++ {
		out = append(out, github.Issue{Repo: repo, Number: n, Title: fmt.Sprintf("Title %d", n), Author: "someone", State: "open", CreatedAt: newest.Add(-time.Duration(n) * time.Minute)})
	}
	return out
}

func start(t *testing.T, f *fakeClient, opts ui.Options) tea.Model {
	t.Helper()
	var m tea.Model = ui.New(f, opts)
	m = run(t, m, m.Init())
	return send(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
}

func send(t *testing.T, m tea.Model, msg tea.Msg) tea.Model {
	t.Helper()
	m, cmd := m.Update(msg)
	return run(t, m, cmd)
}

// run executes cmd and every command it leads to, feeding each message back into the
// model: a synchronous stand-in for the Bubble Tea runtime.
func run(t *testing.T, m tea.Model, cmds ...tea.Cmd) tea.Model {
	t.Helper()
	pending := cmds
	for steps := 0; len(pending) > 0; steps++ {
		if steps > 10000 {
			t.Fatal("commands never settled")
		}
		c := pending[0]
		pending = pending[1:]
		if c == nil {
			continue
		}
		switch msg := c().(type) {
		case nil, tea.QuitMsg:
		case tea.BatchMsg:
			pending = append(pending, msg...)
		default:
			var next tea.Cmd
			m, next = m.Update(msg)
			pending = append(pending, next)
		}
	}
	return m
}

func press(t *testing.T, m tea.Model, keys ...string) tea.Model {
	t.Helper()
	for _, k := range keys {
		m = send(t, m, key(k))
	}
	return m
}

// pressOnly delivers keys without running the commands they return — a user typing
// faster than GitHub answers. Run the returned commands afterwards with run.
func pressOnly(m tea.Model, keys ...string) (tea.Model, []tea.Cmd) {
	var cmds []tea.Cmd
	for _, k := range keys {
		var cmd tea.Cmd
		m, cmd = m.Update(key(k))
		cmds = append(cmds, cmd)
	}
	return m, cmds
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "ctrl+s":
		return tea.KeyMsg{Type: tea.KeyCtrlS}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func lines(m tea.Model) []string { return strings.Split(ansi.Strip(m.View()), "\n") }

func text(m tea.Model) string { return ansi.Strip(m.View()) }

func header(m tea.Model) string { return lines(m)[0] }

func selectedRow(t *testing.T, m tea.Model) string {
	t.Helper()
	for _, l := range lines(m) {
		if strings.HasPrefix(l, "> ") {
			return l
		}
	}
	t.Fatalf("no selected row in:\n%s", strings.Join(lines(m), "\n"))
	return ""
}

func repeat(k string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = k
	}
	return out
}
