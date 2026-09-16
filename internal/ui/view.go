package ui

import (
	"fmt"
	"path"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/hlouis/herdr-glab/internal/gitlab"
	"github.com/hlouis/herdr-glab/internal/repo"
	"github.com/hlouis/herdr-glab/internal/token"
)

const helpText = "enter jump · c checkout · r review · o/b browser · y copy · R refresh · tab filter · / search · ? help · q quit"

// Content is capped so a wide terminal gets a readable column instead of
// full-width rules and titles stretched across the screen.
const (
	minWidth = 40
	maxWidth = 120
)

var (
	boldStyle    = lipgloss.NewStyle().Bold(true)
	dimStyle     = lipgloss.NewStyle().Faint(true)
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	warnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	groupStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	// The selected block is filled with the sidebar's own selection color.
	// Inner colors are dropped there: their resets would clear the background.
	selectedStyle = lipgloss.NewStyle().Background(lipgloss.Color("#45475a"))
)

func (m model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	return v
}

func (m model) render() string {
	width := min(max(m.width, minWidth), maxWidth)
	if m.help {
		return m.helpScreen(width)
	}

	head := []string{m.header(width)}
	if m.cache.Error != "" {
		head = append(head, errorStyle.Render(fit(m.cache.Error, width)))
	}
	head = append(head, "")

	body, spans := m.body(width)
	body = window(body, spans, m.cursor, max(m.height-len(head)-1, 1))
	return strings.Join(append(append(head, body...), m.footer(width)), "\n")
}

// span is the line range one MR block occupies in the body.
type span struct{ start, end int }

// body renders group headers and one two-line block per MR.
func (m model) body(width int) ([]string, []span) {
	spans := make([]span, len(m.rows))
	if len(m.rows) == 0 {
		return []string{dimStyle.Render("no merge requests")}, spans
	}

	var lines []string
	for gi, g := range m.groups {
		if gi > 0 {
			lines = append(lines, "")
		}
		lines = append(lines,
			groupStyle.Render(fmt.Sprintf("%s (%d)", g.title, g.count)),
			dimStyle.Render(strings.Repeat("─", width)))
		for i := g.start; i < g.start+g.count; i++ {
			if i > g.start {
				lines = append(lines, "")
			}
			start := len(lines)
			lines = append(lines, m.block(m.rows[i], i == m.cursor, width)...)
			spans[i] = span{start: start, end: len(lines) - 1}
		}
	}
	return lines, spans
}

// block is one MR: a title line and a metadata line below it.
func (m model) block(mr gitlab.MergeRequest, selected bool, width int) []string {
	title := mr.Title
	if mr.Draft {
		title = "[draft] " + title
	}
	bar := "  "
	if selected {
		bar = "▌ "
	}
	// The title is the only part that may be cut, so the roles badge at the end
	// of the line always stays visible.
	prefix := fmt.Sprintf("%s%s !%d  ", bar, path.Base(mr.Project), mr.IID)
	badge := ""
	if extra := otherRoles(mr); extra != "" {
		badge = "  " + extra
	}
	head := prefix + fit(title, max(width-lipgloss.Width(prefix)-lipgloss.Width(badge), 10)) + badge
	meta := "    " + strings.Join(m.metaParts(mr, !selected), " · ")
	if selected {
		return []string{
			selectedStyle.Render(fit(head, width)),
			selectedStyle.Render(fit(meta, width)),
		}
	}
	return []string{fit(head, width), fit(meta, width)}
}

// metaParts is the second line of a block. Colors are dropped inside the
// selected block, where they would punch holes in its background.
func (m model) metaParts(mr gitlab.MergeRequest, colored bool) []string {
	paint := func(style lipgloss.Style, s string) string {
		if colored {
			return style.Render(s)
		}
		return s
	}

	var parts []string
	if symbol := token.PipelineSymbol(mr.Pipeline); symbol != "" {
		parts = append(parts, paint(pipelineStyle(mr.Pipeline), symbol+" "+strings.ToLower(mr.Pipeline)))
	}
	// An MR in a project that requires no approvals reports approved = true with
	// nobody having approved it, so name the approvers instead of the flag.
	if len(mr.ApprovedBy) > 0 {
		parts = append(parts, paint(successStyle, "✓ "+strings.Join(mr.ApprovedBy, ", ")))
	}
	if mr.ApprovalsLeft > 0 {
		parts = append(parts, paint(warnStyle, fmt.Sprintf("+%d approvals", mr.ApprovalsLeft)))
	}
	if pending := pendingReviewers(mr); len(pending) > 0 {
		parts = append(parts, paint(dimStyle, "⧗ "+strings.Join(pending, ", ")))
	}
	if len(mr.ApprovedBy) == 0 && mr.ApprovalsLeft == 0 && len(mr.Reviewers) == 0 {
		parts = append(parts, paint(dimStyle, "no approvals"))
	}
	if mr.ThreadsTotal > 0 {
		threads := fmt.Sprintf("✎%d/%d threads", mr.ThreadsTotal-mr.ThreadsUnresolved, mr.ThreadsTotal)
		if mr.ThreadsUnresolved > 0 {
			threads = paint(warnStyle, threads)
		} else {
			threads = paint(dimStyle, threads)
		}
		parts = append(parts, threads)
	}
	if status := token.MergeStatusText(mr.MergeStatus); status != "" {
		parts = append(parts, paint(errorStyle, status))
	}
	parts = append(parts,
		paint(dimStyle, mr.SourceBranch+" → "+mr.TargetBranch),
		paint(dimStyle, mr.Author),
		paint(dimStyle, age(mr.UpdatedAt)))
	if mark := m.localMark(mr); mark != "" {
		parts = append(parts, mark)
	}
	return parts
}

func (m model) header(width int) string {
	tabs := make([]string, len(filters))
	for i, f := range filters {
		if i == m.filter {
			tabs[i] = boldStyle.Render("[" + f.name + "]")
		} else {
			tabs[i] = dimStyle.Render(" " + f.name + " ")
		}
	}
	updated := "never"
	if !m.cache.FetchedAt.IsZero() {
		updated = age(m.cache.FetchedAt)
	}
	info := dimStyle.Render(fmt.Sprintf("%s@%s · updated %s", m.cache.Username, m.deps.Config.Host, updated))
	return fit(boldStyle.Render("GitLab MRs")+"  "+strings.Join(tabs, "")+"  "+info, width)
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
		{"o / b", "open the MR in the default browser"},
		{"y", "copy the MR link"},
		{"tab", "cycle filter: all / review / assigned / mine / mentions"},
		{"/", "search title, project and branch; esc clears it"},
		{"R", "fetch from GitLab now"},
		{"q / esc", "close the panel"},
	}

	lines := []string{
		boldStyle.Render("GitLab MRs — help"),
		"",
		fit("Opened merge requests you are asked to review, are assigned, authored, or are", width),
		fit("mentioned in. Each one is listed once, under the first of those that applies.", width),
		fit(fmt.Sprintf("A background poller fetches them from %s every %s; the panel reads", m.deps.Config.Host, m.deps.Config.FetchInterval), width),
		fit("that cache, so it opens instantly and never waits for the network.", width),
		"",
		boldStyle.Render("Keys"),
	}
	for _, k := range keys {
		lines = append(lines, "  "+fit(k[0], 8)+" "+fit(k[1], max(width-11, 10)))
	}
	legend := [][2]string{
		{"▌", "the selected merge request; also … lists its other roles"},
		{"✔ ✖ ↻", "pipeline: success, failed, running"},
		{"⋯ ⊘ ⚙", "pipeline: pending, canceled or skipped, manual"},
		{"✓ name", "approved by that person"},
		{"+N", "N more approvals are required"},
		{"⧗ name", "reviewer who has not approved yet"},
		{"✎ n/m", "n of m discussion threads resolved; yellow while any is open"},
		{"rebase", "the branch needs a rebase"},
		{"conflict", "the branch conflicts with its target"},
		{"●", "a workspace has this branch checked out"},
		{"○", "the repository is open locally, the branch is not checked out"},
		{"(blank)", "the repository is not on this machine"},
	}

	lines = append(lines, "", boldStyle.Render("Symbols"))
	for _, l := range legend {
		lines = append(lines, "  "+fit(l[0], 8)+" "+fit(l[1], max(width-11, 10)))
	}
	lines = append(lines,
		"",
		fit("The sidebar's $mr token is the short form of the same status:", width),
		fit("  !iid [draft] [rebase|conflict] [pipeline] [✎resolved/total]", width),
	)
	for len(lines) < max(m.height-1, 0) {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n") + "\n" + dimStyle.Render(fit("press any key to close", width))
}

// window scrolls the body so the selected block stays visible, then pads it to
// a fixed height so the footer does not move.
func window(lines []string, spans []span, cursor, height int) []string {
	offset := 0
	if cursor >= 0 && cursor < len(spans) {
		s := spans[cursor]
		if s.end >= height {
			offset = s.end - height + 1
		}
		offset = min(offset, s.start)
	}
	end := min(offset+height, len(lines))
	visible := append([]string{}, lines[min(offset, len(lines)):end]...)
	for len(visible) < height {
		visible = append(visible, "")
	}
	return visible
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

// pendingReviewers are the reviewers who have not approved yet.
func pendingReviewers(mr gitlab.MergeRequest) []string {
	var pending []string
	for _, r := range mr.Reviewers {
		if !r.Approved && !slices.Contains(mr.ApprovedBy, r.Username) {
			pending = append(pending, r.Username)
		}
	}
	return pending
}

// otherRoles names the roles beyond the group the MR is listed under.
func otherRoles(mr gitlab.MergeRequest) string {
	var extra []string
	for _, g := range groupOrder {
		if mr.HasRole(g.role) {
			extra = append(extra, string(g.role))
		}
	}
	if len(extra) < 2 {
		return ""
	}
	return "also " + strings.Join(extra[1:], ", ")
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

func age(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// fit truncates s to width cells and pads it so columns line up with CJK text.
func fit(s string, width int) string {
	s = ansi.Truncate(s, width, "…")
	return s + strings.Repeat(" ", max(width-lipgloss.Width(s), 0))
}
