// Package action implements the MR operations offered by the panel. See doc/design.md §8.
package action

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/hlouis/herdr-glab/internal/cache"
	"github.com/hlouis/herdr-glab/internal/git"
	"github.com/hlouis/herdr-glab/internal/gitlab"
	"github.com/hlouis/herdr-glab/internal/refresh"
	"github.com/hlouis/herdr-glab/internal/repo"
	"github.com/hlouis/herdr-glab/internal/run"
)

type Runner struct {
	Deps refresh.Deps
}

// Review opens tuicr for mr in a new tab of the repository's workspace, or of
// the workspace the panel was opened from.
func (a Runner) Review(ctx context.Context, repos []repo.WorkspaceRepo, mr gitlab.MergeRequest) error {
	if _, err := exec.LookPath(a.Deps.Config.TuicrPath); err != nil {
		return fmt.Errorf("tuicr is not installed; see https://tuicr.dev (%v)", err)
	}
	workspaceID := a.Deps.Env.InvocationWorkspaceID()
	cwd, _ := os.UserHomeDir()
	if wr, _, ok := repo.FindRepo(repos, mr.Project); ok {
		workspaceID, cwd = wr.WorkspaceID, wr.Root
	}
	if workspaceID == "" {
		return errors.New("no workspace to open the review in")
	}

	h := a.Deps.Herdr
	if err := h.FocusWorkspace(ctx, workspaceID); err != nil {
		return err
	}
	paneID, err := h.CreateTab(ctx, workspaceID, cwd, fmt.Sprintf("review !%d", mr.IID))
	if err != nil {
		return err
	}
	return h.PaneRun(ctx, paneID, shellJoin(a.Deps.Config.TuicrPath, "mr", mr.WebURL))
}

// Checkout opens mr's source branch in a worktree workspace, reusing an
// existing worktree for that branch.
func (a Runner) Checkout(ctx context.Context, repos []repo.WorkspaceRepo, mr gitlab.MergeRequest) error {
	wr, remote, ok := repo.FindRepo(repos, mr.Project)
	if !ok {
		return fmt.Errorf("no local repository for %s", mr.Project)
	}
	branch := mr.SourceBranch
	if mr.IsFork() {
		branch = repo.ForkBranch(mr.IID)
	}
	label := fmt.Sprintf("!%d %s", mr.IID, mr.SourceBranch)

	h := a.Deps.Herdr
	entries, err := h.WorktreeList(ctx, wr.Root)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Branch != branch {
			continue
		}
		if e.OpenWorkspaceID != "" {
			return h.FocusWorkspace(ctx, e.OpenWorkspaceID)
		}
		return h.WorktreeOpen(ctx, wr.Root, e.Path, label)
	}

	if mr.IsFork() {
		ref := fmt.Sprintf("+refs/merge-requests/%d/head:refs/heads/%s", mr.IID, branch)
		if _, err := git.Output(ctx, wr.Root, "fetch", remote, ref); err != nil {
			return err
		}
		return h.WorktreeCreate(ctx, wr.Root, branch, "", label)
	}
	if _, err := git.Output(ctx, wr.Root, "fetch", remote, branch); err != nil {
		return err
	}
	return h.WorktreeCreate(ctx, wr.Root, branch, remote+"/"+branch, label)
}

// Focus jumps to the workspace that already has mr checked out.
func (a Runner) Focus(ctx context.Context, c cache.Cache, repos []repo.WorkspaceRepo, mr gitlab.MergeRequest) error {
	wr, ok := repo.WorkspaceWith(c, repos, mr)
	if !ok {
		return fmt.Errorf("!%d is not checked out in any workspace", mr.IID)
	}
	return a.Deps.Herdr.FocusWorkspace(ctx, wr.WorkspaceID)
}

func OpenBrowser(ctx context.Context, url string) error {
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	_, err := run.Output(ctx, opener, url)
	return err
}

func Copy(ctx context.Context, text string) error {
	name, args := "pbcopy", []string(nil)
	if runtime.GOOS != "darwin" {
		name = "wl-copy"
		if _, err := exec.LookPath(name); err != nil {
			name, args = "xclip", []string{"-selection", "clipboard"}
		}
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

// shellJoin quotes args for the shell that `herdr pane run` types into.
func shellJoin(args ...string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
	}
	return strings.Join(quoted, " ")
}
