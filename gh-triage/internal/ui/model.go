// Package ui is the Bubble Tea interface for gh-triage.
package ui

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/cli/go-gh/v2/pkg/browser"
	"github.com/trepidity/gh-triage/internal/github"
)

// Options configures a Model.
type Options struct {
	Query     string        // initial search; empty opens the repo switcher
	Pins      []github.Repo // listed first in the repo switcher
	RepoCache string        // repo-list cache file; empty disables caching
	Style     string        // glamour style: "dark", "light" or "notty" (default)
}

type screen int

const (
	screenList screen = iota
	screenIssue
)

type mode int

const (
	modeNone mode = iota
	modeFilter
	modeSearch
	modeComment
	modeCloseReason
	modeSwitcher
)

// prefetchWindow is how close the cursor may get to the end of the loaded issues
// before the next search page is requested.
const prefetchWindow = 10

const loadingMoreStatus = "loading more issues…"

type Model struct {
	client        github.Client
	opts          Options
	width, height int
	screen        screen
	mode          mode
	status        string
	initCmd       tea.Cmd

	// queue
	query       string
	gen         int // bumped on every new search so late pages from an old one are dropped
	issues      []github.Issue
	closed      map[string]bool
	hasMore     bool
	loadingPage bool

	// list screen
	filter  textinput.Model
	visible []int // indices into issues that pass the filter
	cursor  int   // index into visible; shared by both screens
	offset  int   // first row drawn in the list

	// issue screen
	comments        map[string][]github.Comment // present = loaded
	commentsLoading map[string]bool
	viewport        viewport.Model
	renderer        *glamour.TermRenderer

	editor            textarea.Model
	closeAfterComment bool
	busy              bool // a comment or close request is in flight; keys are ignored

	prompt textinput.Model
	sw     switcher
}

type (
	searchLoadedMsg struct {
		gen     int
		issues  []github.Issue
		hasMore bool
		err     error
	}
	commentsLoadedMsg struct {
		key      string
		comments []github.Comment
		err      error
	}
	statusMsg string
)

func New(client github.Client, opts Options) Model {
	if opts.Style == "" {
		opts.Style = "notty"
	}
	m := Model{
		client: client, opts: opts, width: 80, height: 24,
		closed: map[string]bool{}, comments: map[string][]github.Comment{}, commentsLoading: map[string]bool{},
		filter: newInput("/ "), prompt: newInput("search: "),
		viewport: viewport.New(80, 20),
		editor:   newEditor(),
		sw:       switcher{input: newInput("repo: ")},
	}
	m.renderer = newRenderer(opts.Style, m.width)
	if opts.Query != "" {
		m.initCmd = m.startSearch(opts.Query)
	} else {
		m.initCmd = m.openSwitcher()
	}
	return m
}

func newInput(prompt string) textinput.Model {
	ti := textinput.New()
	ti.Prompt = prompt
	ti.Cursor.SetMode(cursor.CursorStatic)
	return ti
}

func newRenderer(style string, width int) *glamour.TermRenderer {
	r, err := glamour.NewTermRenderer(glamour.WithStandardStyle(style), glamour.WithWordWrap(max(width-4, 20)))
	if err != nil {
		return nil
	}
	return r
}

// RepoQuery is the queue for one repo: its open issues.
func RepoQuery(r github.Repo) string { return "repo:" + r.String() + " is:issue is:open" }

// issueQuery keeps pull requests out of any user-supplied query.
func issueQuery(q string) string {
	if strings.Contains(q, "is:issue") {
		return q
	}
	return strings.TrimSpace(q) + " is:issue"
}

func (m Model) Init() tea.Cmd { return m.initCmd }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.renderer = newRenderer(m.opts.Style, m.width)
		m.refreshIssue()
	case searchLoadedMsg:
		cmd = m.onSearchLoaded(msg)
	case commentsLoadedMsg:
		m.onCommentsLoaded(msg)
	case statusMsg:
		m.status = string(msg)
	case commentPostedMsg:
		m.onCommentPosted(msg)
	case closedMsg:
		cmd = m.onClosed(msg)
	case reposLoadedMsg:
		m.onReposLoaded(msg)
	case tea.KeyMsg:
		cmd = m.onKey(msg)
	}
	m.layout()
	return m, cmd
}

func (m *Model) onKey(k tea.KeyMsg) tea.Cmd {
	if k.String() == "ctrl+c" {
		return tea.Quit
	}
	if m.busy {
		return nil
	}
	switch m.mode {
	case modeFilter:
		return m.keyFilter(k)
	case modeSearch:
		return m.keySearch(k)
	case modeComment:
		return m.keyComment(k)
	case modeCloseReason:
		return m.keyCloseReason(k)
	case modeSwitcher:
		return m.keySwitcher(k)
	}
	if m.screen == screenIssue {
		return m.keyIssue(k)
	}
	return m.keyList(k)
}

func (m *Model) keyList(k tea.KeyMsg) tea.Cmd {
	switch k.String() {
	case "q":
		return tea.Quit
	case "r":
		return m.openSwitcher()
	case "j", "down":
		m.moveCursor(m.cursor + 1)
		return m.maybeFetchMore()
	case "k", "up":
		m.moveCursor(m.cursor - 1)
	case "enter":
		if len(m.visible) > 0 {
			return m.openIssue(m.cursor)
		}
	case "/":
		m.mode = modeFilter
		return m.filter.Focus()
	case "s":
		m.mode = modeSearch
		m.prompt.SetValue("")
		return m.prompt.Focus()
	}
	return nil
}

func (m *Model) keyFilter(k tea.KeyMsg) tea.Cmd {
	switch k.String() {
	case "enter":
		m.mode = modeNone
		m.filter.Blur()
		return nil
	case "esc":
		m.mode = modeNone
		m.filter.Blur()
		m.filter.SetValue("")
		m.applyFilter()
		return nil
	}
	var cmd tea.Cmd
	m.filter, cmd = m.filter.Update(k)
	m.applyFilter()
	return cmd
}

func (m *Model) keySearch(k tea.KeyMsg) tea.Cmd {
	switch k.String() {
	case "enter":
		q := strings.TrimSpace(m.prompt.Value())
		m.prompt.Blur()
		m.mode = modeNone
		if q == "" {
			return nil
		}
		return m.startSearch(q)
	case "esc":
		m.prompt.Blur()
		m.mode = modeNone
		return nil
	}
	var cmd tea.Cmd
	m.prompt, cmd = m.prompt.Update(k)
	return cmd
}

func (m *Model) keyIssue(k tea.KeyMsg) tea.Cmd {
	switch k.String() {
	case "esc":
		m.screen = screenList
		m.status = ""
		return nil
	case "n":
		if m.cursor+1 < len(m.visible) {
			return m.openIssue(m.cursor + 1)
		}
		if m.hasMore || m.loadingPage {
			m.status = loadingMoreStatus
			return m.maybeFetchMore()
		}
		m.status = "end of queue"
		return nil
	case "p":
		if m.cursor > 0 {
			return m.openIssue(m.cursor - 1)
		}
		m.status = "start of queue"
		return nil
	case "c":
		return m.startComment(false)
	case "X":
		return m.startComment(true)
	case "x":
		return m.startClose()
	case "r":
		return m.openSwitcher()
	case "o":
		if is, ok := m.current(); ok {
			return openBrowser(is.URL)
		}
		return nil
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(k)
	return cmd
}

func (m *Model) moveCursor(to int) {
	if to >= 0 && to < len(m.visible) {
		m.cursor = to
	}
}

func (m *Model) current() (github.Issue, bool) {
	if m.cursor < len(m.visible) {
		return m.issues[m.visible[m.cursor]], true
	}
	return github.Issue{}, false
}

func (m *Model) currentIndex() int {
	if m.cursor < len(m.visible) {
		return m.visible[m.cursor]
	}
	return -1
}

func (m *Model) indexOf(key string) int {
	for i, is := range m.issues {
		if is.Key() == key {
			return i
		}
	}
	return -1
}

// startSearch replaces the queue with the results of q.
func (m *Model) startSearch(q string) tea.Cmd {
	m.gen++
	m.query = issueQuery(q)
	m.issues, m.visible = nil, nil
	m.closed = map[string]bool{}
	m.cursor, m.offset = 0, 0
	m.hasMore, m.loadingPage = false, true
	m.filter.SetValue("")
	m.screen, m.mode, m.status = screenList, modeNone, ""
	return m.fetchPage()
}

// fetchPage loads the issues after the oldest one already loaded (results are newest first).
func (m *Model) fetchPage() tea.Cmd {
	client, q, gen := m.client, m.query, m.gen
	var before time.Time
	if n := len(m.issues); n > 0 {
		before = m.issues[n-1].CreatedAt
	}
	return func() tea.Msg {
		issues, hasMore, err := client.SearchIssues(context.Background(), q, before)
		return searchLoadedMsg{gen: gen, issues: issues, hasMore: hasMore, err: err}
	}
}

// maybeFetchMore requests the next page when the cursor nears the last visible row, so a
// filter that matches little keeps pulling pages. At most one request is in flight.
func (m *Model) maybeFetchMore() tea.Cmd {
	if !m.hasMore || m.loadingPage || m.cursor < len(m.visible)-prefetchWindow {
		return nil
	}
	m.loadingPage = true
	return m.fetchPage()
}

func (m *Model) onSearchLoaded(msg searchLoadedMsg) tea.Cmd {
	if msg.gen != m.gen {
		return nil
	}
	m.loadingPage = false
	if m.status == loadingMoreStatus {
		m.status = ""
	}
	if msg.err != nil {
		m.status = "search failed: " + msg.err.Error()
		return nil
	}
	seen := make(map[string]bool, len(m.issues)+len(msg.issues))
	for _, is := range m.issues {
		seen[is.Key()] = true
	}
	added := 0
	for _, is := range msg.issues {
		if !seen[is.Key()] { // created:<= repeats issues sharing the boundary timestamp
			seen[is.Key()] = true
			m.issues = append(m.issues, is)
			added++
		}
	}
	m.hasMore = msg.hasMore && added > 0 // a page of only repeats would request itself forever
	m.applyFilter()
	return m.maybeFetchMore()
}

// applyFilter recomputes visible rows, keeping the cursor on the same issue when it survives.
func (m *Model) applyFilter() {
	current := m.currentIndex()
	f := strings.ToLower(m.filter.Value())
	m.visible = m.visible[:0]
	m.cursor = 0
	for i, is := range m.issues {
		if f != "" && !strings.Contains(strings.ToLower(is.Key()+" "+is.Title+" "+is.Author), f) {
			continue
		}
		if i == current {
			m.cursor = len(m.visible)
		}
		m.visible = append(m.visible, i)
	}
}

// openIssue shows visible row vi and loads its comments, prefetching the next issue's.
func (m *Model) openIssue(vi int) tea.Cmd {
	m.cursor = vi
	m.screen, m.mode, m.status = screenIssue, modeNone, ""
	m.refreshIssue()
	m.viewport.GotoTop()
	cmds := []tea.Cmd{m.maybeFetchMore()}
	for _, i := range []int{vi, vi + 1} {
		if i < len(m.visible) {
			cmds = append(cmds, m.loadComments(m.issues[m.visible[i]]))
		}
	}
	return tea.Batch(cmds...)
}

func (m *Model) loadComments(is github.Issue) tea.Cmd {
	key := is.Key()
	if _, loaded := m.comments[key]; loaded || m.commentsLoading[key] {
		return nil
	}
	m.commentsLoading[key] = true
	client := m.client
	return func() tea.Msg {
		cs, err := client.GetComments(context.Background(), is.Repo, is.Number)
		return commentsLoadedMsg{key: key, comments: cs, err: err}
	}
}

func (m *Model) onCommentsLoaded(msg commentsLoadedMsg) {
	delete(m.commentsLoading, msg.key)
	is, ok := m.current()
	isCurrent := ok && is.Key() == msg.key
	if msg.err != nil {
		if isCurrent {
			m.status = "loading comments failed: " + msg.err.Error()
		}
		return
	}
	m.comments[msg.key] = msg.comments
	if isCurrent && m.screen == screenIssue {
		m.refreshIssue()
	}
}

func openBrowser(url string) tea.Cmd {
	return func() tea.Msg {
		if err := browser.New("", io.Discard, io.Discard).Browse(url); err != nil {
			return statusMsg("open browser failed: " + err.Error())
		}
		return nil
	}
}
