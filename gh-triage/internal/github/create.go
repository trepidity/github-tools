package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

var repoName = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
var ownerName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*$`)

func (g *REST) ListOrganizations(ctx context.Context) ([]string, error) {
	raw, err := getAll[apiUser](ctx, g.c, "user/orgs?per_page=100")
	if err != nil {
		return nil, err
	}
	orgs := logins(raw)
	slices.Sort(orgs)
	return orgs, nil
}

// ValidateNewRepo rejects missing names and path/query syntax before any request.
// GitHub owns remaining rules, availability and organization policy.
func ValidateNewRepo(owner, name string) error {
	if name == "" {
		return fmt.Errorf("repository name is required")
	}
	if !repoName.MatchString(name) || strings.Trim(name, ".") == "" {
		return fmt.Errorf("repository name must use letters, numbers, dots, hyphens or underscores")
	}
	if owner != "" && !ownerName.MatchString(owner) {
		return fmt.Errorf("owner must be a GitHub login or organization name")
	}
	return nil
}

func ValidateNewIssue(repo Repo, title string) error {
	if err := ValidateNewRepo(repo.owner, repo.name); err != nil || repo.owner == "" {
		return fmt.Errorf("invalid repo: want owner/name")
	}
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("issue title is required")
	}
	return nil
}

func (g *REST) CreateIssue(ctx context.Context, repo Repo, title, body string) (Issue, error) {
	if err := ValidateNewIssue(repo, title); err != nil {
		return Issue{}, err
	}
	var got apiIssue
	err := g.send(ctx, http.MethodPost, "repos/"+repo.String()+"/issues", map[string]string{"title": strings.TrimSpace(title), "body": body}, &got)
	if err != nil {
		return Issue{}, err
	}
	// The destination is already known; unlike search results this response may
	// omit repository_url. Do not turn a successful write into a retryable error.
	got.RepositoryURL = "https://api.github.com/repos/" + repo.String()
	return got.toIssue()
}

func (g *REST) CreateRepo(ctx context.Context, owner, name, description string, private bool) (Repo, error) {
	if err := ValidateNewRepo(owner, name); err != nil {
		return Repo{}, err
	}
	path := "user/repos"
	if owner != "" {
		me, err := g.CurrentUser(ctx)
		if err != nil {
			return Repo{}, err
		}
		if !strings.EqualFold(me, owner) {
			path = "orgs/" + url.PathEscape(owner) + "/repos"
		}
	}
	var got struct {
		FullName string `json:"full_name"`
	}
	err := g.send(ctx, http.MethodPost, path, map[string]any{"name": name, "description": description, "private": private, "has_issues": true}, &got)
	if err != nil {
		return Repo{}, err
	}
	return ParseRepo(got.FullName)
}
