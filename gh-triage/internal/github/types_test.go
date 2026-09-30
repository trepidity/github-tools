package github

import "testing"

func TestParseRepo_rejects_anything_but_exactly_owner_slash_name(t *testing.T) {
	valid := map[string]string{
		"cli/cli":        "cli/cli",
		"my-org/my.repo": "my-org/my.repo",
	}
	for in, want := range valid {
		r, err := ParseRepo(in)
		if err != nil {
			t.Errorf("ParseRepo(%q) error: %v", in, err)
			continue
		}
		if r.String() != want {
			t.Errorf("ParseRepo(%q) = %q, want %q", in, r, want)
		}
	}
	for _, in := range []string{"", "cli", "/cli", "cli/", "a/b/c", "a /b", "a/b ", "a\t/b"} {
		if _, err := ParseRepo(in); err == nil {
			t.Errorf("ParseRepo(%q) accepted, want error", in)
		}
	}
}

// Protects (spec test 9): the duplicate prompt accepts only an issue in this repo or an
// explicit owner/repo#n, so a typo cannot close an issue as a duplicate of nothing.
func TestParseIssueRef_accepts_numbers_and_full_refs_only(t *testing.T) {
	here, _ := ParseRepo("o/r")
	valid := map[string]string{"123": "o/r#123", "#123": "o/r#123", " #7 ": "o/r#7", "cli/cli#42": "cli/cli#42"}
	for in, want := range valid {
		repo, n, err := ParseIssueRef(in, here)
		if err != nil {
			t.Errorf("ParseIssueRef(%q) error: %v", in, err)
			continue
		}
		if got := (Issue{Repo: repo, Number: n}).Key(); got != want {
			t.Errorf("ParseIssueRef(%q) = %s, want %s", in, got, want)
		}
	}
	for _, in := range []string{"", "o/r", "abc", "#0", "#-1", "+5", "o/r#", "o#1", "#12a", "1 2"} {
		if _, _, err := ParseIssueRef(in, here); err == nil {
			t.Errorf("ParseIssueRef(%q) accepted, want error", in)
		}
	}
}
