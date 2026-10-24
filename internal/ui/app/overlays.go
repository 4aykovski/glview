package app

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/4aykovski/glview/internal/ui/common"
)

// Confirm is a yes/cancel modal. "Cancel" is selected by default so a
// reflexive enter never triggers a destructive action.
type Confirm struct {
	env   *Env
	title string
	body  string
	onYes tea.Cmd
	yes   bool // currently selected button
}

// NewConfirm creates a confirm modal; onYes runs when the user confirms.
func NewConfirm(env *Env, title, body string, onYes tea.Cmd) *Confirm {
	return &Confirm{env: env, title: title, body: body, onYes: onYes}
}

func (c *Confirm) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return c, nil
	}
	switch k.String() {
	case "y":
		return nil, c.onYes
	case "n", "esc", "q":
		return nil, nil
	case "left", "right", "tab", "shift+tab", "h", "l":
		c.yes = !c.yes
	case "enter", "space":
		if c.yes {
			return nil, c.onYes
		}
		return nil, nil
	}
	return c, nil
}

func (c *Confirm) View(width, _ int) string {
	s := c.env.Styles
	no := s.Button.Render(c.env.Msg.Confirm.No)
	yes := s.Button.Render(c.env.Msg.Confirm.Yes)
	if c.yes {
		yes = s.ButtonActive.Render(c.env.Msg.Confirm.Yes)
	} else {
		no = s.ButtonActive.Render(c.env.Msg.Confirm.No)
	}
	inner := min(64, width-6)
	body := lipgloss.NewStyle().Width(inner).Render(c.body)
	buttons := lipgloss.PlaceHorizontal(inner, lipgloss.Right, no+"  "+yes)
	return s.Modal.Render(s.ModalTitle.Render(c.title) + "\n\n" + body + "\n\n" + buttons)
}

// Help lists the top screen's key groups and the global keys. When the
// list is taller than the terminal, the key groups scroll (j/k, pgup/pgdn).
type Help struct {
	env    *Env
	groups []common.Group

	off    int // first visible line of the key groups
	maxOff int // largest off for the height of the last View
	page   int // visible lines of the key groups in the last View
}

// NewHelp builds the help overlay from the screen and global key maps;
// extra groups follow the global keys.
func NewHelp(env *Env, screen common.KeyMap, global []common.Binding, extra ...common.Group) *Help {
	var groups []common.Group
	if len(screen.Groups) > 0 {
		groups = append(groups, screen.Groups...)
	} else if len(screen.Short) > 0 {
		groups = append(groups, common.Group{Title: env.Msg.Help.Screen, Bindings: screen.Short})
	}
	groups = append(groups, common.Group{Title: env.Msg.Help.Global, Bindings: global})
	groups = append(groups, extra...)
	return &Help{env: env, groups: groups}
}

func (h *Help) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "esc", "?", "q", "enter":
			return nil, nil
		case "j", "down":
			h.off = min(h.off+1, h.maxOff)
		case "k", "up":
			h.off = max(h.off-1, 0)
		case "pgdown", "ctrl+d", "space":
			h.off = min(h.off+max(1, h.page-1), h.maxOff)
		case "pgup", "ctrl+u":
			h.off = max(h.off-max(1, h.page-1), 0)
		case "home", "g":
			h.off = 0
		case "end", "G":
			h.off = h.maxOff
		}
	}
	return h, nil
}

// helpChrome is the number of terminal rows kept free around the box's
// scrolling content: the border (2) and two free rows of the screen.
const helpChrome = 4

func (h *Help) View(width, height int) string {
	s := h.env.Styles
	var lines []string
	for gi, g := range h.groups {
		if gi > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, s.Muted.Render(strings.ToUpper(g.Title)))
		keyW := 0
		for _, bd := range g.Bindings {
			if bd.Visible() {
				keyW = max(keyW, lipgloss.Width(bd.Help().Key))
			}
		}
		for _, bd := range g.Bindings {
			if !bd.Visible() {
				continue
			}
			hk := bd.Help()
			lines = append(lines, s.Key.Render(hk.Key)+strings.Repeat(" ", keyW-lipgloss.Width(hk.Key)+2)+hk.Desc)
		}
	}
	// Fixed rows: title, blank, blank, language line, close line.
	h.page = max(3, height-helpChrome-5)
	total := len(lines)
	h.maxOff = max(0, total-h.page)
	h.off = min(h.off, h.maxOff)
	closeLine := s.Muted.Render(h.env.Msg.Help.Close)
	if h.maxOff > 0 {
		lines = lines[h.off : h.off+h.page]
		closeLine = s.Muted.Render(h.env.Msg.Help.Close + " · " + h.env.Msg.Help.Scroll + " " +
			strconv.Itoa(h.off+1) + "–" + strconv.Itoa(h.off+h.page) + "/" + strconv.Itoa(total))
	}
	var b strings.Builder
	b.WriteString(s.ModalTitle.Render(h.env.Msg.Help.Title) + "\n\n")
	b.WriteString(strings.Join(lines, "\n"))
	b.WriteString("\n\n" + s.Muted.Render(h.env.Msg.Help.LanguageLine))
	b.WriteString("\n" + closeLine)
	return s.Modal.MaxWidth(width - 2).Render(b.String())
}
