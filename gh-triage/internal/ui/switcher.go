package ui

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/trepidity/gh-triage/internal/config"
	"github.com/trepidity/gh-triage/internal/github"
)

type reposLoadedMsg struct {
	repos []github.Repo
	err   error
}

// orderRepos lists pins first (in pin order), then every other repo, each once.
// GitHub names are case-insensitive, so "O/C" and "o/c" are the same repo.
func orderRepos(pins, all []github.Repo) []github.Repo {
	seen := map[string]bool{}
	out := make([]github.Repo, 0, len(pins)+len(all))
	for _, list := range [][]github.Repo{pins, all} {
		for _, r := range list {
			k := strings.ToLower(r.String())
			if !seen[k] {
				seen[k] = true
				out = append(out, r)
			}
		}
	}
	return out
}

func (m *Model) openSwitcher() tea.Cmd { return m.openRepoPicker(pickRepo, "switch repo") }

// openRepoPicker lists pins, then every accessible repo: from the cache at once, and from
// GitHub once per session.
func (m *Model) openRepoPicker(kind pickerKind, title string) tea.Cmd {
	if m.repos == nil && m.opts.RepoCache != "" {
		for _, name := range config.ReadRepoCache(m.opts.RepoCache) {
			if r, err := github.ParseRepo(name); err == nil {
				m.repos = append(m.repos, r)
			}
		}
	}
	p := newPicker(kind, title, "repo: ", m.repoItems())
	p.loading = m.repos == nil
	return tea.Batch(m.openPicker(p), m.loadRepos())
}

func (m *Model) repoItems() []pickerItem {
	var items []pickerItem
	for _, r := range orderRepos(m.opts.Pins, m.repos) {
		items = append(items, pickerItem{label: r.String(), value: r.String()})
	}
	return items
}

// isRepoPicker reports whether the open picker lists repos (and so refreshes with them).
func (m *Model) isRepoPicker() bool {
	return m.mode == modePicker && (m.pk.kind == pickRepo || m.pk.kind == pickTransfer)
}

// loadRepos refreshes the repo list from GitHub once per session.
func (m *Model) loadRepos() tea.Cmd {
	if m.reposFetched {
		return nil
	}
	m.reposFetched = true
	client := m.client
	return m.fetch(func(ctx context.Context) tea.Msg {
		repos, err := client.ListRepos(ctx)
		return reposLoadedMsg{repos: repos, err: err}
	})
}

func (m *Model) onReposLoaded(msg reposLoadedMsg) {
	if m.isRepoPicker() {
		m.pk.loading = false
	}
	if msg.err != nil {
		m.reposFetched = false // retry next time a repo picker opens
		m.status = "loading repos failed: " + firstLine(msg.err.Error())
		return
	}
	m.repos = msg.repos
	if m.opts.RepoCache != "" {
		names := make([]string, len(msg.repos))
		for i, r := range msg.repos {
			names[i] = r.String()
		}
		_ = config.WriteRepoCache(m.opts.RepoCache, names) // best effort; next run refetches anyway
	}
	if m.isRepoPicker() {
		m.pk.setItems(m.repoItems()) // keeps the highlight on the same repo as rows shift
	}
}
