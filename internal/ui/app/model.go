package app

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/4aykovski/glview/internal/i18n"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/localgit"
)

const (
	toastTTL      = 4 * time.Second
	toastErrorTTL = 8 * time.Second
)

// Options configure the root model.
type Options struct {
	// SaveLanguage persists the language choice (config.Set). May be nil.
	SaveLanguage func(i18n.Lang) error
	// InitialToasts are shown right after start (config warnings…).
	InitialToasts []ToastMsg
}

// Model is the root tea.Model.
type Model struct {
	env     *Env
	opts    Options
	stack   []Screen
	overlay Overlay

	width, height int
	isDark        bool

	toast   *ToastMsg
	toastID int

	starredTried bool // the background starred refresh ran this session

	// The To-Do counter of the header (todos.go).
	todoStarted       bool
	todoTick, todoReq common.Gen
}

// New creates the root with a first screen.
func New(env *Env, first Screen, opts Options) *Model {
	if env.Now == nil {
		env.Now = time.Now
	}
	if env.Msg == nil {
		env.Msg = i18n.Get(env.Lang)
	}
	m := &Model{env: env, opts: opts, isDark: true, todoTick: common.NewGen(), todoReq: common.NewGen()}
	if env.Styles == nil {
		env.Styles = common.NewStyles(common.ThemeByName(env.ThemeName, true))
	} else {
		m.isDark = env.Styles.Theme.IsDark
	}
	m.stack = []Screen{first}
	m.registerWorkspace()
	return m
}

// topWorkspace is the index of the topmost Workspace of the stack, -1 if
// there is none.
func (m *Model) topWorkspace() int {
	for i := len(m.stack) - 1; i >= 0; i-- {
		if _, ok := m.stack[i].(WorkspaceRoot); ok {
			return i
		}
	}
	return -1
}

// registerWorkspace tells the catalog which Workspace (if any) is the
// topmost one of the stack: the picker lists its projects.
func (m *Model) registerWorkspace() {
	var w WorkspaceRoot
	if i := m.topWorkspace(); i >= 0 {
		w = m.stack[i].(WorkspaceRoot)
	}
	m.env.Catalog.SetWorkspace(w)
}

// refreshStarred starts the background starred refresh once per session,
// as soon as there is a client.
func (m *Model) refreshStarred() tea.Cmd {
	if m.starredTried || m.env.Client == nil {
		return nil
	}
	m.starredTried = true
	return m.env.Catalog.refreshStarred(m.env.Client)
}

// Env returns the shared environment (for tests and main).
func (m *Model) Env() *Env { return m.env }

// Top returns the screen on top of the stack.
func (m *Model) Top() Screen { return m.stack[len(m.stack)-1] }

// Overlay returns the open overlay, nil when none.
func (m *Model) Overlay() Overlay { return m.overlay }

// Depth returns the stack size.
func (m *Model) Depth() int { return len(m.stack) }

func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{tea.RequestBackgroundColor, m.Top().Init(), m.sendTop(ActivatedMsg{}), m.listenCache(), m.listenNet(), m.refreshStarred(), m.startTodoCount()}
	for _, t := range m.opts.InitialToasts {
		cmds = append(cmds, msgCmd(t))
	}
	return tea.Batch(cmds...)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		for _, s := range m.stack {
			s.SetSize(m.contentSize())
		}
		return m, nil

	case tea.KeyPressMsg:
		return m, m.handleKey(msg)

	case tea.BackgroundColorMsg:
		if dark := msg.IsDark(); dark != m.isDark {
			m.isDark = dark
			m.env.Styles = common.NewStyles(common.ThemeByName(m.env.ThemeName, dark))
			return m, m.broadcast(ThemeChangedMsg{Styles: m.env.Styles})
		}
		return m, nil

	case PushMsg:
		return m, m.push(msg.Screen)
	case PopMsg:
		return m, m.pop()
	case ReplaceRootMsg:
		return m, m.replaceRoot(msg.Screen)
	case OpenProjectMsg:
		return m, m.openProject(msg.Screen)
	case OpenGroupMsg:
		return m, m.openAbove(0, msg.Screen)

	case OpenOverlayMsg:
		m.overlay = msg.Overlay
		return m, nil
	case CloseOverlayMsg:
		m.overlay = nil
		return m, nil

	case ToastMsg:
		m.toastID++
		t := msg
		m.toast = &t
		ttl := toastTTL
		if msg.Level == common.ToastError {
			ttl = toastErrorTTL
		}
		id := m.toastID
		return m, tea.Tick(ttl, func(time.Time) tea.Msg { return toastExpiredMsg{id: id} })
	case toastExpiredMsg:
		if msg.id == m.toastID {
			m.toast = nil
		}
		return m, nil

	case tea.FocusMsg:
		m.env.Focused = true
		return m, m.broadcast(FocusChangedMsg{Focused: true})
	case tea.BlurMsg:
		m.env.Focused = false
		return m, m.broadcast(FocusChangedMsg{Focused: false})

	case CacheRevalidatedMsg:
		return m, tea.Batch(m.broadcast(msg), m.listenCache())

	case NetChangedMsg:
		return m, m.onNetChanged(msg)

	case AuthLostMsg:
		return m, m.onAuthLost(msg)
	case authCheckedMsg:
		return m, m.onAuthChecked(msg)

	case starredLoadedMsg:
		m.env.Catalog.onStarred(msg)
		return m, nil

	case todoTickMsg:
		return m, m.onTodoTick(msg)
	case todoPendingMsg:
		m.onTodoPending(msg)
		return m, nil
	case TodoCountMsg:
		m.onTodoCount(msg)
		return m, nil

	case localgit.ProgressMsg, localgit.DoneMsg, localgit.AheadBehindMsg:
		return m, m.onGit(msg)

	case langSavedMsg:
		if msg.err != nil {
			return m, Toast(common.ToastError, m.env.Msg.Toast.ConfigSaveFailed(msg.err.Error()), "")
		}
		return m, nil
	}

	// Everything else (data responses, ticks, app messages) goes to all
	// screens: each one filters by its own message types and Gen tokens, so
	// a screen covered by another still receives answers to its requests.
	return m, m.broadcast(msg)
}

// onGit feeds the git tracker, shows the final toast of an operation and
// tells every screen (the one under the stack is up to date on return).
func (m *Model) onGit(msg tea.Msg) tea.Cmd {
	_, done, next := m.env.Git.Handle(msg)
	cmds := []tea.Cmd{next, m.broadcast(msg)}
	if done {
		text, failed, hint := m.env.Git.Summary(m.env.Msg)
		level := common.ToastSuccess
		if failed {
			level = common.ToastError
		}
		cmds = append(cmds, Toast(level, text, hint))
	}
	return tea.Batch(cmds...)
}

func (m *Model) quit() tea.Cmd {
	m.env.Git.Stop()
	return tea.Quit
}

func (m *Model) handleKey(k tea.KeyPressMsg) tea.Cmd {
	if k.String() == "ctrl+c" {
		return m.quit()
	}
	if m.overlay != nil {
		o, cmd := m.overlay.Update(k)
		m.overlay = o
		return cmd
	}
	if c, ok := m.Top().(InputCapturer); ok && c.CapturesInput() {
		return m.updateTop(k)
	}
	switch k.String() {
	case "r":
		if m.env.AuthLost {
			return m.checkAuth()
		}
	case "?":
		m.overlay = NewHelp(m.env, m.Top().KeyMap(), m.globalBindings(), m.searchBindings()...)
		return nil
	case "p", "F":
		if m.env.Search != nil && m.env.Client != nil {
			m.overlay = m.env.Search(k.String() == "F")
			return nil
		}
	case "T":
		if m.env.Todos != nil && m.env.Client != nil {
			return m.openTodos()
		}
	case "ctrl+l":
		return m.toggleLanguage()
	case "q":
		if len(m.stack) == 1 {
			return m.quit()
		}
	case "esc":
		if len(m.stack) > 1 {
			return m.pop()
		}
		return nil
	}
	return m.updateTop(k)
}

func (m *Model) toggleLanguage() tea.Cmd {
	m.env.Lang = m.env.Lang.Next()
	m.env.Msg = i18n.Get(m.env.Lang)
	cmds := []tea.Cmd{
		m.broadcast(LangChangedMsg{Lang: m.env.Lang}),
		Toast(common.ToastInfo, m.env.Msg.Toast.LanguageSwitched, ""),
	}
	if save := m.opts.SaveLanguage; save != nil {
		lang := m.env.Lang
		cmds = append(cmds, func() tea.Msg { return langSavedMsg{err: save(lang)} })
	}
	return tea.Batch(cmds...)
}

func (m *Model) push(s Screen) tea.Cmd {
	deact := m.sendTop(DeactivatedMsg{})
	m.stack = append(m.stack, s)
	m.registerWorkspace()
	s.SetSize(m.contentSize())
	return tea.Batch(deact, s.Init(), m.sendTop(ActivatedMsg{}))
}

func (m *Model) pop() tea.Cmd {
	if len(m.stack) <= 1 {
		return nil
	}
	deact := m.sendTop(DeactivatedMsg{})
	m.stack = m.stack[:len(m.stack)-1]
	m.registerWorkspace()
	return tea.Batch(deact, m.sendTop(ActivatedMsg{}))
}

func (m *Model) replaceRoot(s Screen) tea.Cmd {
	var cmds []tea.Cmd
	for i := len(m.stack) - 1; i >= 0; i-- {
		_, cmd := m.stack[i].Update(DeactivatedMsg{})
		cmds = append(cmds, cmd)
	}
	m.stack = []Screen{s}
	m.registerWorkspace()
	s.SetSize(m.contentSize())
	return tea.Batch(append(cmds, s.Init(), m.sendTop(ActivatedMsg{}), m.refreshStarred(), m.startTodoCount())...)
}

// openProject puts a project chosen in search above the topmost
// Workspace of the stack, or above the root without one; every screen
// above is closed (SPEC §2.6).
func (m *Model) openProject(s Screen) tea.Cmd {
	return m.openAbove(max(0, m.topWorkspace()), s)
}

// openAbove closes every screen above stack[keep] and puts s on top. The
// start screen at the bottom is never replaced.
func (m *Model) openAbove(keep int, s Screen) tea.Cmd {
	var cmds []tea.Cmd
	for i := len(m.stack) - 1; i > keep; i-- {
		_, cmd := m.stack[i].Update(DeactivatedMsg{})
		cmds = append(cmds, cmd)
	}
	if len(m.stack) == keep+1 {
		cmds = append(cmds, m.sendTop(DeactivatedMsg{}))
	}
	m.stack = append(m.stack[:keep+1], s)
	m.registerWorkspace()
	s.SetSize(m.contentSize())
	return tea.Batch(append(cmds, s.Init(), m.sendTop(ActivatedMsg{}))...)
}

func (m *Model) updateTop(msg tea.Msg) tea.Cmd { return m.sendTop(msg) }

func (m *Model) sendTop(msg tea.Msg) tea.Cmd {
	i := len(m.stack) - 1
	s, cmd := m.stack[i].Update(msg)
	m.stack[i] = s
	return cmd
}

func (m *Model) broadcast(msg tea.Msg) tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(m.stack)+1)
	for i := range m.stack {
		s, cmd := m.stack[i].Update(msg)
		m.stack[i] = s
		cmds = append(cmds, cmd)
	}
	if m.overlay != nil {
		o, cmd := m.overlay.Update(msg)
		m.overlay = o
		cmds = append(cmds, cmd)
	}
	return tea.Batch(cmds...)
}

// contentSize is the area given to screens: everything above the footer.
// A thin rule separates the content from the footer line.
func (m *Model) contentSize() (int, int) { return m.width, max(0, m.height-2) }

// searchBindings is the help group of the p and F overlays (none when
// search is not available).
func (m *Model) searchBindings() []common.Group {
	if m.env.Search == nil {
		return nil
	}
	k, s := m.env.Msg.Keys, m.env.Msg.Search
	return []common.Group{{Title: s.HelpTitle, Bindings: []common.Binding{
		common.NewBinding([]string{"down", "up", "ctrl+n", "ctrl+p"}, "↓/↑ ctrl+n/ctrl+p", s.KeyMove, common.PriorityLow),
		common.NewBinding([]string{"ctrl+d", "ctrl+u", "pgdown", "pgup"}, "ctrl+d/ctrl+u pgdown/pgup", s.KeyHalfPage, common.PriorityLow),
		common.NewBinding([]string{"enter"}, "enter", k.Open, common.PriorityLow),
		common.NewBinding([]string{"esc"}, "esc", s.KeyClose, common.PriorityLow),
	}}}
}

func (m *Model) globalBindings() []common.Binding {
	k := m.env.Msg.Keys
	search := func(b common.Binding) common.Binding {
		b.Hidden = m.env.Search == nil
		return b
	}
	todos := func(b common.Binding) common.Binding {
		b.Hidden = m.env.Todos == nil
		return b
	}
	return []common.Binding{
		common.NewBinding([]string{"j", "down"}, "j/k ↑/↓", k.Down+" / "+k.Up, common.PriorityLow),
		common.NewBinding([]string{"g"}, "g/G", k.Top+" / "+k.Bottom, common.PriorityLow),
		common.NewBinding([]string{"enter"}, "enter", k.Open, common.PriorityLow),
		common.NewBinding([]string{"esc"}, "esc", k.Back, common.PriorityLow),
		common.NewBinding([]string{"r"}, "r", k.Refresh, common.PriorityLow),
		common.NewBinding([]string{"o"}, "o", k.Browser, common.PriorityLow),
		search(common.NewBinding([]string{"p"}, "p", m.env.Msg.Search.KeyPicker, common.PriorityLow)),
		search(common.NewBinding([]string{"F"}, "F", m.env.Msg.Search.KeyGlobal, common.PriorityLow)),
		todos(common.NewBinding([]string{"T"}, "T", m.env.Msg.Todo.KeyGlobal, common.PriorityLow)),
		common.NewBinding([]string{"ctrl+l"}, "ctrl+l", k.Language, common.PriorityLow),
		common.NewBinding([]string{"q"}, "q", k.Quit, common.PriorityLow),
		common.NewBinding([]string{"?"}, "?", k.Help, common.PriorityHelp),
	}
}

func (m *Model) footer() string {
	var right string
	if m.toast != nil {
		text := m.toast.Text
		if m.toast.Hint != "" {
			text += " — " + m.toast.Hint
		}
		right = m.env.Styles.Toast[m.toast.Level].Render(text)
	} else if sp, ok := m.Top().(StatusProvider); ok {
		right = m.env.Styles.Muted.Render(sp.StatusHint())
	}
	bindings := append([]common.Binding(nil), m.Top().KeyMap().Short...)
	bindings = append(bindings, common.NewBinding([]string{"?"}, "?", m.env.Msg.Keys.Help, common.PriorityHelp))
	return common.RenderFooter(m.env.Styles, bindings, right, m.width)
}

func (m *Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.ReportFocus = true
	v.WindowTitle = "glview — " + m.Top().Title()
	return v
}

func (m *Model) render() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	if m.width < common.MinWidth || m.height < common.MinHeight {
		msg := m.env.Msg.Startup.TooSmall(common.MinWidth, common.MinHeight)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, msg)
	}
	_, h := m.contentSize()
	base := common.FitHeight(m.Top().View(), h) + "\n" + common.Rule(m.env.Styles, m.width) + "\n" + m.footer()
	if m.overlay == nil {
		return base
	}
	box := m.overlay.View(m.width, m.height)
	bw, bh := lipgloss.Width(box), lipgloss.Height(box)
	x, y := max(0, (m.width-bw)/2), max(0, (m.height-bh)/3)
	return lipgloss.NewCompositor(
		lipgloss.NewLayer(base),
		lipgloss.NewLayer(box).X(x).Y(y).Z(1),
	).Render()
}
