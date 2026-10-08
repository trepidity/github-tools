# gh triage

Work through GitHub issues one at a time from the terminal, without losing your place.

## Install

Requires [Go](https://go.dev/dl/) and the [GitHub CLI](https://cli.github.com/), logged in with `gh auth login`.
gh-triage uses that session, so there is no separate login.

```bash
git clone git@github.com:trepidity/github-tools.git
cd github-tools/gh-triage
go build -o gh-triage .
gh extension install .
```

`gh extension install .` links this folder, so `gh triage` runs the `gh-triage` binary built here.
(`gh extension install trepidity/github-tools` does not work: gh only installs remote extensions
from repos whose names start with `gh-`.)

### Update

```bash
git pull
go build -o gh-triage .
```

### Uninstall

```bash
gh extension remove triage
```

## Use

```bash
gh triage                                # pick a repo
gh triage owner/repo                     # open issues in one repo
gh triage --query "org:foo assignee:@me" # any GitHub issue search
```

| Screen | Keys |
|---|---|
| List | `N` new issue · `ctrl+r` new repo · `j/k` move · `enter` open · `146G` or `:146` then `enter` jump to issue #146 · `/` filter · `s` search (saved or typed) · `r` repos · `u` undo last close · `q` quit |
| Issue | `N` new issue · `ctrl+r` new repo · `n/p` next/prev · `c` comment · `x` close · `X` comment+close · `d` close as duplicate · `l` labels · `a` assign me · `A` assignees · `+` react · `L` lock/unlock · `t` transfer · `u` undo last close · `R` retry comments · `o` browser · `r` repos · `?` all keys · `esc` list |
| Create | `tab`/`shift+tab` fields · `ctrl+s` create · `ctrl+e` issue body in `$EDITOR` · `esc` close (draft kept) |
| Comment | `ctrl+s` send · `ctrl+t` insert template · `ctrl+e` edit in `$EDITOR` · `esc` close (draft kept) |
| Close | `c` completed · `n` not planned · `esc` cancel |
| Pickers | type to filter · `↑/↓` move · `space` toggle (labels, assignees) · `enter` choose · `esc` cancel |

### Create issues and repositories

Press `N` from the list or an issue to enter a repository (`owner/name`), title and
optional Markdown body. The destination starts with the current issue's repository
or a single repository from the search; cross-repository lists require a destination.
After creation, the app opens the returned issue in its repository's open-issue queue,
even while GitHub's search index is catching up.

Press `ctrl+r` from the list, issue or repository switcher to create a repository.
Leave owner blank for your signed-in account, or enter your login or an organization.
Enter a name and optional description, then tab to visibility and press `space` to
switch between **private** (the default) and **public**. `ctrl+s` creates the repository
with issues enabled, adds it to the switcher and opens its issue queue. This creates
the remote repository; it does not clone it or push local files.

Creation uses your existing `gh` authentication. GitHub permission and validation
errors remain on the form with your draft intact. `esc` also keeps each creation draft
for the current session; quitting discards unsent drafts. Successful creations appear
in the exit summary.

### Navigation

Jumps select an issue by its GitHub number in the current filtered list, loading more pages
if needed. Press `enter` to open it. `esc` or another list command cancels a pending jump;
`backspace` edits the number. In searches spanning repositories, the first matching number wins.

Quitting prints a one-line summary (`closed 12 · commented 5 · …`). Reopening the same repo or
query puts you back on the last issue you viewed.

## Config

`~/.config/gh-triage/config.yml`:

```yaml
pins:                 # listed first in the repo switcher
  - owner/repo
queries:              # listed first when you press s, in this order
  mine: "assignee:@me is:open"
  stale: "org:foo is:open updated:<2026-01-01"
templates:            # ctrl+t in the comment editor
  repro: "Thanks! Could you share steps to reproduce?"
watch:                # optional; absent or omitted interval disables polling
  poll_interval: 1m   # minimum 30s
```

When enabled, `gh triage` polls the current query's update feed while it is open. Issues with
activity since you opened the query move above the created-time queue, most recent first, and
are marked `●` until you view them. Your own comments, labels, closes and other changes do not
count as activity, and a poll never overwrites an edit you just made with an older copy from
GitHub's search index. If the issue on screen changes, its thread reloads. The selected issue
stays selected.

Each poll reads only what changed since the last one (re-reading two minutes back, because the
search index can list updates out of order), so a burst larger than one search page is read in
full. Failed polls back off, doubling the interval up to 16×, and recover on the next success.
Polling stops when the program exits; there is no daemon or webhook. Paging and the resume
position still follow creation time.

Activity is judged against your computer's clock, allowing ten seconds of drift from GitHub's.
An issue that stops matching the query (closed by someone else under `is:open`, say) is not
removed from the list.
