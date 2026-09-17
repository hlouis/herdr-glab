package gitlab

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/hlouis/herdr-glab/internal/run"
)

const discussionsQuery = `
query($project: ID!, $iid: String!, $after: String) {
  project(fullPath: $project) {
    mergeRequest(iid: $iid) {
      discussions(first: 20, after: $after) {
        pageInfo { hasNextPage endCursor }
        nodes {
          id resolved resolvable
          notes(first: 20) {
            nodes {
              system createdAt body
              author { username }
              position { filePath oldLine newLine }
            }
          }
        }
      }
    }
  }
}`

// Note is one comment inside a discussion.
type Note struct {
	Author    string
	Body      string
	CreatedAt time.Time
}

// Discussion is one resolvable review thread on a merge request.
type Discussion struct {
	ID       string
	Resolved bool
	FilePath string
	Line     int
	Notes    []Note
}

// ShortID is the hex id glab accepts as a --reply or resolve target.
func (d Discussion) ShortID() string {
	if i := strings.LastIndex(d.ID, "/"); i >= 0 {
		return d.ID[i+1:]
	}
	return d.ID
}

// Where names the file and line the thread hangs off, empty for an overall thread.
func (d Discussion) Where() string {
	if d.FilePath == "" {
		return ""
	}
	if d.Line == 0 {
		return d.FilePath
	}
	return d.FilePath + ":" + strconv.Itoa(d.Line)
}

// Summary is the first line of the opening comment.
func (d Discussion) Summary() string {
	if len(d.Notes) == 0 {
		return ""
	}
	line, _, _ := strings.Cut(strings.TrimSpace(d.Notes[0].Body), "\n")
	return line
}

type discussionNode struct {
	ID         string `json:"id"`
	Resolved   bool   `json:"resolved"`
	Resolvable bool   `json:"resolvable"`
	Notes      struct {
		Nodes []struct {
			System    bool   `json:"system"`
			CreatedAt string `json:"createdAt"`
			Body      string `json:"body"`
			Author    *struct {
				Username string `json:"username"`
			} `json:"author"`
			Position *struct {
				FilePath string `json:"filePath"`
				OldLine  *int   `json:"oldLine"`
				NewLine  *int   `json:"newLine"`
			} `json:"position"`
		} `json:"nodes"`
	} `json:"notes"`
}

// FetchDiscussions returns the review threads of one merge request. System
// notes ("requested review from …") and non-resolvable comments are dropped:
// only threads a human has to act on are interesting here.
func (c *Client) FetchDiscussions(ctx context.Context, project string, iid int) ([]Discussion, error) {
	var out []Discussion
	cursor := ""
	for {
		vars := []string{"-f", "project=" + project, "-f", "iid=" + strconv.Itoa(iid)}
		if cursor != "" {
			vars = append(vars, "-f", "after="+cursor)
		}
		var data struct {
			Project *struct {
				MergeRequest *struct {
					Discussions struct {
						PageInfo pageInfo         `json:"pageInfo"`
						Nodes    []discussionNode `json:"nodes"`
					} `json:"discussions"`
				} `json:"mergeRequest"`
			} `json:"project"`
		}
		if err := c.graphql(ctx, discussionsQuery, vars, &data); err != nil {
			return nil, err
		}
		if data.Project == nil || data.Project.MergeRequest == nil {
			return nil, nil
		}

		page := data.Project.MergeRequest.Discussions
		for _, n := range page.Nodes {
			if !n.Resolvable {
				continue
			}
			d := Discussion{ID: n.ID, Resolved: n.Resolved}
			for _, note := range n.Notes.Nodes {
				if note.System {
					continue
				}
				created, _ := time.Parse(time.RFC3339, note.CreatedAt)
				author := ""
				if note.Author != nil {
					author = note.Author.Username
				}
				if d.FilePath == "" && note.Position != nil {
					d.FilePath = note.Position.FilePath
					switch {
					case note.Position.NewLine != nil:
						d.Line = *note.Position.NewLine
					case note.Position.OldLine != nil:
						d.Line = *note.Position.OldLine
					}
				}
				d.Notes = append(d.Notes, Note{Author: author, Body: note.Body, CreatedAt: created})
			}
			if len(d.Notes) > 0 {
				out = append(out, d)
			}
		}
		if !page.PageInfo.HasNextPage {
			return out, nil
		}
		cursor = page.PageInfo.EndCursor
	}
}

// ResolveDiscussion marks a thread resolved, or reopens it.
func (c *Client) ResolveDiscussion(ctx context.Context, project string, iid int, shortID string, resolved bool) error {
	verb := "resolve"
	if !resolved {
		verb = "reopen"
	}
	_, err := run.OutputEnv(ctx, []string{"GITLAB_HOST=" + c.host}, c.glab,
		"mr", "note", verb, shortID, strconv.Itoa(iid), "--repo", project)
	return classify(err)
}
