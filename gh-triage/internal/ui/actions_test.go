package ui_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func threeIssues(t *testing.T) *fakeClient {
	return &fakeClient{open: issues(t, 1, 3)}
}

func closedMark(t *testing.T, view []string, key string) bool {
	for _, l := range view {
		if strings.Contains(l, key+" ") {
			return strings.Contains(l, "✓ closed")
		}
	}
	t.Fatalf("no row for %s", key)
	return false
}

func rowCount(view []string) int {
	n := 0
	for _, l := range view {
		if strings.Contains(l, "o/r#") {
			n++
		}
	}
	return n
}

// Protects: close-and-advance — the close reason reaches GitHub, the row is marked,
// the view advances, and the queue does not shift.
func TestClose_marks_row_advances_and_keeps_queue_positions(t *testing.T) {
	for k, want := range map[string]string{"c": "completed", "n": "not_planned"} {
		f := threeIssues(t)
		m := start(t, f, repoOpts)

		m = press(t, m, "enter", "x", k)
		if !reflect.DeepEqual(f.closes, []string{"o/r#1 " + want}) {
			t.Fatalf("key %s: closes = %v", k, f.closes)
		}
		if !strings.HasPrefix(header(m), "o/r#2 ") {
			t.Fatalf("key %s: header after close = %q, want o/r#2", k, header(m))
		}
		m = press(t, m, "esc")
		if v := lines(m); !closedMark(t, v, "o/r#1") || rowCount(v) != 3 {
			t.Fatalf("key %s: list after close:\n%s", k, strings.Join(v, "\n"))
		}
	}
}

// Protects: refusal path — a failed close leaves the issue open and the user on it.
func TestCloseFailure_keeps_issue_open_and_stays_on_it(t *testing.T) {
	f := threeIssues(t)
	f.closeErr = errors.New("403 forbidden")
	m := start(t, f, repoOpts)

	m = press(t, m, "enter", "x", "c")
	if !strings.HasPrefix(header(m), "o/r#1 ") || !strings.Contains(text(m), "close failed: 403 forbidden") {
		t.Fatalf("after failed close:\n%s", text(m))
	}
	if closedMark(t, lines(press(t, m, "esc")), "o/r#1") {
		t.Fatal("o/r#1 marked closed after a failed close")
	}
}

// Protects: refusal path — a failed comment keeps the draft so it can be resent.
func TestCommentFailure_keeps_the_draft_for_retry(t *testing.T) {
	f := threeIssues(t)
	f.commentErr = errors.New("boom")
	m := start(t, f, repoOpts)

	m = press(t, m, "enter", "c", "looks like a dupe", "ctrl+s")
	if v := text(m); !strings.Contains(v, "looks like a dupe") || !strings.Contains(v, "comment failed: boom") {
		t.Fatalf("after failed comment:\n%s", v)
	}
	f.commentErr = nil
	m = press(t, m, "ctrl+s")
	if got := f.comments[len(f.comments)-1]; got != "o/r#1 looks like a dupe" {
		t.Fatalf("retry posted %q", got)
	}
	if !strings.HasPrefix(header(m), "o/r#1 ") {
		t.Fatalf("comment should not advance; header = %q", header(m))
	}
}

// Protects (Review Focus 2): comment+close where the close fails posts the comment once
// and retrying the close does not re-post it.
func TestCommentThenClose_when_close_fails_posts_comment_once(t *testing.T) {
	f := threeIssues(t)
	f.closeErr = errors.New("403 forbidden")
	m := start(t, f, repoOpts)

	m = press(t, m, "enter", "X", "dupe of #2", "ctrl+s", "c")
	if !strings.Contains(text(m), "close failed") || !strings.HasPrefix(header(m), "o/r#1 ") {
		t.Fatalf("after X with failing close:\n%s", text(m))
	}
	f.closeErr = nil
	m = press(t, m, "x", "c")
	if len(f.comments) != 1 || len(f.closes) != 2 || !strings.HasPrefix(header(m), "o/r#2 ") {
		t.Fatalf("comments=%v closes=%v header=%q", f.comments, f.closes, header(m))
	}
}

// Protects (Review Focus 3): keys pressed while a close is in flight do not send a second close.
func TestKeysDuringInFlightClose_send_exactly_one_request(t *testing.T) {
	f := threeIssues(t)
	m := start(t, f, repoOpts)
	m = press(t, m, "enter")

	m, cmds := pressOnly(m, "x", "c", "x", "c")
	run(t, m, cmds...)
	if len(f.closes) != 1 {
		t.Fatalf("closes = %v, want exactly one", f.closes)
	}
}
