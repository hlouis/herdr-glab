package token

import (
	"testing"

	"github.com/hlouis/herdr-glab/internal/gitlab"
)

func TestLabel(t *testing.T) {
	tests := []struct {
		name string
		mr   gitlab.MergeRequest
		want string
	}{
		{"open with threads", gitlab.MergeRequest{IID: 412, Pipeline: "SUCCESS", ThreadsUnresolved: 1}, "!412 ✔ ✎1"},
		{"draft running", gitlab.MergeRequest{IID: 67, Draft: true, Pipeline: "RUNNING"}, "!67 draft ↻"},
		{"needs rebase", gitlab.MergeRequest{IID: 318, MergeStatus: "NEED_REBASE", Pipeline: "SUCCESS"}, "!318 rebase ✔"},
		{"no pipeline", gitlab.MergeRequest{IID: 1, MergeStatus: "MERGEABLE"}, "!1"},
	}
	for _, tt := range tests {
		if got := Label(tt.mr); got != tt.want {
			t.Errorf("%s: Label() = %q, want %q", tt.name, got, tt.want)
		}
	}
}
