package ui_test

import (
	"strings"
	"testing"

	"github.com/trepidity/gh-triage/internal/config"
	"github.com/trepidity/gh-triage/internal/ui"
)

// Protects (Review Focus 4): with saved queries configured, typed text runs as a new
// search, and typing part of a saved query's name runs that saved query.
func TestSearchPicker_runs_typed_text_or_the_named_saved_query(t *testing.T) {
	opts := ui.Options{Query: "repo:o/r", Queries: []config.Named{{Name: "triage", Value: "label:triage is:open"}}}
	m := start(t, threeIssues(t), opts)

	// "tag" fuzzy-matches the name "triage" (t…a…g) but is a search the user typed.
	m = press(t, m, "s", "tag", "enter")
	if h := header(m); !strings.Contains(h, "· tag is:issue ·") {
		t.Fatalf("typed search: header = %q", h)
	}
	m = press(t, m, "s", "tri", "enter")
	if h := header(m); !strings.Contains(h, "· label:triage is:open is:issue ·") {
		t.Fatalf("saved search: header = %q", h)
	}
}
