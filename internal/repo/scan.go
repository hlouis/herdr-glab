package repo

import (
	"context"
	"path/filepath"

	"github.com/hlouis/herdr-glab/internal/git"
	"github.com/hlouis/herdr-glab/internal/herdr"
)

// Remote is a git remote that points at a project on the configured GitLab host.
type Remote struct {
	Name    string
	Project string
}

// WorkspaceRepo describes one herdr workspace. Checkout is empty when the
// workspace is not inside a git repository.
type WorkspaceRepo struct {
	WorkspaceID string
	Label       string
	Root        string
	Checkout    string
	Branch      string
	Linked      bool
	Remotes     []Remote
}

// Scan inspects every workspace, or only onlyWorkspace when it is non-empty.
func Scan(ctx context.Context, h *herdr.Client, host, onlyWorkspace string) ([]WorkspaceRepo, error) {
	workspaces, err := h.Workspaces(ctx)
	if err != nil {
		return nil, err
	}
	var repos []WorkspaceRepo
	for _, ws := range workspaces {
		if onlyWorkspace != "" && ws.ID != onlyWorkspace {
			continue
		}
		repos = append(repos, inspect(ctx, h, host, ws))
	}
	return repos, nil
}

func inspect(ctx context.Context, h *herdr.Client, host string, ws herdr.Workspace) WorkspaceRepo {
	wr := WorkspaceRepo{WorkspaceID: ws.ID, Label: ws.Label}
	if ws.Worktree != nil {
		wr.Checkout = ws.Worktree.CheckoutPath
		wr.Root = ws.Worktree.RepoRoot
		wr.Linked = ws.Worktree.IsLinkedWorktree
	} else if !locateFromPane(ctx, h, &wr) {
		return wr
	}

	remotes, err := git.Remotes(ctx, wr.Checkout)
	if err != nil {
		return WorkspaceRepo{WorkspaceID: ws.ID, Label: ws.Label}
	}
	for _, r := range remotes {
		if h, project, ok := ParseRemote(r.URL); ok && h == host {
			wr.Remotes = append(wr.Remotes, Remote{Name: r.Name, Project: project})
		}
	}
	wr.Branch, _ = git.Output(ctx, wr.Checkout, "branch", "--show-current")
	return wr
}

// locateFromPane resolves the repository of a workspace herdr did not tag with
// worktree provenance, using its first pane's cwd.
func locateFromPane(ctx context.Context, h *herdr.Client, wr *WorkspaceRepo) bool {
	panes, err := h.Panes(ctx, wr.WorkspaceID)
	if err != nil || len(panes) == 0 {
		return false
	}
	top, err := git.Output(ctx, panes[0].Cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return false
	}
	common, err := git.Output(ctx, top, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return false
	}
	wr.Checkout = top
	wr.Root = filepath.Dir(common)
	wr.Linked = wr.Root != top
	return true
}

// RemoteFor returns the remote name pointing at project.
func (wr WorkspaceRepo) RemoteFor(project string) (string, bool) {
	for _, r := range wr.Remotes {
		if r.Project == project {
			return r.Name, true
		}
	}
	return "", false
}
