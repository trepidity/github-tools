package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
)

// rewrite sends every request to the test server, whatever host go-gh aimed at.
type rewrite struct{ target *url.URL }

func (t rewrite) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.URL.Scheme, r.URL.Host = t.target.Scheme, t.target.Host
	return http.DefaultTransport.RoundTrip(r)
}

type recorded struct {
	method, path string
	body         map[string]any
}

func newTestREST(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) (*REST, *[]recorded) {
	t.Helper()
	var reqs []recorded
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := recorded{method: r.Method, path: r.URL.Path}
		if b, _ := io.ReadAll(r.Body); len(b) > 0 {
			_ = json.Unmarshal(b, &rec.body)
		}
		reqs = append(reqs, rec)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	target, _ := url.Parse(srv.URL)
	c, err := api.NewRESTClient(api.ClientOptions{Host: "github.com", AuthToken: "test-token", Transport: rewrite{target}})
	if err != nil {
		t.Fatal(err)
	}
	return NewREST(c), &reqs
}

func mustRepo(t *testing.T, s string) Repo {
	t.Helper()
	r, err := ParseRepo(s)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestCloseIssue_sends_closed_state_with_the_state_reason_github_expects(t *testing.T) {
	for reason, want := range map[CloseReason]string{Completed: "completed", NotPlanned: "not_planned"} {
		g, reqs := newTestREST(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{}`) })
		if err := g.CloseIssue(context.Background(), mustRepo(t, "o/r"), 7, reason); err != nil {
			t.Fatal(err)
		}
		got := (*reqs)[0]
		if got.method != http.MethodPatch || got.path != "/repos/o/r/issues/7" {
			t.Fatalf("%v: request = %s %s, want PATCH /repos/o/r/issues/7", reason, got.method, got.path)
		}
		if got.body["state"] != "closed" || got.body["state_reason"] != want {
			t.Fatalf("%v: body = %v, want state=closed state_reason=%s", reason, got.body, want)
		}
	}
}

func TestAddComment_posts_body_to_the_issue_comments_endpoint(t *testing.T) {
	g, reqs := newTestREST(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"body":"hi","user":{"login":"me"},"created_at":"2026-01-01T00:00:00Z"}`)
	})
	c, err := g.AddComment(context.Background(), mustRepo(t, "o/r"), 7, "hi")
	if err != nil {
		t.Fatal(err)
	}
	got := (*reqs)[0]
	if got.method != http.MethodPost || got.path != "/repos/o/r/issues/7/comments" || got.body["body"] != "hi" {
		t.Fatalf("request = %s %s %v", got.method, got.path, got.body)
	}
	if c.Author != "me" || c.Body != "hi" {
		t.Fatalf("comment = %+v", c)
	}
}

func TestSearchIssues_maps_repository_and_stops_at_githubs_1000_result_cap(t *testing.T) {
	g, _ := newTestREST(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"total_count":5000,"items":[{"number":3,"title":"t","repository_url":"https://api.github.com/repos/o/r","user":{"login":"a"},"html_url":"https://github.com/o/r/issues/3","created_at":"2026-01-01T00:00:00Z"}]}`)
	})
	issues, more, err := g.SearchIssues(context.Background(), "is:issue", 9)
	if err != nil {
		t.Fatal(err)
	}
	if !more {
		t.Fatal("page 9 of a 5000-result search: hasMore = false, want true")
	}
	if issues[0].Key() != "o/r#3" || issues[0].Author != "a" {
		t.Fatalf("issue = %+v", issues[0])
	}
	if _, more, _ = g.SearchIssues(context.Background(), "is:issue", 10); more {
		t.Fatal("page 10 reaches GitHub's 1000-result cap: hasMore = true, want false")
	}
}

func TestListRepos_follows_link_pagination_past_the_first_100(t *testing.T) {
	g, _ := newTestREST(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			fmt.Fprint(w, `[{"full_name":"org/b"}]`)
			return
		}
		w.Header().Set("Link", `<https://api.github.com/user/repos?page=2>; rel="next", <https://api.github.com/user/repos?page=2>; rel="last"`)
		fmt.Fprint(w, `[{"full_name":"me/a"}]`)
	})
	repos, err := g.ListRepos(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 2 || repos[0].String() != "me/a" || repos[1].String() != "org/b" {
		t.Fatalf("repos = %v, want [me/a org/b]", repos)
	}
}
