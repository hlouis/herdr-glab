package repo

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hlouis/herdr-glab/internal/cache"
	"github.com/hlouis/herdr-glab/internal/gitlab"
)

const forkBranchPrefix = "mr/"

// ForkBranch is the local branch name used to check out a fork MR.
func ForkBranch(iid int) string {
	return fmt.Sprintf("%s%d", forkBranchPrefix, iid)
}

func forkBranchIID(branch string) (int, bool) {
	rest, ok := strings.CutPrefix(branch, forkBranchPrefix)
	if !ok {
		return 0, false
	}
	iid, err := strconv.Atoi(rest)
	return iid, err == nil
}

// CurrentMR finds the MR for the workspace's current branch.
func CurrentMR(c cache.Cache, wr WorkspaceRepo) (gitlab.MergeRequest, bool) {
	if wr.Branch == "" {
		return gitlab.MergeRequest{}, false
	}
	iid, isForkBranch := forkBranchIID(wr.Branch)
	for _, r := range wr.Remotes {
		if isForkBranch {
			if mr, ok := c.FindByIID(r.Project, iid); ok {
				return mr, true
			}
		}
		if mr, ok := c.FindBySourceBranch(r.Project, wr.Branch); ok {
			return mr, true
		}
	}
	return gitlab.MergeRequest{}, false
}

// FindRepo returns a local repository for project and the remote pointing at
// it, preferring a primary checkout over linked worktrees.
func FindRepo(repos []WorkspaceRepo, project string) (WorkspaceRepo, string, bool) {
	project = strings.ToLower(project)
	var found WorkspaceRepo
	var remote string
	for _, wr := range repos {
		name, ok := wr.RemoteFor(project)
		if !ok {
			continue
		}
		if remote == "" || (found.Linked && !wr.Linked) {
			found, remote = wr, name
		}
	}
	return found, remote, remote != ""
}

// WorkspaceWith returns the workspace whose current branch is mr.
func WorkspaceWith(c cache.Cache, repos []WorkspaceRepo, mr gitlab.MergeRequest) (WorkspaceRepo, bool) {
	for _, wr := range repos {
		if current, ok := CurrentMR(c, wr); ok && current.WebURL == mr.WebURL {
			return wr, true
		}
	}
	return WorkspaceRepo{}, false
}
