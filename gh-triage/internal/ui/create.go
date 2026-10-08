package ui

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/trepidity/gh-triage/internal/github"
)

// Creation drafts are separate from comment drafts and survive Esc and errors.
type creationForm struct {
	ready   bool
	fields  [3]textinput.Model
	body    textarea.Model
	focus   int
	private bool
}

func (m *Model) creation() *creationForm {
	if m.creatingRepo {
		return &m.repoDraft
	}
	return &m.issueDraft
}

func (m *Model) startCreation(repo bool) tea.Cmd {
	m.creatingRepo, m.createReturn = repo, m.mode
	f := m.creation()
	if !f.ready {
		*f = creationForm{ready: true, private: true, body: newEditor()}
		if repo {
			f.fields = [3]textinput.Model{newInput("owner: "), newInput("name: "), newInput("description: ")}
			f.fields[0].Placeholder = "signed-in account (or enter an organization)"
		} else {
			f.fields = [3]textinput.Model{newInput("repo: "), newInput("title: "), newInput("")}
			f.fields[0].Placeholder = "owner/name"
			f.fields[0].SetValue(m.creationRepo())
			f.body.Placeholder = "Describe the issue (optional)"
		}
	}
	m.mode, m.status = modeCreate, ""
	return m.focusCreation()
}

// Only a single, simple repo query or the issue being read implies a destination.
func (m Model) creationRepo() string {
	if m.screen == screenIssue {
		if is, ok := m.current(); ok {
			return is.Repo.String()
		}
	}
	var repo string
	for _, token := range strings.Fields(m.query) {
		if strings.EqualFold(token, "OR") {
			return ""
		}
		if strings.HasPrefix(token, "repo:") {
			if repo != "" {
				return ""
			}
			r, err := github.ParseRepo(strings.TrimPrefix(token, "repo:"))
			if err != nil {
				return ""
			}
			repo = r.String()
		}
	}
	return repo
}

func (m *Model) focusCreation() tea.Cmd {
	f := m.creation()
	for i := range f.fields {
		f.fields[i].Blur()
	}
	f.body.Blur()
	if !m.creatingRepo && f.focus == 2 {
		return f.body.Focus()
	}
	if f.focus < 3 {
		return f.fields[f.focus].Focus()
	}
	return nil
}

func (m *Model) keyCreate(k tea.KeyMsg) tea.Cmd {
	f := m.creation()
	switch k.String() {
	case "esc":
		m.mode, m.status = m.createReturn, "creation draft kept"
		return nil
	case "tab", "shift+tab":
		n := 3
		if m.creatingRepo {
			n = 4
		}
		step := 1
		if k.String() == "shift+tab" {
			step = n - 1
		}
		f.focus = (f.focus + step) % n
		return m.focusCreation()
	case "ctrl+s":
		return m.submitCreation()
	case "ctrl+e":
		if !m.creatingRepo {
			return m.openTextEditor(f.body.Value())
		}
	case "enter":
		if m.creatingRepo || f.focus != 2 {
			return m.keyCreate(tea.KeyMsg{Type: tea.KeyTab})
		}
	case " ":
		if m.creatingRepo && f.focus == 3 {
			f.private = !f.private
			return nil
		}
	}
	var cmd tea.Cmd
	if !m.creatingRepo && f.focus == 2 {
		f.body, cmd = f.body.Update(k)
	} else if f.focus < 3 {
		f.fields[f.focus], cmd = f.fields[f.focus].Update(k)
	}
	return cmd
}

func (m *Model) submitCreation() tea.Cmd {
	f := m.creation()
	first, name := strings.TrimSpace(f.fields[0].Value()), strings.TrimSpace(f.fields[1].Value())
	client := m.client
	if m.creatingRepo {
		if err := github.ValidateNewRepo(first, name); err != nil {
			m.status = err.Error()
			return nil
		}
		description, private := f.fields[2].Value(), f.private
		return m.run("create repository", "", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
			repo, err := client.CreateRepo(ctx, first, name, description, private)
			if err != nil {
				return nil, err
			}
			return func(m *Model) tea.Cmd {
				m.repoDraft = creationForm{}
				m.count(actRepoCreated)
				m.createdRepos = append(m.createdRepos, repo)
				m.repos = orderRepos(m.createdRepos, m.repos)
				m.cacheRepos()
				cmd := m.startSearch(RepoQuery(repo))
				m.status = "created repository " + repo.String()
				return cmd
			}, nil
		})
	}
	repo, err := github.ParseRepo(first)
	if err == nil {
		err = github.ValidateNewIssue(repo, name)
	}
	if err != nil {
		m.status = err.Error()
		return nil
	}
	body := f.body.Value()
	return m.run("create issue", "", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
		is, err := client.CreateIssue(ctx, repo, name, body)
		if err != nil {
			return nil, err
		}
		return func(m *Model) tea.Cmd {
			m.issueDraft = creationForm{}
			m.count(actIssueCreated)
			page := m.startSearch(RepoQuery(is.Repo))
			m.resume = nil
			m.appendIssue(is)
			m.touched[is.Key()] = time.Now()
			m.applyFilter()
			open := m.openIssue(0)
			m.status = "created " + is.Key() + " · " + is.URL
			return tea.Batch(page, open)
		}, nil
	})
}

func (m Model) viewCreate() string {
	f := m.issueDraft
	title := "Create issue"
	if m.creatingRepo {
		f = m.repoDraft
		title = "Create repository"
	}
	lines := []string{headerStyle.Render(title), f.fields[0].View(), f.fields[1].View()}
	if m.creatingRepo {
		visibility := "private"
		if !f.private {
			visibility = "public"
		}
		line := "visibility: " + visibility + " (space toggles)"
		if f.focus == 3 {
			line = selectedStyle.Render(line)
		}
		lines = append(lines, f.fields[2].View(), line)
	} else {
		lines = append(lines, "body:", f.body.View())
	}
	help := "tab/shift+tab fields · ctrl+s create · esc close (draft kept)"
	if !m.creatingRepo {
		help += " · ctrl+e $EDITOR"
	}
	lines = append(lines, m.footer(strings.Join(wrapKeys(strings.Split(help, " · "), m.width), "\n")))
	return strings.Join(lines, "\n")
}
