package ui

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/hlouis/herdr-glab/internal/action"
	"github.com/hlouis/herdr-glab/internal/cache"
	"github.com/hlouis/herdr-glab/internal/gitlab"
	"github.com/hlouis/herdr-glab/internal/refresh"
	"github.com/hlouis/herdr-glab/internal/repo"
)

type (
	threadsLoadedMsg struct {
		threads []gitlab.Discussion
		err     error
	}
	threadResolvedMsg struct{ err error }
)

// threadsModel is the panel's drawer: the review threads of one merge request,
// which it also hands to the agent working on that branch. It is never a pane
// of its own — see the note in drawer.go.
type threadsModel struct {
	ctx    context.Context
	deps   refresh.Deps
	runner action.Runner

	project string
	iid     int
	mr      gitlab.MergeRequest

	threads  []gitlab.Discussion
	loaded   bool
	cursor   int
	expanded map[string]bool
	picked   map[string]bool

	cache       cache.Cache
	repos       []repo.WorkspaceRepo
	reposLoaded bool

	status string
	busy   bool
}

func newThreadsModel(ctx context.Context, deps refresh.Deps, project string, iid int) threadsModel {
	return threadsModel{
		ctx: ctx, deps: deps, runner: action.Runner{Deps: deps},
		project: project, iid: iid,
		expanded: map[string]bool{}, picked: map[string]bool{},
		status: "loading threads…",
	}
}

// update runs inside the panel's loop: the panel routes the drawer's own
// messages and, while the drawer has focus, its keys.
func (m threadsModel) update(msg tea.Msg) (threadsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case threadsLoadedMsg:
		m.busy = false
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		threads := msg.threads
		slices.SortStableFunc(threads, func(a, b gitlab.Discussion) int {
			return cmp.Compare(resolvedRank(a), resolvedRank(b))
		})
		m.threads, m.loaded, m.status = threads, true, ""
		m.cursor = min(m.cursor, max(len(m.threads)-1, 0))
	case threadResolvedMsg:
		m.busy = false
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.status = "updated"
		return m, m.fetch()
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// handleKey runs for the keys the drawer does not handle itself.
func (m threadsModel) handleKey(msg tea.KeyPressMsg) (threadsModel, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		m.cursor = min(m.cursor+1, max(len(m.threads)-1, 0))
	case "k", "up":
		m.cursor = max(m.cursor-1, 0)
	case "enter":
		if t, ok := m.current(); ok {
			m.expanded[t.ID] = !m.expanded[t.ID]
		}
	case " ", "space":
		if t, ok := m.current(); ok {
			if m.picked[t.ID] {
				delete(m.picked, t.ID)
			} else {
				m.picked[t.ID] = true
			}
		}
	case "a":
		if m.busy {
			return m, nil
		}
		if !m.reposLoaded {
			m.status = "still scanning workspaces…"
			return m, nil
		}
		m.busy, m.status = true, "sending to the agent…"
		return m, m.send(m.selection())
	case "R", "shift+r":
		if t, ok := m.current(); ok && !m.busy {
			m.busy, m.status = true, "updating thread…"
			return m, m.resolve(t)
		}
	case "o", "b", "y":
		// The merge request keys mean the same thing as in the panel.
		if m.mr.WebURL == "" {
			return m, nil
		}
		url, ctx := m.mr.WebURL, m.ctx
		if msg.String() == "y" {
			return m, func() tea.Msg {
				return actionMsg{err: action.Copy(ctx, url), note: "copied link"}
			}
		}
		return m, func() tea.Msg {
			return actionMsg{err: action.OpenBrowser(ctx, url), note: "opened in browser"}
		}
	}
	return m, nil
}

func (m threadsModel) current() (gitlab.Discussion, bool) {
	if m.cursor < 0 || m.cursor >= len(m.threads) {
		return gitlab.Discussion{}, false
	}
	return m.threads[m.cursor], true
}

// selection is every picked thread, or the one under the cursor.
func (m threadsModel) selection() []gitlab.Discussion {
	var picked []gitlab.Discussion
	for _, t := range m.threads {
		if m.picked[t.ID] {
			picked = append(picked, t)
		}
	}
	if len(picked) > 0 {
		return picked
	}
	if t, ok := m.current(); ok {
		return []gitlab.Discussion{t}
	}
	return nil
}

func (m threadsModel) fetch() tea.Cmd {
	ctx, deps, project, iid := m.ctx, m.deps, m.project, m.iid
	return func() tea.Msg {
		threads, err := deps.GitLab.FetchDiscussions(ctx, project, iid)
		return threadsLoadedMsg{threads: threads, err: err}
	}
}

func (m threadsModel) resolve(t gitlab.Discussion) tea.Cmd {
	ctx, deps, project, iid := m.ctx, m.deps, m.project, m.iid
	id, resolved := t.ShortID(), !t.Resolved
	return func() tea.Msg {
		return threadResolvedMsg{err: deps.GitLab.ResolveDiscussion(ctx, project, iid, id, resolved)}
	}
}

func (m threadsModel) send(threads []gitlab.Discussion) tea.Cmd {
	ctx, runner, c, repos, mr := m.ctx, m.runner, m.cache, m.repos, m.mr
	return func() tea.Msg {
		label, err := runner.SendThreadsToAgent(ctx, c, repos, mr, threads)
		if err != nil {
			return actionMsg{err: err}
		}
		return actionMsg{note: fmt.Sprintf("sent %d thread(s) to the agent in %s", len(threads), label)}
	}
}

// columnHeader names the merge request the drawer is showing, since the list
// beside it only carries the cursor.
func (m threadsModel) columnHeader(width int) string {
	summary := "loading…"
	if m.loaded {
		unresolved := m.count(false)
		switch {
		case len(m.threads) == 0:
			summary = "no threads"
		case unresolved == 0:
			summary = fmt.Sprintf("%d threads, all resolved", len(m.threads))
		default:
			summary = fmt.Sprintf("%d threads, %d unresolved", len(m.threads), unresolved)
		}
	}
	return fit(boldStyle.Render(fmt.Sprintf("%s !%d", m.project, m.iid))+dimStyle.Render("  "+summary), width)
}

// columnLines renders the threads for the panel's drawer: the body only, since
// the panel draws the header and the help line.
func (m threadsModel) columnLines(width, height int) []string {
	var body []string
	var spans []span
	switch {
	case !m.loaded:
		body = []string{dimStyle.Render(fit("loading…", width))}
	case len(m.threads) == 0:
		body = []string{dimStyle.Render(fit("no review threads", width))}
	default:
		body, spans = m.body(width)
	}
	return window(body, spans, m.cursor, height)
}

// body groups the threads: the ones still open first, then the resolved ones.
func (m threadsModel) body(width int) ([]string, []span) {
	spans := make([]span, len(m.threads))
	var lines []string
	group := ""
	for i, t := range m.threads {
		name := "Unresolved"
		if t.Resolved {
			name = "Resolved"
		}
		if name != group {
			if group != "" {
				lines = append(lines, "")
			}
			lines = append(lines,
				groupStyle.Render(fmt.Sprintf("%s (%d)", name, m.count(t.Resolved))),
				dimStyle.Render(strings.Repeat("─", width)))
			group = name
		}
		start := len(lines)
		lines = append(lines, m.threadLines(t, i == m.cursor, width)...)
		spans[i] = span{start: start, end: len(lines) - 1}
	}
	return lines, spans
}

func (m threadsModel) count(resolved bool) int {
	n := 0
	for _, t := range m.threads {
		if t.Resolved == resolved {
			n++
		}
	}
	return n
}

// resolvedRank sorts the threads that still need work to the top.
func resolvedRank(d gitlab.Discussion) int {
	if d.Resolved {
		return 1
	}
	return 0
}

// wrapBody wraps a comment to the pane width, keeping its own line breaks.
// ansi.Wrap, not Wordwrap: CJK text has no spaces to break on.
func wrapBody(body string, width int) []string {
	var out []string
	for _, line := range strings.Split(strings.TrimRight(body, "\n"), "\n") {
		out = append(out, strings.Split(ansi.Wrap(strings.TrimRight(line, " \t"), width, ""), "\n")...)
	}
	return out
}

func (m threadsModel) threadLines(t gitlab.Discussion, selected bool, width int) []string {
	mark := "·"
	if t.Resolved {
		mark = "✔"
	}
	pick := " "
	if m.picked[t.ID] {
		pick = "✓"
	}
	where := t.Where()
	if where == "" {
		where = "overall"
	}
	author := ""
	if len(t.Notes) > 0 {
		author = t.Notes[0].Author
	}
	replies := ""
	if n := len(t.Notes); n > 1 {
		replies = fmt.Sprintf("  +%d", n-1)
	}

	head := fmt.Sprintf("%s %s %s  %s  %s%s", pick, mark, where, author, t.Summary(), replies)
	if selected {
		head = "▌" + head
	} else {
		head = " " + head
	}
	out := []string{fit(head, width)}
	if !selected {
		out[0] = dimResolved(t, out[0])
	} else {
		out[0] = selectedStyle.Render(out[0])
	}

	if m.expanded[t.ID] {
		for _, note := range t.Notes {
			out = append(out, dimStyle.Render(fit("    "+note.Author+":", width)))
			for _, line := range wrapBody(note.Body, width-4) {
				out = append(out, fit("    "+line, width))
			}
		}
		out = append(out, "")
	}
	return out
}

func dimResolved(t gitlab.Discussion, line string) string {
	if t.Resolved {
		return dimStyle.Render(line)
	}
	return line
}
