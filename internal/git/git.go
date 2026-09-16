// Package git wraps the git commands the plugin needs.
package git

import (
	"context"
	"strings"

	"github.com/hlouis/herdr-glab/internal/run"
)

// Output runs git -C dir with args and returns trimmed stdout.
func Output(ctx context.Context, dir string, args ...string) (string, error) {
	out, err := run.Output(ctx, "git", append([]string{"-C", dir}, args...)...)
	return strings.TrimSpace(string(out)), err
}

// Remote is one named git remote with its fetch URL.
type Remote struct {
	Name string
	URL  string
}

// Remotes lists the fetch URL of every remote in dir.
func Remotes(ctx context.Context, dir string) ([]Remote, error) {
	out, err := Output(ctx, dir, "remote", "-v")
	if err != nil {
		return nil, err
	}
	var remotes []Remote
	for line := range strings.Lines(out) {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[2] == "(fetch)" {
			remotes = append(remotes, Remote{Name: fields[0], URL: fields[1]})
		}
	}
	return remotes, nil
}
