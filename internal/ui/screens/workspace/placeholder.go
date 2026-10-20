package workspace

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/ui/common"
)

// placeholderTab is the body of a tab without content yet (Tags until
// M3); it makes no requests.
type placeholderTab struct {
	sh   *shared
	idx  int
	w, h int
}

func newPlaceholder(sh *shared, idx int) *placeholderTab { return &placeholderTab{sh: sh, idx: idx} }

func (t *placeholderTab) Update(tea.Msg) (tab, tea.Cmd) { return t, nil }
func (t *placeholderTab) SetSize(w, h int)              { t.w, t.h = w, h }
func (t *placeholderTab) KeyMap() common.KeyMap         { return common.KeyMap{} }
func (t *placeholderTab) CapturesInput() bool           { return false }
func (t *placeholderTab) Activate() tea.Cmd             { return nil }
func (t *placeholderTab) Deactivate()                   {}
func (t *placeholderTab) Refresh() tea.Cmd              { return nil }
func (t *placeholderTab) Count() int                    { return -1 }
func (t *placeholderTab) Status() string                { return "" }
func (t *placeholderTab) UpdatedAt() time.Time          { return time.Time{} }
func (t *placeholderTab) Rebuild()                      {}

func (t *placeholderTab) View() string {
	m := t.sh.env.Msg
	name := []string{m.Tabs.Projects, m.Tabs.MRs, m.Tabs.Pipelines, m.Tabs.Tags}[t.idx]
	return "\n  " + t.sh.env.Styles.Muted.Render(m.Project.Later(name))
}
