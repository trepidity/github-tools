package ui_test

import (
	"strings"
	"testing"

	"github.com/trepidity/gh-triage/internal/ui"
)

var repoOpts = ui.Options{Query: "repo:o/r"}

// Protects: the navigation requirement — n/p move through the queue and stop at its ends.
func TestNextAndPrevious_stop_at_the_ends_of_the_queue(t *testing.T) {
	f := &fakeClient{open: issues(t, 1, 3)}
	m := start(t, f, repoOpts)

	m = press(t, m, "enter", "p")
	if !strings.HasPrefix(header(m), "o/r#1 ") || !strings.Contains(text(m), "start of queue") {
		t.Fatalf("after p on first issue: header %q", header(m))
	}
	m = press(t, m, "n", "n", "n")
	if !strings.HasPrefix(header(m), "o/r#3 ") || !strings.Contains(text(m), "end of queue") {
		t.Fatalf("after n past last issue: header %q", header(m))
	}
}

// Protects: the core browser pain point — going back lands on the issue you were viewing.
func TestBackToList_puts_the_cursor_on_the_issue_just_viewed(t *testing.T) {
	f := &fakeClient{open: issues(t, 1, 5)}
	m := start(t, f, repoOpts)

	m = press(t, m, "enter", "n", "n", "esc")
	if row := selectedRow(t, m); !strings.Contains(row, "o/r#3 ") {
		t.Fatalf("selected row after esc = %q, want o/r#3", row)
	}
}

// Protects: paging state — nearing the end requests the next page once, even when
// keys arrive faster than GitHub answers, and never asks past the last page.
func TestNearingTheEnd_fetches_the_next_page_exactly_once(t *testing.T) {
	f := &fakeClient{open: issues(t, 1, 150)}
	m := start(t, f, repoOpts)

	m, cmds := pressOnly(m, repeat("j", 95)...)
	m = run(t, m, cmds...)
	m = press(t, m, repeat("j", 60)...)

	if f.searches != 2 {
		t.Fatalf("searches = %d, want 2", f.searches)
	}
	if row := selectedRow(t, m); !strings.Contains(row, "o/r#150 ") {
		t.Fatalf("selected row = %q, want the last issue o/r#150", row)
	}
}

// Protects (Review Focus 1): an empty queue must not crash on open or navigation keys.
func TestEmptyQueue_ignores_navigation_and_says_so(t *testing.T) {
	f := &fakeClient{}
	m := start(t, f, repoOpts)

	m = press(t, m, "enter", "n", "p", "j", "k")
	if !strings.Contains(text(m), "No issues match.") {
		t.Fatalf("view:\n%s", text(m))
	}
}

// Protects (review C1): closing issues shrinks GitHub's open-issue results, so the next
// page must continue after the last loaded issue, not skip ahead by a fixed offset.
func TestClosingIssues_does_not_skip_issues_on_later_pages(t *testing.T) {
	f := &fakeClient{open: issues(t, 1, 150)}
	m := start(t, f, repoOpts)

	m = press(t, m, "enter")
	for range 95 {
		m = press(t, m, "x", "c")
	}
	m = press(t, m, repeat("n", 60)...)
	if !strings.HasPrefix(header(m), "o/r#150 ") {
		t.Fatalf("after closing 95 and walking to the end: header %q, want o/r#150", header(m))
	}
}

// Protects (review I1): with a local filter, reaching the last match still loads later
// pages, so matches beyond the first page are reachable with n.
func TestFilteredQueue_keeps_loading_pages_past_the_last_match(t *testing.T) {
	all := issues(t, 1, 150)
	all[4].Title, all[139].Title = "crash on start", "crash on exit"
	f := &fakeClient{open: all}
	m := start(t, f, repoOpts)

	m = press(t, m, "/", "crash", "enter", "enter", "n")
	if !strings.HasPrefix(header(m), "o/r#140 ") {
		t.Fatalf("n from the only loaded match: header %q, want o/r#140; searches=%d", header(m), f.searches)
	}
}
