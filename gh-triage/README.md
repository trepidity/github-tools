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
| List | `j/k` move · `enter` open · `/` filter · `s` new search · `r` repos · `q` quit |
| Issue | `n/p` next/prev · `c` comment · `x` close · `X` comment+close · `o` browser · `r` repos · `esc` list |
| Comment | `ctrl+s` send · `esc` cancel |
| Close | `c` completed · `n` not planned · `esc` cancel |

## Pins

`~/.config/gh-triage/config.yml`:

```yaml
pins:
  - owner/repo
  - other-org/other-repo
```
