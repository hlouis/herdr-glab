package action

import (
	"strings"
	"testing"

	"github.com/hlouis/herdr-glab/internal/gitlab"
)

func TestBuildThreadPrompt(t *testing.T) {
	mr := gitlab.MergeRequest{IID: 412, Project: "platform/api", SourceBranch: "feat/export-snapshot"}
	threads := []gitlab.Discussion{
		{
			FilePath: "cmd/export/main.go",
			Line:     92,
			Notes: []gitlab.Note{
				{Author: "kim", Body: "blocking: the snapshot uses the latest version index,\nso a regenerated reply exports the wrong body."},
				{Author: "you", Body: "good catch, will fix"},
			},
		},
		{
			Notes: []gitlab.Note{{Author: "sam", Body: "the CI job never runs for cmd/**; it will keep using a stale image."}},
		},
	}

	got := BuildThreadPrompt(mr, threads)
	for _, want := range []string{"!412", "platform/api", "feat/export-snapshot", "cmd/export/main.go:92", "overall comment", "kim wrote:", "thread 2 of 2"} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt is missing %q", want)
		}
	}
	t.Logf("生成的提示词:\n%s", got)
}

func TestBuildThreadPromptTruncatesLongBodies(t *testing.T) {
	long := strings.Repeat("x", bodyLimit+500)
	got := BuildThreadPrompt(gitlab.MergeRequest{IID: 1}, []gitlab.Discussion{{Notes: []gitlab.Note{{Author: "kim", Body: long}}}})
	if !strings.Contains(got, "[truncated]") {
		t.Error("a long body should be truncated")
	}
	if len(got) > bodyLimit+400 {
		t.Errorf("prompt is %d bytes, longer than the body limit plus its frame", len(got))
	}
}
