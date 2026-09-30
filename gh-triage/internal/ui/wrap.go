package ui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// listMarker is a bullet or number that continuation lines hang under.
var listMarker = regexp.MustCompile(`^(• |\d+\. )`)

// wrapBody wraps rendered markdown to width, breaking only at spaces. glamour's own
// wrapping works per styled segment and can cut a word that lands near the edge, so the
// body is rendered unwrapped and wrapped here. Continuation lines keep their line's
// indent, list items hang under their text, and leading blank lines are dropped.
func wrapBody(rendered string, width int) string {
	var out []string
	for _, line := range strings.Split(rendered, "\n") {
		out = append(out, wrapLine(line, width)...)
	}
	for len(out) > 0 && strings.TrimSpace(ansi.Strip(out[0])) == "" {
		out = out[1:]
	}
	return strings.Join(out, "\n")
}

func wrapLine(line string, width int) []string {
	if ansi.StringWidth(line) <= width {
		return []string{line}
	}
	plain := ansi.Strip(line)
	text := strings.TrimLeft(plain, " ")
	indent := len(plain) - len(text)
	hang := indent + ansi.StringWidth(listMarker.FindString(text))
	room := max(width-hang, 1)
	pad := strings.Repeat(" ", hang)

	var lines []string
	cur, curW := strings.Repeat(" ", indent), indent
	started := false // cur holds a visible word
	// Escape sequences contain no spaces, so splitting on " " keeps them intact; joining
	// the pieces back with " " reproduces the line exactly.
	for _, w := range strings.Split(ansi.TruncateLeft(line, indent, ""), " ") {
		ww := ansi.StringWidth(w)
		switch {
		case ww > room: // wider than any line: hard-break it
			if started {
				lines = append(lines, cur)
			}
			parts := strings.Split(ansi.Hardwrap(w, room, false), "\n")
			for _, p := range parts[:len(parts)-1] {
				lines = append(lines, pad+p)
			}
			cur, curW, started = pad+parts[len(parts)-1], hang+ansi.StringWidth(parts[len(parts)-1]), true
		case started && ww > 0 && curW+1+ww > width:
			lines = append(lines, cur)
			cur, curW = pad+w, hang+ww
		case !started:
			cur += w
			curW += ww
			started = ww > 0
		default:
			cur += " " + w
			curW += 1 + ww
		}
	}
	return append(lines, cur)
}
