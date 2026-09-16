package token

import (
	"context"
	"errors"
	"time"

	"github.com/hlouis/herdr-glab/internal/cache"
	"github.com/hlouis/herdr-glab/internal/herdr"
	"github.com/hlouis/herdr-glab/internal/repo"
)

// maxTTL is the upper bound herdr accepts for --ttl-ms.
const maxTTL = 24 * time.Hour

type Reporter struct {
	Herdr  *herdr.Client
	Source string
	Name   string
	TTL    time.Duration
}

// NewReporter uses twice the fetch interval as TTL so tokens disappear once
// the poller stops refreshing them.
func NewReporter(h *herdr.Client, source, name string, fetchInterval time.Duration) Reporter {
	return Reporter{Herdr: h, Source: source, Name: name, TTL: min(2*fetchInterval, maxTTL)}
}

// Apply sets the token on workspaces whose branch has an MR and clears it elsewhere.
func (r Reporter) Apply(ctx context.Context, c cache.Cache, repos []repo.WorkspaceRepo) error {
	var errs []error
	for _, wr := range repos {
		var err error
		if mr, ok := repo.CurrentMR(c, wr); ok {
			err = r.Herdr.ReportToken(ctx, wr.WorkspaceID, r.Source, r.Name, Label(mr), r.TTL)
		} else {
			err = r.Herdr.ClearToken(ctx, wr.WorkspaceID, r.Source, r.Name)
		}
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
