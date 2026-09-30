package ui_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// Protects (spec test 2): labels change only when GitHub confirms; a failed set leaves
// the issue as it was.
func TestLabels_change_only_when_github_confirms(t *testing.T) {
	f := threeIssues(t)
	f.labels = []string{"bug", "triage"}
	f.setErr = errors.New("403 forbidden")
	m := start(t, f, repoOpts)

	m = press(t, m, "enter", "l", " ", "enter")
	if meta := lines(m)[1]; strings.Contains(meta, "bug") || !strings.Contains(text(m), "label failed: 403 forbidden") {
		t.Fatalf("after failed label set:\n%s", text(m))
	}
	f.setErr = nil
	m = press(t, m, "l", " ", "enter")
	if meta := lines(m)[1]; !strings.Contains(meta, "bug") {
		t.Fatalf("meta after label set = %q", meta)
	}
}

// Protects (spec test 2, review finding 3): the issue shows who GitHub actually assigned,
// and the status names anyone it silently ignored.
func TestAssignees_show_who_github_actually_assigned(t *testing.T) {
	f := threeIssues(t)
	f.assignable = []string{"bob", "carol"}
	f.dropped = map[string]bool{"carol": true}
	m := start(t, f, repoOpts)

	m = press(t, m, "enter", "A", " ", "down", " ", "enter")
	meta := lines(m)[1]
	if !strings.Contains(meta, "bob") || strings.Contains(meta, "carol") || !strings.Contains(text(m), "ignored: carol") {
		t.Fatalf("after assigning bob+carol (carol dropped):\n%s", text(m))
	}
}

// Protects (Review Focus 3): a label the repo no longer lists stays on the issue when
// other labels are changed.
func TestLabels_keep_a_label_the_repo_no_longer_lists(t *testing.T) {
	f := threeIssues(t)
	f.open[0].Labels = []string{"legacy"}
	f.labels = []string{"bug"}
	m := start(t, f, repoOpts)

	press(t, m, "enter", "l", "bug", " ", "enter")
	if want := []string{"o/r#1 labels=bug,legacy"}; !reflect.DeepEqual(f.sets, want) {
		t.Fatalf("sets = %q, want %q", f.sets, want)
	}
}
