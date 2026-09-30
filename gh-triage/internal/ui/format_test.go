package ui_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
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
