package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// Protects (spec test 10): saved queries and templates appear in the order the user wrote
// them — Go maps would shuffle them on every run.
func TestLoad_keeps_queries_and_templates_in_file_order(t *testing.T) {
	c, err := Load(writeConfig(t, "queries:\n  zeta: \"is:open\"\n  alpha: \"assignee:@me\"\n  mid: \"label:bug\"\ntemplates:\n  z: \"Thanks!\"\n  a: \"Can you share steps?\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	wantQ := []Named{{"zeta", "is:open"}, {"alpha", "assignee:@me"}, {"mid", "label:bug"}}
	wantT := []Named{{"z", "Thanks!"}, {"a", "Can you share steps?"}}
	if !reflect.DeepEqual(c.Queries, wantQ) || !reflect.DeepEqual(c.Templates, wantT) {
		t.Fatalf("queries = %v, templates = %v", c.Queries, c.Templates)
	}
}

// Protects (spec test 10): an entry with an empty name or value, or a list where a map
// belongs, is a startup error rather than a blank row.
func TestLoad_rejects_empty_or_malformed_entries(t *testing.T) {
	for _, body := range []string{
		"queries:\n  mine: \"\"\n",
		"templates:\n  \"\": hi\n",
		"queries:\n  - just a list\n",
		"templates:\n  nested:\n    x: y\n",
	} {
		if _, err := Load(writeConfig(t, body)); err == nil {
			t.Errorf("Load accepted:\n%s", body)
		}
	}
}
