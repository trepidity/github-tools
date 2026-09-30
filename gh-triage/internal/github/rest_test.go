package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

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

func TestSearchIssues_continues_before_the_oldest_loaded_issue_and_stops_on_a_short_page(t *testing.T) {
	item := `{"number":3,"title":"t","state":"closed","repository_url":"https://api.github.com/repos/o/r","user":{"login":"a"},"html_url":"https://github.com/o/r/issues/3","created_at":"2026-01-01T00:00:00Z"}`
	var queries []string
	n := 100
	g, _ := newTestREST(t, func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.Query().Get("q")+" page="+r.URL.Query().Get("page"))
		items := strings.TrimSuffix(strings.Repeat(item+",", n), ",")
		fmt.Fprintf(w, `{"total_count":5000,"items":[%s]}`, items)
	})
	issues, more, err := g.SearchIssues(context.Background(), "repo:o/r is:issue", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if !more || issues[0].Key() != "o/r#3" || issues[0].Author != "a" || issues[0].State != "closed" {
		t.Fatalf("first page: more=%v issue=%+v", more, issues[0])
	}
	n = 7
	before := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if _, more, _ = g.SearchIssues(context.Background(), "repo:o/r is:issue", before); more {
		t.Fatal("a page shorter than 100 is the last one: hasMore = true, want false")
	}
	want := []string{"repo:o/r is:issue page=1", "repo:o/r is:issue created:<=2026-01-02T03:04:05Z page=1"}
	if !reflect.DeepEqual(queries, want) {
		t.Fatalf("queries = %q, want %q", queries, want)
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
