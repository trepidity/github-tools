package ui_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/trepidity/gh-triage/internal/github"
	"github.com/trepidity/gh-triage/internal/ui"
)

func threeIssues(t *testing.T) *fakeClient {
	return &fakeClient{open: issues(t, 1, 3)}
}

// closedMark reports whether the list row for key (e.g. "o/r#1") is marked closed. A
// single-repo queue shows bare numbers, so rows are matched by "#1 ".
func closedMark(t *testing.T, view []string, key string) bool {
	num := key[strings.Index(key, "#"):]
	for _, l := range view {
		if strings.Contains(l, num+" ") {
			return strings.Contains(l, "✓ closed")
		}
	}
	t.Fatalf("no row for %s", key)
	return false
}

func rowCount(view []string) int {
	n := 0
	for _, l := range view {
		if (strings.HasPrefix(l, "> ") || strings.HasPrefix(l, "  ")) && strings.Contains(l, "#") {
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

// Protects (review I2, M1): an issue GitHub already reports closed shows as closed, and
// neither x nor X sends another close (which would overwrite its close reason).
func TestIssueClosedOnGitHub_shows_closed_and_refuses_to_close_again(t *testing.T) {
	f := threeIssues(t)
	f.open[1].State = "closed"
	m := start(t, f, repoOpts)

	if !closedMark(t, lines(m), "o/r#2") {
		t.Fatalf("o/r#2 is closed on GitHub but the list shows it open:\n%s", text(m))
	}
	m = press(t, m, "j", "enter", "x")
	if !strings.Contains(text(m), "already closed") {
		t.Fatalf("x on a closed issue:\n%s", text(m))
	}
	press(t, m, "X", "dupe", "ctrl+s", "n")
	if len(f.closes) != 0 || len(f.comments) != 0 {
		t.Fatalf("closed issue was written to: closes=%v comments=%v", f.closes, f.comments)
	}
}

// Protects (review I2): a close made this session survives a new search, even while
// GitHub's search index still lists the issue as open.
func TestClosedThisSession_stays_closed_after_a_new_search(t *testing.T) {
	f := threeIssues(t)
	f.staleIndex = true
	m := start(t, f, repoOpts)

	m = press(t, m, "enter", "x", "c", "esc", "s", "repo:o/r", "enter")
	if !closedMark(t, lines(m), "o/r#1") {
		t.Fatalf("o/r#1 closed this session shows open after re-search:\n%s", text(m))
	}
}

// Protects: the comment box tells the user how to send, even after the placeholder
// disappears — a user could not find the send key (ctrl+s) in the first real session.
func TestCommentEditor_shows_how_to_send_while_typing(t *testing.T) {
	m := start(t, threeIssues(t), repoOpts)

	m = press(t, m, "enter", "c", "Testing an update")
	v := lines(m)
	if footer := strings.Join(v[len(v)-2:], "\n"); !strings.Contains(footer, "ctrl+s send") || !strings.Contains(footer, "esc cancel") {
		t.Fatalf("footer while typing a comment:\n%s", footer)
	}
}

// Protects (spec test 1, M3): a request GitHub never answers ends as a timeout error,
// and the UI takes keys again instead of staying busy until ctrl+c.
func TestHungRequest_times_out_and_frees_the_ui(t *testing.T) {
	f := threeIssues(t)
	f.hang = true
	m := start(t, f, ui.Options{Query: "repo:o/r", ActionTimeout: 20 * time.Millisecond})

	m = press(t, m, "enter", "x", "c")
	if !strings.Contains(text(m), "close failed: context deadline exceeded") {
		t.Fatalf("after hung close:\n%s", text(m))
	}
	f.hang = false
	m = press(t, m, "x", "c")
	if !strings.HasPrefix(header(m), "o/r#2 ") {
		t.Fatalf("UI still stuck after timeout; header = %q", header(m))
	}
}

// Protects (spec test 6, M8): esc in the comment editor keeps the draft for that issue,
// and a successful post clears it so the next comment starts empty.
func TestCommentDraft_survives_esc_and_clears_after_posting(t *testing.T) {
	f := threeIssues(t)
	m := start(t, f, repoOpts)

	m = press(t, m, "enter", "c", "half a thought", "esc", "c")
	if !strings.Contains(text(m), "half a thought") {
		t.Fatalf("draft lost after esc:\n%s", text(m))
	}
	m = press(t, m, "ctrl+s", "c", "second", "ctrl+s")
	want := []string{"o/r#1 half a thought", "o/r#1 second"}
	if !reflect.DeepEqual(f.comments, want) {
		t.Fatalf("comments = %q, want %q", f.comments, want)
	}
}

// Protects (Review Focus 2, M4): a failed comment load says how to retry instead of
// "Loading comments…" forever, and R loads them.
func TestCommentLoadFailure_offers_retry_and_R_recovers(t *testing.T) {
	f := threeIssues(t)
	f.loadErr = errors.New("502 bad gateway")
	m := start(t, f, repoOpts)

	m = press(t, m, "enter")
	if v := text(m); strings.Contains(v, "Loading comments") || !strings.Contains(v, "press R to retry") {
		t.Fatalf("after failed load:\n%s", v)
	}
	f.loadErr = nil
	f.thread = []github.Comment{{Author: "bob", Body: "first reply"}}
	m = press(t, m, "R")
	if !strings.Contains(text(m), "first reply") {
		t.Fatalf("after R:\n%s", text(m))
	}
}

// Protects (Review Focus 1): a multi-line API error shows only its first line, so the
// issue title stays on screen.
func TestMultiLineError_shows_only_its_first_line(t *testing.T) {
	f := threeIssues(t)
	f.closeErr = errors.New("HTTP 422: Validation Failed\n{\"message\":\"Validation Failed\",\"errors\":[]}")
	m := start(t, f, repoOpts)

	m = press(t, m, "enter", "x", "c")
	v := text(m)
	if !strings.Contains(v, "close failed: HTTP 422: Validation Failed") || strings.Contains(v, `"errors"`) {
		t.Fatalf("status:\n%s", v)
	}
	if !strings.HasPrefix(header(m), "o/r#1 ") {
		t.Fatalf("header pushed off screen: %q", header(m))
	}
}

// Protects (spec test 11): the quit summary counts only confirmed actions — a failed close
// is not counted, and an undone close is taken back.
func TestSummary_counts_only_confirmed_actions(t *testing.T) {
	f := &fakeClient{open: issues(t, 1, 4)}
	f.closeErr = errors.New("403 forbidden")
	m := start(t, f, repoOpts)

	m = press(t, m, "enter", "x", "c")
	f.closeErr = nil
	m = press(t, m, "x", "c", "x", "c", "u", "c", "hi", "ctrl+s")
	if got := m.(ui.Model).Summary(); got != "closed 1 · commented 1" {
		t.Fatalf("summary = %q, want %q", got, "closed 1 · commented 1")
	}
}
