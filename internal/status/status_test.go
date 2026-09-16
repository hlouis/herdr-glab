package status

import (
	"testing"
	"time"

	"github.com/hlouis/herdr-glab/internal/cache"
	"github.com/hlouis/herdr-glab/internal/gitlab"
)

func TestLine(t *testing.T) {
	fetched := time.Now()
	reviewPending := gitlab.MergeRequest{Roles: []gitlab.Role{gitlab.RoleReviewer}}
	reviewDone := gitlab.MergeRequest{Roles: []gitlab.Role{gitlab.RoleReviewer}, MyReviewState: "APPROVED"}
	myBrokenMR := gitlab.MergeRequest{Roles: []gitlab.Role{gitlab.RoleAuthor}, Pipeline: "FAILED"}
	myDraft := gitlab.MergeRequest{Roles: []gitlab.Role{gitlab.RoleAuthor}, Pipeline: "FAILED", Draft: true}
	quiet := gitlab.MergeRequest{Roles: []gitlab.Role{gitlab.RoleAssignee}, Pipeline: "SUCCESS"}

	tests := []struct {
		name  string
		cache cache.Cache
		want  string
	}{
		{name: "never fetched", want: "MR –"},
		{
			name:  "nothing waiting",
			cache: cache.Cache{FetchedAt: fetched, Mine: []gitlab.MergeRequest{reviewDone, quiet, myDraft}},
			want:  "MR 3",
		},
		{
			name:  "review and a broken pipeline wait",
			cache: cache.Cache{FetchedAt: fetched, Mine: []gitlab.MergeRequest{reviewPending, myBrokenMR, quiet}},
			want:  "MR 3 · 2 todo",
		},
		{
			name:  "stale after a failed fetch",
			cache: cache.Cache{FetchedAt: fetched, Error: "glab is not authenticated", Mine: []gitlab.MergeRequest{reviewPending}},
			want:  "MR 1 · 1 todo ⚠",
		},
	}
	for _, tt := range tests {
		if got := Line(tt.cache); got != tt.want {
			t.Errorf("%s: Line() = %q, want %q", tt.name, got, tt.want)
		}
	}
}
