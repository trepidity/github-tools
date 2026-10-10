package ui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/trepidity/gh-triage/internal/github"
)

type pullLoadedMsg struct {
	key string
	pr  github.PullRequest
	err error
}

func (m Model) isPullQueue() bool {
	for _, token := range strings.Fields(m.query) {
		if strings.EqualFold(token, "is:pr") || strings.EqualFold(token, "type:pr") {
			return true
		}
	}
	return false
}

func (m *Model) togglePulls() tea.Cmd {
	kind := "is:pr"
	if m.isPullQueue() {
		kind = "is:issue"
	}
	var tokens []string
	for _, token := range strings.Fields(m.query) {
		switch strings.ToLower(token) {
		case "is:issue", "type:issue", "is:pr", "type:pr":
			tokens = append(tokens, kind)
		default:
			tokens = append(tokens, token)
		}
	}
	if m.query == "" {
		tokens = []string{kind, "is:open"}
	}
	return m.startSearch(strings.Join(tokens, " "))
}

func (m *Model) loadPull(is github.Issue) tea.Cmd {
	key := is.Key()
	if m.pullsLoading[key] {
		return nil
	}
	m.pullsLoading[key] = true
	delete(m.pullsErr, key)
	m.refreshIssue()
	client := m.client
	return m.fetch(func(ctx context.Context) tea.Msg {
		pr, err := client.GetPullRequest(ctx, is.Repo, is.Number)
		return pullLoadedMsg{key: key, pr: pr, err: err}
	})
}

func (m *Model) onPullLoaded(msg pullLoadedMsg) {
	delete(m.pullsLoading, msg.key)
	if msg.err != nil {
		m.pullsErr[msg.key] = msg.err
	} else {
		if m.merged[msg.key] {
			msg.pr.Merged, msg.pr.State = true, "closed"
		}
		m.pulls[msg.key] = msg.pr
		m.updateIssue(msg.key, func(is *github.Issue) {
			is.Title, is.Body, is.State = msg.pr.Title, msg.pr.Body, msg.pr.State
		})
	}
	m.refreshIssue()
}

func (m *Model) reviewablePull() (github.Issue, github.PullRequest, bool) {
	is, ok := m.current()
	pr, loaded := m.pulls[is.Key()]
	switch {
	case !ok || !is.PullRequest:
		m.status = "select a pull request first"
	case !loaded || pr.HeadSHA == "" || m.pullsLoading[is.Key()] || m.pullsErr[is.Key()] != nil:
		m.status = "load the pull request first; R reloads"
	case pr.Merged || pr.State != "open" || m.merged[is.Key()]:
		m.status = "pull request is already closed or merged"
	default:
		return is, pr, true
	}
	return is, pr, false
}

func (m *Model) openReview() tea.Cmd {
	if _, _, ok := m.reviewablePull(); !ok {
		return nil
	}
	m.status = ""
	return m.openPicker(newPicker(pickReview, "Review pull request", "review: ", []pickerItem{
		{label: "Comment", detail: "submit feedback", value: string(github.ReviewComment)},
		{label: "Approve", detail: "approve the reviewed commit", value: string(github.ReviewApprove)},
		{label: "Request changes", detail: "explain required changes", value: string(github.ReviewChanges)},
	}))
}

func reviewLabel(event github.ReviewEvent) string {
	switch event {
	case github.ReviewApprove:
		return "approve"
	case github.ReviewChanges:
		return "request changes"
	default:
		return "comment review"
	}
}

func (m *Model) startReview(event github.ReviewEvent) tea.Cmd {
	is, pr, ok := m.reviewablePull()
	if !ok {
		return nil
	}
	m.mode, m.status = modeReview, ""
	m.reviewEvent, m.reviewHead = event, pr.HeadSHA
	m.editor.Reset()
	m.editor.SetValue(m.reviewDrafts[is.Key()])
	return m.editor.Focus()
}

func (m *Model) keyReview(k tea.KeyMsg) tea.Cmd {
	is, _ := m.current()
	switch k.String() {
	case "esc":
		m.reviewDrafts[is.Key()] = m.editor.Value()
		m.mode = modeNone
		m.editor.Blur()
		return nil
	case "ctrl+e":
		return m.openEditor()
	case "ctrl+s":
		body := strings.TrimSpace(m.editor.Value())
		if body == "" && m.reviewEvent != github.ReviewApprove {
			m.status = "review body is required"
			return nil
		}
		client, event, head := m.client, m.reviewEvent, m.reviewHead
		return m.run(reviewLabel(event), is.Key(), func(ctx context.Context) (func(*Model) tea.Cmd, error) {
			review, err := client.ReviewPullRequest(ctx, is.Repo, is.Number, head, event, body)
			if err != nil {
				return nil, err
			}
			return func(m *Model) tea.Cmd {
				pr := m.pulls[is.Key()]
				pr.Reviews = append(pr.Reviews, review)
				m.pulls[is.Key()] = pr
				delete(m.reviewDrafts, is.Key())
				m.editor.Reset()
				m.editor.Blur()
				m.mode, m.pullFiles = modeNone, false
				m.count(actReviewed)
				m.status = "review submitted: " + reviewLabel(event)
				m.refreshIssue()
				return nil
			}, nil
		})
	}
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(k)
	return cmd
}

func (m *Model) openMerge() tea.Cmd {
	_, pr, ok := m.reviewablePull()
	if !ok {
		return nil
	}
	if pr.Draft {
		m.status = "draft pull requests cannot be merged"
		return nil
	}
	if pr.Mergeable != nil && !*pr.Mergeable {
		m.status = "pull request has merge conflicts; R refreshes"
		return nil
	}
	m.status = ""
	return m.openPicker(newPicker(pickMerge, "Merge method", "method: ", []pickerItem{
		{label: "Merge commit", value: string(github.MergeCommit)},
		{label: "Squash and merge", value: string(github.MergeSquash)},
		{label: "Rebase and merge", value: string(github.MergeRebase)},
	}))
}

func (m *Model) confirmMerge(method github.MergeMethod) tea.Cmd {
	is, pr, ok := m.reviewablePull()
	if !ok {
		return nil
	}
	return m.ask(fmt.Sprintf("%s %s at %s into %s?", method, is.Key(), shortSHA(pr.HeadSHA), pr.BaseLabel), func(m *Model) tea.Cmd {
		client := m.client
		return m.run("merge", is.Key(), func(ctx context.Context) (func(*Model) tea.Cmd, error) {
			if err := client.MergePullRequest(ctx, is.Repo, is.Number, pr.HeadSHA, method); err != nil {
				return nil, err
			}
			return func(m *Model) tea.Cmd {
				m.merged[is.Key()], m.stateOverride[is.Key()] = true, "closed"
				pr.Merged, pr.State = true, "closed"
				m.pulls[is.Key()] = pr
				m.count(actMerged)
				m.status = "merged " + is.Key()
				m.refreshIssue()
				return nil
			}, nil
		})
	})
}

func shortSHA(sha string) string { return sha[:min(len(sha), 12)] }

func (m Model) pullMarkdown(is github.Issue) string {
	key := is.Key()
	if m.pullsLoading[key] {
		return "_Loading pull request…_\n"
	}
	if err := m.pullsErr[key]; err != nil {
		return "Pull request failed to load: " + firstLine(err.Error()) + "\n\nPress R to retry.\n"
	}
	pr, ok := m.pulls[key]
	if !ok {
		return "_Press R to load pull request._\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "**%s → %s** · commit `%s`\n\n%d files · +%d −%d\n\n", pr.HeadLabel, pr.BaseLabel, shortSHA(pr.HeadSHA), pr.ChangedFiles, pr.Additions, pr.Deletions)
	mergeability := "not yet calculated; R refreshes"
	if pr.Mergeable != nil {
		mergeability = "no merge conflicts"
		if !*pr.Mergeable {
			mergeability = "merge conflicts"
		}
	}
	fmt.Fprintf(&b, "Mergeability: %s. GitHub checks repository rules when merging.\n\n", mergeability)
	if m.pullFiles {
		b.WriteString("## Changed files\n\nGitHub patches are shown below; binary or large patches may be omitted. Press o to inspect the full PR in your browser.\n\n")
		if len(pr.Files) < pr.ChangedFiles {
			fmt.Fprintf(&b, "**Only %d of %d files returned by GitHub.**\n\n", len(pr.Files), pr.ChangedFiles)
		}
		for _, f := range pr.Files {
			fmt.Fprintf(&b, "### %s\n\n%s · +%d −%d\n\n", f.Filename, f.Status, f.Additions, f.Deletions)
			if f.PreviousFilename != "" {
				fmt.Fprintf(&b, "Previously: %s\n\n", f.PreviousFilename)
			}
			if f.Patch == "" {
				b.WriteString("_Patch unavailable; open in browser to inspect._\n\n")
			} else {
				b.WriteString(diffBlock(f.Patch))
			}
		}
		return b.String()
	}
	fmt.Fprintf(&b, "%s\n\n## Reviews\n\n", pr.Body)
	if len(pr.Reviews) == 0 {
		b.WriteString("No reviews yet.\n")
	}
	for _, r := range pr.Reviews {
		fmt.Fprintf(&b, "**@%s · %s** · `%s`\n\n%s\n\n", r.Author(), r.State, shortSHA(r.CommitID), r.Body)
	}
	if len(pr.Comments) > 0 {
		b.WriteString("## Code comments\n\n")
	}
	for _, c := range pr.Comments {
		line := c.Line
		note := ""
		if line == 0 {
			line, note = c.OriginalLine, " (outdated or file comment)"
		}
		fmt.Fprintf(&b, "**@%s** · %s:%d%s\n\n%s\n\n", c.Author(), c.Path, line, note, c.Body)
		if c.DiffHunk != "" {
			b.WriteString(diffBlock(c.DiffHunk))
		}
	}
	return b.String()
}

func diffBlock(patch string) string {
	// A fence longer than any run in the patch keeps source text inside the block.
	fence := "```"
	for strings.Contains(patch, fence) {
		fence += "`"
	}
	return fence + "diff\n" + patch + "\n" + fence + "\n\n"
}
