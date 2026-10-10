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
	gansi "github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	"github.com/cli/go-gh/v2/pkg/browser"
	"github.com/trepidity/gh-triage/internal/config"
	"github.com/trepidity/gh-triage/internal/github"
)

// Options configures a Model.
type Options struct {
	Query     string        // initial search; empty opens the repo switcher
	Pins      []github.Repo // listed first in the repo switcher
	RepoCache string        // repo-list cache file; empty disables caching
	Style     string        // glamour style: "dark", "light" or "notty" (default)

	ActionTimeout time.Duration              // limit for each GitHub request; zero means defaultActionTimeout
	Queries       []config.Named             // saved searches, listed first when s is pressed
	Templates     []config.Named             // reply templates, inserted with ctrl+t
	Positions     map[string]config.Position // last viewed issue by normalized query
	WatchInterval time.Duration              // zero disables foreground polling
	// Tick schedules the next poll; tea.Tick when nil. Tests substitute one they fire by hand.
	Tick func(time.Duration, func(time.Time) tea.Msg) tea.Cmd
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
	modeComment
	modeCloseReason
	modePicker
	modeDupRef
	modeConfirm
	modeJump
	modeCreate
	modeReview
)

// prefetchWindow is how close the cursor may get to the end of the loaded issues
// before the next search page is requested.
const prefetchWindow = 10

const loadingMoreStatus = "loading more issues…"

const defaultActionTimeout = 30 * time.Second

type Model struct {
	client        github.Client
	opts          Options
	width, height int
	screen        screen
	mode          mode
	status        string
	initCmd       tea.Cmd

	// queue
	query         string
	positions     map[string]config.Position // updated on every issue open; written by main on quit
	resume        *config.Position           // where the current query should land while pages load
	gen           int                        // bumped on every new search so late pages from an old one are dropped
	issues        []github.Issue
	byKey         map[string]int    // issue key → index in issues
	issueOrder    []int             // presentation order: active issues first, then the created-time queue
	pageOrder     []int             // issues reached through created-time paging, in that order, for resume
	pageKeys      map[string]bool   // keys paging has reached; a poll may have added the issue first
	pageBefore    time.Time         // created-time keyset cursor for the next page
	stateOverride map[string]string // key → "open"/"closed" confirmed this session; wins over search results (the index lags both ways)
	hasMore       bool
	loadingPage   bool

	// watching (watch.go)
	watchFrom     time.Time            // when the current query opened; only later activity is surfaced
	watchSince    time.Time            // newest update the feed has returned for the current query
	activity      map[string]time.Time // key → latest activity by others since the query opened; these rows lead
	unseen        map[string]bool      // activity not yet viewed, marked ● in the list
	touched       map[string]time.Time // key → when a change of ours last landed; the feed skips updates up to then
	watchFailures int                  // consecutive failed polls; each doubles the interval

	// list screen
	filter     textinput.Model
	visible    []int  // indices into issues that pass the filter
	cursor     int    // index into visible; shared by both screens
	offset     int    // first row drawn in the list
	jumpDigits string // Vim number prefix or : command
	jumpTarget int    // issue number being sought across pages; zero means idle

	// issue screen
	comments        map[string][]github.Comment // present = loaded
	commentsLoading map[string]bool
	commentsErr     map[string]bool // present = last load failed; R retries
	viewport        viewport.Model
	renderer        *glamour.TermRenderer

	me           string                   // signed-in login, learned on first `a`
	labelOpts    map[github.Repo][]string // repo labels, fetched once per session
	labelColors  map[string]string        // lowercased label name → GitHub hex color, from every loaded page
	assigneeOpts map[github.Repo][]string // assignable users, fetched once per session

	editor            textarea.Model
	closeAfterComment bool
	drafts            map[string]string // unsent comment text by issue key (esc keeps it)
	ref               textinput.Model   // "duplicate of:" prompt
	dupCommented      map[string]string // key → ref already commented as duplicate; d then only closes
	lastClose         *github.Issue     // what u reopens; replaced by each close
	confirm           confirmation      // pending y/n question while mode is modeConfirm
	transferred       map[string]string // key → new URL for issues moved this session
	busy              bool              // a GitHub write is in flight; keys are ignored
	fullHelp          bool              // ? on the issue screen: every key instead of the short list
	tally             [numActions]int

	pk                    picker
	repos                 []github.Repo // every accessible repo; nil until the cache or API answers
	reposFetched          bool          // ListRepos already requested this session
	createdRepos          []github.Repo // confirmed this session; retained across late repo reads
	issueDraft, repoDraft creationForm
	creatingRepo          bool
	createReturn          mode
	organizations         []string
	organizationsLoaded   bool
	organizationsLoading  bool
	organizationsErr      error
	pulls                 map[string]github.PullRequest
	pullsLoading          map[string]bool
	pullsErr              map[string]error
	merged                map[string]bool
	reviewDrafts          map[string]string
	reviewEvent           github.ReviewEvent
	reviewHead            string
	pullFiles             bool
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
		pulls: map[string]github.PullRequest{}, pullsLoading: map[string]bool{}, pullsErr: map[string]error{},
		merged: map[string]bool{}, reviewDrafts: map[string]string{},
		client: client, opts: opts, width: 80, height: 24,
		stateOverride: map[string]string{}, comments: map[string][]github.Comment{}, commentsLoading: map[string]bool{},
		touched: map[string]time.Time{}, drafts: map[string]string{}, commentsErr: map[string]bool{},
		ref: newInput("duplicate of: "), dupCommented: map[string]string{}, transferred: map[string]string{},
		labelOpts: map[github.Repo][]string{}, labelColors: map[string]string{}, assigneeOpts: map[github.Repo][]string{},
		filter:   newInput("/ "),
		viewport: viewport.New(80, 20),
		editor:   newEditor(),
	}
	m.renderer = newRenderer(opts.Style, 0)
	m.positions = map[string]config.Position{}
	for q, p := range opts.Positions {
		m.positions[q] = p
	}
	if opts.Query != "" {
		m.initCmd = m.startSearch(opts.Query)
	} else {
		m.initCmd = m.openSwitcher()
	}
	m.initCmd = tea.Batch(m.initCmd, m.scheduleWatch())
	return m
}

func newInput(prompt string) textinput.Model {
	ti := textinput.New()
	ti.Prompt = prompt
	ti.Cursor.SetMode(cursor.CursorStatic)
	return ti
}

// readingWidth caps body text so long lines stay readable on wide terminals.
const readingWidth = 100

// newRenderer uses width for tables, or zero for prose wrapped by wrapBody.
func newRenderer(style string, width int) *glamour.TermRenderer {
	styleOpt := glamour.WithStandardStyle(style)
	if cfg, ok := readerStyle(style); ok {
		styleOpt = glamour.WithStyles(cfg)
	}
	r, err := glamour.NewTermRenderer(styleOpt, glamour.WithWordWrap(width))
	if err != nil {
		return nil
	}
	return r
}

// readerStyle is glamour's dark or light style with quieter inline code: a soft color
// instead of a padded red block, so code spans read as part of the sentence.
func readerStyle(style string) (gansi.StyleConfig, bool) {
	var cfg gansi.StyleConfig
	var code string
	switch style {
	case "dark":
		cfg, code = styles.DarkStyleConfig, "117"
	case "light":
		cfg, code = styles.LightStyleConfig, "25"
	default:
		return cfg, false
	}
	cfg.Code = gansi.StyleBlock{StylePrimitive: gansi.StylePrimitive{Color: &code}}
	return cfg, true
}

// RepoQuery is the queue for one repo: its open issues.
func RepoQuery(r github.Repo) string { return "repo:" + r.String() + " is:issue is:open" }

// Searches default to issues unless a type is explicitly selected.
func issueQuery(q string) string {
	for _, token := range strings.Fields(q) {
		switch strings.ToLower(token) {
		case "is:issue", "type:issue", "is:pr", "type:pr":
			return q
		}
	}
	return strings.TrimSpace(q) + " is:issue"
}

func (m Model) Init() tea.Cmd { return m.initCmd }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.refreshIssue()
	case searchLoadedMsg:
		cmd = m.onSearchLoaded(msg)
	case watchTickMsg:
		cmd = m.onWatchTick()
	case watchLoadedMsg:
		cmd = m.onWatchLoaded(msg)
	case commentsLoadedMsg:
		m.onCommentsLoaded(msg)
	case statusMsg:
		m.status = string(msg)
	case actionDoneMsg:
		cmd = m.onActionDone(msg)
	case editorDoneMsg:
		m.onEditorDone(msg)
	case optionsLoadedMsg:
		m.onOptionsLoaded(msg)
	case reposLoadedMsg:
		m.onReposLoaded(msg)
	case pullLoadedMsg:
		m.onPullLoaded(msg)
	case organizationsLoadedMsg:
		m.organizationsLoading = false
		m.organizationsErr = msg.err
		if msg.err == nil {
			m.organizations, m.organizationsLoaded = msg.orgs, true
		}
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
	case modeComment:
		return m.keyComment(k)
	case modeCloseReason:
		return m.keyCloseReason(k)
	case modePicker:
		return m.keyPicker(k)
	case modeDupRef:
		return m.keyDupRef(k)
	case modeConfirm:
		return m.keyConfirm(k)
	case modeJump:
		return m.keyJump(k)
	case modeCreate:
		return m.keyCreate(k)
	case modeReview:
		return m.keyReview(k)
	}
	if k.String() == "P" {
		return m.togglePulls()
	}
	if k.String() == "N" {
		return m.startCreation(false)
	}
	if k.String() == "ctrl+r" {
		return m.startCreation(true)
	}
	if m.screen == screenIssue {
		return m.keyIssue(k)
	}
	return m.keyList(k)
}

func (m *Model) keyList(k tea.KeyMsg) tea.Cmd {
	// Any new list command cancels a seek; an in-flight page may still finish.
	if m.jumpTarget != 0 {
		m.jumpTarget, m.status = 0, ""
	}
	if m.editJumpDigits(k) {
		m.resume = nil
		return nil
	}
	if k.String() == "G" && m.jumpDigits != "" {
		return m.startJump()
	}
	m.jumpDigits = ""
	switch k.String() {
	case ":":
		m.mode, m.resume = modeJump, nil
	case "esc":
		m.status = ""
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
	case "u":
		return m.undoClose()
	case "/":
		m.mode = modeFilter
		return m.filter.Focus()
	case "s":
		return m.openQueries()
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

func (m *Model) keyIssue(k tea.KeyMsg) tea.Cmd {
	if is, ok := m.current(); ok && is.PullRequest {
		switch k.String() {
		case "v":
			return m.openReview()
		case "M":
			return m.openMerge()
		case "f":
			m.pullFiles = !m.pullFiles
			m.refreshIssue()
			m.viewport.GotoTop()
			return nil
		case "R":
			m.status = ""
			delete(m.comments, is.Key())
			delete(m.commentsErr, is.Key())
			return tea.Batch(m.loadPull(is), m.loadComments(is))
		case "x", "X", "d", "t":
			m.status = "use v to review or M to merge this pull request"
			return nil
		}
	}
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
	case "R":
		if is, ok := m.current(); ok && m.commentsErr[is.Key()] {
			delete(m.commentsErr, is.Key())
			m.status = ""
			cmd := m.loadComments(is)
			m.refreshIssue()
			return cmd
		}
		return nil
	case "d":
		return m.startDuplicate()
	case "u":
		return m.undoClose()
	case "c":
		return m.startComment(false)
	case "X":
		return m.startComment(true)
	case "x":
		return m.startClose()
	case "l":
		return m.openOptions(pickLabels)
	case "A":
		return m.openOptions(pickAssignees)
	case "a":
		return m.toggleSelf()
	case "+":
		return m.openReactions()
	case "r":
		return m.openSwitcher()
	case "?":
		m.fullHelp = !m.fullHelp
		return nil
	case "L":
		return m.startLock()
	case "t":
		return m.startTransfer()
	case "o":
		if is, ok := m.current(); ok {
			url := is.URL
			if moved, ok := m.transferred[is.Key()]; ok {
				url = moved
			}
			return openBrowser(url)
		}
		return nil
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(k)
	return cmd
}

func (m *Model) moveCursor(to int) {
	m.resume = nil // the user chose where to be; a late page must not move them
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

func (m *Model) isClosed(is github.Issue) bool {
	if s, ok := m.stateOverride[is.Key()]; ok {
		return s == "closed"
	}
	return is.State == "closed"
}

func (m *Model) timeout() time.Duration {
	if m.opts.ActionTimeout > 0 {
		return m.opts.ActionTimeout
	}
	return defaultActionTimeout
}

// fetch runs a background read under the request timeout. Unlike run it leaves keys live.
func (m *Model) fetch(f func(ctx context.Context) tea.Msg) tea.Cmd {
	timeout := m.timeout()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		return f(ctx)
	}
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

func (m *Model) indexOf(key string) int {
	if i, ok := m.byKey[key]; ok {
		return i
	}
	return -1
}

// appendIssue adds an issue to the backing queue, at the end of the presentation order.
func (m *Model) appendIssue(is github.Issue) int {
	i := len(m.issues)
	m.issues = append(m.issues, is)
	m.byKey[is.Key()] = i
	m.issueOrder = append(m.issueOrder, i)
	for name, c := range is.LabelColors {
		m.labelColors[strings.ToLower(name)] = c
	}
	return i
}

// startSearch replaces the queue with the results of q.
func (m *Model) startSearch(q string) tea.Cmd {
	m.gen++
	m.jumpDigits, m.jumpTarget = "", 0
	m.query = issueQuery(q)
	m.resume = nil
	if p, ok := m.positions[m.query]; ok {
		m.resume = &p
	}
	m.issues, m.issueOrder, m.pageOrder, m.visible = nil, nil, nil, nil
	m.byKey, m.pageKeys = map[string]int{}, map[string]bool{}
	m.pageBefore = time.Time{}
	m.watchFrom = time.Now()
	m.watchSince = m.watchFrom
	m.activity, m.unseen = map[string]time.Time{}, map[string]bool{}
	m.cursor, m.offset = 0, 0
	m.hasMore, m.loadingPage = false, true
	m.filter.SetValue("")
	m.screen, m.mode, m.status = screenList, modeNone, ""
	return m.fetchPage()
}

// fetchPage loads the issues after the oldest one already loaded (results are newest first).
func (m *Model) fetchPage() tea.Cmd {
	client, q, gen := m.client, m.query, m.gen
	before := m.pageBefore
	return m.fetch(func(ctx context.Context) tea.Msg {
		issues, hasMore, err := client.SearchIssues(ctx, q, before)
		return searchLoadedMsg{gen: gen, issues: issues, hasMore: hasMore, err: err}
	})
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
		m.status = "search failed: " + firstLine(msg.err.Error())
		m.resume = nil
		m.jumpTarget = 0
		return nil
	}
	if n := len(msg.issues); n > 0 {
		m.pageBefore = msg.issues[n-1].CreatedAt
	}
	added := 0 // new to paging, even if a poll surfaced the issue first
	for _, is := range msg.issues {
		key := is.Key()
		if m.pageKeys[key] { // created:<= repeats issues sharing the boundary timestamp
			continue
		}
		m.pageKeys[key] = true
		added++
		i, ok := m.byKey[key]
		if !ok {
			i = m.appendIssue(is)
		}
		m.pageOrder = append(m.pageOrder, i)
	}
	m.hasMore = msg.hasMore && added > 0 // a page of only repeats would request itself forever
	m.applyFilter()
	if m.jumpTarget != 0 {
		return m.seekJump()
	}
	if m.resume != nil {
		return m.seekResume()
	}
	return m.maybeFetchMore()
}

// seekResume puts the cursor on the saved issue once it is loaded, or on the first older
// issue once the queue has passed it (it was closed since); otherwise it loads another page.
func (m *Model) seekResume() tea.Cmd {
	target := *m.resume
	row := make(map[int]int, len(m.visible)) // issue index → visible row
	for vi, i := range m.visible {
		row[i] = vi
	}
	for _, i := range m.pageOrder { // created order, so the saved issue comes before any older one
		vi, shown := row[i]
		if is := m.issues[i]; shown && (is.Key() == target.Key || is.CreatedAt.Before(target.CreatedAt)) {
			m.cursor, m.resume = vi, nil
			return nil
		}
	}
	if m.hasMore && !m.loadingPage {
		m.loadingPage = true
		return m.fetchPage()
	}
	m.resume = nil // never found: stay at the top
	return nil
}

// Positions is the last issue viewed per query, for main to save on quit.
func (m Model) Positions() map[string]config.Position { return m.positions }

// applyFilter recomputes visible rows, keeping the cursor on the same issue when it survives.
func (m *Model) applyFilter() {
	current := m.currentIndex()
	f := strings.ToLower(m.filter.Value())
	m.visible = m.visible[:0]
	m.cursor = 0
	for _, i := range m.issueOrder {
		is := m.issues[i]
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
	m.resume = nil // the user chose where to be; a late page must not move them
	if is, ok := m.current(); ok {
		m.positions[m.query] = config.Position{Key: is.Key(), CreatedAt: is.CreatedAt}
		delete(m.unseen, is.Key())
	}
	m.screen, m.mode, m.status = screenIssue, modeNone, ""
	m.refreshIssue()
	m.viewport.GotoTop()
	cmds := []tea.Cmd{m.maybeFetchMore()}
	if is, ok := m.current(); ok && is.PullRequest {
		m.pullFiles = false
		cmds = append(cmds, m.loadPull(is))
	}
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
	return m.fetch(func(ctx context.Context) tea.Msg {
		cs, err := client.GetComments(ctx, is.Repo, is.Number)
		return commentsLoadedMsg{key: key, comments: cs, err: err}
	})
}

func (m *Model) onCommentsLoaded(msg commentsLoadedMsg) {
	delete(m.commentsLoading, msg.key)
	is, ok := m.current()
	isCurrent := ok && is.Key() == msg.key
	if msg.err != nil {
		m.commentsErr[msg.key] = true
		if isCurrent {
			m.status = "loading comments failed: " + firstLine(msg.err.Error())
		}
	} else {
		delete(m.commentsErr, msg.key)
		m.comments[msg.key] = msg.comments
	}
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
