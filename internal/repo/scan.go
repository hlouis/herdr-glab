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
	snap, err := h.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	firstCwd := map[string]string{}
	for _, p := range snap.Panes {
		if _, ok := firstCwd[p.WorkspaceID]; !ok {
			firstCwd[p.WorkspaceID] = p.Cwd
		}
	}
	var repos []WorkspaceRepo
	for _, ws := range snap.Workspaces {
		if onlyWorkspace != "" && ws.ID != onlyWorkspace {
			continue
		}
		repos = append(repos, inspect(ctx, host, ws, firstCwd[ws.ID]))
	}
	return repos, nil
}

// inspect resolves a workspace's repository: herdr's worktree provenance when
// it has one, otherwise the cwd of its first pane.
func inspect(ctx context.Context, host string, ws herdr.Workspace, paneCwd string) WorkspaceRepo {
	wr := WorkspaceRepo{WorkspaceID: ws.ID, Label: ws.Label}
	if ws.Worktree != nil {
		wr.Checkout = ws.Worktree.CheckoutPath
		wr.Root = ws.Worktree.RepoRoot
		wr.Linked = ws.Worktree.IsLinkedWorktree
	} else if !locateFromCwd(ctx, paneCwd, &wr) {
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

func locateFromCwd(ctx context.Context, cwd string, wr *WorkspaceRepo) bool {
	if cwd == "" {
		return false
	}
	top, err := git.Output(ctx, cwd, "rev-parse", "--show-toplevel")
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
