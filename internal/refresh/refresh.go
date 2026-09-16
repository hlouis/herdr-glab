// Package refresh fetches GitLab data into the cache and applies workspace tokens.
package refresh

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/hlouis/herdr-glab/internal/cache"
	"github.com/hlouis/herdr-glab/internal/config"
	"github.com/hlouis/herdr-glab/internal/gitlab"
	"github.com/hlouis/herdr-glab/internal/herdr"
	"github.com/hlouis/herdr-glab/internal/plugin"
	"github.com/hlouis/herdr-glab/internal/repo"
	"github.com/hlouis/herdr-glab/internal/token"
)

type Deps struct {
	Env      plugin.Env
	Config   config.Config
	Herdr    *herdr.Client
	GitLab   *gitlab.Client
	Reporter token.Reporter
}

func NewDeps(env plugin.Env, cfg config.Config) Deps {
	h := herdr.New(env.HerdrBin)
	return Deps{
		Env:      env,
		Config:   cfg,
		Herdr:    h,
		GitLab:   gitlab.NewClient(cfg.GlabPath, cfg.Host),
		Reporter: token.NewReporter(h, env.TokenSource(), cfg.TokenName, cfg.FetchInterval),
	}
}

func (d Deps) CachePath() string {
	return cache.Path(d.Env.StateDir)
}

// All fetches GitLab data, saves the cache and refreshes every workspace token.
// A glab auth or install problem clears all tokens; a transient failure keeps
// the previous cache and tokens until their TTL.
func All(ctx context.Context, d Deps) (cache.Cache, error) {
	prev, err := cache.Load(d.CachePath())
	if err != nil {
		return prev, err
	}
	repos, err := repo.Scan(ctx, d.Herdr, d.Config.Host, "")
	if err != nil {
		return prev, fmt.Errorf("scan workspaces: %w", err)
	}

	next, fetchErr := fetch(ctx, d, repos)
	if fetchErr == nil {
		if err := cache.Save(d.CachePath(), next); err != nil {
			return next, err
		}
		return next, d.Reporter.Apply(ctx, next, repos)
	}

	prev.Host = d.Config.Host
	prev.Error = fetchErr.Error()
	if errors.Is(fetchErr, gitlab.ErrUnauthorized) || errors.Is(fetchErr, gitlab.ErrGlabMissing) {
		prev.Mine, prev.BranchMRs = nil, nil
		fetchErr = errors.Join(fetchErr, d.Reporter.Apply(ctx, prev, repos))
	}
	return prev, errors.Join(fetchErr, cache.Save(d.CachePath(), prev))
}

// Workspace recomputes one workspace's token from the cache without network calls.
func Workspace(ctx context.Context, d Deps, workspaceID string) error {
	c, err := cache.Load(d.CachePath())
	if err != nil {
		return err
	}
	repos, err := repo.Scan(ctx, d.Herdr, d.Config.Host, workspaceID)
	if err != nil {
		return err
	}
	return d.Reporter.Apply(ctx, c, repos)
}

func fetch(ctx context.Context, d Deps, repos []repo.WorkspaceRepo) (cache.Cache, error) {
	username, mine, err := d.GitLab.FetchMine(ctx)
	if err != nil {
		return cache.Cache{}, err
	}
	known := cache.Cache{Mine: mine}
	branchMRs, err := d.GitLab.FetchBranches(ctx, username, workspaceBranches(known, repos))
	if err != nil {
		return cache.Cache{}, err
	}
	return cache.Cache{
		Host:      d.Config.Host,
		Username:  username,
		FetchedAt: time.Now(),
		Mine:      mine,
		BranchMRs: branchMRs,
	}, nil
}

// workspaceBranches lists, per project, the workspace branches whose MR is not
// already among the user's own MRs.
func workspaceBranches(known cache.Cache, repos []repo.WorkspaceRepo) map[string][]string {
	branches := map[string][]string{}
	for _, wr := range repos {
		if wr.Branch == "" {
			continue
		}
		if _, ok := repo.CurrentMR(known, wr); ok {
			continue
		}
		for _, r := range wr.Remotes {
			if !slices.Contains(branches[r.Project], wr.Branch) {
				branches[r.Project] = append(branches[r.Project], wr.Branch)
			}
		}
	}
	return branches
}
