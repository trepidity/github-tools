# gh triage

Work through GitHub issues one at a time from the terminal, without losing your place.

## Install

```bash
cd gh-triage
go build -o gh-triage .
gh extension install .
```

Uses your existing `gh auth login` session.

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
