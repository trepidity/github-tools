package ui_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/trepidity/gh-triage/internal/github"
	"github.com/trepidity/gh-triage/internal/ui"
)

func (f *fakeClient) GetPullRequest(context.Context, github.Repo, int) (github.PullRequest, error) {
	return f.pull, f.pullErr
}

func (f *fakeClient) ReviewPullRequest(_ context.Context, repo github.Repo, number int, head string, event github.ReviewEvent, body string) (github.Review, error) {
	if f.reviewErr != nil {
		return github.Review{}, f.reviewErr
	}
	f.reviews = append(f.reviews, fmt.Sprintf("%s#%d %s %s %s", repo, number, head, event, body))
	state := map[github.ReviewEvent]string{github.ReviewApprove: "APPROVED", github.ReviewComment: "COMMENTED", github.ReviewChanges: "CHANGES_REQUESTED"}[event]
	return github.Review{Body: body, State: state, CommitID: head}, nil
}

func (f *fakeClient) MergePullRequest(_ context.Context, repo github.Repo, number int, head string, method github.MergeMethod) error {
	if f.mergeErr != nil {
		return f.mergeErr
	}
	f.merges = append(f.merges, fmt.Sprintf("%s#%d %s %s", repo, number, head, method))
	return nil
}

func pullClient(t *testing.T) *fakeClient {
	is := issues(t, 1, 1)
	is[0].PullRequest = true
	return &fakeClient{open: is, pull: github.PullRequest{
		State: "open", Title: "Improve the app", Body: "PR description", HeadSHA: "abc123", HeadLabel: "o:feature", BaseLabel: "o:main",
		ChangedFiles: 1, Additions: 1, Deletions: 1,
		Files:    []github.PullFile{{Filename: "main.go", Status: "modified", Patch: "@@ -1 +1 @@\n-old\n+new"}},
		Reviews:  []github.Review{{State: "COMMENTED", Body: "Existing review"}},
		Comments: []github.ReviewCommentDetail{{Path: "main.go", Line: 1, Body: "Existing code comment"}},
	}}
}

func openPull(t *testing.T, f *fakeClient) tea.Model {
	return press(t, start(t, f, ui.Options{Query: "repo:o/r is:pr is:open"}), "enter")
}

func TestPullQueue_toggle_and_explicit_queries_keep_PRs_out_of_issue_queue(t *testing.T) {
	f := pullClient(t)
	m := start(t, f, repoOpts)
	if strings.Contains(text(m), "Title 1") {
		t.Fatalf("PR in issue queue: %s", text(m))
	}
	m = press(t, m, "P")
	if !strings.Contains(header(m), "repo:o/r") || !strings.Contains(header(m), "is:pr") || !strings.Contains(text(m), "Title 1") {
		t.Fatalf("PR queue: %s", text(m))
	}
	m = press(t, m, "P")
	if !strings.Contains(header(m), "is:issue") || strings.Contains(text(m), "Title 1") {
		t.Fatalf("issue queue: %s", text(m))
	}
	for _, kind := range []string{"is:pr", "type:pr"} {
		m = start(t, f, ui.Options{Query: "repo:o/r " + kind})
		if strings.Contains(header(m), "is:issue") || !strings.Contains(text(m), "Title 1") {
			t.Fatalf("explicit PR query: %s", text(m))
		}
	}
}

func TestPullReader_shows_reviews_code_comments_and_changed_files(t *testing.T) {
	m := openPull(t, pullClient(t))
	for _, want := range []string{"PR o/r#1", "PR description", "Existing review", "Existing code comment", "abc123"} {
		if !strings.Contains(text(m), want) {
			t.Fatalf("missing %q: %s", want, text(m))
		}
	}
	m = press(t, m, "f")
	for _, want := range []string{"main.go", "-old", "+new"} {
		if !strings.Contains(text(m), want) {
			t.Fatalf("missing diff %q: %s", want, text(m))
		}
	}
	m = press(t, m, "f")
	if !strings.Contains(text(m), "PR description") {
		t.Fatalf("conversation not restored: %s", text(m))
	}
}

func TestPullReview_approval_uses_reviewed_commit_and_submits_once(t *testing.T) {
	f := pullClient(t)
	m := press(t, openPull(t, f), "v", "Approve", "enter")
	m, cmds := pressOnly(m, "ctrl+s", "ctrl+s", "esc")
	m = run(t, m, cmds...)
	if len(f.reviews) != 1 || f.reviews[0] != "o/r#1 abc123 APPROVE " || !strings.Contains(text(m), "APPROVED") {
		t.Fatalf("review=%v view=%s", f.reviews, text(m))
	}
	if m.(ui.Model).Summary() != "PRs reviewed 1" {
		t.Fatal(m.(ui.Model).Summary())
	}
}

func TestPullReview_body_validation_failure_and_cancel_keep_draft(t *testing.T) {
	for _, kind := range []string{"Comment", "Request changes"} {
		t.Run(kind, func(t *testing.T) {
			f := pullClient(t)
			f.reviewErr = errors.New("permission denied")
			m := press(t, openPull(t, f), "v", kind, "enter", "ctrl+s")
			if !strings.Contains(text(m), "body is required") {
				t.Fatalf("missing validation: %s", text(m))
			}
			m = press(t, m, "Please fix this", "ctrl+s")
			if !strings.Contains(text(m), "permission denied") || !strings.Contains(text(m), "Please fix this") || m.(ui.Model).Summary() != "" {
				t.Fatalf("failed review: %s", text(m))
			}
			m = press(t, m, "esc", "v", kind, "enter")
			if !strings.Contains(text(m), "Please fix this") {
				t.Fatalf("draft lost: %s", text(m))
			}
			f.reviewErr = nil
			m = press(t, m, "ctrl+s")
			if len(f.reviews) != 1 || !strings.Contains(f.reviews[0], "Please fix this") {
				t.Fatalf("review=%v", f.reviews)
			}
		})
	}
}

func TestPullMerge_requires_confirmation_and_records_only_confirmed_merge(t *testing.T) {
	for _, method := range []string{"merge", "squash", "rebase"} {
		t.Run(method, func(t *testing.T) {
			f := pullClient(t)
			m := press(t, openPull(t, f), "M", method, "enter")
			if len(f.merges) != 0 || !strings.Contains(text(m), "abc123 into o:main?") {
				t.Fatalf("missing confirmation: %s", text(m))
			}
			m = press(t, m, "n")
			if len(f.merges) != 0 {
				t.Fatal("cancel merged the PR")
			}
			m = press(t, m, "M", method, "enter")
			m, cmds := pressOnly(m, "y", "y", "M")
			m = run(t, m, cmds...)
			if len(f.merges) != 1 || f.merges[0] != "o/r#1 abc123 "+method || !strings.Contains(text(m), "✓ merged") {
				t.Fatalf("merge=%v view=%s", f.merges, text(m))
			}
			m = press(t, m, "M", "u")
			if len(f.merges) != 1 || len(f.reopens) != 0 || m.(ui.Model).Summary() != "PRs merged 1" {
				t.Fatalf("merge incorrectly repeated or undone: %s", text(m))
			}
			m = press(t, m, "esc", "enter") // stale API snapshot must not undo the merge
			if !strings.Contains(text(m), "✓ merged") {
				t.Fatalf("merge lost after reload: %s", text(m))
			}
		})
	}
}

func TestPullMerge_failure_is_retryable_and_does_not_mark_merged(t *testing.T) {
	f := pullClient(t)
	f.mergeErr = errors.New("required checks have not passed")
	m := press(t, openPull(t, f), "M", "enter", "y")
	if !strings.Contains(text(m), "required checks") || strings.Contains(text(m), "✓ merged") || m.(ui.Model).Summary() != "" {
		t.Fatalf("failed merge: %s", text(m))
	}
	f.mergeErr = nil
	m = press(t, m, "M", "enter", "y")
	if len(f.merges) != 1 {
		t.Fatalf("retry failed: %s", text(m))
	}
}

func TestPullActions_block_loading_failed_closed_draft_and_conflicting_PRs(t *testing.T) {
	f := pullClient(t)
	m := start(t, f, ui.Options{Query: "is:pr"})
	m, load := m.Update(key("enter"))
	m = press(t, m, "v", "M")
	if !strings.Contains(text(m), "load the pull request first") {
		t.Fatalf("loading permits writes: %s", text(m))
	}
	f.pullErr = errors.New("load denied")
	m = run(t, m, load)
	m = press(t, m, "v")
	if !strings.Contains(text(m), "load denied") {
		t.Fatalf("load error missing: %s", text(m))
	}
	f.pullErr = nil
	f.pull.Draft = true
	m = press(t, m, "R", "M")
	if !strings.Contains(text(m), "draft pull requests cannot") {
		t.Fatalf("draft merge allowed: %s", text(m))
	}
	f.pull.Draft = false
	no := false
	f.pull.Mergeable = &no
	m = press(t, m, "R", "M")
	if !strings.Contains(text(m), "merge conflicts") {
		t.Fatalf("conflict merge allowed: %s", text(m))
	}
	f.pull.State = "closed"
	m = press(t, m, "R", "v", "M")
	if !strings.Contains(text(m), "already closed") || len(f.reviews)+len(f.merges) != 0 {
		t.Fatalf("closed PR writes: %s", text(m))
	}
}

func TestPullReader_disables_issue_only_actions(t *testing.T) {
	f := pullClient(t)
	m := openPull(t, f)
	for _, k := range []string{"x", "X", "d", "t"} {
		m = press(t, m, k)
		if !strings.Contains(text(m), "use v to review") {
			t.Fatalf("issue action %s enabled: %s", k, text(m))
		}
	}
	if len(f.closes)+len(f.transfers)+len(f.comments) != 0 {
		t.Fatal("PR mutated by issue-only actions")
	}
}

func TestPullReader_identifies_missing_files_and_unavailable_patches(t *testing.T) {
	f := pullClient(t)
	f.pull.ChangedFiles = 3001
	f.pull.Files[0].Patch = ""
	m := press(t, openPull(t, f), "f")
	if !strings.Contains(text(m), "Only 1 of 3001") || !strings.Contains(text(m), "Patch unavailable") {
		t.Fatalf("incomplete diff presented as complete: %s", text(m))
	}
}

func TestPullQueue_repo_switch_keeps_PR_type_and_review_draft_is_separate_from_comment(t *testing.T) {
	f := pullClient(t)
	f.repos = []github.Repo{mustRepo(t, "o/r")}
	m := press(t, openPull(t, f), "v", "Comment", "enter", "Review draft", "esc", "c", "Discussion draft", "esc", "r", "enter")
	if !strings.Contains(header(m), "is:pr") || strings.Contains(header(m), "is:issue") {
		t.Fatalf("repo switch lost PR queue: %s", text(m))
	}
	m = press(t, m, "enter", "v", "Comment", "enter")
	if !strings.Contains(text(m), "Review draft") || strings.Contains(text(m), "Discussion draft") {
		t.Fatalf("review draft mixed with comment: %s", text(m))
	}
}
