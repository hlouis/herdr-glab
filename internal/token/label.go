// Package token renders and reports the workspace MR token. See doc/design.md §9.
package token

import (
	"fmt"
	"strings"

	"github.com/hlouis/herdr-glab/internal/gitlab"
)

// Label renders `!<iid>[ draft][ <merge status>][ <pipeline>][ ✎<unresolved>]`.
func Label(m gitlab.MergeRequest) string {
	parts := []string{fmt.Sprintf("!%d", m.IID)}
	if m.Draft {
		parts = append(parts, "draft")
	}
	if s := MergeStatusText(m.MergeStatus); s != "" {
		parts = append(parts, s)
	}
	if s := PipelineSymbol(m.Pipeline); s != "" {
		parts = append(parts, s)
	}
	if m.ThreadsUnresolved > 0 {
		parts = append(parts, fmt.Sprintf("✎%d", m.ThreadsUnresolved))
	}
	return strings.Join(parts, " ")
}

// MergeStatusText shows only merge states that need action from the author.
func MergeStatusText(status string) string {
	switch status {
	case "NEED_REBASE":
		return "rebase"
	case "CONFLICT":
		return "conflict"
	}
	return ""
}

func PipelineSymbol(status string) string {
	switch status {
	case "SUCCESS":
		return "✔"
	case "FAILED":
		return "✖"
	case "RUNNING":
		return "↻"
	case "PENDING", "CREATED", "WAITING_FOR_RESOURCE", "PREPARING", "SCHEDULED":
		return "⋯"
	case "CANCELED", "SKIPPED":
		return "⊘"
	case "MANUAL":
		return "⚙"
	}
	return ""
}
