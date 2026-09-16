// Package status renders the one-line summary herdr shows in the tab bar
// status area. See doc/design.md §9.
package status

import (
	"fmt"

	"github.com/hlouis/herdr-glab/internal/cache"
)

// Line is `MR <total>` plus `· <n> todo` when some of them wait on the user,
// and a trailing ⚠ when the last fetch failed. An empty cache renders `MR –`.
func Line(c cache.Cache) string {
	if c.FetchedAt.IsZero() && c.Error == "" {
		return "MR –"
	}

	needs := 0
	for _, mr := range c.Mine {
		if mr.NeedsMe() {
			needs++
		}
	}
	line := fmt.Sprintf("MR %d", len(c.Mine))
	if needs > 0 {
		line += fmt.Sprintf(" · %d todo", needs)
	}
	if c.Error != "" {
		line += " ⚠"
	}
	return line
}
