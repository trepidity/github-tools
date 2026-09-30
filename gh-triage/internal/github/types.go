// Package github is the only code in gh-triage that talks to GitHub.
package github

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Repo is an owner/name pair. Build it with ParseRepo so it is always well-formed.
type Repo struct{ owner, name string }

// ParseRepo accepts exactly "owner/name".
func ParseRepo(s string) (Repo, error) {
	owner, name, ok := strings.Cut(s, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") || strings.ContainsAny(s, " \t\r\n") {
		return Repo{}, fmt.Errorf("invalid repo %q: want owner/name", s)
	}
	return Repo{owner: owner, name: name}, nil
}

func (r Repo) String() string { return r.owner + "/" + r.name }

type Issue struct {
	Repo      Repo
	Number    int
	Title     string
	Body      string
	Author    string
	URL       string
	State     string // "open" or "closed"
	Labels    []string
	Comments  int
	CreatedAt time.Time
}

// Key identifies an issue across repos, e.g. "cli/cli#123".
func (i Issue) Key() string { return fmt.Sprintf("%s#%d", i.Repo, i.Number) }

type Comment struct {
	Author    string
	Body      string
	CreatedAt time.Time
}

type CloseReason int

const (
	Completed CloseReason = iota
	NotPlanned
)

// String is the state_reason value GitHub's API expects.
func (r CloseReason) String() string {
	switch r {
	case Completed:
		return "completed"
	case NotPlanned:
		return "not_planned"
	}
	panic(fmt.Sprintf("unknown CloseReason %d", int(r)))
}

// Client is everything the UI needs from GitHub.
type Client interface {
	// SearchIssues returns the newest matching issues created at or before `before`
	// (zero = no bound). hasMore reports that older matches may remain.
	SearchIssues(ctx context.Context, query string, before time.Time) (issues []Issue, hasMore bool, err error)
	GetComments(ctx context.Context, repo Repo, number int) ([]Comment, error)
	AddComment(ctx context.Context, repo Repo, number int, body string) (Comment, error)
	CloseIssue(ctx context.Context, repo Repo, number int, reason CloseReason) error
	ListRepos(ctx context.Context) ([]Repo, error)
}
