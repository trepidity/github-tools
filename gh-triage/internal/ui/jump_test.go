package ui_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/trepidity/gh-triage/internal/ui"
)

func TestJumpSelectsIssueNumberAndLoadsNeededPages(t *testing.T) {
	for name, keys := range map[string][]string{
		"number G": {"2", "4", "0", "G"},
		"colon":    {":", "2", "4", "0", "enter"},
	} {
		t.Run(name, func(t *testing.T) {
			f := &fakeClient{open: issues(t, 101, 250)}
			m := start(t, f, repoOpts)
			m = press(t, m, keys...)
			if row := selectedRow(t, m); !strings.Contains(row, "#240 ") {
				t.Fatalf("selected %q, want issue #240", row)
			}
			if f.searches != 2 {
				t.Fatalf("searches = %d, want 2", f.searches)
			}
			m = press(t, m, "enter")
			if !strings.HasPrefix(header(m), "o/r#240 ") {
				t.Fatalf("enter did not open selected issue: %s", header(m))
			}
		})
	}
}

func TestJumpMissingPreservesSelection(t *testing.T) {
	f := &fakeClient{open: issues(t, 101, 250)}
	m := press(t, start(t, f, repoOpts), "j", "9", "9", "9", "G")
	if row := selectedRow(t, m); !strings.Contains(row, "#102 ") {
		t.Fatalf("missing jump changed selection: %s", row)
	}
	if !strings.Contains(text(m), "#999 is not in the current list") || f.searches != 2 {
		t.Fatalf("missing jump did not settle: searches=%d\n%s", f.searches, text(m))
	}
}

func TestJumpCancellationPreventsLatePageFromMovingCursor(t *testing.T) {
	for _, cancel := range []string{"esc", "j", "/"} {
		t.Run(cancel, func(t *testing.T) {
			f := &fakeClient{open: issues(t, 101, 350)}
			m := start(t, f, repoOpts)
			m, cmds := pressOnly(m, "3", "4", "0", "G", cancel)
			m = run(t, m, cmds...)
			want := "#101 "
			if cancel == "j" {
				want = "#102 "
			}
			if row := selectedRow(t, m); !strings.Contains(row, want) || f.searches != 2 {
				t.Fatalf("cancelled jump continued: row=%q searches=%d", row, f.searches)
			}
		})
	}
}

func TestJumpWhileInitialPageIsLoading(t *testing.T) {
	f := &fakeClient{open: issues(t, 101, 250)}
	var m tea.Model = ui.New(f, repoOpts)
	initial := m.Init()
	m, cmds := pressOnly(m, ":", "2", "4", "0", "enter")
	m = run(t, m, append(cmds, initial)...)
	if row := selectedRow(t, m); !strings.Contains(row, "#240 ") || f.searches != 2 {
		t.Fatalf("jump during load: row=%q searches=%d", row, f.searches)
	}
}

func TestJumpRespectsFilterAndDoesNotCaptureFilterNumbers(t *testing.T) {
	f := &fakeClient{open: issues(t, 101, 105)}
	m := press(t, start(t, f, repoOpts), "/", "104", "enter", "1", "0", "5", "G")
	if row := selectedRow(t, m); !strings.Contains(row, "#104 ") || !strings.Contains(text(m), "#105 is not in the current list") {
		t.Fatalf("jump ignored filter: %s", text(m))
	}
}

func TestJumpInputCanBeEditedOrCancelled(t *testing.T) {
	for name, keys := range map[string][]string{
		"backspace": {"1", "0", "9", "backspace", "4", "G"},
		"escape":    {":", "9", "esc", "1", "0", "4", "G"},
		"other key": {"9", "j", "1", "0", "4", "G"},
	} {
		t.Run(name, func(t *testing.T) {
			m := start(t, &fakeClient{open: issues(t, 101, 105)}, repoOpts)
			m = press(t, m, keys...)
			if row := selectedRow(t, m); !strings.Contains(row, "#104 ") {
				t.Fatalf("selected %q, want #104", row)
			}
		})
	}
}

func TestJumpRejectsInvalidNumbers(t *testing.T) {
	for _, number := range []string{"", "0", "99999999999999999999"} {
		m := start(t, &fakeClient{open: issues(t, 101, 105)}, repoOpts)
		m = press(t, m, ":", number, "enter")
		if !strings.Contains(text(m), "enter a positive issue number") || !strings.Contains(selectedRow(t, m), "#101 ") {
			t.Fatalf("invalid number %q: %s", number, text(m))
		}
	}
}
