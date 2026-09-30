# gh-triage — Design

Date: 2026-09-30
Status: Approved in conversation, pending spec review

## Intent

Work through hundreds of GitHub issues quickly from the terminal. For each issue: read it,
comment, and/or close it, then move straight to the next one — without losing your place
(the browser returns you to the top of the list after every action).

**Stated by the user**
- Focus on issues only for now (PR approve/merge deferred).
- Actions: comment, close. Navigation: next, previous, search, back to list.
- Reuse existing GitHub authentication; no new login.
- Queue sources: a single repo **and** an arbitrary GitHub search query.
- Many repos across several orgs: must be able to jump to any repo in any org the user belongs to,
  plus a user-maintained pin list.
- Start simple; don't boil the ocean.

**Assumed (confirmed during design)**
- Terminal UI, not a GUI. Delivered as a `gh` CLI extension written in Go with Bubble Tea.

**Success looks like:** close-and-advance is a single keystroke plus a reason key; back-to-list
lands on the issue just viewed; switching repos never requires leaving the tool.

## Invocation

```
gh triage                         # opens the repo switcher
gh triage owner/repo              # queue = repo:owner/repo is:issue is:open
gh triage --query "<search>"      # queue = arbitrary GitHub issue search
```

`owner/repo` and `--query` are mutually exclusive; supplying both is a usage error.
When `--query` lacks `is:issue`, it is appended so PRs never enter the queue.

## Screens and keys

### List screen
One row per issue: `repo#num · title · author · age · 💬count`, closed rows marked `✓ closed`.

| Key | Action |
|---|---|
| `j`/`k`, `↓`/`↑` | Move cursor |
| `enter` | Open issue screen |
| `/` | Filter loaded rows locally (instant, no API call) |
| `s` | New GitHub search → replaces queue |
| `r` | Repo switcher |
| `q` | Quit |

### Issue screen
Title, labels, state, then body and all comments rendered as markdown (glamour), scrollable.

| Key | Action |
|---|---|
| `n` / `p` | Next / previous issue in queue (no wrap at ends) |
| `c` | Comment: textarea, `ctrl+s` submit, `esc` cancel |
| `x` | Close: prompt `c` = completed, `n` = not planned |
| `X` | Comment then close (same textarea, then reason prompt) |
| `esc` | Back to list, cursor on this issue |
| `o` | Open in browser |
| `r` | Repo switcher |
| `j`/`k`, `space` | Scroll |

### Overlays
- **Repo switcher:** pinned repos first, then fuzzy search over all accessible repos. Selecting one
  replaces the queue with that repo's open issues.
- **Search prompt:** single-line input for a GitHub search query.

### Post-action behavior
- **Close** (and comment+close): on success, row marked `✓ closed`, auto-advance to next issue.
  If it was the last issue, stay on it.
- **Comment:** on success, stay on the issue; the new comment appears in the thread.
- Closed issues remain in the queue so positions never shift and `p` reaches them.

## Architecture

```
gh-triage/
  main.go             flag parsing → initial query; wires config, client, UI
  internal/github/    only package that talks to GitHub (go-gh REST client, gh's stored token)
  internal/config/    ~/.config/gh-triage/config.yml  →  pins: [owner/repo, ...]
  internal/ui/        Bubble Tea: app (router), list, issue, switcher, prompt
```

### internal/github

```go
type Repo struct{ owner, name string }   // only constructible via ParseRepo("owner/repo")

type Client interface {
    SearchIssues(ctx, query string, page int) (issues []Issue, hasMore bool, err error)
    GetComments(ctx, repo Repo, number int) ([]Comment, error)
    AddComment(ctx, repo Repo, number int, body string) (Comment, error)
    CloseIssue(ctx, repo Repo, number int, reason CloseReason) error
    ListRepos(ctx) ([]Repo, error)
}

type CloseReason int  // Completed | NotPlanned → "completed" | "not_planned"
```

- `SearchIssues`: `GET /search/issues?q=…&per_page=100&page=N`. Search API caps at 1000 results.
- `GetComments`: `GET /repos/{o}/{r}/issues/{n}/comments` (paginated).
- `AddComment`: `POST /repos/{o}/{r}/issues/{n}/comments` `{body}`.
- `CloseIssue`: `PATCH /repos/{o}/{r}/issues/{n}` `{state:"closed", state_reason}`.
- `ListRepos`: `GET /user/repos?affiliation=owner,collaborator,organization_member&per_page=100`, all pages.

The UI depends on the `Client` interface; production uses the go-gh implementation.

### Data flow
- **Queue** = results of one search query. First page loads at start; the next page is fetched
  when the cursor (list or issue screen) comes within 10 of the end of loaded rows, at most one
  in-flight page fetch at a time.
- **Issue open:** body comes from the search result; comments fetched on open. Comments for the
  next issue are prefetched in the background and cached by `repo#num`.
- **Repo list:** cached at `~/.cache/gh-triage/repos.json`. Switcher opens from cache immediately
  and refreshes in the background; first run shows a loading state.
- **Actions are confirmed, not optimistic:** UI state changes only after the API returns success.

### Error handling
- Bottom status bar shows the last error (e.g. `close failed: 403 Resource not accessible`).
- Failed close: issue stays open, user stays on it.
- Failed comment: textarea stays open with the draft intact.
- Rate limit (search: 30 req/min) surfaces as a plain status-bar error.
- Missing/invalid auth at startup: exit with a message suggesting `gh auth login`.

## Testing

Per `~/.claude/skills/test-selection/SKILL.md`. Each test names what it protects.

| # | Behavior | Protects | Level |
|---|---|---|---|
| 1 | `n`/`p` stop at queue ends | Navigation requirement | UI model + fake Client |
| 2 | `esc` returns to list with cursor on viewed issue | The core browser pain point | UI model + fake Client |
| 3 | Close success marks row, advances, queue length unchanged | Close-and-advance requirement | UI model + fake Client |
| 4 | Nearing end triggers exactly one next-page fetch | Pagination state | UI model + fake Client |
| 5 | Close failure: stays open, stays on issue, error shown | Refusal path | UI model + fake Client |
| 6 | Comment failure keeps draft | Refusal path | UI model + fake Client |
| 7 | Close sends `state=closed` + correct `state_reason`; comment hits correct URL | Wire contract with GitHub | `httptest` server |
| 8 | Switcher list: pins first, no duplicates | Ordering invariant | Pure function |

**Enforced by types, not tests:** `Repo` validity (`ParseRepo` is the only constructor);
`CloseReason` is a closed enum mapped with an exhaustive switch.

**Not tested:** rendering, YAML loading, keymap wiring, go-gh auth.

## Out of scope (v1)

Pull requests (approve/merge), labels/assignees/milestones, bulk actions, editing issues,
reactions, GUI/Tauri, offline mode.
