// Command herdr-glab is the single entry point for every herdr-glab plugin
// command. Subcommands are listed in doc/design.md §12.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"github.com/hlouis/herdr-glab/internal/cache"
	"github.com/hlouis/herdr-glab/internal/config"
	"github.com/hlouis/herdr-glab/internal/gitlab"
	"github.com/hlouis/herdr-glab/internal/herdr"
	"github.com/hlouis/herdr-glab/internal/plugin"
	"github.com/hlouis/herdr-glab/internal/poller"
	"github.com/hlouis/herdr-glab/internal/refresh"
	"github.com/hlouis/herdr-glab/internal/status"
	"github.com/hlouis/herdr-glab/internal/ui"
)

const usage = "usage: herdr-glab <ensure|poller|tokens|refresh|stop|panel-open|panel|detail|threads|link-open|url-open|status>"

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1], plugin.FromOS()); err != nil {
		fmt.Fprintf(os.Stderr, "herdr-glab %s: %v\n", os.Args[1], err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cmd string, env plugin.Env) error {
	switch cmd {
	case "ensure":
		return poller.Ensure(env)
	case "stop":
		return poller.Stop(env)
	case "panel-open":
		return herdr.New(env.HerdrBin).OpenPluginPane(ctx, env.ID, plugin.PanelEntrypoint)
	case "link-open":
		url := os.Getenv("HERDR_PLUGIN_CLICKED_URL")
		if url == "" {
			return errors.New("no clicked URL in the plugin context")
		}
		return openDetail(ctx, env, url)
	case "url-open":
		url, err := urlFromUser(ctx, env)
		if err != nil {
			return err
		}
		return openDetail(ctx, env, url)
	case "poller", "tokens", "refresh", "panel", "detail", "threads", "status":
	default:
		return fmt.Errorf("unknown command; %s", usage)
	}

	if cmd == "status" {
		// Runs from herdr's tab bar every few seconds: cache only, no config,
		// no network, and never an error that would blank the status area.
		c, _ := cache.Load(cache.Path(env.StateDir))
		fmt.Println(status.Line(c))
		return nil
	}

	cfg, err := config.Load(env.ConfigDir)
	if err != nil {
		return err
	}
	deps := refresh.NewDeps(env, cfg)

	switch cmd {
	case "poller":
		log.SetFlags(log.LstdFlags)
		return poller.Run(ctx, deps)
	case "tokens":
		if err := poller.Ensure(env); err != nil {
			return err
		}
		workspaceID := env.InvocationWorkspaceID()
		if workspaceID == "" {
			return nil
		}
		return refresh.Workspace(ctx, deps, workspaceID)
	case "refresh":
		if err := poller.ClearStop(env); err != nil {
			return err
		}
		if err := poller.Ensure(env); err != nil {
			return err
		}
		_, err := refresh.All(ctx, deps)
		return err
	case "threads":
		data, err := os.ReadFile(env.SelectedMRPath())
		if err != nil {
			return fmt.Errorf("read the selected merge request: %w", err)
		}
		var selection struct {
			Project string `json:"project"`
			IID     int    `json:"iid"`
		}
		if err := json.Unmarshal(data, &selection); err != nil {
			return fmt.Errorf("parse the selected merge request: %w", err)
		}
		return ui.RunThreads(ctx, deps, selection.Project, selection.IID)
	case "detail":
		url, err := os.ReadFile(env.ClickedURLPath())
		if err != nil {
			return fmt.Errorf("read clicked URL: %w", err)
		}
		return ui.RunDetail(ctx, deps, strings.TrimSpace(string(url)))
	default:
		if err := poller.Ensure(env); err != nil {
			return err
		}
		return ui.Run(ctx, deps)
	}
}

// openDetail hands the URL to the single-MR pane, which herdr starts without
// the clicked-URL environment the action itself receives.
func openDetail(ctx context.Context, env plugin.Env, url string) error {
	if err := os.MkdirAll(env.StateDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(env.ClickedURLPath(), []byte(url), 0o644); err != nil {
		return err
	}
	return herdr.New(env.HerdrBin).OpenPluginPane(ctx, env.ID, plugin.DetailEntrypoint)
}

// urlFromUser reads the MR URL from the pane selection, then the clipboard.
// It is the keyboard path to the single-MR pane, for terminals where herdr's
// ctrl+click never reaches the plugin.
func urlFromUser(ctx context.Context, env plugin.Env) (string, error) {
	for _, candidate := range []string{env.SelectedText(), clipboard(ctx)} {
		url := strings.TrimSpace(candidate)
		if _, _, _, ok := gitlab.ParseMRURL(url); ok {
			return url, nil
		}
	}
	return "", errors.New("select a merge request URL in a pane, or copy one to the clipboard, then try again")
}

func clipboard(ctx context.Context) string {
	name, args := "pbpaste", []string(nil)
	if runtime.GOOS != "darwin" {
		name = "wl-paste"
		if _, err := exec.LookPath(name); err != nil {
			name, args = "xclip", []string{"-selection", "clipboard", "-o"}
		}
	}
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return ""
	}
	return string(out)
}
