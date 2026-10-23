// Package issuedetail is the full-screen view of an issue (SPEC §3.15).
package issuedetail

import (
	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
)

// Screen is the issue screen.
type Screen struct {
	env   *app.Env
	ref   gitlab.ProjectRef
	issue gitlab.Issue
}

// New opens the issue; initial (from a list row) is shown until the details
// arrive.
func New(env *app.Env, ref gitlab.ProjectRef, initial gitlab.Issue) *Screen {
	return &Screen{env: env, ref: ref, issue: initial}
}

func (s *Screen) Init() tea.Cmd                        { return nil }
func (s *Screen) Update(tea.Msg) (app.Screen, tea.Cmd) { return s, nil }
func (s *Screen) View() string                         { return "" }
func (s *Screen) Title() string                        { return s.ref.FullPath }
func (s *Screen) KeyMap() common.KeyMap                { return common.KeyMap{} }
func (s *Screen) SetSize(int, int)                     {}
