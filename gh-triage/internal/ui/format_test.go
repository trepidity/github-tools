package ui_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/trepidity/gh-triage/internal/ui"
)

// Protects: on a wide terminal the issue body wraps at a readable width instead of
// running edge to edge.
func TestIssueBody_wraps_at_a_readable_width_on_wide_terminals(t *testing.T) {
	f := threeIssues(t)
	f.open[0].Body = strings.Repeat("lorem ipsum dolor sit amet ", 30)
	m := start(t, f, repoOpts)
	m = send(t, m, tea.WindowSizeMsg{Width: 150, Height: 40})

	for _, l := range lines(press(t, m, "enter")) {
		if w := ansi.StringWidth(strings.TrimRight(l, " ")); strings.Contains(l, "lorem") && w > 102 {
			t.Fatalf("body line is %d columns wide:\n%s", w, l)
		}
	}
}

// Protects: a long title wraps onto a second line instead of losing its end to "…".
func TestIssueTitle_wraps_instead_of_truncating(t *testing.T) {
	f := threeIssues(t)
	f.open[0].Title = "Question: should the options portfolio read the account and chain from the engine instead of a JSON copy"
	m := start(t, f, repoOpts)
	m = send(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})

	v := lines(press(t, m, "enter"))
	head := v[0] + " " + v[1]
	if !strings.HasPrefix(v[0], "o/r#1 ") || !strings.Contains(head, "JSON copy") || strings.Contains(head, "…") {
		t.Fatalf("title lines:\n%s\n%s", v[0], v[1])
	}
}

// Protects: the issue help fits the terminal, and ? shows the keys the short help leaves
// out without cutting any off.
func TestIssueHelp_question_mark_shows_every_key_without_truncating(t *testing.T) {
	m := start(t, threeIssues(t), repoOpts)
	m = send(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})

	m = press(t, m, "enter")
	if v := text(m); strings.Contains(v, "…") || strings.Contains(v, "t transfer") || !strings.Contains(v, "? more") {
		t.Fatalf("short help:\n%s", v)
	}
	m = press(t, m, "?")
	if v := text(m); strings.Contains(v, "…") || !strings.Contains(v, "t transfer") || !strings.Contains(v, "r repos") {
		t.Fatalf("full help:\n%s", v)
	}
	if v := text(press(t, m, "?")); strings.Contains(v, "t transfer") {
		t.Fatalf("? again should hide the full help:\n%s", v)
	}
}

// Protects: body lines break only between words — glamour's own wrapping cut
// "implementation.md" into "implementation." / "md" when it landed near the edge.
func TestIssueBody_breaks_lines_only_between_words(t *testing.T) {
	f := threeIssues(t)
	f.open[0].Body = "**Normalized local location:** `docs/superpowers/specs/2026-09-30-json-removal-track-j-implementation.md` § Decision gaps and blockers, TJ-Q1."
	m := start(t, f, ui.Options{Query: "repo:o/r", Style: "dark"})
	m = send(t, m, tea.WindowSizeMsg{Width: 146, Height: 40})

	if v := text(press(t, m, "enter")); !strings.Contains(v, "docs/superpowers/specs/2026-09-30-json-removal-track-j-implementation.md") {
		t.Fatalf("path split across lines:\n%s", v)
	}
}

// Protects: the body starts right under the header rule instead of after a blank line.
func TestIssueBody_starts_directly_under_the_header(t *testing.T) {
	m := start(t, threeIssues(t), repoOpts)

	if v := lines(press(t, m, "enter")); !strings.Contains(v[3], "No description provided") {
		t.Fatalf("first body line = %q", v[3])
	}
}
