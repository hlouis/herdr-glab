package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hlouis/herdr-glab/internal/gitlab"
	"github.com/hlouis/herdr-glab/internal/token"
)

// The drawer keeps the merge request list on the left and the threads of the
// selected one on the right, in the same pane: herdr restores focus and zoom to
// whatever was under an overlay, so a pane must never open another pane.
const (
	focusList = iota
	focusThreads
)

const (
	drawerListWidth = 46
	drawerMaxWidth  = 170
	drawerSeparator = " │ "
	drawerHelp      = "h/l switch side · j/k move · enter expand · R resolve · esc close drawer · q quit"
)

// openDrawer shows the threads of one merge request beside the list.
func (m model) openDrawer(mr gitlab.MergeRequest) (model, tea.Cmd) {
	d := newThreadsModel(m.ctx, m.deps, mr.Project, mr.IID)
	d.mr = mr
	m.drawer, m.focus, m.status = &d, focusThreads, ""
	return m, d.fetch()
}

func (m model) closeDrawer() model {
	m.drawer, m.focus = nil, focusList
	return m
}

func (m model) drawerRender() string {
	width := min(max(m.width, minWidth), drawerMaxWidth)
	listWidth := min(drawerListWidth, width/2)
	threadWidth := width - listWidth - lipgloss.Width(drawerSeparator)

	head := []string{m.header(width)}
	if m.cache.Error != "" {
		head = append(head, errorStyle.Render(fit(m.cache.Error, width)))
	}
	head = append(head, "")

	height := max(m.height-len(head)-1, 3)
	listLines, spans := m.compactBody(listWidth)
	listLines = window(listLines, spans, m.cursor, height)
	threadLines := append(
		[]string{m.drawer.columnHeader(threadWidth), ""},
		m.drawer.columnLines(threadWidth, max(height-2, 1))...)

	rows := make([]string, height)
	separator := dimStyle.Render(drawerSeparator)
	for i := range rows {
		rows[i] = fit(at(listLines, i), listWidth) + separator + fit(at(threadLines, i), threadWidth)
	}

	footer := dimStyle.Render(fit(drawerHelp, width))
	if status := m.drawerStatus(); status != "" {
		footer = fit(status, width)
	}
	return strings.Join(append(append(head, rows...), footer), "\n")
}

// drawerStatus prefers whichever side last had something to say.
func (m model) drawerStatus() string {
	if m.status != "" {
		return m.status
	}
	if m.drawer != nil {
		return m.drawer.status
	}
	return ""
}

// compactBody is the list at one line per merge request, to leave room for the
// threads beside it.
func (m model) compactBody(width int) ([]string, []span) {
	spans := make([]span, len(m.rows))
	if len(m.rows) == 0 {
		return []string{dimStyle.Render(fit("no merge requests", width))}, spans
	}

	var lines []string
	for gi, g := range m.groups {
		if gi > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, groupStyle.Render(fit(fmt.Sprintf("%s (%d)", g.title, g.count), width)))
		for i := g.start; i < g.start+g.count; i++ {
			mr := m.rows[i]
			prefix := "  "
			if i == m.cursor {
				prefix = "▌ "
			}
			marks := token.PipelineSymbol(mr.Pipeline)
			if mr.ThreadsUnresolved > 0 {
				marks += fmt.Sprintf(" ✎%d", mr.ThreadsUnresolved)
			}
			line := fit(fmt.Sprintf("%s!%d %s %s", prefix, mr.IID, marks, mr.Title), width)
			if i == m.cursor && m.focus == focusList {
				line = selectedStyle.Render(line)
			}
			spans[i] = span{start: len(lines), end: len(lines)}
			lines = append(lines, line)
		}
	}
	return lines, spans
}

// drawerKey routes a key press while the drawer is open.
func (m model) drawerKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key := msg.String(); key {
	case "ctrl+c":
		return m, tea.Quit
	case "q":
		return m, tea.Quit
	case "esc":
		return m.closeDrawer(), nil
	case "left", "h":
		m.focus = focusList
		return m, nil
	case "right", "l":
		m.focus = focusThreads
		return m, nil
	case "j", "down", "k", "up":
		if m.focus == focusList {
			// Moving the selection reloads the drawer for the new merge request.
			if key == "j" || key == "down" {
				m.cursor = min(m.cursor+1, max(len(m.rows)-1, 0))
			} else {
				m.cursor = max(m.cursor-1, 0)
			}
			if mr, ok := m.selected(); ok {
				return m.openDrawerKeepingFocus(mr)
			}
			return m, nil
		}
	}

	if m.focus == focusThreads {
		d, cmd := m.drawer.update(msg)
		m.drawer = &d
		return m, cmd
	}
	return m, nil
}

// openDrawerKeepingFocus reloads the drawer without stealing focus from the list.
func (m model) openDrawerKeepingFocus(mr gitlab.MergeRequest) (tea.Model, tea.Cmd) {
	next, cmd := m.openDrawer(mr)
	next.focus = focusList
	return next, cmd
}

func at(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}
	return ""
}
