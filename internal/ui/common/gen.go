package common

import (
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
)

var genIDs atomic.Uint64

// Token identifies one request generation of one screen instance. Responses
// carry the token they were issued with; a screen drops responses whose
// token is no longer current. The instance ID makes tokens unique even when
// two screens of the same type are on the stack.
type Token struct{ ID, N uint64 }

// Gen is a per-screen generation counter.
type Gen struct {
	id, n uint64
}

// NewGen returns a counter with a process-unique instance ID.
func NewGen() Gen { return Gen{id: genIDs.Add(1)} }

// Next starts a new generation (filter changed, refresh, tab switch…) and
// returns its token. Earlier tokens become invalid.
func (g *Gen) Next() Token {
	g.n++
	return Token{ID: g.id, N: g.n}
}

// Current returns the token of the current generation.
func (g Gen) Current() Token { return Token{ID: g.id, N: g.n} }

// Valid reports whether t belongs to this counter's current generation.
func (g Gen) Valid(t Token) bool { return t.ID == g.id && t.N == g.n }

// Owns reports whether t was issued by this counter (any generation).
func (g Gen) Owns(t Token) bool { return t.ID == g.id }

// PollTickMsg asks the screen that owns Token to refresh.
type PollTickMsg struct{ Token Token }

// PollAfter schedules a PollTickMsg for t after d.
func PollAfter(d time.Duration, t Token) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return PollTickMsg{Token: t} })
}

// maxBlurredPoll caps the slowed-down interval of an unfocused terminal.
const maxBlurredPoll = 5 * time.Minute

// PollInterval is base while the terminal is focused, and base×5 (but not
// rarer than every 5 minutes) while it is not.
func PollInterval(base time.Duration, focused bool) time.Duration {
	if focused {
		return base
	}
	return max(base, min(base*5, maxBlurredPoll))
}
