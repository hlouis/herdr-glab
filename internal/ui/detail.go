package ui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/hlouis/herdr-glab/internal/action"
	"github.com/hlouis/herdr-glab/internal/cache"
	"github.com/hlouis/herdr-glab/internal/gitlab"
	"github.com/hlouis/herdr-glab/internal/plugin"
	"github.com/hlouis/herdr-glab/internal/refresh"
	"github.com/hlouis/herdr-glab/internal/repo"
)

const detailHelp = "c checkout · r review · o/b browser · y copy · enter jump · p panel · R reload · q close"

type mrLoadedMsg struct {
	mr  gitlab.MergeRequest
	err error
}

// detailModel shows a single merge request, the pane a ctrl+click on an MR URL
// opens. Unlike the panel it is not limited to merge requests related to you.
type detailModel struct {
	ctx    context.Context
	deps   refresh.Deps
	runner action.Runner

	url     string
	project string
	iid     int
	mr      gitlab.MergeRequest
	found   bool

	cache       cache.Cache
	repos       []repo.WorkspaceRepo
	reposLoaded bool

	fatal  string
	status string
	busy   bool
	width  int
	height int
}

// RunDetail shows the merge request the given URL points at.
func RunDetail(ctx context.Context, deps refresh.Deps, url string) error {
	m := detailModel{ctx: ctx, deps: deps, runner: action.Runner{Deps: deps}, url: url}
	m.cache, _ = cache.Load(deps.CachePath())

	host, project, iid, ok := gitlab.ParseMRURL(url)
	switch {
	case !ok:
		m.fatal = "not a merge request URL: " + url
	case !strings.EqualFold(host, deps.Config.Host):
		m.fatal = fmt.Sprintf("%s is on another GitLab host; this plugin is set up for %s", host, deps.Config.Host)
	default:
		m.project, m.iid = project, iid
		m.mr, m.found = m.cache.FindByIID(project, iid)
	}

	_, err := tea.NewProgram(m, tea.WithContext(ctx)).Run()
	return err
}

func (m detailModel) Init() tea.Cmd {
	if m.fatal != "" {
		return nil
	}
	cmds := []tea.Cmd{m.scanRepos()}
	if !m.found {
		cmds = append(cmds, m.fetch())
	}
	return tea.Batch(cmds...)
}

func (m detailModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case reposMsg:
		if msg.err != nil {
			m.status = "scan workspaces: " + msg.err.Error()
			return m, nil
		}
		m.repos, m.reposLoaded = msg.repos, true
	case mrLoadedMsg:
		m.busy = false
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.mr, m.found, m.status = msg.mr, true, ""
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
		return m.handleKey(msg)
	}
	return m, nil
}

func (m detailModel) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key := msg.String(); key {
	case "ctrl+c", "q", "esc":
		return m, tea.Quit
	case "p":
		return m, m.openPanel()
	case "R", "shift+r":
		if !m.busy && m.fatal == "" {
			m.busy, m.status = true, "loading…"
			return m, m.fetch()
		}
	case "enter", "c", "r", "o", "b", "y":
		if !m.found || m.busy {
			return m, nil
		}
		if needsRepos(key) && !m.reposLoaded {
			m.status = "still scanning workspaces…"
			return m, nil
		}
		cmd, status := mrAction(m.ctx, m.runner, key, m.cache, m.repos, m.mr)
		if cmd == nil {
			return m, nil
		}
		m.busy, m.status = true, status
		return m, cmd
	}
	return m, nil
}

func (m detailModel) fetch() tea.Cmd {
	ctx, deps, project, iid, username := m.ctx, m.deps, m.project, m.iid, m.cache.Username
	return func() tea.Msg {
		mr, err := deps.GitLab.FetchOne(ctx, username, project, iid)
		return mrLoadedMsg{mr: mr, err: err}
	}
}

func (m detailModel) scanRepos() tea.Cmd {
	ctx, deps := m.ctx, m.deps
	return func() tea.Msg {
		repos, err := repo.Scan(ctx, deps.Herdr, deps.Config.Host, "")
		return reposMsg{repos: repos, err: err}
	}
}

// openPanel hands over to the full panel and closes this pane.
func (m detailModel) openPanel() tea.Cmd {
	ctx, deps := m.ctx, m.deps
	return func() tea.Msg {
		err := deps.Herdr.OpenPluginPane(ctx, deps.Env.ID, plugin.PanelEntrypoint)
		return actionMsg{err: err, quit: err == nil}
	}
}

func (m detailModel) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	return v
}

func (m detailModel) render() string {
	width := min(max(m.width, minWidth), maxWidth)
	lines := []string{fit(boldStyle.Render("GitLab MR")+"  "+dimStyle.Render(m.url), width), ""}

	switch {
	case m.fatal != "":
		lines = append(lines, errorStyle.Render(fit(m.fatal, width)))
	case !m.found:
		lines = append(lines, dimStyle.Render(fit("loading…", width)))
	default:
		lines = append(lines, blockLines(m.mr, false, width, localMark(m.cache, m.repos, m.mr))...)
		if state := strings.ToLower(m.mr.State); state != "" && state != "opened" {
			lines = append(lines, "", warnStyle.Render(fit("this merge request is "+state, width)))
		}
	}

	for len(lines) < max(m.height-1, 0) {
		lines = append(lines, "")
	}
	footer := dimStyle.Render(fit(detailHelp, width))
	if m.status != "" {
		footer = fit(m.status, width)
	}
	return strings.Join(append(lines, footer), "\n")
}
