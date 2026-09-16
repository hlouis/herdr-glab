// Command herdr-glab is the single entry point for every herdr-glab plugin
// command. Subcommands are listed in doc/design.md §12.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/hlouis/herdr-glab/internal/config"
	"github.com/hlouis/herdr-glab/internal/herdr"
	"github.com/hlouis/herdr-glab/internal/plugin"
	"github.com/hlouis/herdr-glab/internal/poller"
	"github.com/hlouis/herdr-glab/internal/refresh"
	"github.com/hlouis/herdr-glab/internal/ui"
)

const usage = "usage: herdr-glab <ensure|poller|tokens|refresh|stop|panel-open|panel>"

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
	case "poller", "tokens", "refresh", "panel":
	default:
		return fmt.Errorf("unknown command; %s", usage)
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
	default:
		if err := poller.Ensure(env); err != nil {
			return err
		}
		return ui.Run(ctx, deps)
	}
}
