package ui_test

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/trepidity/gh-triage/internal/ui"
)

func TestGuide_stays_at_bottom_across_screens_and_terminal_sizes(t *testing.T) {
	cases := []struct {
		name string
		keys []string
		pull bool
		want string
	}{
		{name: "list", want: "q quit"},
		{name: "issue", keys: []string{"enter"}, want: "? more"},
		{name: "expanded issue guide", keys: []string{"enter", "?"}, want: "? less"},
		{name: "comment", keys: []string{"enter", "c"}, want: "esc cancel"},
		{name: "close", keys: []string{"enter", "x"}, want: "? more"},
		{name: "repos", keys: []string{"r"}, want: "esc cancel"},
		{name: "search", keys: []string{"s"}, want: "esc cancel"},
		{name: "new issue", keys: []string{"N"}, want: "ctrl+e $EDITOR"},
		{name: "new repo", keys: []string{"ctrl+r"}, want: "esc close (draft kept)"},
		{name: "pull", pull: true, keys: []string{"enter"}, want: "? more"},
		{name: "review picker", pull: true, keys: []string{"enter", "v"}, want: "esc cancel"},
		{name: "review", pull: true, keys: []string{"enter", "v", "enter"}, want: "esc cancel (draft kept)"},
	}
	for _, size := range []tea.WindowSizeMsg{{Width: 60, Height: 24}, {Width: 120, Height: 40}} {
		for _, tc := range cases {
			t.Run(fmt.Sprintf("%s/%dx%d", tc.name, size.Width, size.Height), func(t *testing.T) {
				f, opts := threeIssues(t), repoOpts
				if tc.pull {
					f, opts = pullClient(t), ui.Options{Query: "repo:o/r is:pr is:open"}
				}
				m := send(t, start(t, f, opts), size)
				if tc.name == "new repo" {
					m = send(t, m, tea.KeyMsg{Type: tea.KeyCtrlR})
				} else {
					m = press(t, m, tc.keys...)
				}
				v := lines(m)
				if len(v) != size.Height || !strings.Contains(v[len(v)-1], tc.want) {
					t.Fatalf("guide should end at row %d with %q:\n%s", size.Height, tc.want, text(m))
				}
				for _, line := range v {
					if ansi.StringWidth(line) > size.Width {
						t.Fatalf("line exceeds terminal width: %q", line)
					}
				}
			})
		}
	}
}

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

// Protects: long cells wrap inside their column instead of wrapping the whole
// rendered row and separating its label from the decision.
func TestIssueBody_tables_keep_columns_aligned_when_resized(t *testing.T) {
	for _, style := range []string{"notty", "dark", "light"} {
		for _, width := range []int{60, 120, 150} {
			t.Run(fmt.Sprintf("%s/%d", style, width), func(t *testing.T) {
				f := threeIssues(t)
				f.open[0].Body = "## Owner decisions\n\n| # | Decision |\n|---|---|\n| Rows | Phone lists are **single full-width rows**. Tapping a row opens a dialog holding its actions; no text-left/button-right rows. |\n| Doc delete | Documents are not deletable in this work. |\n| Archive | Archive is not destructive. One tap plus Undo, always restorable. |\n\nAfter the table."
				m := start(t, f, ui.Options{Query: "repo:o/r", Style: style})
				m = press(t, m, "enter")
				m = send(t, m, tea.WindowSizeMsg{Width: width, Height: 60})
				v := text(m)
				if !strings.Contains(v, "Doc delete") || !strings.Contains(v, "After the table.") || strings.Contains(v, "GHTRIAGETABLE") {
					t.Fatalf("table lost content:\n%s", v)
				}
				separator := -1
				divider := "│"
				if style == "notty" {
					divider = "|"
				}
				rows := 0
				rules := 0
				lastRule := ""
				for _, line := range strings.Split(v, "\n") {
					if ansi.StringWidth(line) > width {
						t.Fatalf("line exceeds terminal width: %q", line)
					}
					if pos := strings.Index(line, divider); pos >= 0 {
						if separator >= 0 && pos != separator {
							t.Fatalf("column separator moved: %q", line)
						}
						separator = pos
						rows++
					}
					if separator >= 0 && strings.ContainsAny(line, "─-") && strings.Trim(line, " ─┼-+|\t") == "" {
						if lastRule != "" && line != lastRule {
							t.Fatalf("row rule differs from header rule: %q != %q", line, lastRule)
						}
						lastRule = line
						rules++
					}
					if strings.Contains(line, "Doc delete") && rules != 2 {
						t.Fatalf("missing separator before Doc delete:\n%s", v)
					}
					if strings.Contains(line, "Archive is") && rules != 3 {
						t.Fatalf("missing separator before Archive:\n%s", v)
					}
				}
				if rules != 3 {
					t.Fatalf("want header rule and two body row rules, got %d:\n%s", rules, v)
				}
				if rows < 4 {
					t.Fatalf("missing table rows:\n%s", v)
				}
			})
		}
	}
}

func TestIssueBody_table_boundaries_preserve_surrounding_markdown(t *testing.T) {
	for _, body := range []string{
		"Before.\n\n| A | B |\n|---|---|\n\nAfter.",
		"Before.\n\n| A | B |\n|---|---|",
		"Before.\n\n| A | B |\n|---|---|\n| left \\| right | **value** |\n\nAfter.\n\n| C | D |\n|---|---|\n| next | last |",
		"Before.\n\n```text\n| A | B |\n|---|---|\n```\n\nAfter.",
	} {
		f := threeIssues(t)
		f.open[0].Body = body
		m := start(t, f, repoOpts)
		m = send(t, m, tea.WindowSizeMsg{Width: 80, Height: 60})
		v := text(press(t, m, "enter"))
		if !strings.Contains(v, "Before.") || (strings.Contains(body, "After.") && !strings.Contains(v, "After.")) || strings.Contains(v, "GHTRIAGETABLE") {
			t.Fatalf("lost surrounding content:\n%s", v)
		}
	}
}

func TestIssueBody_table_reference_links_keep_their_destination(t *testing.T) {
	f := threeIssues(t)
	f.open[0].Body = "| Topic | Decision |\n|---|---|\n| Docs | [Read the guide][guide] |\n\n[guide]: https://example.com/guide \"Guide\"\n\nAfter."
	m := start(t, f, ui.Options{Query: "repo:o/r", Style: "dark"})
	m = send(t, m, tea.WindowSizeMsg{Width: 80, Height: 40})
	if v := text(press(t, m, "enter")); !strings.Contains(v, "https://example.com/guide") || !strings.Contains(v, "After.") {
		t.Fatalf("reference link lost its destination:\n%s", v)
	}
}
