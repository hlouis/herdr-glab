package ui

import (
	"fmt"
	"path"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/hlouis/herdr-glab/internal/gitlab"
	"github.com/hlouis/herdr-glab/internal/repo"
	"github.com/hlouis/herdr-glab/internal/token"
)

const (
	projectWidth   = 16
	iidWidth       = 6
	approvalsWidth = 3
	threadsWidth   = 7
	detailHeight   = 3
	helpText       = "enter jump · c checkout · r review · o open · y copy · R refresh · tab filter · / search · ? help · q quit"
)

var (
	boldStyle     = lipgloss.NewStyle().Bold(true)
	dimStyle      = lipgloss.NewStyle().Faint(true)
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	successStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	warnStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	selectedStyle = lipgloss.NewStyle().Reverse(true)
)

func (m model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	return v
}

func (m model) render() string {
	width := max(m.width, 40)
	if m.help {
		return m.helpScreen(width)
	}
	var lines []string

	lines = append(lines, m.header(width))
	if m.cache.Error != "" {
		lines = append(lines, errorStyle.Render(fit(m.cache.Error, width)))
	}

	listHeight := max(m.height-len(lines)-detailHeight-2, 1)
	offset := max(m.cursor-listHeight+1, 0)
	for i := offset; i < min(offset+listHeight, len(m.rows)); i++ {
		lines = append(lines, m.row(m.rows[i], i == m.cursor, width))
	}
	if len(m.rows) == 0 {
		lines = append(lines, dimStyle.Render("no merge requests"))
	}
	for len(lines) < max(m.height-detailHeight-1, 0) {
		lines = append(lines, "")
	}

	lines = append(lines, m.detail(width)...)
	lines = append(lines, m.footer(width))
	return strings.Join(lines, "\n")
}

func (m model) header(width int) string {
	tabs := make([]string, len(filterNames))
	for i, name := range filterNames {
		if filterMode(i) == m.filter {
			tabs[i] = boldStyle.Render("[" + name + "]")
		} else {
			tabs[i] = dimStyle.Render(" " + name + " ")
		}
	}
	updated := "never"
	if !m.cache.FetchedAt.IsZero() {
		updated = time.Since(m.cache.FetchedAt).Truncate(time.Second).String() + " ago"
	}
	info := dimStyle.Render(fmt.Sprintf("%s@%s · updated %s", m.cache.Username, m.deps.Config.Host, updated))
	return fit(boldStyle.Render("GitLab MRs")+"  "+strings.Join(tabs, "")+"  "+info, width)
}

func (m model) row(mr gitlab.MergeRequest, selected bool, width int) string {
	title := mr.Title
	if mr.Draft {
		title = "[draft] " + title
	}
	approvals := fmt.Sprintf("+%d", mr.ApprovalsLeft)
	if mr.Approved {
		approvals = "✓"
	}
	threads := ""
	if mr.ThreadsTotal > 0 {
		threads = fmt.Sprintf("✎%d/%d", mr.ThreadsUnresolved, mr.ThreadsTotal)
	}

	fixed := 1 + projectWidth + iidWidth + 1 + approvalsWidth + threadsWidth + 1 + 7
	cells := []string{
		fit(roleBadge(mr), 1),
		fit(path.Base(mr.Project), projectWidth),
		fit(fmt.Sprintf("!%d", mr.IID), iidWidth),
		fit(title, max(width-fixed, 10)),
		fit(token.PipelineSymbol(mr.Pipeline), 1),
		fit(approvals, approvalsWidth),
		fit(threads, threadsWidth),
		fit(m.localMark(mr), 1),
	}
	if selected {
		return selectedStyle.Render(strings.Join(cells, " "))
	}
	cells[4] = pipelineStyle(mr.Pipeline).Render(cells[4])
	if mr.ThreadsUnresolved > 0 {
		cells[6] = warnStyle.Render(cells[6])
	}
	return strings.Join(cells, " ")
}

func (m model) detail(width int) []string {
	mr, ok := m.selected()
	if !ok {
		return make([]string, detailHeight)
	}
	reviewers := make([]string, len(mr.Reviewers))
	for i, r := range mr.Reviewers {
		reviewers[i] = fmt.Sprintf("%s(%s)", r.Username, strings.ToLower(r.State))
	}
	return []string{
		fit(boldStyle.Render(fmt.Sprintf("%s!%d", mr.Project, mr.IID))+" "+mr.Title, width),
		dimStyle.Render(fit(fmt.Sprintf("%s · %s → %s · %s · updated %s",
			mr.Author, mr.SourceBranch, mr.TargetBranch, strings.ToLower(mr.MergeStatus), mr.UpdatedAt.Local().Format("2006-01-02 15:04")), width)),
		dimStyle.Render(fit("reviewers: "+strings.Join(reviewers, " "), width)),
	}
}

func (m model) footer(width int) string {
	switch {
	case m.searching:
		return fit("/"+m.search+"▏", width)
	case m.status != "":
		return fit(m.status, width)
	case m.search != "":
		return dimStyle.Render(fit("filter: "+m.search+" (esc to clear) · "+helpText, width))
	}
	return dimStyle.Render(fit(helpText, width))
}

func (m model) helpScreen(width int) string {
	keys := [][2]string{
		{"j / k", "move"},
		{"enter", "jump to the workspace that has this MR checked out"},
		{"c", "fetch the source branch and open it as a worktree workspace"},
		{"r", "review in tuicr, in a new tab of the repository's workspace"},
		{"o", "open the MR in the browser"},
		{"y", "copy the MR link"},
		{"tab", "cycle filter: all / review requested / authored by me"},
		{"/", "search title, project and branch; esc clears it"},
		{"R", "fetch from GitLab now"},
		{"q / esc", "close the panel"},
	}

	lines := []string{
		boldStyle.Render("GitLab MRs — help"),
		"",
		fit("Opened merge requests you authored (A), are assigned (S), or are asked to review (R),", width),
		fit(fmt.Sprintf("fetched from %s every %s by a background poller. The panel reads that cache,", m.deps.Config.Host, m.deps.Config.FetchInterval), width),
		fit("so it opens instantly and never waits for the network.", width),
		"",
		boldStyle.Render("Keys"),
	}
	for _, k := range keys {
		lines = append(lines, "  "+fit(k[0], 8)+" "+fit(k[1], max(width-11, 10)))
	}
	lines = append(lines,
		"",
		boldStyle.Render("Columns"),
		fit("  role · project · !iid · title · pipeline · approvals · threads · local", width),
		fit("  pipeline  ✔ passed  ✖ failed  ↻ running  ⋯ pending  ⊘ canceled  ⚙ manual", width),
		fit("  approvals ✓ approved  +N approvals still needed", width),
		fit("  threads   ✎unresolved/total", width),
		fit("  local     ● checked out in a workspace  ○ repository is open locally", width),
		"",
		fit("The same status shows as the $mr token on each workspace row in the sidebar.", width),
	)
	for len(lines) < max(m.height-1, 0) {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n") + "\n" + dimStyle.Render(fit("press any key to close", width))
}

// localMark shows ● when a workspace has the MR checked out and ○ when only
// its repository is open locally.
func (m model) localMark(mr gitlab.MergeRequest) string {
	if _, ok := repo.WorkspaceWith(m.cache, m.repos, mr); ok {
		return "●"
	}
	if _, _, ok := repo.FindRepo(m.repos, mr.Project); ok {
		return "○"
	}
	return ""
}

func roleBadge(mr gitlab.MergeRequest) string {
	switch {
	case mr.HasRole(gitlab.RoleReviewer):
		return "R"
	case mr.HasRole(gitlab.RoleAuthor):
		return "A"
	default:
		return "S"
	}
}

func pipelineStyle(status string) lipgloss.Style {
	switch status {
	case "SUCCESS":
		return successStyle
	case "FAILED":
		return errorStyle
	case "RUNNING", "PENDING":
		return warnStyle
	}
	return dimStyle
}

// fit truncates s to width cells and pads it so columns line up with CJK text.
func fit(s string, width int) string {
	s = ansi.Truncate(s, width, "…")
	return s + strings.Repeat(" ", max(width-lipgloss.Width(s), 0))
}
