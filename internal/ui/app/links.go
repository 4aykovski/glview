package app

import (
	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/ui/common"
)

// LinksRefreshMsg replaces the list of the open links overlay of the MR
// identified by Key (a screen sends it when it gets fresh MR details).
type LinksRefreshMsg struct {
	Key   string
	Links []gitlab.Link
}

// LinksOverlay is the L overlay: the links of an MR, enter opens one in the
// browser, y copies its URL, esc closes (SPEC 3.5, "MR links").
type LinksOverlay struct {
	env   *Env
	key   string // identifies the MR for LinksRefreshMsg
	ref   string // "!479", shown in the title
	modal *common.LinksModal
}

var _ Overlay = (*LinksOverlay)(nil)

// NewLinksOverlay opens the links list of the MR identified by key.
func NewLinksOverlay(env *Env, key, ref string, links []gitlab.Link) *LinksOverlay {
	return &LinksOverlay{env: env, key: key, ref: ref, modal: common.NewLinksModal(links)}
}

func (o *LinksOverlay) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	switch msg := msg.(type) {
	case LinksRefreshMsg:
		if msg.Key == o.key {
			o.modal.SetLinks(msg.Links)
		}
	case tea.KeyPressMsg:
		switch k := msg.String(); k {
		case "esc", "q", "L":
			return nil, nil
		case "enter":
			if l, ok := o.modal.Selected(); ok {
				return nil, OpenURL(o.env, l.URL)
			}
		case "y":
			if l, ok := o.modal.Selected(); ok {
				return nil, Copy(o.env, l.URL, o.env.Msg.Links.What)
			}
		default:
			o.modal.Move(k)
		}
	}
	return o, nil
}

func (o *LinksOverlay) View(width, height int) string {
	m := o.env.Msg.Links
	return o.modal.View(o.env.Styles, m.Title(o.ref), m.Hints, width, height)
}

// LinksRefresh tells the open links overlay of the MR identified by key
// that its list changed.
func LinksRefresh(key string, links []gitlab.Link) tea.Cmd {
	return msgCmd(LinksRefreshMsg{Key: key, Links: links})
}
