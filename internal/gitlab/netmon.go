package gitlab

import (
	"errors"
	"sync"
	"time"
)

// Monitor tracks whether the host is reachable, from the outcome of every
// request (SPEC §11.1, "Офлайн"). The HTTP transport reports into it; the UI
// reads it to slow polling down, mark the header and announce the recovery.
// A nil *Monitor is valid and always online.
type Monitor struct {
	mu      sync.Mutex
	fails   int
	changes chan bool
}

// NewMonitor returns a Monitor that is online.
func NewMonitor() *Monitor { return &Monitor{changes: make(chan bool, 8)} }

// Report records the outcome of one request: nil err (any HTTP answer counts)
// or a *Error of kind ErrNetwork. Other errors say nothing about
// reachability and are ignored.
func (m *Monitor) Report(err error) {
	if m == nil {
		return
	}
	var ge *Error
	if err != nil && (!errors.As(err, &ge) || ge.Kind != ErrNetwork) {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	switch {
	case err == nil && m.fails > 0:
		m.fails = 0
		m.notify(true)
	case err == nil:
	default:
		m.fails++
		if m.fails == 1 {
			m.notify(false)
		}
	}
}

func (m *Monitor) notify(online bool) {
	select {
	case m.changes <- online:
	default:
	}
}

// Offline reports whether the last request failed at the network level.
func (m *Monitor) Offline() bool { return m.Fails() > 0 }

// Fails is the number of consecutive network failures.
func (m *Monitor) Fails() int {
	if m == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.fails
}

// Changes delivers true when the host is reachable again after an outage
// and false when an outage starts. Nil for a nil Monitor.
func (m *Monitor) Changes() <-chan bool {
	if m == nil {
		return nil
	}
	return m.changes
}

// MaxOfflineDelay is the longest delay between polls while offline.
const MaxOfflineDelay = 60 * time.Second

// Backoff is base stretched exponentially by the number of consecutive
// network failures (base×2, ×4, …), up to MaxOfflineDelay but never below
// base itself.
func (m *Monitor) Backoff(base time.Duration) time.Duration {
	n := m.Fails()
	if n == 0 {
		return base
	}
	d := base
	for i := 0; i < n && d < MaxOfflineDelay; i++ {
		d *= 2
	}
	return max(base, min(d, MaxOfflineDelay))
}
