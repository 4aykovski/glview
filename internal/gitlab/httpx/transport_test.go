package httpx

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/4aykovski/glview/internal/gitlab"
)

func TestRetryOn429WithRetryAfter(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	hc, err := NewClient(Options{})
	if err != nil {
		t.Fatal(err)
	}
	var slept time.Duration
	hc.Transport.(*Transport).Sleep = func(_ context.Context, d time.Duration) error { slept = d; return nil }

	resp, err := hc.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 || hits.Load() != 2 {
		t.Fatalf("status %d after %d hits", resp.StatusCode, hits.Load())
	}
	if slept != 2*time.Second {
		t.Fatalf("slept %v, want 2s", slept)
	}
}

func TestSecond429IsReturned(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	hc, _ := NewClient(Options{})
	hc.Transport.(*Transport).Sleep = func(context.Context, time.Duration) error { return nil }

	resp, err := hc.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests || hits.Load() != 2 {
		t.Fatalf("status %d hits %d, want 429 after exactly one retry", resp.StatusCode, hits.Load())
	}
}

func TestLongRetryAfterNotWaited(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	hc, _ := NewClient(Options{MaxRetryWait: 30 * time.Second})
	resp, err := hc.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if hits.Load() != 1 {
		t.Fatalf("must not retry when Retry-After exceeds the limit, hits=%d", hits.Load())
	}
}

func TestRetryReplaysBody(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if hits.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write(body)
	}))
	defer srv.Close()
	hc, _ := NewClient(Options{})
	hc.Transport.(*Transport).Sleep = func(context.Context, time.Duration) error { return nil }
	resp, err := hc.Post(srv.URL, "application/json", strings.NewReader(`{"q":1}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	if string(got) != `{"q":1}` {
		t.Fatalf("body on retry = %q", got)
	}
}

func TestConcurrencyLimit(t *testing.T) {
	var cur, peak atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := cur.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(30 * time.Millisecond)
		cur.Add(-1)
	}))
	defer srv.Close()
	hc, _ := NewClient(Options{MaxConcurrent: 3})
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if resp, err := hc.Get(srv.URL); err == nil {
				resp.Body.Close()
			}
		}()
	}
	wg.Wait()
	if peak.Load() > 3 {
		t.Fatalf("peak concurrency %d > 3", peak.Load())
	}
}

func TestUserAgent(t *testing.T) {
	var ua string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { ua = r.UserAgent() }))
	defer srv.Close()
	hc, _ := NewClient(Options{UserAgent: "glview/test"})
	resp, err := hc.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if ua != "glview/test" {
		t.Fatalf("ua = %q", ua)
	}
}

func TestTLSErrorMapped(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	hc, _ := NewClient(Options{})
	_, err := hc.Get(srv.URL)
	if !errors.Is(FromTransport(err), gitlab.ErrTLS) {
		t.Fatalf("want ErrTLS, got %v", FromTransport(err))
	}

	insecure, _ := NewClient(Options{InsecureSkipVerify: true})
	resp, err := insecure.Get(srv.URL)
	if err != nil {
		t.Fatalf("insecure_skip_verify must accept self-signed cert: %v", err)
	}
	resp.Body.Close()
}

func TestBadCAFile(t *testing.T) {
	if _, err := NewClient(Options{CAFile: "/nope/ca.pem"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestRetryAfterParsing(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	h := http.Header{}
	h.Set("Retry-After", now.Add(5*time.Second).Format(http.TimeFormat))
	if d, ok := RetryAfter(h, now); !ok || d != 5*time.Second {
		t.Fatalf("http date: %v %v", d, ok)
	}
	h.Set("Retry-After", "abc")
	if _, ok := RetryAfter(h, now); ok {
		t.Fatal("garbage must not parse")
	}
}

func TestMonitorSeesOutageAndRecovery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) }))
	mon := gitlab.NewMonitor()
	hc, err := NewClient(Options{Monitor: mon, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	get := func(url string) {
		resp, err := hc.Get(url)
		if err == nil {
			resp.Body.Close()
		}
	}
	get(srv.URL) // an HTTP error is an answer: online
	if mon.Offline() {
		t.Fatal("a 502 is not an outage")
	}
	addr := srv.URL
	srv.Close() // connection refused
	get(addr)
	get(addr)
	if !mon.Offline() || mon.Fails() != 2 {
		t.Fatalf("fails = %d", mon.Fails())
	}
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv2.Close()
	get(srv2.URL)
	if mon.Offline() {
		t.Fatal("an answer ends the outage")
	}
	// A canceled request says nothing.
	get(addr)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv2.URL, nil)
	_, _ = hc.Do(req)
	if mon.Fails() != 1 {
		t.Fatalf("a canceled request must not count: fails = %d", mon.Fails())
	}
}
