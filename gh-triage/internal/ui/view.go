package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	headerStyle   = lipgloss.NewStyle().Bold(true)
	selectedStyle = lipgloss.NewStyle().Reverse(true)
	dimStyle      = lipgloss.NewStyle().Faint(true)
	statusStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	helpStyle     = lipgloss.NewStyle().Faint(true)
)

const (
	listHelp  = "j/k move · enter open · / filter · s search · r repos · q quit"
	issueHelp = "n/p next/prev · c comment · x close · X comment+close · o browser · r repos · esc list"
)

func (m Model) View() string {
	switch {
	case m.mode == modeSwitcher:
		return m.viewSwitcher()
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

// chromeHeight is the issue screen's non-viewport lines: title, meta, status, help,
// plus the comment editor while it is open.
func (m Model) chromeHeight() int {
	if m.mode == modeComment {
		return 4 + editorHeight
	}
	return 4
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
	if m.isClosed(is) {
		state = "✓ closed  "
	}
	line := truncate(fmt.Sprintf("%s%s  %s%s  @%s  %s  💬%d", marker, is.Key(), state, is.Title, is.Author, age(is.CreatedAt), is.Comments), m.width)
	switch {
	case vi == m.cursor:
		return selectedStyle.Render(line)
	case m.isClosed(is):
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
	meta := fmt.Sprintf("%s · @%s · %s · %d of %d", state, is.Author, age(is.CreatedAt), m.cursor+1, len(m.visible))
	if len(is.Labels) > 0 {
		meta += " · " + strings.Join(is.Labels, ", ")
	}
	return headerStyle.Render(truncate(is.Key()+" · "+is.Title, m.width)) + "\n" +
		truncate(meta, m.width) + "\n" +
		m.viewport.View() + "\n" + m.editorView() +
		m.footer(issueHelp)
}

// footer is the status line (or the active input) above the help line.
func (m Model) footer(help string) string {
	var line string
	switch {
	case m.busy:
		line = statusStyle.Render(m.status)
	case m.mode == modeFilter:
		line = m.filter.View()
	case m.mode == modeSearch:
		line = m.prompt.View()
	case m.mode == modeCloseReason:
		line = statusStyle.Render("close as: c completed · n not planned · esc cancel")
	default:
		line = statusStyle.Render(m.status)
	}
	return line + "\n" + helpStyle.Render(truncate(help, m.width))
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
	if !loaded {
		b.WriteString("\n---\n\n_Loading comments…_\n")
	}
	for _, c := range cs {
		fmt.Fprintf(&b, "\n---\n\n**@%s** · %s\n\n%s\n", c.Author, age(c.CreatedAt), c.Body)
	}
	m.viewport.SetContent(m.render(b.String()))
}

func (m *Model) render(md string) string {
	if m.renderer != nil {
		if out, err := m.renderer.Render(md); err == nil {
			return out
		}
	}
	return md
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
