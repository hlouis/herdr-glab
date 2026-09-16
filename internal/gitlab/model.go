// Package gitlab fetches merge requests through `glab api graphql`. See doc/design.md §4.
package gitlab

import (
	"slices"
	"strings"
	"time"
)

type Role string

const (
	RoleReviewer Role = "reviewer"
	RoleAuthor   Role = "author"
	RoleAssignee Role = "assignee"
)

type Reviewer struct {
	Username string `json:"username"`
	State    string `json:"state"`
	Approved bool   `json:"approved"`
}

type MergeRequest struct {
	Roles             []Role     `json:"roles,omitempty"`
	IID               int        `json:"iid"`
	Title             string     `json:"title"`
	WebURL            string     `json:"web_url"`
	Project           string     `json:"project"`
	SourceProject     string     `json:"source_project"`
	SourceBranch      string     `json:"source_branch"`
	TargetBranch      string     `json:"target_branch"`
	HeadSHA           string     `json:"head_sha"`
	Draft             bool       `json:"draft"`
	Author            string     `json:"author"`
	UpdatedAt         time.Time  `json:"updated_at"`
	MergeStatus       string     `json:"merge_status"`
	Pipeline          string     `json:"pipeline"`
	Approved          bool       `json:"approved"`
	ApprovalsLeft     int        `json:"approvals_left"`
	ThreadsTotal      int        `json:"threads_total"`
	ThreadsUnresolved int        `json:"threads_unresolved"`
	Notes             int        `json:"notes"`
	Reviewers         []Reviewer `json:"reviewers,omitempty"`
	MyReviewState     string     `json:"my_review_state"`
}

func (m MergeRequest) HasRole(r Role) bool {
	return slices.Contains(m.Roles, r)
}

// IsFork reports whether the source branch lives in a different project.
func (m MergeRequest) IsFork() bool {
	return !strings.EqualFold(m.SourceProject, m.Project)
}
