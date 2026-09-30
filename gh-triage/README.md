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
| List | `j/k` move · `enter` open · `/` filter · `s` search (saved or typed) · `r` repos · `u` undo last close · `q` quit |
| Issue | `n/p` next/prev · `c` comment · `x` close · `X` comment+close · `d` close as duplicate · `l` labels · `a` assign me · `A` assignees · `+` react · `L` lock/unlock · `t` transfer · `u` undo last close · `R` retry comments · `o` browser · `r` repos · `?` all keys · `esc` list |
| Comment | `ctrl+s` send · `ctrl+t` insert template · `ctrl+e` edit in `$EDITOR` · `esc` close (draft kept) |
| Close | `c` completed · `n` not planned · `esc` cancel |
| Pickers | type to filter · `↑/↓` move · `space` toggle (labels, assignees) · `enter` choose · `esc` cancel |

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
```
