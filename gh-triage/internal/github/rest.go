package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
)

const perPage = 100

// REST implements Client with go-gh, which reuses the token `gh auth login` stored.
type REST struct{ c *api.RESTClient }

func NewREST(c *api.RESTClient) *REST { return &REST{c: c} }

type apiUser struct {
	Login string `json:"login"`
}

type apiIssue struct {
	Number        int       `json:"number"`
	Title         string    `json:"title"`
	Body          string    `json:"body"`
	HTMLURL       string    `json:"html_url"`
	State         string    `json:"state"`
	RepositoryURL string    `json:"repository_url"`
	User          apiUser   `json:"user"`
	Comments      int       `json:"comments"`
	CreatedAt     time.Time `json:"created_at"`
	NodeID        string    `json:"node_id"`
	Assignees     []apiUser `json:"assignees"`
	Locked        bool      `json:"locked"`
	Labels        []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

type apiComment struct {
	Body      string    `json:"body"`
	User      apiUser   `json:"user"`
	CreatedAt time.Time `json:"created_at"`
}

func (c apiComment) toComment() Comment {
	return Comment{Author: c.User.Login, Body: c.Body, CreatedAt: c.CreatedAt}
}

// SearchIssues pages by creation time rather than page number: page offsets shift as
// issues are closed out of an is:open search, which would silently skip issues.
func (g *REST) SearchIssues(ctx context.Context, query string, before time.Time) ([]Issue, bool, error) {
	if !before.IsZero() {
		query += " created:<=" + before.UTC().Format(time.RFC3339)
	}
	path := fmt.Sprintf("search/issues?q=%s&sort=created&order=desc&per_page=%d&page=1", url.QueryEscape(query), perPage)
	var resp struct {
		Items []apiIssue `json:"items"`
	}
	if err := g.c.DoWithContext(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, false, err
	}
	issues := make([]Issue, 0, len(resp.Items))
	for _, it := range resp.Items {
		repo, err := repoFromURL(it.RepositoryURL)
		if err != nil {
			return nil, false, err
		}
		labels := make([]string, 0, len(it.Labels))
		for _, l := range it.Labels {
			labels = append(labels, l.Name)
		}
		issues = append(issues, Issue{
			Repo: repo, Number: it.Number, Title: it.Title, Body: it.Body, Author: it.User.Login,
			URL: it.HTMLURL, State: it.State, Labels: labels, Comments: it.Comments, CreatedAt: it.CreatedAt,
			NodeID: it.NodeID, Assignees: logins(it.Assignees), Locked: it.Locked,
		})
	}
	return issues, len(resp.Items) == perPage, nil
}

// repoFromURL reads the repo out of a repository_url like https://api.github.com/repos/{owner}/{name}.
func repoFromURL(u string) (Repo, error) {
	_, rest, ok := strings.Cut(u, "/repos/")
	if !ok {
		return Repo{}, fmt.Errorf("unexpected repository_url %q", u)
	}
	return ParseRepo(rest)
}

func (g *REST) GetComments(ctx context.Context, repo Repo, number int) ([]Comment, error) {
	raw, err := getAll[apiComment](ctx, g.c, fmt.Sprintf("repos/%s/issues/%d/comments?per_page=%d", repo, number, perPage))
	if err != nil {
		return nil, err
	}
	out := make([]Comment, len(raw))
	for i, c := range raw {
		out[i] = c.toComment()
	}
	return out, nil
}

func (g *REST) AddComment(ctx context.Context, repo Repo, number int, body string) (Comment, error) {
	var c apiComment
	if err := g.send(ctx, http.MethodPost, issuePath(repo, number)+"/comments", map[string]string{"body": body}, &c); err != nil {
		return Comment{}, err
	}
	return c.toComment(), nil
}

func (g *REST) CloseIssue(ctx context.Context, repo Repo, number int, reason CloseReason) error {
	return g.send(ctx, http.MethodPatch, issuePath(repo, number), map[string]string{"state": "closed", "state_reason": reason.String()}, nil)
}

func (g *REST) CurrentUser(ctx context.Context) (string, error) {
	var u apiUser
	if err := g.c.DoWithContext(ctx, http.MethodGet, "user", nil, &u); err != nil {
		return "", err
	}
	return u.Login, nil
}

type apiLabel struct {
	Name string `json:"name"`
}

func labelNames(ls []apiLabel) []string {
	out := make([]string, len(ls))
	for i, l := range ls {
		out[i] = l.Name
	}
	return out
}

func logins(us []apiUser) []string {
	out := make([]string, len(us))
	for i, u := range us {
		out[i] = u.Login
	}
	return out
}

func (g *REST) ListLabels(ctx context.Context, repo Repo) ([]string, error) {
	raw, err := getAll[apiLabel](ctx, g.c, fmt.Sprintf("repos/%s/labels?per_page=%d", repo, perPage))
	if err != nil {
		return nil, err
	}
	return labelNames(raw), nil
}

func (g *REST) SetLabels(ctx context.Context, repo Repo, number int, labels []string) ([]string, error) {
	if labels == nil {
		labels = []string{} // GitHub needs [] to clear; null is a validation error
	}
	var got []apiLabel
	if err := g.send(ctx, http.MethodPut, issuePath(repo, number)+"/labels", map[string][]string{"labels": labels}, &got); err != nil {
		return nil, err
	}
	return labelNames(got), nil
}

func (g *REST) ListAssignees(ctx context.Context, repo Repo) ([]string, error) {
	raw, err := getAll[apiUser](ctx, g.c, fmt.Sprintf("repos/%s/assignees?per_page=%d", repo, perPage))
	if err != nil {
		return nil, err
	}
	return logins(raw), nil
}

// SetAssignees uses the issue PATCH, which replaces the whole set in one request.
func (g *REST) SetAssignees(ctx context.Context, repo Repo, number int, assignees []string) ([]string, error) {
	if assignees == nil {
		assignees = []string{}
	}
	var got apiIssue
	if err := g.send(ctx, http.MethodPatch, issuePath(repo, number), map[string][]string{"assignees": assignees}, &got); err != nil {
		return nil, err
	}
	return logins(got.Assignees), nil
}

func (g *REST) AddReaction(ctx context.Context, repo Repo, number int, r Reaction) error {
	return g.send(ctx, http.MethodPost, issuePath(repo, number)+"/reactions", map[string]string{"content": r.String()}, nil)
}

func issuePath(repo Repo, number int) string { return fmt.Sprintf("repos/%s/issues/%d", repo, number) }

// send makes one request with a JSON body; out may be nil.
func (g *REST) send(ctx context.Context, method, path string, body, out any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	return g.c.DoWithContext(ctx, method, path, r, out)
}

func (g *REST) ListRepos(ctx context.Context) ([]Repo, error) {
	type apiRepo struct {
		FullName string `json:"full_name"`
	}
	raw, err := getAll[apiRepo](ctx, g.c, fmt.Sprintf("user/repos?affiliation=owner,collaborator,organization_member&sort=full_name&per_page=%d", perPage))
	if err != nil {
		return nil, err
	}
	repos := make([]Repo, 0, len(raw))
	for _, r := range raw {
		repo, err := ParseRepo(r.FullName)
		if err != nil {
			return nil, err
		}
		repos = append(repos, repo)
	}
	return repos, nil
}

var nextLink = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

// getAll GETs path and every page after it, following the Link header.
func getAll[T any](ctx context.Context, c *api.RESTClient, path string) ([]T, error) {
	var all []T
	for path != "" {
		resp, err := c.RequestWithContext(ctx, http.MethodGet, path, nil)
		if err != nil {
			return nil, err
		}
		var page []T
		err = json.NewDecoder(resp.Body).Decode(&page)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		path = ""
		if m := nextLink.FindStringSubmatch(resp.Header.Get("Link")); m != nil {
			path = m[1] // absolute URL; go-gh uses it as-is
		}
	}
	return all, nil
}
