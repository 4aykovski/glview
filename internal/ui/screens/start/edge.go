package start

import (
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
)

// NotOnInstanceInfo describes why project mode could not open a project.
type NotOnInstanceInfo struct {
	Reason     string // already translated
	RemoteURL  string
	RemoteHost string
	Project    string
}

// NotOnInstance explains why the project could not be opened and offers
// the next steps (SPEC §2.4). p and F are wired in M2.
type NotOnInstance struct {
	env           *app.Env
	info          NotOnInstanceInfo
	openWorkspace func() app.Screen // nil hides "w"
	w, h          int
}

// NewNotOnInstance creates the screen; openWorkspace builds the workspace of
// the parent folder for the "w" action (nil to hide it).
func NewNotOnInstance(env *app.Env, info NotOnInstanceInfo, openWorkspace func() app.Screen) *NotOnInstance {
	return &NotOnInstance{env: env, info: info, openWorkspace: openWorkspace}
}

func (n *NotOnInstance) Init() tea.Cmd    { return nil }
func (n *NotOnInstance) Title() string    { return n.info.Reason }
func (n *NotOnInstance) SetSize(w, h int) { n.w, n.h = w, h }

func (n *NotOnInstance) KeyMap() common.KeyMap {
	m := n.env.Msg
	b := []common.Binding{
		common.NewBinding([]string{"p"}, "", m.Startup.PickProject, common.PriorityHigh),
		common.NewBinding([]string{"F"}, "", m.Startup.GlobalSearch, common.PriorityHigh),
	}
	w := common.NewBinding([]string{"w"}, "", m.Startup.OpenWorkspace, common.PriorityHigh)
	w.Hidden = n.openWorkspace == nil
	b = append(b, w, common.NewBinding([]string{"q"}, "", m.Keys.Quit, common.PriorityNormal))
	return common.KeyMap{Short: b}
}

func (n *NotOnInstance) Update(msg tea.Msg) (app.Screen, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "p", "F":
			return n, app.Toast(common.ToastInfo, n.env.Msg.Startup.ComingSoon, "")
		case "w":
			if n.openWorkspace != nil {
				return n, app.ReplaceRoot(n.openWorkspace())
			}
		}
	}
	return n, nil
}

func (n *NotOnInstance) View() string {
	s, m := n.env.Styles, n.env.Msg
	var b strings.Builder
	b.WriteString(common.Header(s, s.Bold.Render(n.info.Reason), "@"+n.env.User.Username, n.w) + "\n\n")
	row := func(k, v string) {
		if v != "" {
			b.WriteString("  " + s.Muted.Render(padRight(k, 22)) + v + "\n")
		}
	}
	row(m.Fields.Project, n.info.Project)
	row(m.Startup.Remote, n.info.RemoteURL)
	row(m.Startup.RemoteHost, n.info.RemoteHost)
	row(m.Startup.InstanceHost, n.env.Host)
	return b.String()
}

// EmptyWorkspace is shown when a workspace has no GitLab projects.
type EmptyWorkspace struct {
	env                   *app.Env
	dir                   string
	scanned, foreign, raw int
	w, h                  int
}

// NewEmptyWorkspace creates the empty-state screen.
func NewEmptyWorkspace(env *app.Env, dir string, scanned, foreign, noGit int) *EmptyWorkspace {
	return &EmptyWorkspace{env: env, dir: dir, scanned: scanned, foreign: foreign, raw: noGit}
}

func (e *EmptyWorkspace) Init() tea.Cmd    { return nil }
func (e *EmptyWorkspace) Title() string    { return e.dir }
func (e *EmptyWorkspace) SetSize(w, h int) { e.w, e.h = w, h }

func (e *EmptyWorkspace) KeyMap() common.KeyMap {
	m := e.env.Msg
	return common.KeyMap{Short: []common.Binding{
		common.NewBinding([]string{"p"}, "", m.Startup.PickProject, common.PriorityHigh),
		common.NewBinding([]string{"F"}, "", m.Startup.GlobalSearch, common.PriorityHigh),
		common.NewBinding([]string{"q"}, "", m.Keys.Quit, common.PriorityNormal),
	}}
}

func (e *EmptyWorkspace) Update(msg tea.Msg) (app.Screen, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "p", "F":
			return e, app.Toast(common.ToastInfo, e.env.Msg.Startup.ComingSoon, "")
		}
	}
	return e, nil
}

func (e *EmptyWorkspace) View() string {
	s, m := e.env.Styles, e.env.Msg
	ctx := homeRel(e.dir) + " " + s.Muted.Render("· "+m.Header.Workspace+" · "+m.Plural.GitLabProjects(0))
	return common.Header(s, ctx, "@"+e.env.User.Username, e.w) + "\n\n" +
		"  " + s.Bold.Render(m.Startup.EmptyWorkspace(homeRel(e.dir))) + "\n" +
		"  " + s.Muted.Render(m.Startup.EmptyWorkspaceDetail(e.scanned, e.foreign, e.raw)) + "\n"
}

// homeRel shortens /Users/me/src to ~/src.
func homeRel(dir string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return dir
	}
	if dir == home {
		return "~"
	}
	if rel, err := filepath.Rel(home, dir); err == nil && !strings.HasPrefix(rel, "..") {
		return "~/" + rel
	}
	return dir
}

func padRight(s string, w int) string {
	if n := len([]rune(s)); n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s + " "
}
