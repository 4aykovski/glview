package common

import (
	"charm.land/bubbles/v2/key"
)

// Footer priorities: higher survives narrower terminals.
const (
	PriorityLinks  = 5 // L (MR links): last to claim room, so the o/y hints stay
	PriorityLow    = 10
	PriorityNormal = 50
	PriorityHigh   = 80
	PriorityHelp   = 1000 // "?" is always shown
)

// Binding is a key binding with a footer priority. Hidden bindings still
// work as input but are never shown (e.g. glab actions without glab).
type Binding struct {
	key.Binding
	Priority int
	Hidden   bool
}

// NewBinding creates a binding for keys with a help label and priority.
// helpKey is what the footer shows ("j/k"); empty means the first key.
func NewBinding(keys []string, helpKey, desc string, priority int) Binding {
	if helpKey == "" && len(keys) > 0 {
		helpKey = keys[0]
	}
	return Binding{
		Binding:  key.NewBinding(key.WithKeys(keys...), key.WithHelp(helpKey, desc)),
		Priority: priority,
	}
}

// Visible reports whether the binding should appear in footer and help.
func (b Binding) Visible() bool { return !b.Hidden && b.Enabled() }

// Group is a titled set of bindings for the help overlay.
type Group struct {
	Title    string
	Bindings []Binding
}

// KeyMap is what a screen exposes for the footer (Short) and help (Groups).
type KeyMap struct {
	Short  []Binding
	Groups []Group
}
