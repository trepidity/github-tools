package ui_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/trepidity/gh-triage/internal/ui"
)

func listRows(v []string) []string {
	var rows []string
	for _, l := range v {
		if (strings.HasPrefix(l, "> ") || strings.HasPrefix(l, "  ")) && strings.Contains(l, "#") {
			rows = append(rows, l)
		}
	}
	return rows
}

// Protects: a single-repo queue shows bare issue numbers (the repo is in the header), and a
// queue spanning repos names each row's repo.
func TestListRows_name_the_repo_only_when_the_queue_spans_repos(t *testing.T) {
	m := start(t, threeIssues(t), repoOpts)
	if rows := listRows(lines(m)); strings.Contains(rows[0], "o/r#") || !strings.Contains(rows[0], "#1 ") {
		t.Fatalf("single-repo row = %q", rows[0])
	}

	mixed := issues(t, 1, 2)
	mixed[1].Repo = mustRepo(t, "x/y")
	m = start(t, &fakeClient{open: mixed}, ui.Options{Query: "org:o"})
	if rows := listRows(lines(m)); !strings.Contains(rows[0], "o/r#1 ") || !strings.Contains(rows[1], "x/y#2 ") {
		t.Fatalf("multi-repo rows:\n%s", strings.Join(rows, "\n"))
	}
}

// Protects: author, age and comment columns line up however long each title is.
func TestListRows_align_their_columns(t *testing.T) {
	f := threeIssues(t)
	f.open[1].Title = "A much longer title than the others, but still short enough to fit"
	m := start(t, f, repoOpts)
	m = send(t, m, tea.WindowSizeMsg{Width: 150, Height: 20})

	rows := listRows(lines(m))
	if a, b := strings.Index(rows[0], "@someone"), strings.Index(rows[1], "@someone"); a < 0 || a != b {
		t.Fatalf("author columns differ (%d vs %d):\n%s\n%s", a, b, rows[0], rows[1])
	}
}

// Protects: labels show on a wide terminal and are the first column dropped on a narrow
// one, so the title keeps its room.
func TestListRows_show_labels_and_drop_them_first_when_narrow(t *testing.T) {
	f := threeIssues(t)
	f.open[0].Labels = []string{"question"}
	f.open[0].LabelColors = map[string]string{"question": "d876e3"}
	m := start(t, f, repoOpts)

	m = send(t, m, tea.WindowSizeMsg{Width: 150, Height: 20})
	if row := listRows(lines(m))[0]; !strings.Contains(row, "question") {
		t.Fatalf("wide row = %q", row)
	}
	m = send(t, m, tea.WindowSizeMsg{Width: 60, Height: 20})
	if row := listRows(lines(m))[0]; strings.Contains(row, "question") || !strings.Contains(row, "@someone") || !strings.Contains(row, "Title 1") {
		t.Fatalf("narrow row = %q", row)
	}
}
