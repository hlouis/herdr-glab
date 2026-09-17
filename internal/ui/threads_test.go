package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestWrapBody(t *testing.T) {
	const width = 40
	tests := []struct {
		name string
		body string
	}{
		{"CJK without spaces", "我希望将配置文件分开，将这个工具需要的配置，以及自己的OSS配置独立到这个cmd目录中，不要和主程序的配置混合在一起。"},
		{"English prose", strings.Repeat("the pipeline never runs for this path ", 4)},
		{"mixed with its own breaks", "第一行\n\nsecond paragraph with a very long line that has to be broken somewhere sensible"},
		{"long unbroken token", strings.Repeat("x", 120)},
	}
	for _, tt := range tests {
		lines := wrapBody(tt.body, width)
		if len(lines) == 0 {
			t.Errorf("%s: wrapBody returned nothing", tt.name)
			continue
		}
		for i, line := range lines {
			if w := lipgloss.Width(line); w > width {
				t.Errorf("%s: line %d is %d cells wide, over the %d limit: %q", tt.name, i, w, width, line)
			}
		}
	}
}

func TestWrapBodyKeepsBlankLines(t *testing.T) {
	lines := wrapBody("one\n\ntwo", 40)
	if len(lines) != 3 || lines[1] != "" {
		t.Errorf("wrapBody() = %q, want the blank line kept between paragraphs", lines)
	}
}
