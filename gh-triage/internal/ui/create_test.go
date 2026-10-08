package ui_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/trepidity/gh-triage/internal/github"
	"github.com/trepidity/gh-triage/internal/ui"
)

func (f *fakeClient) CreateIssue(_ context.Context, r github.Repo, title, body string) (github.Issue, error) {
	if f.createErr != nil {
		return github.Issue{}, f.createErr
	}
	is := github.Issue{Repo: r, Number: 42, Title: title, Body: body, State: "open", CreatedAt: time.Now(), URL: "https://github.com/" + r.String() + "/issues/42"}
	f.createdIssues = append(f.createdIssues, is)
	return is, nil // search intentionally lags behind the write
}

func (f *fakeClient) CreateRepo(_ context.Context, owner, name, description string, private bool) (github.Repo, error) {
	if f.createErr != nil {
		return github.Repo{}, f.createErr
	}
	if owner == "" {
		owner = "me"
	}
	r, err := github.ParseRepo(owner + "/" + name)
	if err != nil {
		return github.Repo{}, err
	}
	f.createdRepos = append(f.createdRepos, r)
	f.createdPrivate, f.createdDescription = private, description
	return r, nil
}

func tab(t *testing.T, m tea.Model) tea.Model { return send(t, m, tea.KeyMsg{Type: tea.KeyTab}) }

// Consumer seam: real UI model with fake Client (the project's design Testing section).
// Catches duplicate writes and losing the created issue while the search index lags.
func TestCreateIssue_opens_confirmed_issue_once_even_with_stale_search(t *testing.T) {
	f := &fakeClient{}
	m := start(t, f, ui.Options{Query: "repo:o/r is:open"})
	m = press(t, m, "N")
	m = tab(t, m)
	m = press(t, m, "A new issue")
	m = tab(t, m)
	m = press(t, m, "Details")
	m, cmds := pressOnly(m, "ctrl+s", "ctrl+s", "esc")
	m = run(t, m, cmds...)
	if len(f.createdIssues) != 1 || f.createdIssues[0].Key() != "o/r#42" || f.createdIssues[0].Body != "Details" {
		t.Fatalf("created: %+v", f.createdIssues)
	}
	if !strings.Contains(header(m), "o/r#42 · A new issue") || !strings.Contains(text(m), "Details") {
		t.Fatalf("created issue not opened:\n%s", text(m))
	}
}

// Catches dropped drafts, writes with blank titles, or tallying a failed write.
func TestCreateIssue_validation_failure_and_cancel_preserve_draft(t *testing.T) {
	f := &fakeClient{createErr: errors.New("permission denied")}
	m := start(t, f, ui.Options{Query: "repo:o/r is:open"})
	m = press(t, m, "N", "ctrl+s")
	if !strings.Contains(text(m), "title is required") {
		t.Fatalf("missing validation: %s", text(m))
	}
	m = tab(t, m)
	m = press(t, m, "Keep this title", "ctrl+s")
	if !strings.Contains(text(m), "permission denied") || !strings.Contains(text(m), "Keep this title") || m.(ui.Model).Summary() != "" {
		t.Fatalf("failed draft: %s", text(m))
	}
	m = press(t, m, "esc", "N")
	if !strings.Contains(text(m), "Keep this title") {
		t.Fatalf("canceled draft lost: %s", text(m))
	}
	f.createErr = nil
	m = press(t, m, "ctrl+s")
	if len(f.createdIssues) != 1 || !strings.Contains(header(m), "Keep this title") {
		t.Fatalf("retry: %s", text(m))
	}
}

// Catches implicit destination selection in cross-repo searches and malformed targets.
func TestCreateIssue_cross_repo_search_requires_explicit_destination(t *testing.T) {
	f := &fakeClient{open: issues(t, 1, 1)}
	m := start(t, f, ui.Options{Query: "assignee:@me"})
	m = press(t, m, "N", "ctrl+s")
	if !strings.Contains(text(m), "owner/name") {
		t.Fatalf("destination required: %s", text(m))
	}
	m = press(t, m, "org/project")
	m = tab(t, m)
	m = press(t, m, "Cross repo issue", "ctrl+s")
	if len(f.createdIssues) != 1 || f.createdIssues[0].Repo.String() != "org/project" {
		t.Fatalf("created: %+v", f.createdIssues)
	}
}

// Catches startup dependency on an existing repo, incorrect visibility and stale switcher lists.
func TestCreateRepo_from_empty_switcher_opens_repo_and_keeps_it_in_switcher(t *testing.T) {
	for _, public := range []bool{false, true} {
		t.Run(map[bool]string{false: "private", true: "public"}[public], func(t *testing.T) {
			f := &fakeClient{repos: []github.Repo{}}
			m := start(t, f, ui.Options{})
			m = send(t, m, tea.KeyMsg{Type: tea.KeyCtrlR})
			m = press(t, m, "org")
			m = tab(t, m)
			m = press(t, m, "new-project")
			m = tab(t, m)
			m = press(t, m, "A useful project")
			m = tab(t, m)
			if public {
				m = press(t, m, " ")
			}
			m, cmds := pressOnly(m, "ctrl+s", "ctrl+s")
			m = run(t, m, cmds...)
			if len(f.createdRepos) != 1 || f.createdPrivate == public || f.createdDescription != "A useful project" {
				t.Fatalf("repo creation: %+v", f)
			}
			if !strings.Contains(header(m), "repo:org/new-project") {
				t.Fatalf("repo not opened: %s", text(m))
			}
			m = press(t, m, "r")
			if !strings.Contains(text(m), "org/new-project") {
				t.Fatalf("repo absent from switcher: %s", text(m))
			}
		})
	}
}

// Catches invalid-name writes and losing repo form state on a rejected request.
func TestCreateRepo_validation_failure_and_cancel_preserve_draft(t *testing.T) {
	f := &fakeClient{createErr: errors.New("name already exists")}
	m := start(t, f, ui.Options{})
	m = send(t, m, tea.KeyMsg{Type: tea.KeyCtrlR})
	m = press(t, m, "ctrl+s")
	if !strings.Contains(text(m), "name is required") {
		t.Fatalf("missing validation: %s", text(m))
	}
	m = tab(t, m)
	m = press(t, m, "new-project", "ctrl+s")
	if !strings.Contains(text(m), "name already exists") || m.(ui.Model).Summary() != "" {
		t.Fatalf("failure: %s", text(m))
	}
	m = press(t, m, "esc")
	m = send(t, m, tea.KeyMsg{Type: tea.KeyCtrlR})
	if !strings.Contains(text(m), "new-project") {
		t.Fatalf("draft lost: %s", text(m))
	}
	f.createErr = nil
	m = press(t, m, "ctrl+s")
	if len(f.createdRepos) != 1 || f.createdRepos[0].String() != "me/new-project" {
		t.Fatalf("retry: %s", text(m))
	}
}

// Catches a late repo read leaving the underlying switcher empty/loading forever.
func TestCreateRepo_cancel_returns_to_switcher_refreshed_behind_form(t *testing.T) {
	f := &fakeClient{repos: []github.Repo{mustRepo(t, "o/existing")}}
	var m tea.Model = ui.New(f, ui.Options{})
	refresh := m.Init()
	m = send(t, m, tea.KeyMsg{Type: tea.KeyCtrlR})
	m = run(t, m, refresh)
	m = press(t, m, "esc")
	if !strings.Contains(text(m), "o/existing") || strings.Contains(header(m), "loading") {
		t.Fatalf("stale switcher: %s", text(m))
	}
}

// Catches a late pre-creation repo snapshot erasing a confirmed creation.
func TestCreateRepo_late_list_preserves_confirmed_repository(t *testing.T) {
	f := &fakeClient{repos: []github.Repo{mustRepo(t, "o/existing")}}
	var m tea.Model = ui.New(f, ui.Options{})
	refresh := m.Init()
	m = send(t, m, tea.KeyMsg{Type: tea.KeyCtrlR})
	m = tab(t, m)
	m = press(t, m, "new-project", "ctrl+s")
	m = run(t, m, refresh)
	m = press(t, m, "r")
	if !strings.Contains(text(m), "me/new-project") || !strings.Contains(text(m), "o/existing") {
		t.Fatalf("lost repos: %s", text(m))
	}
}
