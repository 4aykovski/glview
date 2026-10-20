// Package project is the project-mode root screen: header, tabs and the
// MRs / Pipelines / Issues / Tags tabs.
package project

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitremote"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/screens/start"
)

// shared is the state every tab of one project screen reads.
type shared struct {
	env           *app.Env
	ref           gitlab.ProjectRef
	repo          *gitremote.Repo // nil without a local folder (-p)
	defaultBranch string
	webURL        string // project page, set when the project is loaded
}

// projectURL is the project page in the web UI without a trailing slash:
// webURL, or the instance URL and the full path until the project loads.
func (sh *shared) projectURL() string {
	base := sh.webURL
	if base == "" {
		base = strings.TrimRight(sh.env.Host, "/") + "/" + sh.ref.FullPath
	}
	return strings.TrimRight(base, "/")
}

// tab is one tab of the project screen.
type tab interface {
	Update(msg tea.Msg) (tab, tea.Cmd)
	View() string
	SetSize(w, h int)
	KeyMap() common.KeyMap
	CapturesInput() bool
	// Activate is called when the tab becomes the visible tab of the top
	// screen: it loads missing data and starts polling.
	Activate() tea.Cmd
	// Deactivate stops polling.
	Deactivate()
	Refresh() tea.Cmd
	UpdatedAt() time.Time
	// Count is the number shown in the tab bar (-1 = unknown).
	Count() int
	// Status is the text for the right side of the footer.
	Status() string
}

type projectLoadedMsg struct {
	tok     common.Token
	project gitlab.Project
	err     error
}

// Project is the screen.
type Project struct {
	sh     *shared
	gen    common.Gen
	tabs   []tab
	cur    int
	active bool
	w, h   int
	loaded bool
}

// New creates the screen for path; repo may be nil.
func New(env *app.Env, path string, repo *gitremote.Repo) *Project {
	sh := &shared{env: env, ref: gitlab.ProjectRef{FullPath: path}, repo: repo}
	p := &Project{sh: sh, gen: common.NewGen()}
	p.tabs = []tab{newMRsTab(sh), newPipelinesTab(sh), newPlaceholder(sh, 2), newPlaceholder(sh, 3)}
	return p
}

func (p *Project) Init() tea.Cmd { return p.loadProject() }

func (p *Project) loadProject() tea.Cmd {
	tok := p.gen.Next()
	client, ref := p.sh.env.Client, p.sh.ref
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		pr, err := client.Projects().Get(ctx, ref)
		return projectLoadedMsg{tok: tok, project: pr, err: err}
	}
}

func (p *Project) Title() string { return p.sh.ref.FullPath }

func (p *Project) SetSize(w, h int) {
	p.w, p.h = w, h
	for _, t := range p.tabs {
		t.SetSize(w, max(0, h-3))
	}
}

func (p *Project) KeyMap() common.KeyMap {
	k := p.sh.env.Msg.Keys
	km := p.tabs[p.cur].KeyMap()
	short := append([]common.Binding{}, km.Short...)
	short = append(short,
		common.NewBinding([]string{"1", "2", "3", "4"}, "1-4", k.Tabs, common.PriorityNormal),
		common.NewBinding([]string{"r"}, "", k.Refresh, common.PriorityNormal),
		common.NewBinding([]string{"q"}, "", k.Quit, common.PriorityLow),
	)
	return common.KeyMap{Short: short, Groups: km.Groups}
}

// CapturesInput lets the active tab own the keyboard while a filter is open.
func (p *Project) CapturesInput() bool { return p.tabs[p.cur].CapturesInput() }

// Current returns the index of the active tab (for tests).
func (p *Project) Current() int { return p.cur }

func (p *Project) switchTo(i int) tea.Cmd {
	i = (i%len(p.tabs) + len(p.tabs)) % len(p.tabs)
	if i == p.cur {
		return nil
	}
	if p.active {
		p.tabs[p.cur].Deactivate()
	}
	p.cur = i
	if p.active {
		return p.tabs[p.cur].Activate()
	}
	return nil
}

func (p *Project) Update(msg tea.Msg) (app.Screen, tea.Cmd) {
	env := p.sh.env
	switch msg := msg.(type) {
	case app.ActivatedMsg:
		p.active = true
		if p.loaded && p.sh.repo != nil {
			_ = p.sh.repo.RefreshHEAD()
		}
		return p, p.tabs[p.cur].Activate()
	case app.DeactivatedMsg:
		p.active = false
		p.tabs[p.cur].Deactivate()
		return p, nil

	case projectLoadedMsg:
		if !p.gen.Valid(msg.tok) {
			return p, nil
		}
		if msg.err != nil {
			if errors.Is(msg.err, gitlab.ErrNotFound) {
				return p, app.ReplaceRoot(start.NewNotOnInstance(env, start.NotOnInstanceInfo{
					Reason: env.Msg.Startup.ProjectNotFound, Project: p.sh.ref.FullPath,
				}, nil))
			}
			return p, env.Fail(msg.err)
		}
		p.loaded = true
		p.sh.ref = msg.project.ProjectRef
		p.sh.defaultBranch = msg.project.DefaultBranch
		p.sh.webURL = msg.project.WebURL
		return p, nil

	case app.AuthRestoredMsg:
		return p, tea.Batch(p.loadProject(), p.broadcast(msg))

	case tea.KeyPressMsg:
		return p, p.key(msg)
	}
	return p, p.broadcast(msg)
}

// broadcast delivers a non-key message to every tab: each filters by its
// own tokens.
func (p *Project) broadcast(msg tea.Msg) tea.Cmd {
	cmds := make([]tea.Cmd, len(p.tabs))
	for i, t := range p.tabs {
		p.tabs[i], cmds[i] = t.Update(msg)
	}
	return tea.Batch(cmds...)
}

func (p *Project) key(k tea.KeyPressMsg) tea.Cmd {
	cur := p.tabs[p.cur]
	if !cur.CapturesInput() {
		switch s := k.String(); s {
		case "1", "2", "3", "4":
			return p.switchTo(int(s[0] - '1'))
		case "tab":
			return p.switchTo(p.cur + 1)
		case "shift+tab":
			return p.switchTo(p.cur - 1)
		case "r":
			if p.sh.repo != nil {
				_ = p.sh.repo.RefreshHEAD()
			}
			return tea.Batch(p.loadProject(), cur.Refresh())
		}
	}
	var cmd tea.Cmd
	p.tabs[p.cur], cmd = cur.Update(k)
	return cmd
}

func (p *Project) View() string {
	env := p.sh.env
	s, m := env.Styles, env.Msg
	host := strings.TrimPrefix(strings.TrimPrefix(env.Host, "https://"), "http://")
	ctx := p.sh.ref.FullPath + " " + s.Muted.Render("· "+host)
	if r := p.sh.repo; r != nil && r.Branch != "" {
		branch := r.Branch
		if r.Detached {
			branch += " (" + m.Fields.Detached + ")"
		}
		ctx += " " + s.Muted.Render("· ⎇ ") + branch
	}
	header := common.Header(s, ctx, env.HeaderRight(p.tabs[p.cur].UpdatedAt()), p.w)
	bar := common.Tabs(s, []common.Tab{
		{Key: "1", Label: m.Tabs.MRs, Count: p.tabs[0].Count()},
		{Key: "2", Label: m.Tabs.Pipelines, Count: p.tabs[1].Count()},
		{Key: "3", Label: m.Tabs.Issues, Count: p.tabs[2].Count()},
		{Key: "4", Label: m.Tabs.Tags, Count: p.tabs[3].Count()},
	}, p.cur)
	return header + "\n" + bar + "\n" + common.Rule(s, p.w) + "\n" + p.tabs[p.cur].View()
}

// StatusHint is shown in the footer when there is no toast.
func (p *Project) StatusHint() string { return p.tabs[p.cur].Status() }
