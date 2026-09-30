package ui_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/trepidity/gh-triage/internal/config"
	"github.com/trepidity/gh-triage/internal/github"
	"github.com/trepidity/gh-triage/internal/ui"
)

func savedAt(is github.Issue) map[string]config.Position {
	return map[string]config.Position{ui.RepoQuery(is.Repo): {Key: is.Key(), CreatedAt: is.CreatedAt}}
}

// Protects (spec test 7, review finding 4): starting a repo from the picker (plain
// `gh triage`) lands on the saved issue on page 2, without paging past it.
func TestResume_from_the_repo_picker_lands_on_the_saved_issue(t *testing.T) {
	all := issues(t, 1, 250)
	f := &fakeClient{open: all, repos: []github.Repo{mustRepo(t, "o/r")}}
	m := start(t, f, ui.Options{Positions: savedAt(all[180])})

	m = press(t, m, "enter")
	if row := selectedRow(t, m); !strings.Contains(row, "o/r#181 ") || f.searches != 2 {
		t.Fatalf("selected %q after %d searches, want o/r#181 after 2", row, f.searches)
	}
}

// Protects (spec test 7): if the saved issue is gone (closed since), resume lands on the
// next older one and stops paging there.
func TestResume_when_the_saved_issue_is_gone_lands_on_the_next_older(t *testing.T) {
	all := issues(t, 1, 250)
	saved := all[180]
	f := &fakeClient{open: slices.Delete(slices.Clone(all), 180, 181)}
	m := start(t, f, ui.Options{Query: ui.RepoQuery(saved.Repo), Positions: savedAt(saved)})

	if row := selectedRow(t, m); !strings.Contains(row, "o/r#182 ") || f.searches != 2 {
		t.Fatalf("selected %q after %d searches, want o/r#182 after 2", row, f.searches)
	}
}

// Protects (final review, critical): opening an issue while resume pages are still loading
// ends the resume, so a late page cannot move the cursor and x closes the issue on screen.
func TestResume_stops_when_the_user_opens_an_issue_before_it_lands(t *testing.T) {
	all := issues(t, 1, 250)
	f := &fakeClient{open: all}
	var m tea.Model = ui.New(f, ui.Options{Query: ui.RepoQuery(all[0].Repo), Positions: savedAt(all[180])})
	m = send(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m, page2 := m.Update(m.Init()()) // page 1 arrives; resume asks for page 2
	m = press(t, m, "enter")         // the user opens o/r#1 before page 2 lands
	m = run(t, m, page2)

	press(t, m, "x", "c")
	if !reflect.DeepEqual(f.closes, []string{"o/r#1 completed"}) {
		t.Fatalf("closes = %v, want the issue on screen (o/r#1)", f.closes)
	}
}
