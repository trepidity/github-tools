package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/trepidity/gh-triage/internal/github"
)

var (
	headerStyle   = lipgloss.NewStyle().Bold(true)
	selectedStyle = lipgloss.NewStyle().Reverse(true)
	dimStyle      = lipgloss.NewStyle().Faint(true)
	statusStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	helpStyle     = lipgloss.NewStyle().Faint(true)
)

const (
	listHelp    = "j/k move · enter open · / filter · s search · r repos · u undo close · q quit"
	issueHelp   = "esc list · n/p next/prev · c comment · x close · ? more"
	commentHelp = "ctrl+s send · ctrl+t template · ctrl+e $EDITOR · esc cancel"
)

func (m Model) View() string {
	switch {
	case m.mode == modePicker:
		return m.viewPicker()
	case m.screen == screenIssue:
		return m.viewIssue()
	}
	return m.viewList()
}

// layout sizes the viewport and list scroll to the terminal. Called after every Update.
func (m *Model) layout() {
	m.editor.SetWidth(m.width)
	m.viewport.Width = m.width
	m.viewport.Height = max(m.height-m.chromeHeight(), 3)
	rows := m.listRows()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+rows {
		m.offset = m.cursor - rows + 1
	}
}

// issueKeys is the full issue-screen key list shown by ?.
var issueKeys = []string{"esc list", "n/p next/prev", "c comment", "x close", "X comment+close", "d dup",
	"l labels", "a/A assign", "+ react", "L lock", "t transfer", "u undo", "R retry comments",
	"o browser", "r repos", "? less"}

// chromeHeight is the issue screen's non-viewport lines: title (one or two), meta, rule,
// status and help, plus the comment editor while it is open.
func (m Model) chromeHeight() int {
	h := 3 + strings.Count(m.issueHelp(), "\n") + 1
	if is, ok := m.current(); ok {
		h += len(m.titleLines(is))
	} else {
		h++
	}
	if m.mode == modeComment {
		h += editorHeight
	}
	return h
}

// titleLines is the issue title wrapped to at most two lines; only a longer title is cut.
func (m Model) titleLines(is github.Issue) []string {
	lines := strings.Split(ansi.Wrap(is.Key()+" · "+is.Title, m.width, ""), "\n")
	if len(lines) > 2 {
		lines = []string{lines[0], truncate(strings.Join(lines[1:], " "), m.width)}
	}
	return lines
}

// wrapKeys joins key hints with " · ", starting a new line rather than splitting a hint.
func wrapKeys(keys []string, width int) []string {
	var lines []string
	line := ""
	for _, k := range keys {
		switch {
		case line == "":
			line = k
		case ansi.StringWidth(line+" · "+k) <= width:
			line += " · " + k
		default:
			lines = append(lines, line)
			line = k
		}
	}
	return append(lines, line)
}

// listRows is how many issue rows fit: everything but the header, status and help lines.
func (m Model) listRows() int { return max(m.height-3, 1) }

func (m Model) viewList() string {
	var b strings.Builder
	head := fmt.Sprintf("gh triage · %s · %d loaded", m.query, len(m.issues))
	if m.hasMore {
		head += "+"
	}
	if m.loadingPage {
		head += " · loading…"
	}
	if v := m.filter.Value(); v != "" && m.mode != modeFilter {
		head += " · filter: " + v
	}
	b.WriteString(headerStyle.Render(truncate(head, m.width)) + "\n")
	if len(m.visible) == 0 && !m.loadingPage {
		b.WriteString("  No issues match.\n")
	}
	for vi := m.offset; vi < len(m.visible) && vi < m.offset+m.listRows(); vi++ {
		b.WriteString(m.row(vi) + "\n")
	}
	b.WriteString(m.footer(listHelp))
	return b.String()
}

func (m Model) row(vi int) string {
	is := m.issues[m.visible[vi]]
	marker, state := "  ", ""
	if vi == m.cursor {
		marker = "> "
	}
	_, moved := m.transferred[is.Key()]
	switch {
	case moved:
		state = "→ moved  "
	case m.isClosed(is):
		state = "✓ closed  "
	}
	line := truncate(fmt.Sprintf("%s%s  %s%s  @%s  %s  💬%d", marker, is.Key(), state, is.Title, is.Author, age(is.CreatedAt), is.Comments), m.width)
	switch {
	case vi == m.cursor:
		return selectedStyle.Render(line)
	case m.isClosed(is) || moved:
		return dimStyle.Render(line)
	}
	return line
}

func (m Model) viewIssue() string {
	is, ok := m.current()
	if !ok {
		return m.viewList()
	}
	state := "open"
	if m.isClosed(is) {
		state = "✓ closed"
	}
	if url, moved := m.transferred[is.Key()]; moved {
		state = "→ moved to " + url
	}
	if is.Locked {
		state += " · 🔒 locked"
	}
	meta := fmt.Sprintf("%s · @%s · %s · %d of %d", state, is.Author, age(is.CreatedAt), m.cursor+1, len(m.visible))
	if len(is.Labels) > 0 {
		meta += " · " + strings.Join(is.Labels, ", ")
	}
	if len(is.Assignees) > 0 {
		meta += " · assigned: " + strings.Join(is.Assignees, ", ")
	}
	return headerStyle.Render(strings.Join(m.titleLines(is), "\n")) + "\n" +
		truncate(meta, m.width) + "\n" +
		dimStyle.Render(strings.Repeat("─", m.width)) + "\n" +
		m.viewport.View() + "\n" + m.editorView() +
		m.footer(m.issueHelp())
}

// footer is the status line (or the active input) above the help line.
func (m Model) footer(help string) string {
	var line string
	switch {
	case m.busy:
		line = statusStyle.Render(m.status)
	case m.mode == modeFilter:
		line = m.filter.View()
	case m.mode == modeConfirm:
		line = statusStyle.Render(m.confirm.question + " (y/n)")
	case m.mode == modeDupRef:
		line = m.ref.View() + "  " + statusStyle.Render(m.status)
	case m.mode == modeCloseReason:
		line = statusStyle.Render("close as: c completed · n not planned · esc cancel")
	case m.mode == modeComment && m.status == "":
		line = statusStyle.Render("writing comment · " + commentHelp)
	default:
		line = statusStyle.Render(m.status)
	}
	helpLines := strings.Split(help, "\n")
	for i, l := range helpLines {
		helpLines[i] = truncate(l, m.width)
	}
	return line + "\n" + helpStyle.Render(strings.Join(helpLines, "\n"))
}

// refreshIssue re-renders the current issue's body and comments into the viewport.
func (m *Model) refreshIssue() {
	is, ok := m.current()
	if !ok {
		m.viewport.SetContent("")
		return
	}
	var b strings.Builder
	body := strings.TrimSpace(is.Body)
	if body == "" {
		body = "_No description provided._"
	}
	b.WriteString(body + "\n")
	cs, loaded := m.comments[is.Key()]
	switch {
	case !loaded && m.commentsErr[is.Key()]:
		b.WriteString("\n---\n\n_Comments failed to load — press R to retry._\n")
	case !loaded:
		b.WriteString("\n---\n\n_Loading comments…_\n")
	}
	for _, c := range cs {
		fmt.Fprintf(&b, "\n---\n\n**@%s** · %s\n\n%s\n", c.Author, age(c.CreatedAt), c.Body)
	}
	m.viewport.SetContent(m.render(b.String()))
}

func (m *Model) render(md string) string {
	out := md
	if m.renderer != nil {
		if r, err := m.renderer.Render(md); err == nil {
			out = r
		}
	}
	return wrapBody(out, max(min(m.width-2, readingWidth+2), 20)) // +2: the document margin
}

func truncate(s string, width int) string { return ansi.Truncate(s, width, "…") }

func age(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
	return fmt.Sprintf("%dy", int(d.Hours()/24/365))
}

func (m Model) editorView() string {
	if m.mode != modeComment {
		return ""
	}
	return m.editor.View() + "\n"
}

// issueHelp swaps in the editor's keys while a comment is being written.
func (m Model) issueHelp() string {
	switch {
	case m.mode == modeComment:
		return commentHelp
	case m.fullHelp:
		return strings.Join(wrapKeys(issueKeys, m.width), "\n")
	}
	return issueHelp
}
