// Package github is the only code in gh-triage that talks to GitHub.
package github

import (
	"context"
	"fmt"
	"strconv"
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
	PullRequest bool
	Repo        Repo
	Number      int
	Title       string
	Body        string
	Author      string
	URL         string
	NodeID      string // GraphQL id, needed to transfer
	State       string // "open" or "closed"
	Labels      []string
	LabelColors map[string]string // label name → GitHub hex color, e.g. "d73a4a"
	Assignees   []string
	Locked      bool
	Comments    int
	CreatedAt   time.Time
	UpdatedAt   time.Time
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
	Duplicate
)

// String is the state_reason value GitHub's API expects.
func (r CloseReason) String() string {
	switch r {
	case Completed:
		return "completed"
	case NotPlanned:
		return "not_planned"
	case Duplicate:
		return "duplicate"
	}
	panic(fmt.Sprintf("unknown CloseReason %d", int(r)))
}

// Client is everything the UI needs from GitHub.
type Client interface {
	GetPullRequest(ctx context.Context, repo Repo, number int) (PullRequest, error)
	ReviewPullRequest(ctx context.Context, repo Repo, number int, head string, event ReviewEvent, body string) (Review, error)
	MergePullRequest(ctx context.Context, repo Repo, number int, head string, method MergeMethod) error
	CreateIssue(ctx context.Context, repo Repo, title, body string) (Issue, error)
	// CreateRepo uses the signed-in account when owner is empty, otherwise that
	// account or an organization. It returns GitHub's canonical repository name.
	CreateRepo(ctx context.Context, owner, name, description string, private bool) (Repo, error)
	// SearchIssues returns the newest matching issues created at or before `before`
	// (zero = no bound). hasMore reports that older matches may remain.
	SearchIssues(ctx context.Context, query string, before time.Time) (issues []Issue, hasMore bool, err error)
	// SearchUpdatedIssues returns matching issues updated at or after since, oldest update
	// first, so the caller pages forward by passing the last one's UpdatedAt. hasMore
	// reports a full page.
	SearchUpdatedIssues(ctx context.Context, query string, since time.Time) (issues []Issue, hasMore bool, err error)
	GetComments(ctx context.Context, repo Repo, number int) ([]Comment, error)
	AddComment(ctx context.Context, repo Repo, number int, body string) (Comment, error)
	CloseIssue(ctx context.Context, repo Repo, number int, reason CloseReason) error
	ListRepos(ctx context.Context) ([]Repo, error)
	ListOrganizations(ctx context.Context) ([]string, error)
	CurrentUser(ctx context.Context) (string, error)
	ListLabels(ctx context.Context, repo Repo) ([]string, error)
	// SetLabels replaces the issue's labels and returns the set GitHub now has.
	SetLabels(ctx context.Context, repo Repo, number int, labels []string) ([]string, error)
	ListAssignees(ctx context.Context, repo Repo) ([]string, error)
	// SetAssignees replaces the issue's assignees and returns who GitHub actually assigned:
	// it silently drops users who cannot be assigned.
	SetAssignees(ctx context.Context, repo Repo, number int, assignees []string) ([]string, error)
	AddReaction(ctx context.Context, repo Repo, number int, r Reaction) error
	ReopenIssue(ctx context.Context, repo Repo, number int) error
	Lock(ctx context.Context, repo Repo, number int, reason LockReason) error
	Unlock(ctx context.Context, repo Repo, number int) error
	// TransferIssue moves the issue to another repo and returns its new URL.
	TransferIssue(ctx context.Context, is Issue, to Repo) (string, error)
}

type Reaction int

const (
	ThumbsUp Reaction = iota
	ThumbsDown
	Laugh
	Confused
	Heart
	Hooray
	Rocket
	Eyes
)

// AllReactions lists every reaction in GitHub's order.
func AllReactions() []Reaction {
	return []Reaction{ThumbsUp, ThumbsDown, Laugh, Confused, Heart, Hooray, Rocket, Eyes}
}

// String is the reaction content value GitHub's API expects.
func (r Reaction) String() string {
	switch r {
	case ThumbsUp:
		return "+1"
	case ThumbsDown:
		return "-1"
	case Laugh:
		return "laugh"
	case Confused:
		return "confused"
	case Heart:
		return "heart"
	case Hooray:
		return "hooray"
	case Rocket:
		return "rocket"
	case Eyes:
		return "eyes"
	}
	panic(fmt.Sprintf("unknown Reaction %d", int(r)))
}

func (r Reaction) Emoji() string {
	switch r {
	case ThumbsUp:
		return "👍"
	case ThumbsDown:
		return "👎"
	case Laugh:
		return "😄"
	case Confused:
		return "😕"
	case Heart:
		return "❤️"
	case Hooray:
		return "🎉"
	case Rocket:
		return "🚀"
	case Eyes:
		return "👀"
	}
	panic(fmt.Sprintf("unknown Reaction %d", int(r)))
}

type LockReason int

const (
	OffTopic LockReason = iota
	TooHeated
	Resolved
	Spam
)

func AllLockReasons() []LockReason { return []LockReason{OffTopic, TooHeated, Resolved, Spam} }

// String is the lock_reason value GitHub's API expects.
func (r LockReason) String() string {
	switch r {
	case OffTopic:
		return "off-topic"
	case TooHeated:
		return "too heated"
	case Resolved:
		return "resolved"
	case Spam:
		return "spam"
	}
	panic(fmt.Sprintf("unknown LockReason %d", int(r)))
}

// ParseIssueRef reads "123", "#123" or "owner/repo#123". A bare number is in current.
func ParseIssueRef(s string, current Repo) (Repo, int, error) {
	bad := fmt.Errorf("invalid issue %q: want 123, #123 or owner/repo#123", s)
	ref := strings.TrimSpace(s)
	repo := current
	if r, num, ok := strings.Cut(ref, "#"); ok && r != "" {
		parsed, err := ParseRepo(r)
		if err != nil {
			return Repo{}, 0, bad
		}
		repo, ref = parsed, num
	} else {
		ref = strings.TrimPrefix(ref, "#")
	}
	if ref == "" || strings.Trim(ref, "0123456789") != "" {
		return Repo{}, 0, bad
	}
	n, err := strconv.Atoi(ref)
	if err != nil || n < 1 {
		return Repo{}, 0, bad
	}
	return repo, n, nil
}
