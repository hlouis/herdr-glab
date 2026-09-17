package action

import (
	"context"
	"fmt"
	"strings"

	"github.com/hlouis/herdr-glab/internal/cache"
	"github.com/hlouis/herdr-glab/internal/gitlab"
	"github.com/hlouis/herdr-glab/internal/repo"
)

// bodyLimit keeps one thread from filling the agent's context.
const bodyLimit = 1200

// SendThreadsToAgent hands review threads to the agent running in the workspace
// that has this merge request checked out. It returns that workspace's label.
func (a Runner) SendThreadsToAgent(ctx context.Context, c cache.Cache, repos []repo.WorkspaceRepo, mr gitlab.MergeRequest, threads []gitlab.Discussion) (string, error) {
	if len(threads) == 0 {
		return "", fmt.Errorf("no threads selected")
	}
	wr, ok := repo.WorkspaceWith(c, repos, mr)
	if !ok {
		return "", fmt.Errorf("!%d is not checked out in any workspace; press c in the panel first", mr.IID)
	}

	agents, err := a.Deps.Herdr.Agents(ctx)
	if err != nil {
		return "", err
	}
	target := ""
	for _, agent := range agents {
		if agent.WorkspaceID == wr.WorkspaceID {
			target = agent.PaneID
			break
		}
	}
	if target == "" {
		return "", fmt.Errorf("no agent is running in %s", wr.Label)
	}
	return wr.Label, a.Deps.Herdr.AgentPrompt(ctx, target, BuildThreadPrompt(mr, threads))
}

// BuildThreadPrompt turns review threads into one prompt. Resolving is left to
// the human: the agent cannot judge whether a reviewer is satisfied.
func BuildThreadPrompt(mr gitlab.MergeRequest, threads []gitlab.Discussion) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Address review feedback on !%d (%s), branch %s.\n", mr.IID, mr.Project, mr.SourceBranch)
	b.WriteString("Work in this worktree. Do not resolve the threads in GitLab and do not push; I will review your changes first.\n")

	for i, t := range threads {
		where := t.Where()
		if where == "" {
			where = "overall comment"
		}
		fmt.Fprintf(&b, "\n--- thread %d of %d: %s\n", i+1, len(threads), where)
		for _, note := range t.Notes {
			body := strings.TrimSpace(note.Body)
			if len(body) > bodyLimit {
				body = body[:bodyLimit] + "\n[truncated]"
			}
			fmt.Fprintf(&b, "%s wrote:\n%s\n", note.Author, body)
		}
	}
	return b.String()
}
