package ui_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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
	if row := selectedRow(t, m); !strings.Contains(row, "#1 ") {
		t.Fatalf("selected row = %q", row)
	}
}

// Protects (review I3): typing a filter highlights the best match, so enter opens it
// rather than whatever row the cursor happened to be on.
func TestSwitcher_typing_highlights_the_top_match(t *testing.T) {
	f := &fakeClient{repos: []github.Repo{mustRepo(t, "o/aa"), mustRepo(t, "o/ab"), mustRepo(t, "o/ac"), mustRepo(t, "x/zed"), mustRepo(t, "x/zeta")}}
	m := start(t, f, ui.Options{})

	m = press(t, m, "down", "ze")
	top := switcherRows(lines(m))[0]
	m = press(t, m, "enter")
	if h := header(m); !strings.Contains(h, "repo:"+top+" ") {
		t.Fatalf("top match %s, but enter opened: %q", top, h)
	}
}

// Protects (review I3): when the cached repo list is refreshed from GitHub, the
// highlight stays on the repo the user picked instead of shifting to a neighbour.
func TestSwitcher_keeps_the_highlighted_repo_when_the_list_refreshes(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "repos.json")
	if err := os.WriteFile(cache, []byte(`["o/b","o/c"]`), 0o644); err != nil {
		t.Fatal(err)
	}
	f := &fakeClient{repos: []github.Repo{mustRepo(t, "o/a"), mustRepo(t, "o/b"), mustRepo(t, "o/c")}}
	var m tea.Model = ui.New(f, ui.Options{RepoCache: cache})
	refresh := m.Init()
	m = send(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})

	m, _ = m.Update(key("down")) // o/c, from the cache
	m = run(t, m, refresh)
	if row := selectedRow(t, m); !strings.Contains(row, "o/c") {
		t.Fatalf("highlight after refresh = %q, want o/c", row)
	}
}
