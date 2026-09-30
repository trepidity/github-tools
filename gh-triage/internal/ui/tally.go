package ui

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
