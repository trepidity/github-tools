package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sahilm/fuzzy"
	"github.com/trepidity/gh-triage/internal/github"
)

// pickerKind says what a picker's choice is for; picked dispatches on it.
type pickerKind int

const (
	pickRepo      pickerKind = iota // replace the queue with a repo's open issues
	pickLabels                      // set the current issue's labels
	pickAssignees                   // set the current issue's assignees
	pickReaction                    // react to the current issue
	pickQuery                       // run a saved or typed search
	pickTemplate                    // insert a reply template into the comment
	pickLock                        // lock the current issue with a reason
	pickTransfer                    // move the current issue to another repo
)

type pickerItem struct {
	label  string // shown and matched by the filter
	detail string // shown dimmed after the label
	value  string // what picked receives
}

// picker is the one list overlay (repos, labels, assignees, …). It lives in m.pk while
// mode is modePicker.
type picker struct {
	kind     pickerKind
	title    string
	input    textinput.Model
	all      []pickerItem
	items    []pickerItem // all, narrowed by the typed filter
	cursor   int
	multi    bool            // space toggles rows; enter submits every checked value
	checked  map[string]bool // by value
	freeText bool            // substring filter; enter with no matching row submits the typed text
	loading  bool
	returnTo mode // restored when the picker closes
}

func newPicker(kind pickerKind, title, prompt string, items []pickerItem) picker {
	p := picker{kind: kind, title: title, input: newInput(prompt), checked: map[string]bool{}}
	p.setItems(items)
	return p
}

// setItems replaces the rows, keeping the highlight on the same value when it survives.
func (p *picker) setItems(items []pickerItem) {
	var selected string
	if p.cursor < len(p.items) {
		selected = p.items[p.cursor].value
	}
	p.all = items
	p.filter()
	p.cursor = 0
	for i, it := range p.items {
		if it.value == selected {
			p.cursor = i
			break
		}
	}
}

func (p *picker) filter() {
	q := p.input.Value()
	switch {
	case q == "":
		p.items = p.all
	case p.freeText:
		p.items = nil
		for _, it := range p.all {
			if strings.Contains(strings.ToLower(it.label), strings.ToLower(q)) {
				p.items = append(p.items, it)
			}
		}
	default:
		labels := make([]string, len(p.all))
		for i, it := range p.all {
			labels[i] = it.label
		}
		p.items = nil
		for _, match := range fuzzy.Find(q, labels) {
			p.items = append(p.items, p.all[match.Index])
		}
	}
	p.cursor = min(p.cursor, max(len(p.items)-1, 0))
}

func (m *Model) openPicker(p picker) tea.Cmd {
	p.returnTo = m.mode
	m.pk = p
	m.mode = modePicker
	return m.pk.input.Focus()
}

func (m *Model) closePicker() tea.Cmd {
	m.mode = m.pk.returnTo
	m.pk.input.Blur()
	if m.mode == modeComment {
		return m.editor.Focus()
	}
	return nil
}

func (m *Model) keyPicker(k tea.KeyMsg) tea.Cmd {
	if m.pk.kind == pickRepo && k.String() == "ctrl+r" {
		return m.startCreation(true)
	}
	p := &m.pk
	switch k.String() {
	case "esc":
		return m.closePicker()
	case "down", "ctrl+n":
		if p.cursor+1 < len(p.items) {
			p.cursor++
		}
		return nil
	case "up", "ctrl+p":
		if p.cursor > 0 {
			p.cursor--
		}
		return nil
	case "enter":
		return m.submitPicker()
	case " ":
		if p.multi {
			if p.cursor < len(p.items) {
				v := p.items[p.cursor].value
				p.checked[v] = !p.checked[v]
			}
			return nil
		}
	}
	before := p.input.Value()
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(k)
	if p.input.Value() != before {
		p.cursor = 0 // new filter: highlight the best match
		p.filter()
	}
	return cmd
}

func (m *Model) submitPicker() tea.Cmd {
	p := m.pk
	var chosen []string
	if p.multi {
		for _, it := range p.all {
			if p.checked[it.value] {
				chosen = append(chosen, it.value)
			}
		}
	} else if p.cursor < len(p.items) {
		chosen = []string{p.items[p.cursor].value}
	}
	typed := strings.TrimSpace(p.input.Value())
	if !p.multi && len(chosen) == 0 && (!p.freeText || typed == "") {
		return nil // nothing to pick yet; stay open
	}
	return tea.Batch(m.closePicker(), m.picked(p.kind, chosen, typed))
}

// picked acts on a picker's result. chosen is the highlighted value (single) or every
// checked value (multi); typed is the filter text, used by free-text pickers.
func (m *Model) picked(kind pickerKind, chosen []string, typed string) tea.Cmd {
	switch kind {
	case pickRepo:
		r, err := github.ParseRepo(chosen[0])
		if err != nil {
			m.status = err.Error()
			return nil
		}
		return m.startSearch(RepoQuery(r))
	case pickLabels:
		return m.setLabels(chosen)
	case pickAssignees:
		return m.setAssignees(chosen)
	case pickReaction:
		for _, r := range github.AllReactions() {
			if r.String() == chosen[0] {
				return m.react(r)
			}
		}
		return nil
	case pickQuery:
		q := typed
		if len(chosen) > 0 {
			q = chosen[0]
		}
		return m.startSearch(q)
	case pickTemplate:
		m.editor.InsertString(chosen[0])
		return nil
	case pickLock:
		is, _ := m.current()
		for _, r := range github.AllLockReasons() {
			if r.String() == chosen[0] {
				return m.lock(is, r)
			}
		}
		return nil
	case pickTransfer:
		to, err := github.ParseRepo(chosen[0])
		if err != nil {
			m.status = err.Error()
			return nil
		}
		return m.confirmTransfer(to)
	}
	panic(fmt.Sprintf("unknown pickerKind %d", int(kind)))
}

func (m Model) viewPicker() string {
	p := m.pk
	var b strings.Builder
	head := fmt.Sprintf("%s · %d", p.title, len(p.items))
	if p.loading {
		head += " · loading…"
	}
	b.WriteString(headerStyle.Render(truncate(head, m.width)) + "\n" + p.input.View() + "\n")
	rows := max(m.height-4, 1)
	first := max(0, p.cursor-rows+1)
	for i := first; i < len(p.items) && i < first+rows; i++ {
		it := p.items[i]
		text := it.label
		if p.multi {
			box := "[ ] "
			if p.checked[it.value] {
				box = "[x] "
			}
			text = box + text
		}
		if it.detail != "" {
			text += "  " + dimStyle.Render(it.detail)
		}
		if i == p.cursor {
			b.WriteString(selectedStyle.Render(truncate("> "+text, m.width)) + "\n")
		} else {
			b.WriteString(truncate("  "+text, m.width) + "\n")
		}
	}
	b.WriteString(statusStyle.Render(m.status) + "\n" + helpStyle.Render(truncate(p.help(), m.width)))
	return b.String()
}

func (p picker) help() string {
	switch {
	case p.kind == pickRepo:
		return "ctrl+r new repo · type to filter · ↑/↓ move · enter choose · esc cancel"
	case p.multi:
		return "type to filter · ↑/↓ move · space toggle · enter apply · esc cancel"
	case p.freeText:
		return "type a search or pick a saved one · ↑/↓ move · enter run · esc cancel"
	}
	return "type to filter · ↑/↓ move · enter choose · esc cancel"
}
