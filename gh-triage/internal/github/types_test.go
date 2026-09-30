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
