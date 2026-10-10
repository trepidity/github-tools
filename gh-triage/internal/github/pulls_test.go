package github

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

const pullJSON = `{"state":"open","title":"Improve app","body":"Description","head":{"sha":"reviewed","label":"o:feature"},"base":{"sha":"base","label":"o:main"},"changed_files":2,"additions":3,"deletions":1,"mergeable":null}`

func TestPullRequest_loads_paginated_files_reviews_and_code_comments(t *testing.T) {
	g, _ := newTestREST(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/o/r/pulls/7" {
			fmt.Fprint(w, pullJSON)
			return
		}
		if r.URL.Query().Get("page") != "2" {
			if r.URL.Query().Get("per_page") != "100" {
				t.Error("missing page size")
			}
			w.Header().Set("Link", "<https://api.github.com"+r.URL.Path+"?page=2>; rel=\"next\"")
		}
		switch r.URL.Path {
		case "/repos/o/r/pulls/7/files":
			fmt.Fprint(w, `[{"filename":"new.go","previous_filename":"old.go","status":"renamed","additions":3,"deletions":1,"patch":"@@ -1 +1 @@\n-old\n+new"}]`)
		case "/repos/o/r/pulls/7/reviews":
			fmt.Fprint(w, `[{"user":{"login":"reviewer"},"body":"Looks good","state":"APPROVED","commit_id":"reviewed"}]`)
		case "/repos/o/r/pulls/7/comments":
			fmt.Fprint(w, `[{"user":{"login":"reviewer"},"body":"Explain this","path":"new.go","line":2,"diff_hunk":"@@ -1 +1 @@"}]`)
		default:
			t.Errorf("unexpected request %s", r.URL)
		}
	})
	p, err := g.GetPullRequest(context.Background(), mustRepo(t, "o/r"), 7)
	if err != nil {
		t.Fatal(err)
	}
	if p.HeadSHA != "reviewed" || p.BaseSHA != "base" || p.Title != "Improve app" || p.Mergeable != nil || len(p.Files) != 2 || len(p.Reviews) != 2 || len(p.Comments) != 2 {
		t.Fatalf("pull=%+v", p)
	}
	if p.Files[0].PreviousFilename != "old.go" || p.Reviews[0].Author() != "reviewer" || p.Comments[0].Line != 2 {
		t.Fatalf("details=%+v", p)
	}
}

func TestPullRequest_rejects_diff_if_head_or_base_moves_while_loading(t *testing.T) {
	for _, changed := range []string{"reviewed", "base"} {
		t.Run(changed, func(t *testing.T) {
			reads := 0
			g, _ := newTestREST(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/repos/o/r/pulls/7" {
					reads++
					body := pullJSON
					if reads == 2 {
						body = strings.Replace(body, `"sha":"`+changed+`"`, `"sha":"new"`, 1)
					}
					fmt.Fprint(w, body)
				} else {
					fmt.Fprint(w, `[]`)
				}
			})
			if _, err := g.GetPullRequest(context.Background(), mustRepo(t, "o/r"), 7); err == nil || !strings.Contains(err.Error(), "changed while loading") {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestPullReview_posts_explicit_event_and_reviewed_commit(t *testing.T) {
	for _, event := range []ReviewEvent{ReviewComment, ReviewApprove, ReviewChanges} {
		t.Run(string(event), func(t *testing.T) {
			g, reqs := newTestREST(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					fmt.Fprint(w, pullJSON)
					return
				}
				fmt.Fprint(w, `{"user":{"login":"me"},"state":"APPROVED","commit_id":"reviewed"}`)
			})
			review, err := g.ReviewPullRequest(context.Background(), mustRepo(t, "o/r"), 7, "reviewed", event, "Feedback")
			if err != nil {
				t.Fatal(err)
			}
			got := (*reqs)[1]
			if got.method != http.MethodPost || got.path != "/repos/o/r/pulls/7/reviews" || got.body["event"] != string(event) || got.body["commit_id"] != "reviewed" || got.body["body"] != "Feedback" || review.Author() != "me" {
				t.Fatalf("request=%+v review=%+v", got, review)
			}
		})
	}
}

func TestPullWrites_reject_changed_or_closed_head_before_sending(t *testing.T) {
	for _, response := range []string{strings.Replace(pullJSON, "reviewed", "new", 1), strings.Replace(pullJSON, `"open"`, `"closed"`, 1)} {
		g, reqs := newTestREST(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, response) })
		repo := mustRepo(t, "o/r")
		if _, err := g.ReviewPullRequest(context.Background(), repo, 7, "reviewed", ReviewApprove, ""); err == nil {
			t.Fatal("review accepted changed/closed PR")
		}
		if err := g.MergePullRequest(context.Background(), repo, 7, "reviewed", MergeSquash); err == nil {
			t.Fatal("merge accepted changed/closed PR")
		}
		for _, r := range *reqs {
			if r.method != http.MethodGet {
				t.Fatalf("unexpected write: %+v", r)
			}
		}
	}
}

func TestPullWrites_validate_before_requests(t *testing.T) {
	g, reqs := newTestREST(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected request") })
	repo, ctx := mustRepo(t, "o/r"), context.Background()
	for _, event := range []ReviewEvent{ReviewComment, ReviewChanges, "invalid"} {
		if _, err := g.ReviewPullRequest(ctx, repo, 7, "reviewed", event, " "); err == nil {
			t.Errorf("accepted event=%s", event)
		}
	}
	if _, err := g.ReviewPullRequest(ctx, repo, 7, "", ReviewApprove, ""); err == nil {
		t.Error("accepted blank review head")
	}
	if err := g.MergePullRequest(ctx, repo, 7, "", MergeCommit); err == nil {
		t.Error("accepted blank merge head")
	}
	if err := g.MergePullRequest(ctx, repo, 7, "reviewed", "invalid"); err == nil {
		t.Error("accepted invalid merge method")
	}
	if len(*reqs) != 0 {
		t.Fatal(*reqs)
	}
}

func TestPullMerge_sends_SHA_and_method_and_requires_confirmed_success(t *testing.T) {
	for _, method := range []MergeMethod{MergeCommit, MergeSquash, MergeRebase} {
		for _, merged := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/%t", method, merged), func(t *testing.T) {
				g, reqs := newTestREST(t, func(w http.ResponseWriter, r *http.Request) {
					if r.Method == http.MethodGet {
						fmt.Fprint(w, pullJSON)
						return
					}
					fmt.Fprintf(w, `{"merged":%t,"message":"result"}`, merged)
				})
				err := g.MergePullRequest(context.Background(), mustRepo(t, "o/r"), 7, "reviewed", method)
				if (err == nil) != merged {
					t.Fatalf("merged=%t err=%v", merged, err)
				}
				got := (*reqs)[1]
				if got.method != http.MethodPut || got.path != "/repos/o/r/pulls/7/merge" || got.body["sha"] != "reviewed" || got.body["merge_method"] != string(method) {
					t.Fatalf("request=%+v", got)
				}
			})
		}
	}
}

func TestPullMerge_preserves_GitHub_conflict_and_permission_errors(t *testing.T) {
	for _, code := range []int{403, 405, 409, 422} {
		g, _ := newTestREST(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == http.MethodGet {
				fmt.Fprint(w, pullJSON)
				return
			}
			w.WriteHeader(code)
			fmt.Fprint(w, `{"message":"Cannot merge this head"}`)
		})
		if err := g.MergePullRequest(context.Background(), mustRepo(t, "o/r"), 7, "reviewed", MergeCommit); err == nil || !strings.Contains(err.Error(), "Cannot merge this head") {
			t.Fatalf("code=%d err=%v", code, err)
		}
	}
}

func TestSearchIssues_identifies_pull_requests_for_safe_UI_dispatch(t *testing.T) {
	g, _ := newTestREST(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"items":[{"number":7,"repository_url":"https://api.github.com/repos/o/r","pull_request":{"url":"https://api.github.com/repos/o/r/pulls/7"}},{"number":8,"repository_url":"https://api.github.com/repos/o/r"}]}`)
	})
	items, _, err := g.SearchIssues(context.Background(), "repo:o/r", time.Time{})
	if err != nil || len(items) != 2 || !items[0].PullRequest || items[1].PullRequest {
		t.Fatalf("items=%v err=%v", items, err)
	}
}
