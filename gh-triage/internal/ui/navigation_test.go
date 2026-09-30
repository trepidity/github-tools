package ui_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/trepidity/gh-triage/internal/github"
	"github.com/trepidity/gh-triage/internal/ui"
)

var repoOpts = ui.Options{Query: "repo:o/r"}

// Protects: the navigation requirement — n/p move through the queue and stop at its ends.
func TestNextAndPrevious_stop_at_the_ends_of_the_queue(t *testing.T) {
	f := &fakeClient{pages: map[int][]github.Issue{1: issues(t, 1, 3)}}
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
	f := &fakeClient{pages: map[int][]github.Issue{1: issues(t, 1, 5)}}
	m := start(t, f, repoOpts)

	m = press(t, m, "enter", "n", "n", "esc")
	if row := selectedRow(t, m); !strings.Contains(row, "o/r#3 ") {
		t.Fatalf("selected row after esc = %q, want o/r#3", row)
	}
}

// Protects: paging state — nearing the end requests the next page once, even when
// keys arrive faster than GitHub answers, and never asks past the last page.
func TestNearingTheEnd_fetches_the_next_page_exactly_once(t *testing.T) {
	f := &fakeClient{pages: map[int][]github.Issue{1: issues(t, 1, 100), 2: issues(t, 101, 150)}}
	m := start(t, f, repoOpts)

	m, cmds := pressOnly(m, repeat("j", 95)...)
	m = run(t, m, cmds...)
	m = press(t, m, repeat("j", 60)...)

	if !reflect.DeepEqual(f.searches, []int{1, 2}) {
		t.Fatalf("pages requested = %v, want [1 2]", f.searches)
	}
	if row := selectedRow(t, m); !strings.Contains(row, "o/r#150 ") {
		t.Fatalf("selected row = %q, want the last issue o/r#150", row)
	}
}

// Protects (Review Focus 1): an empty queue must not crash on open or navigation keys.
func TestEmptyQueue_ignores_navigation_and_says_so(t *testing.T) {
	f := &fakeClient{pages: map[int][]github.Issue{1: nil}}
	m := start(t, f, repoOpts)

	m = press(t, m, "enter", "n", "p", "j", "k")
	if !strings.Contains(text(m), "No issues match.") {
		t.Fatalf("view:\n%s", text(m))
	}
}
