# gh-triage v2 — Design

Date: 2026-09-30
Status: Implemented
Builds on: `2026-09-30-gh-triage-design.md` (v1). Everything in v1 still holds unless changed here.

## Intent

v1 lets you read, comment on and close issues one at a time without losing your place. v2 adds
the other triage actions that still send you to the browser, and makes long sessions resumable.

**Stated by the user**
- Tier 1: labels, assign, saved queries, reactions, `$EDITOR` comments, reply templates.
- Tier 2: close as duplicate, transfer, lock, resume position, session summary, undo close.
- Transfer is included even though it needs GraphQL.
- Resume loads the queue from the top and jumps to the saved issue (new issues stay reachable above).
- Undo covers closes only (close, comment+close, duplicate-close).
- Fold in the deferred v1 review issues the new work touches: M3 (no request timeout),
  M4 (stuck "Loading comments…"), M8 (esc discards the comment draft).

**Assumed (confirmed during design)**
- Every action is one or two keystrokes, updates the issue in place, and never moves the cursor
  except where v1 already advances (close) or v2 treats a move like a close (transfer).
- Actions stay confirmed, not optimistic: UI state changes only after GitHub says yes.
- Delivered in two phases (Tier 1, then Tier 2), each shippable alone.

**Success looks like:** a triage session never needs `o` for labels, assignees, reactions,
duplicates, transfer or lock; quitting and relaunching lands on the same issue; a hung request
never freezes the UI for more than 30 seconds.

**Still out of scope:** pull requests, bulk actions, milestones, editing issue title/body, undo for
anything but closes, markdown-rendering changes, the other deferred v1 minors (M2 beyond new
actions, M5, M6, M7, M9, emoji cosmetic).

## Keys

### Issue screen (additions)

| Key | Action |
|---|---|
| `l` | Label picker: the repo's labels, current ones pre-checked; `space` toggles, `enter` applies the whole set, `esc` cancels |
| `a` | Toggle assigning yourself |
| `A` | Assignee picker (multi-select), same keys as the label picker |
| `+` | Reaction picker (GitHub's 8 reactions); `enter` adds the chosen one |
| `d` | Close as duplicate: prompt `duplicate of:` accepting `123`, `#123` or `owner/repo#123` (bare numbers mean this issue's repo); posts `Duplicate of <ref>` then closes with `state_reason: duplicate` |
| `t` | Transfer: repo picker for the target, then `transfer to owner/repo? this cannot be undone (y/n)` |
| `L` | Lock: reason picker (off-topic, too heated, resolved, spam). If already locked: `unlock? (y/n)` |
| `u` | Undo the last close this session by reopening it |
| `R` | Retry loading comments after a failure |

### List screen (additions/changes)

| Key | Action |
|---|---|
| `s` | Search picker: saved queries from config first; typing filters them, and `enter` on typed text that matches no saved query runs it as a new search |
| `u` | Undo the last close (same as issue screen) |

### Comment editor (additions/changes)

| Key | Action |
|---|---|
| `ctrl+e` | Open `$EDITOR` (then `$VISUAL`, then `vi`) on the draft; its saved contents replace the draft |
| `ctrl+t` | Template picker; the chosen template is inserted at the cursor |
| `esc` | Close the editor **keeping** the draft for this issue; the next `c`/`X` on it restores the draft |

### Quit
After the alternate screen closes, one line is printed to stdout summarizing confirmed actions,
omitting zero counts, e.g. `closed 12 · commented 5 · labeled 8 · assigned 3 · transferred 1`.
Nothing is printed if there were no actions.

## Architecture

### Action runner (replaces per-action messages)

```go
// run starts an action: sets busy + status and applies the timeout. do returns apply for
// whatever GitHub confirmed (nil if nothing) and err for whatever failed; both may be set.
// apply runs on the UI goroutine and is the only place state changes.
func (m *Model) run(label string, do func(ctx context.Context) (apply func(*Model) tea.Cmd, err error)) tea.Cmd

type actionDoneMsg struct {
    label string
    apply func(*Model) tea.Cmd
    err   error
}
```

- Timeout comes from `Options.ActionTimeout` (zero means the 30s default), applied with
  `context.WithTimeout`. This fixes M3. Search and comment loading also use it.
- `busy` clears. If `apply` is non-nil it runs, and its returned command (e.g. advance) runs.
  If `err` is non-nil the status shows `<label> failed: <first line of error>` (after any status
  `apply` set, so a partial success reads e.g. `commented; duplicate failed: …`).
- Full failure = `apply` nil: state unchanged. Partial success = both set: only the confirmed part
  is applied, so a retry never repeats a step GitHub already accepted.
- v1's `commentPostedMsg` and `closedMsg` are migrated onto `run`; their behavior is unchanged.

### Picker (generalizes the repo switcher)

`switcher.go` becomes `picker.go`: a fuzzy-filtered list with a cursor, optional multi-select
(`space` toggles a checked set), and optional free-text submit. It holds `[]pickerItem{label, value}`
and reports the choice through a callback. Used by the repo switcher, label, assignee, reaction,
lock-reason, template, saved-query and transfer-target pickers. Repo ordering (`orderRepos`: pins
first, no duplicates) is unchanged.

### internal/github

`Issue` gains `NodeID string`, `Assignees []string`, `Locked bool` (all present in search results).

```go
type Client interface {
    // v1
    SearchIssues(ctx, query string, before time.Time) ([]Issue, bool, error)
    GetComments(ctx, repo Repo, number int) ([]Comment, error)
    AddComment(ctx, repo Repo, number int, body string) (Comment, error)
    CloseIssue(ctx, repo Repo, number int, reason CloseReason) error // CloseReason gains Duplicate
    ListRepos(ctx) ([]Repo, error)
    // v2
    CurrentUser(ctx) (string, error)
    ListLabels(ctx, repo Repo) ([]string, error)
    SetLabels(ctx, repo Repo, number int, labels []string) ([]string, error)       // labels GitHub now has
    ListAssignees(ctx, repo Repo) ([]string, error)
    SetAssignees(ctx, repo Repo, number int, assignees []string) ([]string, error) // assignees GitHub now has
    AddReaction(ctx, repo Repo, number int, r Reaction) error
    ReopenIssue(ctx, repo Repo, number int) error
    Lock(ctx, repo Repo, number int, reason LockReason) error
    Unlock(ctx, repo Repo, number int) error
    TransferIssue(ctx, is Issue, to Repo) (newURL string, err error)
}
```

| Method | Endpoint |
|---|---|
| `CurrentUser` | `GET /user` → `login` (cached by the UI for the session) |
| `ListLabels` | `GET /repos/{o}/{r}/labels?per_page=100`, all pages |
| `SetLabels` | `PUT /repos/{o}/{r}/issues/{n}/labels` `{labels:[…]}` (replaces the set; `[]` clears); returns the response's label names |
| `ListAssignees` | `GET /repos/{o}/{r}/assignees?per_page=100`, all pages |
| `SetAssignees` | `PATCH …/issues/{n}` `{assignees:[…]}` — one request that replaces the set; returns the response issue's `assignees` logins. GitHub silently drops users who can't be assigned, so the response, not the request, is the truth |
| `AddReaction` | `POST …/issues/{n}/reactions` `{content}` |
| `CloseIssue` | v1 endpoint; `Duplicate` → `state_reason: "duplicate"` |
| `ReopenIssue` | `PATCH …/issues/{n}` `{state:"open"}` |
| `Lock` | `PUT …/issues/{n}/lock` `{lock_reason}` |
| `Unlock` | `DELETE …/issues/{n}/lock` |
| `TransferIssue` | GraphQL: `repository(owner,name){id}` then `transferIssue(input:{issueId, repositoryId}){issue{url}}` via go-gh's GraphQL client (same stored token) |

New closed enums, each mapped to its API string with an exhaustive switch that panics on an
unknown value (same pattern as `CloseReason`):
- `Reaction`: `+1`, `-1`, `laugh`, `confused`, `heart`, `hooray`, `rocket`, `eyes`.
- `LockReason`: `off-topic`, `too heated`, `resolved`, `spam`.

`ParseIssueRef(s string, current Repo) (Repo, int, error)` parses the duplicate target:
`123`, `#123`, `owner/repo#123`. Rejects anything else, and numbers < 1.

### internal/config

```yaml
pins: [owner/repo]
queries:              # s picker, in file order
  mine: "assignee:@me is:open"
templates:            # ctrl+t, in file order
  repro: "Thanks! Could you share steps to reproduce?"
```

`Config` gains `Queries []Named` and `Templates []Named` (`Named{Name, Value string}`), decoded via
`yaml.Node` so file order is kept. An empty name or value is a load error naming the entry.

Positions: `ReadPositions(path) map[string]Position` / `WritePositions(path, map)` at
`~/.cache/gh-triage/positions.json`, keyed by the normalized query (`issueQuery` output).
`Position{Key string, CreatedAt time.Time}`. A missing or unreadable file means no positions.

### internal/ui state (additions)

| Field | Purpose |
|---|---|
| `me string` | Current user login, fetched once (for `a`) |
| `labels, assignees map[Repo][]string` | Per-repo option lists, fetched once per session |
| `drafts map[string]string` | Comment drafts by issue key (M8) |
| `commentsErr map[string]bool` | Comment load failed for this key; shows retry hint (M4) |
| `stateOverride map[string]string` | Replaces v1's `closed map[string]bool`: issue key → `open`/`closed` confirmed this session; wins over search `State` across searches (the index lags both ways) |
| `dupCommented map[string]string` | Issue key → duplicate ref already commented; `d` on it skips the prompt and comment and only closes |
| `lastClose *github.Issue` | What `u` reopens; replaced by each close, cleared by a successful undo |
| `transferred map[string]string` | Issue key → new URL; row shows `→ moved`, persists across searches |
| `tally map[string]int` | Confirmed action counts for the quit summary |
| `positions map[string]config.Position` | Loaded at startup, updated on issue open, written on quit |
| `resume *config.Position` | Target for the current query; set by every `startSearch`, cleared when reached or passed |

`ui.New` takes the loaded positions via `Options.Positions`. `main` runs the program, takes the
final `ui.Model` from `Program.Run`, writes `Model.Positions()`, and prints `Model.Summary()`.

## Data flow

- **Labels / assignees:** opening the picker fetches the option list if the repo's list isn't
  cached (picker shows `loading…`). Applying sends the full desired set in one request. On success
  the issue's `Labels`/`Assignees` are replaced with the set **GitHub returned**. If the returned
  set differs from the requested one, the status says so (e.g. `assigned: me · ignored: bob`). The tally
  counts `labeled`/`assigned` only when the returned set differs from the previous one.
- **Assign self (`a`):** adds `me` if absent, removes it if present.
- **Reaction:** fire and confirm; status `reacted 👍`. Reactions are not shown in the thread (no
  display change beyond status).
- **Duplicate:** `AddComment` then `CloseIssue(Duplicate)` inside one `run`. Success = both.
  If the comment posts and the close fails, `do` returns an `apply` that records the comment (so it
  appears) together with the close error (partial success); the status reads
  `commented; duplicate failed: …` and the issue stays open. `d` again offers only the close.
- **Close (any kind) success:** v1 behavior (`stateOverride[key] = closed`, advance) plus `lastClose` = that issue
  and `tally["closed"]++`.
- **Undo:** `ReopenIssue(lastClose)`. Success sets `stateOverride[key] = open` (whether or not the
  issue is in the current queue), clears `lastClose`, status `reopened <key>`. The cursor does not
  move. A later search that returns the stale `closed` state still shows it open and closable.
  Undo only reopens: a comment posted with the close (`X`, `d`) stays, and `tally["closed"]` is
  decremented.
- **Transfer:** success records `transferred[key] = newURL`, then advances like a close. Moved
  issues refuse further actions with status `moved to <url>`. `o` opens the new URL.
- **Lock/unlock:** success flips `Locked`; the issue header shows `🔒 locked`.
- **Templates:** inserted at the textarea cursor; no API call.
- **`$EDITOR`:** writes the draft to a temp file, runs the editor via `tea.ExecProcess`, reads the
  file back on exit 0 and replaces the textarea contents. Non-zero exit: draft unchanged, status
  `editor exited with error`. The temp file is removed either way.
- **Resume:** positions are loaded into the model at startup. Every `startSearch` (CLI argument,
  `--query`, repo picker, search picker) replaces `resume` with `positions[normalized query]`, or
  clears it if there is none, so a target from a previous query never carries over. Each loaded page: if the
  target key is present, move the cursor there and clear `resume`; else if the oldest loaded issue
  is older than `resume.CreatedAt`, put the cursor on the first issue created before it and clear
  `resume`; else, while `resume` is set and `hasMore`, fetch the next page. Opening an issue records
  its position (in memory); positions are written to disk on quit.

## Error handling

- All actions: `<label> failed: <first line>`; state unchanged except for any confirmed partial step.
- Timeout: `<label> failed: context deadline exceeded` after `ActionTimeout`; UI usable again.
- Comment load failure: body shows `comments failed to load — R to retry` instead of loading (M4).
- Duplicate partial failure: as in Data flow.
- Transfer: target lookup or mutation failure leaves the issue untouched.
- Undo failure: issue stays closed, `lastClose` kept, so `u` retries.
- Actions on a moved issue: refused with `moved to <url>`.
- `u` with nothing to undo: status `nothing to undo`.
- Bad `queries`/`templates` config: startup error naming the entry, like bad pins.
- Resume target never found: cursor stays at the top, no message.

## Testing

Per `~/.claude/skills/test-selection/SKILL.md`. Each test names what it protects.

| # | Behavior | Protects | Level |
|---|---|---|---|
| 1 | Hung client call ends as a timeout error and clears `busy` | M3 fix; refusal path | UI model + fake Client (short `ActionTimeout`) |
| 2 | Failed label/assignee set leaves the issue unchanged; a successful set applies GitHub's returned set, not the requested one | Confirmed-not-optimistic rule; silent-drop case | UI + fake |
| 3 | `u` reopens the last close; a failed reopen keeps it closed and still undoable; after undo, a new search returning stale `closed` still shows it open | Undo transition + refusal; stale-index override | UI + fake |
| 4 | Duplicate: comment OK, close fails → still open, comment shown, status says so | Partial-failure path | UI + fake |
| 5 | Transfer success marks the row moved, advances, queue length unchanged; later actions refused | Positions-never-shift invariant; refusal | UI + fake |
| 6 | Esc in editor then `c` restores the draft; a successful post clears it | M8 fix | UI + fake |
| 7 | Resume lands on the saved issue across pages; stops paging once past its `CreatedAt`; lands on the next older issue if the saved one is gone; works when the query starts from the repo picker, and switching queries drops the old target | Resume requirement; paging bound | UI + fake |
| 8 | Wire bodies/paths: SetLabels PUT, SetAssignees single PATCH with full set, reaction `content`, `state_reason: duplicate`, lock `lock_reason`, reopen, transfer GraphQL variables | Agreement with GitHub's API | `httptest` |
| 9 | `ParseIssueRef`: accepts `123`, `#123`, `o/r#123`; rejects `o/r`, `abc`, `#0`, `o/r#` | Dense parser | Pure unit |
| 10 | Config `queries`/`templates` keep file order; empty name or value rejected | Ordering invariant; refusal | Pure unit on `config.Load` |
| 11 | Tally counts only confirmed actions (a failed close is not counted) | Summary accuracy | UI + fake |

**Enforced by types, not tests:** `Reaction`, `LockReason`, `CloseReason` are closed enums with
exhaustive switches; `Repo` only via `ParseRepo`.

**Not tested:** rendering, key wiring, picker visuals, the `$EDITOR` process, position-file I/O,
go-gh auth.

## Phasing

1. **Foundation:** action runner with timeout (migrating comment/close), picker extraction,
   M4 retry, M8 drafts.
2. **Tier 1:** labels, assign, reactions, saved queries, templates, `$EDITOR`.
3. **Tier 2:** duplicate, lock, transfer (GraphQL), undo, session summary, resume.
