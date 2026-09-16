package gitlab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/hlouis/herdr-glab/internal/run"
)

var (
	ErrGlabMissing  = errors.New("glab not found")
	ErrUnauthorized = errors.New("glab is not authenticated")
)

// Branch queries stay under GitLab's GraphQL complexity limit of 300: four
// project aliases with the mr fragment and first: 20 score 191.
const (
	projectsPerBranchQuery = 4
	mrsPerBranchQuery      = 20
)

var unauthorized = regexp.MustCompile(`(?i)\b401\b|unauthori[sz]ed|not logged in|glab auth login|invalid token|token (has )?expired`)

type Client struct {
	glab string
	host string
}

func NewClient(glab, host string) *Client {
	return &Client{glab: glab, host: host}
}

// FetchMine returns the current username and every opened MR the user
// authored, is assigned to, or is asked to review, merged by web URL.
func (c *Client) FetchMine(ctx context.Context) (string, []MergeRequest, error) {
	lists := []*mineList{
		{role: RoleAuthor, include: "withAuthored", after: "authoredAfter", page: func(d mineData) *mrPage { return d.CurrentUser.AuthoredMergeRequests }},
		{role: RoleReviewer, include: "withReview", after: "reviewAfter", page: func(d mineData) *mrPage { return d.CurrentUser.ReviewRequestedMergeRequests }},
		{role: RoleAssignee, include: "withAssigned", after: "assignedAfter", page: func(d mineData) *mrPage { return d.CurrentUser.AssignedMergeRequests }},
	}

	var username string
	var nodes []roleNode
	for slices.ContainsFunc(lists, func(l *mineList) bool { return !l.done }) {
		var vars []string
		for _, l := range lists {
			vars = append(vars, "-F", fmt.Sprintf("%s=%t", l.include, !l.done))
			if l.cursor != "" {
				vars = append(vars, "-f", l.after+"="+l.cursor)
			}
		}

		var data mineData
		if err := c.graphql(ctx, mineQuery, vars, &data); err != nil {
			return "", nil, err
		}
		if data.CurrentUser == nil {
			return "", nil, ErrUnauthorized
		}
		username = data.CurrentUser.Username

		for _, l := range lists {
			if l.done {
				continue
			}
			page := l.page(data)
			if page == nil {
				l.done = true
				continue
			}
			for _, n := range page.Nodes {
				nodes = append(nodes, roleNode{role: l.role, node: n})
			}
			l.cursor = page.PageInfo.EndCursor
			l.done = !page.PageInfo.HasNextPage
		}
	}

	var mrs []MergeRequest
	index := map[string]int{}
	for _, rn := range nodes {
		if i, ok := index[rn.node.WebURL]; ok {
			if !mrs[i].HasRole(rn.role) {
				mrs[i].Roles = append(mrs[i].Roles, rn.role)
			}
			continue
		}
		mr := convert(rn.node, username)
		mr.Roles = []Role{rn.role}
		index[mr.WebURL] = len(mrs)
		mrs = append(mrs, mr)
	}
	return username, mrs, nil
}

// FetchBranches returns opened MRs whose source branch is one of the given
// branches, keyed by target project full path.
func (c *Client) FetchBranches(ctx context.Context, username string, branches map[string][]string) ([]MergeRequest, error) {
	var mrs []MergeRequest
	for projects := range slices.Chunk(slices.Sorted(maps.Keys(branches)), projectsPerBranchQuery) {
		chunk, err := c.fetchBranchChunk(ctx, username, projects, branches)
		if err != nil {
			return nil, err
		}
		mrs = append(mrs, chunk...)
	}
	return mrs, nil
}

func (c *Client) fetchBranchChunk(ctx context.Context, username string, projects []string, branches map[string][]string) ([]MergeRequest, error) {
	var q strings.Builder
	q.WriteString("query {\n")
	for i, project := range projects {
		// JSON string and array literals are valid GraphQL literals.
		p, _ := json.Marshal(project)
		b, _ := json.Marshal(branches[project])
		fmt.Fprintf(&q, "  p%d: project(fullPath: %s) { mergeRequests(sourceBranches: %s, state: opened, first: %d) { nodes { ...mr } } }\n", i, p, b, mrsPerBranchQuery)
	}
	q.WriteString("}\n")
	q.WriteString(mrFragment)

	var data map[string]*struct {
		MergeRequests struct {
			Nodes []mrNode `json:"nodes"`
		} `json:"mergeRequests"`
	}
	if err := c.graphql(ctx, q.String(), nil, &data); err != nil {
		return nil, err
	}

	var mrs []MergeRequest
	for i := range projects {
		project := data[fmt.Sprintf("p%d", i)]
		if project == nil {
			continue
		}
		for _, n := range project.MergeRequests.Nodes {
			mrs = append(mrs, convert(n, username))
		}
	}
	return mrs, nil
}

type mineList struct {
	role    Role
	include string
	after   string
	page    func(mineData) *mrPage
	cursor  string
	done    bool
}

type roleNode struct {
	role Role
	node mrNode
}

func (c *Client) graphql(ctx context.Context, query string, vars []string, out any) error {
	args := append([]string{"api", "graphql", "--hostname", c.host, "-f", "query=" + query}, vars...)
	stdout, err := run.Output(ctx, c.glab, args...)
	if err != nil {
		return classify(err)
	}

	var resp struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(stdout, &resp); err != nil {
		return fmt.Errorf("parse graphql response: %w", err)
	}
	if len(resp.Errors) > 0 {
		msgs := make([]string, len(resp.Errors))
		for i, e := range resp.Errors {
			msgs[i] = e.Message
		}
		return fmt.Errorf("graphql: %s", strings.Join(msgs, "; "))
	}
	return json.Unmarshal(resp.Data, out)
}

func classify(err error) error {
	if errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w: %v", ErrGlabMissing, err)
	}
	var runErr *run.Error
	if errors.As(err, &runErr) && unauthorized.MatchString(runErr.Stdout+runErr.Stderr) {
		return fmt.Errorf("%w: %v", ErrUnauthorized, err)
	}
	return err
}

func convert(n mrNode, username string) MergeRequest {
	iid, _ := strconv.Atoi(n.IID)
	updated, _ := time.Parse(time.RFC3339, n.UpdatedAt)
	mr := MergeRequest{
		IID:               iid,
		Title:             n.Title,
		WebURL:            n.WebURL,
		Project:           n.Project.FullPath,
		SourceProject:     n.Project.FullPath,
		SourceBranch:      n.SourceBranch,
		TargetBranch:      n.TargetBranch,
		HeadSHA:           n.DiffHeadSha,
		Draft:             n.Draft,
		UpdatedAt:         updated,
		MergeStatus:       n.DetailedMergeStatus,
		Approved:          n.Approved,
		ThreadsTotal:      n.ResolvableDiscussionsCount,
		ThreadsUnresolved: max(n.ResolvableDiscussionsCount-n.ResolvedDiscussionsCount, 0),
		Notes:             n.UserNotesCount,
	}
	if n.SourceProject != nil {
		mr.SourceProject = n.SourceProject.FullPath
	}
	if n.Author != nil {
		mr.Author = n.Author.Username
	}
	if n.HeadPipeline != nil {
		mr.Pipeline = n.HeadPipeline.Status
	}
	if n.ApprovalsLeft != nil {
		mr.ApprovalsLeft = *n.ApprovalsLeft
	}
	for _, r := range n.Reviewers.Nodes {
		rev := Reviewer{Username: r.Username}
		if r.MergeRequestInteraction != nil {
			rev.State = r.MergeRequestInteraction.ReviewState
			rev.Approved = r.MergeRequestInteraction.Approved
		}
		mr.Reviewers = append(mr.Reviewers, rev)
		if r.Username == username {
			mr.MyReviewState = rev.State
		}
	}
	return mr
}
