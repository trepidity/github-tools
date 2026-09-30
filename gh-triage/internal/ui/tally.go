package ui

import (
	"fmt"
	"strings"
)

// action is a confirmed change counted for the quit summary.
type action int

const (
	actClosed action = iota
	actCommented
	actLabeled
	actAssigned
	actReacted
	actLocked
	actUnlocked
	actTransferred
	numActions
)

// actionNames are the summary words, in summary order.
var actionNames = [numActions]string{"closed", "commented", "labeled", "assigned", "reacted", "locked", "unlocked", "transferred"}

func (m *Model) count(a action) { m.tally[a]++ }

// Summary is one line of confirmed actions, e.g. "closed 12 · commented 5"; empty if none.
func (m Model) Summary() string {
	var parts []string
	for a, n := range m.tally {
		if n > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", actionNames[a], n))
		}
	}
	return strings.Join(parts, " · ")
}
