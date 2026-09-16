// Package ui is the overlay MR panel. See doc/design.md §7.
package ui

import (
	"cmp"
	"context"
	"os"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/hlouis/herdr-glab/internal/action"
	"github.com/hlouis/herdr-glab/internal/cache"
	"github.com/hlouis/herdr-glab/internal/gitlab"
	"github.com/hlouis/herdr-glab/internal/refresh"
	"github.com/hlouis/herdr-glab/internal/repo"
)

const reloadInterval = 5 * time.Second

type filterMode int

const (
	filterAll filterMode = iota
	filterReview
	filterAuthored
)

var filterNames = []string{"all", "review", "mine"}

type (
	tickMsg  time.Time
	reposMsg struct {
		repos []repo.WorkspaceRepo
		err   error
	}
	refreshedMsg struct{ err error }
	actionMsg    struct {
		err  error
		quit bool
		note string
	}
)

type model struct {
	ctx    context.Context
	deps   refresh.Deps
	runner action.Runner

	cache       cache.Cache
	cacheMtime  time.Time
	repos       []repo.WorkspaceRepo
	reposLoaded bool

	rows      []gitlab.MergeRequest
	cursor    int
	filter    filterMode
	search    string
	searching bool

	help   bool
	busy   bool
	status string
	width  int
	height int
}

// Run shows the panel until the user quits or an action moves focus away.
func Run(ctx context.Context, deps refresh.Deps) error {
	m := model{ctx: ctx, deps: deps, runner: action.Runner{Deps: deps}}
	m.reloadCache()
	if m.cache.FetchedAt.IsZero() {
		m.busy = true
		m.status = "fetching merge requests…"
	}
	_, err := tea.NewProgram(m, tea.WithContext(ctx)).Run()
	return err
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.scanRepos(), tick()}
	if m.busy {
		cmds = append(cmds, m.refresh())
	}
	return tea.Batch(cmds...)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tickMsg:
		m.reloadCache()
		return m, tick()
	case reposMsg:
		if msg.err != nil {
			m.status = "scan workspaces: " + msg.err.Error()
			return m, nil
		}
		m.repos, m.reposLoaded = msg.repos, true
	case refreshedMsg:
		m.busy = false
		m.cacheMtime = time.Time{}
		m.reloadCache()
		m.status = "refreshed"
		if msg.err != nil {
			m.status = msg.err.Error()
		}
		return m, m.scanRepos()
	case actionMsg:
		m.busy = false
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		if msg.quit {
			return m, tea.Quit
		}
		m.status = msg.note
	case tea.KeyPressMsg:
		if m.searching {
			return m.handleSearchKey(msg), nil
		}
		return m.handleKey(msg)
	}
	return m, nil
}

func (m model) handleSearchKey(msg tea.KeyPressMsg) model {
	switch msg.String() {
	case "esc":
		m.search, m.searching = "", false
	case "enter":
		m.searching = false
	case "backspace":
		runes := []rune(m.search)
		if len(runes) > 0 {
			m.search = string(runes[:len(runes)-1])
		}
	default:
		m.search += msg.Key().Text
	}
	m.applyRows()
	return m
}

func (m model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if m.help {
		if key == "ctrl+c" || key == "q" {
			return m, tea.Quit
		}
		m.help = false
		return m, nil
	}

	switch key {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "?":
		m.help = true
	case "esc":
		if m.search == "" {
			return m, tea.Quit
		}
		m.search = ""
		m.applyRows()
	case "j", "down":
		m.cursor = min(m.cursor+1, max(len(m.rows)-1, 0))
	case "k", "up":
		m.cursor = max(m.cursor-1, 0)
	case "tab":
		m.filter = (m.filter + 1) % filterMode(len(filterNames))
		m.applyRows()
	case "/":
		m.searching = true
	case "R", "shift+r":
		if !m.busy {
			m.busy, m.status = true, "refreshing…"
			return m, m.refresh()
		}
	case "enter", "c", "r", "o", "y":
		if mr, ok := m.selected(); ok && !m.busy {
			return m.runAction(key, mr)
		}
	}
	return m, nil
}

func (m model) runAction(key string, mr gitlab.MergeRequest) (tea.Model, tea.Cmd) {
	ctx, runner, c, repos := m.ctx, m.runner, m.cache, m.repos
	var do func() error
	quit, note := true, ""
	switch key {
	case "enter":
		do = func() error { return runner.Focus(ctx, c, repos, mr) }
		m.status = "switching workspace…"
	case "c":
		do = func() error { return runner.Checkout(ctx, repos, mr) }
		m.status = "checking out…"
	case "r":
		do = func() error { return runner.Review(ctx, repos, mr) }
		m.status = "opening tuicr…"
	case "o":
		do = func() error { return action.OpenBrowser(ctx, mr.WebURL) }
		quit, note = false, "opened in browser"
	case "y":
		do = func() error { return action.Copy(ctx, mr.WebURL) }
		quit, note = false, "copied link"
	}
	if quit && !m.reposLoaded {
		m.status = "still scanning workspaces…"
		return m, nil
	}
	m.busy = true
	return m, func() tea.Msg { return actionMsg{err: do(), quit: quit, note: note} }
}

func (m model) scanRepos() tea.Cmd {
	ctx, deps := m.ctx, m.deps
	return func() tea.Msg {
		repos, err := repo.Scan(ctx, deps.Herdr, deps.Config.Host, "")
		return reposMsg{repos: repos, err: err}
	}
}

func (m model) refresh() tea.Cmd {
	ctx, deps := m.ctx, m.deps
	return func() tea.Msg {
		_, err := refresh.All(ctx, deps)
		return refreshedMsg{err: err}
	}
}

func tick() tea.Cmd {
	return tea.Tick(reloadInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// reloadCache reads the cache when its file changed since the last read.
func (m *model) reloadCache() {
	info, err := os.Stat(m.deps.CachePath())
	if err != nil || info.ModTime().Equal(m.cacheMtime) {
		return
	}
	c, err := cache.Load(m.deps.CachePath())
	if err != nil {
		m.status = err.Error()
		return
	}
	m.cache, m.cacheMtime = c, info.ModTime()
	m.applyRows()
}

// applyRows filters and sorts the cache, keeping the selected MR selected.
func (m *model) applyRows() {
	selectedURL := ""
	if mr, ok := m.selected(); ok {
		selectedURL = mr.WebURL
	}

	query := strings.ToLower(m.search)
	m.rows = m.rows[:0]
	for _, mr := range m.cache.Mine {
		if m.matchesFilter(mr) && matchesSearch(mr, query) {
			m.rows = append(m.rows, mr)
		}
	}
	slices.SortStableFunc(m.rows, func(a, b gitlab.MergeRequest) int {
		return cmp.Or(cmp.Compare(priority(a), priority(b)), b.UpdatedAt.Compare(a.UpdatedAt))
	})

	m.cursor = min(m.cursor, max(len(m.rows)-1, 0))
	for i, mr := range m.rows {
		if mr.WebURL == selectedURL {
			m.cursor = i
		}
	}
}

func (m model) selected() (gitlab.MergeRequest, bool) {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return gitlab.MergeRequest{}, false
	}
	return m.rows[m.cursor], true
}

func (m model) matchesFilter(mr gitlab.MergeRequest) bool {
	switch m.filter {
	case filterReview:
		return mr.HasRole(gitlab.RoleReviewer)
	case filterAuthored:
		return mr.HasRole(gitlab.RoleAuthor)
	}
	return true
}

func matchesSearch(mr gitlab.MergeRequest, query string) bool {
	if query == "" {
		return true
	}
	for _, field := range []string{mr.Title, mr.Project, mr.SourceBranch} {
		if strings.Contains(strings.ToLower(field), query) {
			return true
		}
	}
	return false
}

// priority orders rows by how much they need the user. See doc/design.md §7.2.
func priority(mr gitlab.MergeRequest) int {
	switch {
	case mr.Draft:
		return 3
	case mr.HasRole(gitlab.RoleReviewer) && mr.MyReviewState != "APPROVED":
		return 0
	case mr.HasRole(gitlab.RoleAuthor) && needsAuthor(mr):
		return 1
	default:
		return 2
	}
}

func needsAuthor(mr gitlab.MergeRequest) bool {
	return mr.Pipeline == "FAILED" || mr.ThreadsUnresolved > 0 ||
		mr.MergeStatus == "NEED_REBASE" || mr.MergeStatus == "CONFLICT"
}
