package ui

import (
	"slices"
	"testing"

	"github.com/hlouis/herdr-glab/internal/gitlab"
)

func TestPendingReviewers(t *testing.T) {
	tests := []struct {
		name string
		mr   gitlab.MergeRequest
		want []string
	}{
		{
			name: "everyone approved",
			mr: gitlab.MergeRequest{
				ApprovedBy: []string{"codebuddy"},
				Reviewers:  []gitlab.Reviewer{{Username: "codebuddy", State: "APPROVED", Approved: true}},
			},
		},
		{
			name: "one reviewer still owes a review",
			mr: gitlab.MergeRequest{
				ApprovedBy: []string{"codebuddy"},
				Reviewers: []gitlab.Reviewer{
					{Username: "codebuddy", State: "APPROVED", Approved: true},
					{Username: "ai.tan", State: "UNREVIEWED"},
				},
			},
			want: []string{"ai.tan"},
		},
		{
			// GitLab can report a reviewer as unapproved while the approval is
			// recorded on the MR; approvedBy wins.
			name: "approved outside the reviewer interaction",
			mr: gitlab.MergeRequest{
				ApprovedBy: []string{"kkdy"},
				Reviewers:  []gitlab.Reviewer{{Username: "kkdy", State: "REVIEWED"}},
			},
		},
		{
			name: "no reviewers",
			mr:   gitlab.MergeRequest{},
		},
	}
	for _, tt := range tests {
		if got := pendingReviewers(tt.mr); !slices.Equal(got, tt.want) {
			t.Errorf("%s: pendingReviewers() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestOtherRoles(t *testing.T) {
	tests := []struct {
		name  string
		roles []gitlab.Role
		want  string
	}{
		{name: "single role", roles: []gitlab.Role{gitlab.RoleAssignee}},
		{name: "assignee also mentioned", roles: []gitlab.Role{gitlab.RoleAssignee, gitlab.RoleMentioned}, want: "also mentioned"},
		{
			name:  "listed under review, also author and assignee",
			roles: []gitlab.Role{gitlab.RoleAuthor, gitlab.RoleReviewer, gitlab.RoleAssignee},
			want:  "also assignee, author",
		},
	}
	for _, tt := range tests {
		if got := otherRoles(gitlab.MergeRequest{Roles: tt.roles}); got != tt.want {
			t.Errorf("%s: otherRoles() = %q, want %q", tt.name, got, tt.want)
		}
	}
}
