package github

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// L1: the public GitHub wire contract; not a serializer round trip.
type creationClient interface {
	CreateIssue(context.Context, Repo, string, string) (Issue, error)
	CreateRepo(context.Context, string, string, string, bool) (Repo, error)
}

func creator(t *testing.T, g *REST) creationClient {
	t.Helper()
	c, ok := any(g).(creationClient)
	if !ok {
		t.Fatal("REST client does not support creation")
	}
	return c
}

func TestCreateIssue_posts_title_body_and_maps_the_confirmed_issue(t *testing.T) {
	g, reqs := newTestREST(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		// Single-issue responses need not contain search's repository_url.
		fmt.Fprint(w, `{"number":42,"title":"New issue","body":"Details\nSecond line","html_url":"https://github.com/o/r/issues/42","state":"open","user":{"login":"me"}}`)
	})
	is, err := creator(t, g).CreateIssue(context.Background(), mustRepo(t, "o/r"), "New issue", "Details\nSecond line")
	if err != nil {
		t.Fatal(err)
	}
	if len(*reqs) != 1 {
		t.Fatalf("requests: %+v", *reqs)
	}
	req := (*reqs)[0]
	if req.method != "POST" || req.path != "/repos/o/r/issues" || req.body["title"] != "New issue" || req.body["body"] != "Details\nSecond line" {
		t.Fatalf("request: %+v", req)
	}
	if is.Key() != "o/r#42" || is.Title != "New issue" || is.URL != "https://github.com/o/r/issues/42" || is.State != "open" {
		t.Fatalf("issue: %+v", is)
	}
}

func TestCreateRepo_routes_personal_and_org_owners_and_sends_visibility(t *testing.T) {
	for _, tc := range []struct {
		owner, path string
		private     bool
	}{
		{"", "/user/repos", true}, {"ME", "/user/repos", false}, {"team", "/orgs/team/repos", true},
	} {
		t.Run(tc.owner+tc.path, func(t *testing.T) {
			g, reqs := newTestREST(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/user" {
					fmt.Fprint(w, `{"login":"me"}`)
					return
				}
				w.WriteHeader(http.StatusCreated)
				fmt.Fprint(w, `{"full_name":"canonical/new-project"}`)
			})
			repo, err := creator(t, g).CreateRepo(context.Background(), tc.owner, "new-project", "Description", tc.private)
			if err != nil {
				t.Fatal(err)
			}
			req := (*reqs)[len(*reqs)-1]
			if req.method != "POST" || req.path != tc.path || req.body["name"] != "new-project" || req.body["description"] != "Description" || req.body["private"] != tc.private || req.body["has_issues"] != true {
				t.Fatalf("request: %+v", req)
			}
			if repo.String() != "canonical/new-project" {
				t.Fatalf("repo: %v", repo)
			}
		})
	}
}

func TestCreation_rejects_invalid_inputs_without_requests_and_propagates_refusals(t *testing.T) {
	g, reqs := newTestREST(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"permission denied"}`)
	})
	c := creator(t, g)
	ctx := context.Background()
	for _, name := range []string{"", "bad/name", "..", "bad?name"} {
		if _, err := c.CreateRepo(ctx, "", name, "", true); err == nil {
			t.Fatalf("accepted %q", name)
		}
	}
	if _, err := c.CreateIssue(ctx, mustRepo(t, "o/r"), "  ", ""); err == nil {
		t.Fatal("accepted empty title")
	}
	if len(*reqs) != 0 {
		t.Fatalf("invalid input sent requests: %+v", *reqs)
	}
	if _, err := c.CreateIssue(ctx, mustRepo(t, "o/r"), "Title", ""); err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("issue refusal: %v", err)
	}
	if _, err := c.CreateRepo(ctx, "", "valid", "", true); err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("repo refusal: %v", err)
	}
}
