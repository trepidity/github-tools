package ui_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/trepidity/gh-triage/internal/github"
	"github.com/trepidity/gh-triage/internal/ui"
)

const pollEvery = time.Minute

// ticker stands in for tea.Tick: it records each delay the model asks for and holds the
// pending poll until the test fires it.
type ticker struct {
	delays []time.Duration
	next   func(time.Time) tea.Msg
}

func (tk *ticker) tick(d time.Duration, fn func(time.Time) tea.Msg) tea.Cmd {
	tk.delays = append(tk.delays, d)
	tk.next = fn
	return nil
}

func (tk *ticker) take(t *testing.T) tea.Msg {
	t.Helper()
	if tk.next == nil {
		t.Fatal("no poll scheduled")
	}
	fn := tk.next
	tk.next = nil
	return fn(time.Now())
}

// poll fires the scheduled poll and runs everything it leads to.
func (tk *ticker) poll(t *testing.T, m tea.Model) tea.Model {
	t.Helper()
	return send(t, m, tk.take(t))
}

func watching(t *testing.T, f *fakeClient) (tea.Model, *ticker) {
	t.Helper()
	tk := &ticker{}
	m := start(t, f, ui.Options{Query: ui.RepoQuery(mustRepo(t, "o/r")), WatchInterval: pollEvery, Tick: tk.tick})
	return m, tk
}

func marked(rows []string) int {
	n := 0
	for _, r := range rows {
		if strings.Contains(r, "●") {
			n++
		}
	}
	return n
}

// Protects: an issue with activity after the query opened moves above the queue, marked
// until viewed; activity from before the query opened moves nothing; the cursor stays on
// the issue it was on.
func TestWatch_promotes_activity_since_the_query_opened_and_keeps_the_selection(t *testing.T) {
	f := &fakeClient{open: issues(t, 1, 5)}
	f.open[1].UpdatedAt = time.Now().Add(-time.Second) // #2: before the query opened
	m, tk := watching(t, f)
	m = press(t, m, "j") // on #2

	f.open[3].UpdatedAt = time.Now().Add(time.Second) // #4: someone comments
	m = tk.poll(t, m)

	rows := listRows(lines(m))
	if !strings.Contains(rows[0], "#4 ") || !strings.Contains(rows[0], "●") || !strings.Contains(rows[1], "#1 ") || marked(rows) != 1 {
		t.Fatalf("rows after #4's activity:\n%s", strings.Join(rows, "\n"))
	}
	if row := selectedRow(t, m); !strings.Contains(row, "#2 ") {
		t.Fatalf("selected %q, want #2 still", row)
	}

	m = press(t, m, "k", "k", "enter", "esc") // view #4
	if rows := listRows(lines(m)); !strings.Contains(rows[0], "#4 ") || marked(rows) != 0 {
		t.Fatalf("rows after viewing #4:\n%s", strings.Join(rows, "\n"))
	}
}

// Protects: the user's own comment is not surfaced as new activity, but someone else's
// later activity on the same issue is.
func TestWatch_skips_the_users_own_changes_but_not_later_ones(t *testing.T) {
	f := &fakeClient{open: issues(t, 1, 3)}
	m, tk := watching(t, f)

	m = press(t, m, "j", "enter", "c", "on it", "ctrl+s", "esc") // comment on #2
	m = tk.poll(t, m)
	if rows := listRows(lines(m)); !strings.Contains(rows[0], "#1 ") || marked(rows) != 0 {
		t.Fatalf("own comment promoted #2:\n%s", strings.Join(rows, "\n"))
	}

	f.open[1].UpdatedAt = time.Now().Add(time.Minute) // the reporter replies
	m = tk.poll(t, m)
	if rows := listRows(lines(m)); !strings.Contains(rows[0], "#2 ") || marked(rows) != 1 {
		t.Fatalf("reply did not promote #2:\n%s", strings.Join(rows, "\n"))
	}
}

// Protects: GitHub's search index lags, so a poll can return a copy older than an edit the
// user just made; the edit stays.
func TestWatch_keeps_a_local_edit_over_an_older_copy_from_the_index(t *testing.T) {
	f := &fakeClient{open: issues(t, 1, 3), labels: []string{"bug"}}
	m, tk := watching(t, f)
	f.open[1].UpdatedAt = time.Now().Add(time.Second)
	m = tk.poll(t, m) // #2 promoted, unlabeled

	m = press(t, m, "k", "enter", "l", " ", "enter", "esc") // label #2; the index still has the old copy
	m = tk.poll(t, m)
	if rows := listRows(lines(m)); !strings.Contains(rows[0], "#2 ") || !strings.Contains(rows[0], "bug") {
		t.Fatalf("label lost after poll:\n%s", strings.Join(rows, "\n"))
	}
}

// Protects: new activity on the issue being read reloads its thread, so the new comment
// shows rather than the thread cached when it was opened.
func TestWatch_reloads_the_thread_of_the_issue_on_screen(t *testing.T) {
	f := &fakeClient{open: issues(t, 1, 3), thread: []github.Comment{{Author: "a", Body: "first", CreatedAt: time.Now()}}}
	m, tk := watching(t, f)
	m = press(t, m, "enter")

	f.thread = append(f.thread, github.Comment{Author: "b", Body: "second thoughts", CreatedAt: time.Now()})
	f.open[0].UpdatedAt, f.open[0].Comments = time.Now().Add(time.Second), 2
	m = tk.poll(t, m)
	if !strings.Contains(text(m), "second thoughts") {
		t.Fatalf("new comment not shown:\n%s", text(m))
	}
}

// Protects: a burst of activity larger than one search page is surfaced in full, not cut
// at the first 100.
func TestWatch_surfaces_every_change_in_a_burst_larger_than_a_page(t *testing.T) {
	f := &fakeClient{open: issues(t, 1, 250)}
	m, tk := watching(t, f)
	m = send(t, m, tea.WindowSizeMsg{Width: 120, Height: 300})
	now := time.Now()
	for i := range 150 {
		f.open[i].UpdatedAt = now.Add(time.Duration(i+1) * time.Second)
	}

	m = tk.poll(t, m)
	if n := marked(listRows(lines(m))); n != 150 {
		t.Fatalf("%d issues marked, want 150", n)
	}
}

// Protects: paging continues past a page whose issues a poll had already surfaced; counting
// only never-seen issues as progress would end the queue there.
func TestWatch_paging_continues_past_a_page_the_poll_already_surfaced(t *testing.T) {
	f := &fakeClient{open: issues(t, 1, 250)}
	m, tk := watching(t, f)
	for i := 100; i < 200; i++ { // all of page 2
		f.open[i].UpdatedAt = time.Now().Add(time.Second)
	}
	m = tk.poll(t, m)

	m = press(t, m, repeat("j", 260)...)
	if h := header(m); !strings.Contains(h, "250 loaded") || strings.Contains(h, "loaded+") {
		t.Fatalf("header = %q, want all 250 loaded", h)
	}
}

// Protects: a poll that lands after the user switched queries does not leak the old
// query's issues into the new one.
func TestWatch_ignores_a_poll_that_lands_after_the_query_changed(t *testing.T) {
	other := issues(t, 9, 9)
	other[0].Repo = mustRepo(t, "o/s")
	f := &fakeClient{open: append(issues(t, 1, 3), other...), repos: []github.Repo{mustRepo(t, "o/r"), mustRepo(t, "o/s")}}
	m, tk := watching(t, f)
	f.open[1].UpdatedAt = time.Now().Add(time.Second)

	m, inFlight := m.Update(tk.take(t))
	m = press(t, m, "r", "o/s", "enter")
	m = run(t, m, inFlight)
	if v := text(m); strings.Contains(v, "Title 2") || !strings.Contains(v, "Title 9") {
		t.Fatalf("o/s queue after a late o/r poll:\n%s", v)
	}
}

// Protects: failed polls back off (rate limits reset on GitHub's clock, not ours) and the
// interval recovers once a poll succeeds.
func TestWatch_backs_off_after_failures_and_recovers(t *testing.T) {
	f := &fakeClient{open: issues(t, 1, 3), pollErr: errors.New("403 rate limited")}
	m, tk := watching(t, f)

	m = tk.poll(t, m)
	m = tk.poll(t, m)
	if !strings.Contains(text(m), "watch failed: 403 rate limited") {
		t.Fatalf("no failure shown:\n%s", text(m))
	}
	f.pollErr = nil
	m = tk.poll(t, m)
	if want := []time.Duration{pollEvery, 2 * pollEvery, 4 * pollEvery, pollEvery}; !reflect.DeepEqual(tk.delays, want) {
		t.Fatalf("delays = %v, want %v", tk.delays, want)
	}
	if strings.Contains(text(m), "watch failed") {
		t.Fatalf("failure still shown after a good poll:\n%s", text(m))
	}
}
