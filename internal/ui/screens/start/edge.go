package start

import (
	"strings"
	"time"

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
// the next steps (SPEC §2.4). p and F are handled by the root.
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
	b.WriteString(common.Header(s, s.Bold.Render(n.info.Reason), n.env.HeaderRight(time.Time{}), n.w) + "\n\n")
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

func padRight(s string, w int) string {
	if n := len([]rune(s)); n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s + " "
}
