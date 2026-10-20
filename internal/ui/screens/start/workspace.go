package start

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/gitremote"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
)

// Workspace is the workspace-mode placeholder: it lists the scanned folders
// (GitLab projects first, foreign repositories grey). Real tabs arrive in M2.
type Workspace struct {
	env     *app.Env
	dir     string
	entries []gitremote.LocalProject // without KindNoGit
	skipped []string
	cursor  int
	w, h    int
}

// NewWorkspace builds the placeholder from scan results.
func NewWorkspace(env *app.Env, dir string, scanned []gitremote.LocalProject) *Workspace {
	ws := &Workspace{env: env, dir: dir}
	var gitlabs, others []gitremote.LocalProject
	for _, e := range scanned {
		switch e.Kind {
		case gitremote.KindNoGit:
			ws.skipped = append(ws.skipped, e.Name)
		case gitremote.KindGitLab:
			gitlabs = append(gitlabs, e)
		default:
			others = append(others, e)
		}
	}
	ws.entries = append(gitlabs, others...)
	return ws
}

// GitLabCount returns the number of GitLab projects found.
func (w *Workspace) GitLabCount() int {
	n := 0
	for _, e := range w.entries {
		if e.Kind == gitremote.KindGitLab {
			n++
		}
	}
	return n
}

func (w *Workspace) Init() tea.Cmd    { return nil }
func (w *Workspace) Title() string    { return w.dir }
func (w *Workspace) SetSize(x, y int) { w.w, w.h = x, y }

func (w *Workspace) KeyMap() common.KeyMap {
	k := w.env.Msg.Keys
	return common.KeyMap{Short: []common.Binding{
		common.NewBinding([]string{"j", "k"}, "j/k", k.Down+"/"+k.Up, common.PriorityHigh),
		common.NewBinding([]string{"q"}, "", k.Quit, common.PriorityLow),
	}}
}

func (w *Workspace) Update(msg tea.Msg) (app.Screen, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "j", "down":
			w.cursor = min(w.cursor+1, max(0, len(w.entries)-1))
		case "k", "up":
			w.cursor = max(w.cursor-1, 0)
		case "g":
			w.cursor = 0
		case "G":
			w.cursor = max(0, len(w.entries)-1)
		}
	}
	return w, nil
}

func (w *Workspace) View() string {
	s, m := w.env.Styles, w.env.Msg
	ctx := homeRel(w.dir) + " " + s.Muted.Render("· "+m.Header.Workspace+" · "+m.Plural.GitLabProjects(w.GitLabCount()))
	header := common.Header(s, ctx, "@"+w.env.User.Username, w.w)
	tabs := common.Tabs(s, []common.Tab{
		{Key: "1", Label: m.Tabs.Projects, Count: w.GitLabCount()},
		{Key: "2", Label: m.Tabs.MRs, Count: -1},
		{Key: "3", Label: m.Tabs.Pipelines, Count: -1},
		{Key: "4", Label: m.Tabs.Tags, Count: -1},
	}, 0)

	nameW, pathW := 8, 12
	for _, e := range w.entries {
		nameW = max(nameW, lipgloss.Width(e.Name))
		pathW = max(pathW, lipgloss.Width(w.remoteText(e)))
	}
	nameW, pathW = min(nameW, 28), min(pathW, 48)

	var b strings.Builder
	b.WriteString(s.ColumnHeader.Render(fmt.Sprintf("  %-*s  %-*s  %s", nameW, m.Columns.Folder, pathW, m.Columns.GitLab, m.Columns.LocalBranch)) + "\n")
	for i, e := range w.entries {
		branch := ""
		if e.Repo != nil {
			branch = e.Repo.Branch
		}
		line := fmt.Sprintf("%-*s  %-*s  %s", nameW, ansi.Truncate(e.Name, nameW, "…"), pathW, ansi.Truncate(w.remoteText(e), pathW, "…"), branch)
		switch {
		case i == w.cursor:
			b.WriteString(s.Cursor.Render("›") + s.RowSelected.Render(" "+line) + "\n")
		case e.Kind != gitremote.KindGitLab:
			b.WriteString("  " + s.RowMuted.Render(line) + "\n")
		default:
			b.WriteString("  " + s.Row.Render(line) + "\n")
		}
	}
	if len(w.skipped) > 0 {
		b.WriteString("\n" + s.Muted.Render(m.Startup.SkippedNoGit(strings.Join(w.skipped, ", "))) + "\n")
	}
	b.WriteString("\n" + s.Muted.Render(m.Startup.ComingSoon))
	return header + "\n" + tabs + "\n\n" + b.String()
}

func (w *Workspace) remoteText(e gitremote.LocalProject) string {
	m := w.env.Msg.Startup
	switch e.Kind {
	case gitremote.KindGitLab:
		return e.ProjectPath
	case gitremote.KindNoRemote:
		return m.NoRemote
	default:
		if e.Remote != nil {
			return e.Remote.Host + "/" + e.Remote.Path + " · " + m.ForeignRemote
		}
		return m.ForeignRemote
	}
}
