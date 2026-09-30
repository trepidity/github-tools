package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sahilm/fuzzy"
	"github.com/trepidity/gh-triage/internal/config"
	"github.com/trepidity/gh-triage/internal/github"
)

type reposLoadedMsg struct {
	repos []github.Repo
	err   error
}

type switcher struct {
	input   textinput.Model
	all     []github.Repo // every accessible repo; nil until the cache or API answers
	items   []github.Repo // rows currently shown
	cursor  int
	fetched bool // ListRepos already requested this session
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

func (m *Model) openSwitcher() tea.Cmd {
	m.mode = modeSwitcher
	m.sw.input.SetValue("")
	m.sw.cursor = 0
	if m.sw.all == nil && m.opts.RepoCache != "" {
		for _, name := range config.ReadRepoCache(m.opts.RepoCache) {
			if r, err := github.ParseRepo(name); err == nil {
				m.sw.all = append(m.sw.all, r)
			}
		}
	}
	m.filterSwitcher()
	return tea.Batch(m.sw.input.Focus(), m.loadRepos())
}

// loadRepos refreshes the repo list from GitHub once per session.
func (m *Model) loadRepos() tea.Cmd {
	if m.sw.fetched {
		return nil
	}
	m.sw.fetched = true
	client := m.client
	return m.fetch(func(ctx context.Context) tea.Msg {
		repos, err := client.ListRepos(ctx)
		return reposLoadedMsg{repos: repos, err: err}
	})
}

func (m *Model) onReposLoaded(msg reposLoadedMsg) {
	if msg.err != nil {
		m.sw.fetched = false // retry next time the switcher opens
		m.status = "loading repos failed: " + firstLine(msg.err.Error())
		return
	}
	var selected string
	if m.sw.cursor < len(m.sw.items) {
		selected = m.sw.items[m.sw.cursor].String()
	}
	m.sw.all = msg.repos
	if m.opts.RepoCache != "" {
		names := make([]string, len(msg.repos))
		for i, r := range msg.repos {
			names[i] = r.String()
		}
		_ = config.WriteRepoCache(m.opts.RepoCache, names) // best effort; next run refetches anyway
	}
	m.filterSwitcher()
	m.selectRepo(selected) // keep the highlight on the same repo as rows shift
}

func (m *Model) filterSwitcher() {
	ordered := orderRepos(m.opts.Pins, m.sw.all)
	q := m.sw.input.Value()
	if q == "" {
		m.sw.items = ordered
	} else {
		names := make([]string, len(ordered))
		for i, r := range ordered {
			names[i] = r.String()
		}
		m.sw.items = nil
		for _, match := range fuzzy.Find(q, names) {
			m.sw.items = append(m.sw.items, ordered[match.Index])
		}
	}
	m.sw.cursor = min(m.sw.cursor, max(len(m.sw.items)-1, 0))
}

func (m *Model) keySwitcher(k tea.KeyMsg) tea.Cmd {
	switch k.String() {
	case "esc":
		m.mode = modeNone
		m.sw.input.Blur()
		return nil
	case "down", "ctrl+n":
		if m.sw.cursor+1 < len(m.sw.items) {
			m.sw.cursor++
		}
		return nil
	case "up", "ctrl+p":
		if m.sw.cursor > 0 {
			m.sw.cursor--
		}
		return nil
	case "enter":
		if len(m.sw.items) == 0 {
			return nil
		}
		m.sw.input.Blur()
		return m.startSearch(RepoQuery(m.sw.items[m.sw.cursor]))
	}
	before := m.sw.input.Value()
	var cmd tea.Cmd
	m.sw.input, cmd = m.sw.input.Update(k)
	if m.sw.input.Value() != before {
		m.sw.cursor = 0 // new filter: highlight the best match
	}
	m.filterSwitcher()
	return cmd
}

func (m *Model) selectRepo(name string) {
	m.sw.cursor = 0
	for i, r := range m.sw.items {
		if r.String() == name {
			m.sw.cursor = i
			return
		}
	}
}

func (m Model) viewSwitcher() string {
	var b strings.Builder
	head := fmt.Sprintf("switch repo · %d repos", len(m.sw.items))
	if m.sw.all == nil {
		head += " · loading…"
	}
	b.WriteString(headerStyle.Render(head) + "\n" + m.sw.input.View() + "\n")
	rows := max(m.height-4, 1)
	first := max(0, m.sw.cursor-rows+1)
	for i := first; i < len(m.sw.items) && i < first+rows; i++ {
		line := "  " + m.sw.items[i].String()
		if i == m.sw.cursor {
			line = selectedStyle.Render("> " + m.sw.items[i].String())
		}
		b.WriteString(line + "\n")
	}
	b.WriteString(statusStyle.Render(m.status) + "\n" + helpStyle.Render("type to filter · ↑/↓ move · enter open · esc cancel"))
	return b.String()
}
