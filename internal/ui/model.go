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

// group is one section of the list. An MR lands in the first group whose role
// it has, so it is never listed twice.
type group struct {
	title string
	role  gitlab.Role
	start int
	count int
}

var groupOrder = []group{
	{title: "Review requested", role: gitlab.RoleReviewer},
	{title: "Assigned to me", role: gitlab.RoleAssignee},
	{title: "Authored by me", role: gitlab.RoleAuthor},
	{title: "Mentioning me", role: gitlab.RoleMentioned},
}

// filters cycle with tab; the empty role means "no filter".
var filters = []struct {
	name string
	role gitlab.Role
}{
	{name: "all"},
	{name: "review", role: gitlab.RoleReviewer},
	{name: "assigned", role: gitlab.RoleAssignee},
	{name: "mine", role: gitlab.RoleAuthor},
	{name: "mentions", role: gitlab.RoleMentioned},
}

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
	groups    []group
	cursor    int
	filter    int
	search    string
	searching bool

	drawer *threadsModel
	focus  int

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
	cmds := []tea.Cmd{m.scanRepos(), tick(), tea.RequestBackgroundColor}
	if m.busy {
		cmds = append(cmds, m.refresh())
	}
	return tea.Batch(cmds...)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.BackgroundColorMsg:
		setTheme(msg.IsDark())
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
		// The panel owns actionMsg because an action can quit it, so it also
		// releases the drawer, whose `a` produces one.
		m.busy = false
		if m.drawer != nil {
			d := *m.drawer
			d.busy = false
			m.drawer = &d
		}
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		if msg.quit {
			return m, tea.Quit
		}
		m.status = msg.note
	case threadsLoadedMsg, threadResolvedMsg:
		if m.drawer != nil {
			d, cmd := m.drawer.update(msg)
			m.drawer = &d
			return m, cmd
		}
	case tea.KeyPressMsg:
		if m.searching {
			return m.handleSearchKey(msg), nil
		}
		if m.drawer != nil && !m.help {
			return m.drawerKey(msg)
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
		m.filter = (m.filter + 1) % len(filters)
		m.applyRows()
	case "/":
		m.searching = true
	case "R", "shift+r":
		if !m.busy {
			m.busy, m.status = true, "refreshing…"
			return m, m.refresh()
		}
	case "t":
		if mr, ok := m.selected(); ok {
			return m.openDrawer(mr)
		}
	case "enter", "c", "r", "o", "b", "y":
		mr, ok := m.selected()
		if !ok || m.busy {
			return m, nil
		}
		if needsRepos(key) && !m.reposLoaded {
			m.status = "still scanning workspaces…"
			return m, nil
		}
		cmd, status := mrAction(m.ctx, m.runner, key, m.cache, m.repos, mr)
		if cmd == nil {
			return m, nil
		}
		m.busy, m.status = true, status
		return m, cmd
	}
	return m, nil
}

// mrAction builds the background command for one of the MR keys, shared by the
// panel and the single-MR pane, plus the status to show while it runs.
func mrAction(ctx context.Context, runner action.Runner, key string, c cache.Cache, repos []repo.WorkspaceRepo, mr gitlab.MergeRequest) (tea.Cmd, string) {
	var do func() error
	quit, note, status := true, "", ""
	switch key {
	case "enter":
		do = func() error { return runner.Focus(ctx, c, repos, mr) }
		status = "switching workspace…"
	case "c":
		do = func() error { return runner.Checkout(ctx, repos, mr) }
		status = "checking out…"
	case "r":
		do = func() error { return runner.Review(ctx, repos, mr) }
		status = "opening tuicr…"
	case "o", "b":
		do = func() error { return action.OpenBrowser(ctx, mr.WebURL) }
		quit, note = false, "opened in browser"
	case "y":
		do = func() error { return action.Copy(ctx, mr.WebURL) }
		quit, note = false, "copied link"
	default:
		return nil, ""
	}
	return func() tea.Msg { return actionMsg{err: do(), quit: quit, note: note} }, status
}

// needsRepos reports whether a key acts on local checkouts.
func needsRepos(key string) bool {
	return key == "enter" || key == "c" || key == "r"
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

// applyRows groups, filters and sorts the cache, keeping the selected MR selected.
func (m *model) applyRows() {
	selectedURL := ""
	if mr, ok := m.selected(); ok {
		selectedURL = mr.WebURL
	}

	query := strings.ToLower(m.search)
	buckets := make([][]gitlab.MergeRequest, len(groupOrder))
	for _, mr := range m.cache.Mine {
		if !m.matchesFilter(mr) || !matchesSearch(mr, query) {
			continue
		}
		for i, g := range groupOrder {
			if mr.HasRole(g.role) {
				buckets[i] = append(buckets[i], mr)
				break
			}
		}
	}

	m.rows, m.groups = m.rows[:0], m.groups[:0]
	for i, g := range groupOrder {
		if len(buckets[i]) == 0 {
			continue
		}
		slices.SortStableFunc(buckets[i], func(a, b gitlab.MergeRequest) int {
			return cmp.Or(cmp.Compare(draftRank(a), draftRank(b)), b.UpdatedAt.Compare(a.UpdatedAt))
		})
		g.start, g.count = len(m.rows), len(buckets[i])
		m.groups = append(m.groups, g)
		m.rows = append(m.rows, buckets[i]...)
	}

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
	role := filters[m.filter].role
	return role == "" || mr.HasRole(role)
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

// draftRank sinks drafts to the bottom of their group.
func draftRank(mr gitlab.MergeRequest) int {
	if mr.Draft {
		return 1
	}
	return 0
}
