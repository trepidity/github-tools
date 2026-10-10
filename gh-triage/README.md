# gh triage

Work through GitHub issues and pull requests from the terminal, without losing your place.

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

Run these commands from the checkout's `gh-triage` directory:

```bash
git pull
go build -o gh-triage .
```

The local extension already points to this directory, so rebuilding updates `gh triage`
without reinstalling it. Restart any running session to use the new build. Verify the
installation with `gh extension list` and `gh triage --help`.

### Uninstall

```bash
gh extension remove triage
```

## Use

```bash
gh triage                                # pick a repo
gh triage owner/repo                     # open issues in one repo
gh triage --query "org:foo assignee:@me" # any GitHub issue search
gh triage --query "repo:owner/repo is:pr is:open" # review pull requests
```

Every screen keeps its keyboard guide at the bottom of the terminal. Hints wrap on
narrow terminals. `Shift+P` toggles issues/PRs, and `r` opens the repository switcher
from a list or reader; `?` expands the reader's guide.

| Screen | Keys |
|---|---|
| List | `P` toggle issues/PRs · `N` new issue · `ctrl+r` new repo · `j/k` move · `enter` open · `146G` or `:146` then `enter` jump to #146 · `/` filter · `s` search (saved or typed) · `r` repos · `u` undo last close · `q` quit |
| Issue | `P` toggle issues/PRs · `N` new issue · `ctrl+r` new repo · `n/p` next/prev · `c` comment · `x` close · `X` comment+close · `d` close as duplicate · `l` labels · `a` assign me · `A` assignees · `+` react · `L` lock/unlock · `t` transfer · `u` undo last close · `R` retry comments · `o` browser · `r` repos · `?` all keys · `esc` list |
| Create | `tab`/`shift+tab` fields · `ctrl+s` create · `ctrl+e` issue body in `$EDITOR` · `esc` close (draft kept) |
| Pull request | `f` files/conversation · `v` review/approve · `M` merge · `R` refresh · `n/p` next/prev · `c` comment · `o` browser · `P` issues/PRs · `esc` list |
| Review | Choose Comment, Approve, or Request changes, then `ctrl+s` submit · `ctrl+e` `$EDITOR` · `esc` cancel (draft kept) |
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
The owner field lists your signed-in account and organizations. Use `↑/↓` to choose,
then `enter` or `tab` to continue. You can also type a login or organization directly;
leave owner blank for your signed-in account. If loading organizations fails, `ctrl+r`
in the owner field retries.
Enter a name and optional description, then tab to visibility and press `space` to
switch between **private** (the default) and **public**. `ctrl+s` creates the repository
with issues enabled, adds it to the switcher and opens its issue queue. This creates
the remote repository; it does not clone it or push local files.

Creation uses your existing `gh` authentication. GitHub permission and validation
errors remain on the form with your draft intact. `esc` also keeps each creation draft
for the current session; quitting discards unsent drafts. Successful creations appear
in the exit summary.

### Review and merge pull requests

Press `P` from a list or reader to switch between issues and pull requests, keeping
the search's repository and other filters. Explicit `is:pr` or `type:pr` queries also
work in `--query` and saved searches. Repository switching keeps the current queue type.

Open a PR with `enter`. The conversation includes its description, reviews, code comments,
and discussion. Press `f` to read changed files and their patches, then `f` again to return.
`R` reloads the PR. GitHub may omit binary or large patches and caps the file list at 3,000;
missing patches and incomplete file lists are identified. Use `o` to inspect the full PR
in your browser. Creating inline code comments is not supported; reviews accept a summary.

Press `v` and choose **Comment**, **Approve**, or **Request changes**. Enter review text
and press `ctrl+s` to submit; approval can have an empty body. Review drafts survive
canceling and failed submissions. Reviews are attached to the displayed commit, and
new commits detected before submission require reloading and reviewing again.
If submission reports new commits, press `esc` to keep the draft, `R` to reload,
and `f` to inspect the changes before opening `v` again. Review drafts last for the
current session; quitting discards them.

Press `M` to choose **Merge commit**, **Squash and merge**, or **Rebase and merge**,
then confirm with `y` (`n` or `esc` cancels). The merge request includes the reviewed
head SHA so a concurrent push cannot silently add unreviewed commits. GitHub enforces
permissions, allowed merge methods, and repository rules; failures appear in the status
line. Draft and conflicting PRs cannot be merged from the form. Merge queues and auto-merge
are not supported. Confirmed merges remain marked as merged and cannot be undone here.

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
  reviews: "is:pr is:open review-requested:@me"
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

Watching also works for PR searches. PR files and review details refresh when you
open the PR or press `R`; polling updates the queue and discussion comments.

Each poll reads only what changed since the last one (re-reading two minutes back, because the
search index can list updates out of order), so a burst larger than one search page is read in
full. Failed polls back off, doubling the interval up to 16×, and recover on the next success.
Polling stops when the program exits; there is no daemon or webhook. Paging and the resume
position still follow creation time.

Activity is judged against your computer's clock, allowing ten seconds of drift from GitHub's.
An issue that stops matching the query (closed by someone else under `is:open`, say) is not
removed from the list.
