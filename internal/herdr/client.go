// Package herdr wraps the herdr CLI. Command shapes: doc/herdr/cli-reference.md.
package herdr

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/hlouis/herdr-glab/internal/run"
)

type Client struct {
	bin string
}

func New(bin string) *Client {
	return &Client{bin: bin}
}

type Worktree struct {
	CheckoutPath     string `json:"checkout_path"`
	RepoRoot         string `json:"repo_root"`
	RepoName         string `json:"repo_name"`
	IsLinkedWorktree bool   `json:"is_linked_worktree"`
}

type Workspace struct {
	ID       string    `json:"workspace_id"`
	Label    string    `json:"label"`
	Focused  bool      `json:"focused"`
	Worktree *Worktree `json:"worktree"`
}

type Pane struct {
	ID    string `json:"pane_id"`
	TabID string `json:"tab_id"`
	Cwd   string `json:"cwd"`
}

type WorktreeEntry struct {
	Branch          string `json:"branch"`
	Path            string `json:"path"`
	OpenWorkspaceID string `json:"open_workspace_id"`
}

func (c *Client) Workspaces(ctx context.Context) ([]Workspace, error) {
	var out struct {
		Workspaces []Workspace `json:"workspaces"`
	}
	err := c.call(ctx, &out, "workspace", "list")
	return out.Workspaces, err
}

func (c *Client) Panes(ctx context.Context, workspaceID string) ([]Pane, error) {
	var out struct {
		Panes []Pane `json:"panes"`
	}
	err := c.call(ctx, &out, "pane", "list", "--workspace", workspaceID)
	return out.Panes, err
}

func (c *Client) FocusWorkspace(ctx context.Context, workspaceID string) error {
	return c.call(ctx, nil, "workspace", "focus", workspaceID)
}

// ReportToken sets one workspace token that expires after ttl.
func (c *Client) ReportToken(ctx context.Context, workspaceID, source, name, value string, ttl time.Duration) error {
	return c.call(ctx, nil, "workspace", "report-metadata", workspaceID,
		"--source", source,
		"--token", name+"="+value,
		"--ttl-ms", strconv.FormatInt(ttl.Milliseconds(), 10))
}

func (c *Client) ClearToken(ctx context.Context, workspaceID, source, name string) error {
	return c.call(ctx, nil, "workspace", "report-metadata", workspaceID, "--source", source, "--clear-token", name)
}

func (c *Client) WorktreeList(ctx context.Context, cwd string) ([]WorktreeEntry, error) {
	var out struct {
		Worktrees []WorktreeEntry `json:"worktrees"`
	}
	err := c.call(ctx, &out, "worktree", "list", "--cwd", cwd)
	return out.Worktrees, err
}

// WorktreeCreate checks out branch in a new worktree workspace; base is used
// only when the local branch does not exist yet.
func (c *Client) WorktreeCreate(ctx context.Context, cwd, branch, base, label string) error {
	args := []string{"worktree", "create", "--cwd", cwd, "--branch", branch, "--label", label, "--focus"}
	if base != "" {
		args = append(args, "--base", base)
	}
	return c.call(ctx, nil, args...)
}

func (c *Client) WorktreeOpen(ctx context.Context, cwd, path, label string) error {
	return c.call(ctx, nil, "worktree", "open", "--cwd", cwd, "--path", path, "--label", label, "--focus")
}

// CreateTab opens a focused tab and returns its root pane id.
func (c *Client) CreateTab(ctx context.Context, workspaceID, cwd, label string) (string, error) {
	var out struct {
		RootPane Pane `json:"root_pane"`
	}
	if err := c.call(ctx, &out, "tab", "create", "--workspace", workspaceID, "--cwd", cwd, "--label", label, "--focus"); err != nil {
		return "", err
	}
	return out.RootPane.ID, nil
}

func (c *Client) PaneRun(ctx context.Context, paneID, command string) error {
	return c.call(ctx, nil, "pane", "run", paneID, command)
}

func (c *Client) OpenPluginPane(ctx context.Context, pluginID, entrypoint string) error {
	return c.call(ctx, nil, "plugin", "pane", "open", "--plugin", pluginID, "--entrypoint", entrypoint)
}

// call runs a herdr subcommand and decodes the `result` object of its JSON reply.
func (c *Client) call(ctx context.Context, out any, args ...string) error {
	stdout, err := run.Output(ctx, c.bin, args...)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	var reply struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(stdout, &reply); err != nil {
		return fmt.Errorf("parse herdr %s %s output: %w", args[0], args[1], err)
	}
	return json.Unmarshal(reply.Result, out)
}

// Agent is one pane herdr has detected a coding agent in.
type Agent struct {
	PaneID      string `json:"pane_id"`
	WorkspaceID string `json:"workspace_id"`
	Agent       string `json:"agent"`
}

func (c *Client) Agents(ctx context.Context) ([]Agent, error) {
	var out struct {
		Agents []Agent `json:"agents"`
	}
	err := c.call(ctx, &out, "agent", "list")
	return out.Agents, err
}

// AgentPrompt submits text to the agent in a pane, without waiting for it.
func (c *Client) AgentPrompt(ctx context.Context, target, text string) error {
	return c.call(ctx, nil, "agent", "prompt", target, text)
}
