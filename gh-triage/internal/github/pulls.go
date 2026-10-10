package github

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type ReviewEvent string

const (
	ReviewComment ReviewEvent = "COMMENT"
	ReviewApprove ReviewEvent = "APPROVE"
	ReviewChanges ReviewEvent = "REQUEST_CHANGES"
)

type MergeMethod string

const (
	MergeCommit MergeMethod = "merge"
	MergeSquash MergeMethod = "squash"
	MergeRebase MergeMethod = "rebase"
)

type PullRequest struct {
	State, Title, Body, HeadSHA, HeadLabel, BaseLabel, BaseSHA string
	Draft, Merged                                              bool
	Mergeable                                                  *bool
	ChangedFiles, Additions, Deletions                         int
	Files                                                      []PullFile
	Reviews                                                    []Review
	Comments                                                   []ReviewCommentDetail
}

type PullFile struct {
	Filename         string `json:"filename"`
	PreviousFilename string `json:"previous_filename"`
	Status           string `json:"status"`
	Additions        int    `json:"additions"`
	Deletions        int    `json:"deletions"`
	Patch            string `json:"patch"`
}

type Review struct {
	User        apiUser   `json:"user"`
	Body        string    `json:"body"`
	State       string    `json:"state"`
	CommitID    string    `json:"commit_id"`
	SubmittedAt time.Time `json:"submitted_at"`
}

func (r Review) Author() string { return r.User.Login }

type ReviewCommentDetail struct {
	User         apiUser `json:"user"`
	Body         string  `json:"body"`
	Path         string  `json:"path"`
	Line         int     `json:"line"`
	OriginalLine int     `json:"original_line"`
	DiffHunk     string  `json:"diff_hunk"`
}

func (c ReviewCommentDetail) Author() string { return c.User.Login }

type apiPull struct {
	State        string                      `json:"state"`
	Title        string                      `json:"title"`
	Body         string                      `json:"body"`
	Draft        bool                        `json:"draft"`
	Merged       bool                        `json:"merged"`
	Mergeable    *bool                       `json:"mergeable"`
	ChangedFiles int                         `json:"changed_files"`
	Additions    int                         `json:"additions"`
	Deletions    int                         `json:"deletions"`
	Head         struct{ SHA, Label string } `json:"head"`
	Base         struct{ SHA, Label string } `json:"base"`
}

func pullPath(repo Repo, number int) string {
	return fmt.Sprintf("repos/%s/pulls/%d", repo, number)
}

func (g *REST) getPull(ctx context.Context, repo Repo, number int) (apiPull, error) {
	var pr apiPull
	err := g.c.DoWithContext(ctx, http.MethodGet, pullPath(repo, number), nil, &pr)
	return pr, err
}

// GetPullRequest rejects a moving diff rather than associating it with the wrong commit.
func (g *REST) GetPullRequest(ctx context.Context, repo Repo, number int) (PullRequest, error) {
	p, err := g.getPull(ctx, repo, number)
	if err != nil {
		return PullRequest{}, err
	}
	path := pullPath(repo, number)
	files, err := getAll[PullFile](ctx, g.c, path+"/files?per_page=100")
	if err != nil {
		return PullRequest{}, err
	}
	reviews, err := getAll[Review](ctx, g.c, path+"/reviews?per_page=100")
	if err != nil {
		return PullRequest{}, err
	}
	comments, err := getAll[ReviewCommentDetail](ctx, g.c, path+"/comments?per_page=100")
	if err != nil {
		return PullRequest{}, err
	}
	latest, err := g.getPull(ctx, repo, number)
	if err != nil {
		return PullRequest{}, err
	}
	if p.Head.SHA != latest.Head.SHA || p.Base.SHA != latest.Base.SHA {
		return PullRequest{}, fmt.Errorf("pull request changed while loading; press R to reload")
	}
	return PullRequest{
		State: latest.State, Title: latest.Title, Body: latest.Body,
		HeadSHA: p.Head.SHA, HeadLabel: p.Head.Label, BaseLabel: p.Base.Label, BaseSHA: p.Base.SHA,
		Draft: latest.Draft, Merged: latest.Merged, Mergeable: latest.Mergeable,
		ChangedFiles: p.ChangedFiles, Additions: p.Additions, Deletions: p.Deletions,
		Files: files, Reviews: reviews, Comments: comments,
	}, nil
}

func (g *REST) checkPullHead(ctx context.Context, repo Repo, number int, head string) error {
	if head == "" {
		return fmt.Errorf("load the pull request before reviewing or merging")
	}
	p, err := g.getPull(ctx, repo, number)
	if err != nil {
		return err
	}
	if p.Merged || p.State != "open" {
		return fmt.Errorf("pull request is already closed or merged")
	}
	if p.Head.SHA != head {
		return fmt.Errorf("pull request has new commits; press R to reload and review them")
	}
	return nil
}

func (g *REST) ReviewPullRequest(ctx context.Context, repo Repo, number int, head string, event ReviewEvent, body string) (Review, error) {
	switch event {
	case ReviewApprove:
	case ReviewComment, ReviewChanges:
		if strings.TrimSpace(body) == "" {
			return Review{}, fmt.Errorf("review body is required")
		}
	default:
		return Review{}, fmt.Errorf("invalid review event %q", event)
	}
	if err := g.checkPullHead(ctx, repo, number, head); err != nil {
		return Review{}, err
	}
	var review Review
	err := g.send(ctx, http.MethodPost, pullPath(repo, number)+"/reviews", map[string]string{
		"commit_id": head, "event": string(event), "body": body,
	}, &review)
	return review, err
}

func (g *REST) MergePullRequest(ctx context.Context, repo Repo, number int, head string, method MergeMethod) error {
	switch method {
	case MergeCommit, MergeSquash, MergeRebase:
	default:
		return fmt.Errorf("invalid merge method %q", method)
	}
	if err := g.checkPullHead(ctx, repo, number, head); err != nil {
		return err
	}
	var result struct {
		Merged  bool
		Message string
	}
	if err := g.send(ctx, http.MethodPut, pullPath(repo, number)+"/merge", map[string]string{
		"sha": head, "merge_method": string(method),
	}, &result); err != nil {
		return err
	}
	if !result.Merged {
		return fmt.Errorf("pull request was not merged: %s", result.Message)
	}
	return nil
}
