package ui_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/trepidity/gh-triage/internal/github"
	"github.com/trepidity/gh-triage/internal/ui"
)

func switcherRows(m []string) []string {
	var rows []string
	for _, l := range m {
		if strings.HasPrefix(l, "> ") || strings.HasPrefix(l, "  ") {
			rows = append(rows, strings.TrimSpace(l[2:]))
		}
	}
	return rows
}

// Protects: switcher ordering — pins first in pin order, then every other repo once,
// matching case-insensitively because GitHub repo names are (Review Focus 5).
func TestSwitcher_lists_pins_first_then_all_repos_without_duplicates(t *testing.T) {
	f := &fakeClient{repos: []github.Repo{mustRepo(t, "o/a"), mustRepo(t, "o/b"), mustRepo(t, "o/c")}}
	m := start(t, f, ui.Options{Pins: []github.Repo{mustRepo(t, "O/C"), mustRepo(t, "o/x")}})

	want := []string{"O/C", "o/x", "o/a", "o/b"}
	if got := switcherRows(lines(m)); !reflect.DeepEqual(got, want) {
		t.Fatalf("switcher rows = %v, want %v", got, want)
	}
}

// Protects: choosing a repo replaces the queue with that repo's open issues.
func TestSwitcher_selecting_a_repo_loads_its_open_issues(t *testing.T) {
	f := &fakeClient{repos: []github.Repo{mustRepo(t, "o/a"), mustRepo(t, "o/r")}, open: issues(t, 1, 2)}
	m := start(t, f, ui.Options{})

	m = press(t, m, "down", "enter")
	if h := header(m); !strings.Contains(h, "repo:o/r is:issue is:open") {
		t.Fatalf("header = %q", h)
	}
	if row := selectedRow(t, m); !strings.Contains(row, "o/r#1 ") {
		t.Fatalf("selected row = %q", row)
	}
}
