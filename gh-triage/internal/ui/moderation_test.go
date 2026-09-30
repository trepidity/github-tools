package ui_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/trepidity/gh-triage/internal/github"
)

// Protects (spec test 4, review finding 1): if the duplicate comment posts but the close
// fails, the comment shows, the issue stays open, and d retries only the close.
func TestDuplicate_when_close_fails_keeps_comment_and_retries_only_the_close(t *testing.T) {
	f := threeIssues(t)
	f.closeErr = errors.New("403 forbidden")
	m := start(t, f, repoOpts)

	m = press(t, m, "enter", "d", "#3", "enter")
	v := text(m)
	if !reflect.DeepEqual(f.comments, []string{"o/r#1 Duplicate of #3"}) || !strings.HasPrefix(header(m), "o/r#1 ") ||
		!strings.Contains(v, "commented; duplicate failed: 403 forbidden") || !strings.Contains(v, "Duplicate of #3") {
		t.Fatalf("comments=%v\n%s", f.comments, v)
	}
	f.closeErr = nil
	m = press(t, m, "d")
	if len(f.comments) != 1 || !reflect.DeepEqual(f.closes, []string{"o/r#1 duplicate", "o/r#1 duplicate"}) || !strings.HasPrefix(header(m), "o/r#2 ") {
		t.Fatalf("retry: comments=%v closes=%v header=%q", f.comments, f.closes, header(m))
	}
}

// Protects (Review Focus 5): an invalid duplicate target sends nothing and keeps the prompt.
func TestDuplicate_with_an_invalid_target_sends_nothing(t *testing.T) {
	f := threeIssues(t)
	m := start(t, f, repoOpts)

	m = press(t, m, "enter", "d", "abc", "enter")
	m = press(t, m, "enter") // still in the prompt: enter again re-validates, sends nothing
	m = press(t, m, "esc", "d", "#1", "enter")
	if len(f.comments) != 0 || len(f.closes) != 0 {
		t.Fatalf("sent comments=%v closes=%v", f.comments, f.closes)
	}
	if !strings.Contains(text(m), "can't be a duplicate of itself") {
		t.Fatalf("self-duplicate:\n%s", text(m))
	}
}

// Protects (spec test 3, review finding 5): u reopens the last close; a failed reopen keeps
// it closed and undoable; after undo, a search whose index still says closed shows it open.
func TestUndo_reopens_the_last_close_and_survives_a_stale_search(t *testing.T) {
	f := threeIssues(t)
	f.staleIndex = true
	f.reopenErr = errors.New("502 bad gateway")
	m := start(t, f, repoOpts)

	m = press(t, m, "enter", "x", "c", "u")
	if !strings.Contains(text(m), "reopen failed: 502 bad gateway") {
		t.Fatalf("after failed undo:\n%s", text(m))
	}
	f.reopenErr = nil
	m = press(t, m, "u")
	if !reflect.DeepEqual(f.reopens, []string{"o/r#1", "o/r#1"}) || !strings.Contains(text(m), "reopened o/r#1") {
		t.Fatalf("reopens=%v\n%s", f.reopens, text(m))
	}
	f.open[0].State = "closed" // GitHub's index still reports the old close
	m = press(t, m, "esc", "s", "repo:o/r", "enter")
	if closedMark(t, lines(m), "o/r#1") {
		t.Fatalf("reopened issue shows closed after re-search:\n%s", text(m))
	}
	m = press(t, m, "k", "enter", "x") // k: after Task 12, resume lands on o/r#2
	if !strings.HasPrefix(header(m), "o/r#1 ") || strings.Contains(text(m), "already closed") {
		t.Fatalf("reopened issue refuses close:\n%s", text(m))
	}
}

// Protects (spec test 5): a confirmed transfer marks the row moved, advances, keeps the
// queue length, and later actions on the moved issue are refused.
func TestTransfer_marks_row_moved_advances_and_refuses_later_actions(t *testing.T) {
	f := threeIssues(t)
	f.repos = []github.Repo{mustRepo(t, "o/r"), mustRepo(t, "o/other")}
	m := start(t, f, repoOpts)

	m = press(t, m, "enter", "t", "other", "enter", "y")
	if !reflect.DeepEqual(f.transfers, []string{"o/r#1 → o/other"}) || !strings.HasPrefix(header(m), "o/r#2 ") {
		t.Fatalf("transfers=%v header=%q", f.transfers, header(m))
	}
	m = press(t, m, "p", "x")
	if !strings.Contains(text(m), "moved to https://github.com/o/other/issues/99") || len(f.closes) != 0 {
		t.Fatalf("x on moved issue: closes=%v\n%s", f.closes, text(m))
	}
	m = press(t, m, "esc")
	v := lines(m)
	if rowCount(v) != 3 || !strings.Contains(strings.Join(v, "\n"), "→ moved") {
		t.Fatalf("list after transfer:\n%s", strings.Join(v, "\n"))
	}
}
