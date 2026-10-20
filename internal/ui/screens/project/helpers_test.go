package project

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
)

func splitLines(s string) []string { return strings.Split(s, "\n") }
func contains(s, sub string) bool  { return strings.Contains(s, sub) }

// dummy is a screen used to cover the project screen.
type dummy struct{ env *app.Env }

func newDummy(env *app.Env) *dummy                    { return &dummy{env: env} }
func (d *dummy) Init() tea.Cmd                        { return nil }
func (d *dummy) Update(tea.Msg) (app.Screen, tea.Cmd) { return d, nil }
func (d *dummy) View() string                         { return "dummy" }
func (d *dummy) Title() string                        { return "dummy" }
func (d *dummy) KeyMap() common.KeyMap                { return common.KeyMap{} }
func (d *dummy) SetSize(int, int)                     {}
