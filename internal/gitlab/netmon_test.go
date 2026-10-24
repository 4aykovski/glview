package gitlab

import (
	"errors"
	"testing"
	"time"
)

func netErr() error  { return &Error{Kind: ErrNetwork, Err: errors.New("dial tcp: no route")} }
func tlsErr() error  { return &Error{Kind: ErrTLS} }
func httpErr() error { return &Error{Kind: ErrServer, Status: 502} }
func recv(m *Monitor) (bool, bool) {
	select {
	case v := <-m.Changes():
		return v, true
	default:
		return false, false
	}
}

func TestMonitorOfflineAndRecovery(t *testing.T) {
	m := NewMonitor()
	if m.Offline() {
		t.Fatal("starts online")
	}
	m.Report(nil)
	if _, ok := recv(m); ok {
		t.Fatal("online -> online is not a change")
	}
	m.Report(netErr())
	m.Report(netErr())
	if !m.Offline() || m.Fails() != 2 {
		t.Fatalf("fails = %d", m.Fails())
	}
	if online, ok := recv(m); !ok || online {
		t.Fatalf("one offline notification expected, got %v %v", online, ok)
	}
	if _, ok := recv(m); ok {
		t.Fatal("a second failure must not notify again")
	}
	m.Report(nil)
	if m.Offline() {
		t.Fatal("a success clears the outage")
	}
	if online, ok := recv(m); !ok || !online {
		t.Fatal("recovery notification expected")
	}
	m.Report(nil)
	if _, ok := recv(m); ok {
		t.Fatal("recovery is announced once")
	}
}

func TestMonitorIgnoresOtherErrors(t *testing.T) {
	m := NewMonitor()
	m.Report(netErr())
	for _, err := range []error{tlsErr(), httpErr(), errors.New("decode")} {
		m.Report(err)
	}
	if m.Fails() != 1 {
		t.Fatalf("only network errors count: fails = %d", m.Fails())
	}
}

func TestMonitorBackoff(t *testing.T) {
	m := NewMonitor()
	base := 3 * time.Second
	if got := m.Backoff(base); got != base {
		t.Fatalf("online: %v", got)
	}
	var seen []time.Duration
	for range 7 {
		m.Report(netErr())
		seen = append(seen, m.Backoff(base))
	}
	want := []time.Duration{6, 12, 24, 48, 60, 60, 60}
	for i, w := range want {
		if seen[i] != w*time.Second {
			t.Fatalf("delays = %v, want 6s 12s 24s 48s 60s 60s 60s", seen)
		}
	}
	if got := m.Backoff(2 * time.Minute); got != 2*time.Minute {
		t.Fatalf("a base above the cap stays: %v", got)
	}
}

func TestNilMonitorIsOnline(t *testing.T) {
	var m *Monitor
	m.Report(netErr())
	if m.Offline() || m.Fails() != 0 || m.Changes() != nil || m.Backoff(time.Second) != time.Second {
		t.Fatal("nil monitor must be inert")
	}
}
