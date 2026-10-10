package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark"
	goldmarkast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	gast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// renderBody gives tables a width before glamour lays out their cells. Prose still
// renders unwrapped so wrapBody can break styled text only between whole words.
func (m *Model) renderBody(md string, proseWidth int) string {
	source := []byte(md)
	context := parser.NewContext()
	doc := goldmark.New(goldmark.WithExtensions(extension.GFM)).Parser().Parse(text.NewReader(source), parser.WithContext(context))
	// A table fragment still needs link definitions from elsewhere in the body.
	var references strings.Builder
	for _, ref := range context.References() {
		fmt.Fprintf(&references, "\n[%s]: <%s> %s\n", ref.Label(), ref.Destination(), strconv.Quote(string(ref.Title())))
	}
	var body strings.Builder
	var replacements []string
	var tableRenderer *glamour.TermRenderer
	end := 0
	for node := doc.FirstChild(); node != nil; node = node.NextSibling() {
		if node.Kind() != gast.KindTable {
			continue
		}
		if tableRenderer == nil {
			tableRenderer = newRenderer(m.opts.Style, max(m.width-2, 20))
			if tableRenderer == nil {
				break
			}
		}
		// Table nodes have no source lines of their own; their cells do. Include
		// complete lines, including the delimiter between header and body.
		start, stop := len(source), 0
		for row := node.FirstChild(); row != nil; row = row.NextSibling() {
			for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
				for i := 0; i < cell.Lines().Len(); i++ {
					line := cell.Lines().At(i)
					start = min(start, line.Start)
					stop = max(stop, line.Stop)
				}
			}
		}
		if start > stop {
			continue
		}
		start = strings.LastIndex(md[:start], "\n") + 1
		stop = markdownLineEnd(md, stop)
		// A header-only table's delimiter lies after its last cell.
		if node.ChildCount() == 1 {
			stop = markdownLineEnd(md, stop)
		}
		fragment, rowMarker := markTableRows(md, node, start, stop)
		table, err := tableRenderer.Render(fragment + "\n" + references.String())
		if err != nil {
			continue
		}
		table = separateTableRows(table, rowMarker)
		marker := fmt.Sprintf("GHTRIAGETABLE%dEND", len(replacements)/2)
		for strings.Contains(md, marker) {
			marker += "X"
		}
		body.WriteString(md[end:start])
		body.WriteString("\n" + marker + "\n\n")
		replacements = append(replacements, marker, strings.TrimRight(wrapBody(table, max(m.width-2, 20)), "\n"))
		end = stop
	}
	body.WriteString(md[end:])
	out := body.String()
	if m.renderer != nil {
		if rendered, err := m.renderer.Render(out); err == nil {
			out = rendered
		}
	}
	if len(replacements) == 0 {
		return wrapBody(out, proseWidth)
	}
	// Restore tables after prose wrapping; wrapping a rendered row as a
	// sentence destroys its cell padding and column alignment.
	var lines []string
	for _, line := range strings.Split(out, "\n") {
		replaced := false
		for i := 0; i < len(replacements); i += 2 {
			if strings.TrimSpace(ansi.Strip(line)) == replacements[i] {
				lines = append(lines, replacements[i+1])
				replaced = true
				break
			}
		}
		if !replaced {
			lines = append(lines, wrapLine(line, proseWidth)...)
		}
	}
	for len(lines) > 0 && strings.TrimSpace(ansi.Strip(lines[0])) == "" {
		lines = lines[1:]
	}
	return strings.Join(lines, "\n")
}

// Glamour exposes border characters but does not enable Lip Gloss's body row
// borders. Insert narrow placeholder rows so it still computes every column's
// width and wraps each cell, then replace the placeholders with its header rule.
func markTableRows(md string, node goldmarkast.Node, start, stop int) (string, string) {
	marker := "\ue000"
	for strings.Contains(md, marker) {
		marker = string([]rune(marker)[0] + 1)
	}
	var out strings.Builder
	end := start
	rowNumber := 0
	for row := node.FirstChild(); row != nil; row = row.NextSibling() {
		if rowNumber > 1 {
			rowStart := stop
			for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
				if cell.Lines().Len() > 0 {
					rowStart = min(rowStart, cell.Lines().At(0).Start)
				}
			}
			rowStart = strings.LastIndex(md[:rowStart], "\n") + 1
			out.WriteString(md[end:rowStart])
			out.WriteString("|" + strings.Repeat(marker+"|", row.ChildCount()) + "\n")
			end = rowStart
		}
		rowNumber++
	}
	out.WriteString(md[end:stop])
	return out.String(), marker
}

func separateTableRows(rendered, marker string) string {
	lines := strings.Split(rendered, "\n")
	rule := ""
	for i, line := range lines {
		plain := ansi.Strip(line)
		if rule == "" && strings.ContainsAny(plain, "─-") && strings.Trim(plain, " ─┼-+|\t") == "" {
			rule = line
		}
		if strings.Contains(plain, marker) {
			lines[i] = rule
		}
	}
	return strings.Join(lines, "\n")
}

func markdownLineEnd(md string, start int) int {
	if next := strings.IndexByte(md[start:], '\n'); next >= 0 {
		return start + next + 1
	}
	return len(md)
}
