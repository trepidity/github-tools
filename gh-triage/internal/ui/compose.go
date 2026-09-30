package ui

import (
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// editorDoneMsg carries the text an external editor saved.
type editorDoneMsg struct {
	text string
	err  error
}

func (m *Model) openQueries() tea.Cmd {
	var items []pickerItem
	for _, q := range m.opts.Queries {
		items = append(items, pickerItem{label: q.Name, detail: q.Value, value: q.Value})
	}
	p := newPicker(pickQuery, "search", "search: ", items)
	p.freeText = true
	return m.openPicker(p)
}

func (m *Model) openTemplates() tea.Cmd {
	if len(m.opts.Templates) == 0 {
		m.status = "no templates in config.yml"
		return nil
	}
	var items []pickerItem
	for _, t := range m.opts.Templates {
		items = append(items, pickerItem{label: t.Name, detail: firstLine(t.Value), value: t.Value})
	}
	return m.openPicker(newPicker(pickTemplate, "insert template", "template: ", items))
}

// openEditor hands the draft to $EDITOR (then $VISUAL, then vi) and takes back what it saved.
func (m *Model) openEditor() tea.Cmd {
	f, err := os.CreateTemp("", "gh-triage-*.md")
	if err != nil {
		m.status = "editor: " + firstLine(err.Error())
		return nil
	}
	path := f.Name()
	_, werr := f.WriteString(m.editor.Value())
	if cerr := f.Close(); werr != nil || cerr != nil {
		os.Remove(path)
		m.status = "editor: could not write the draft"
		return nil
	}
	// sh -c so values like "code --wait" work (as git does); the path is passed as $1,
	// never interpolated into the command string.
	cmd := exec.Command("sh", "-c", editorCommand()+` "$1"`, "sh", path)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		defer os.Remove(path)
		if err != nil {
			return editorDoneMsg{err: err}
		}
		b, err := os.ReadFile(path)
		return editorDoneMsg{text: string(b), err: err}
	})
}

func editorCommand() string {
	for _, v := range []string{"EDITOR", "VISUAL"} {
		if s := strings.TrimSpace(os.Getenv(v)); s != "" {
			return s
		}
	}
	return "vi"
}

func (m *Model) onEditorDone(msg editorDoneMsg) {
	if msg.err != nil {
		m.status = "editor exited with error: " + firstLine(msg.err.Error()) // draft unchanged
		return
	}
	m.editor.SetValue(strings.TrimRight(msg.text, "\n"))
}
