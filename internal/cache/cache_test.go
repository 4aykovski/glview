package cache

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newTestCache() (*Cache, *clock) {
	clk := &clock{t: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)}
	c := New()
	c.now = clk.now
	return c, clk
}

func counter(calls *atomic.Int32, val func(n int32) string) func(context.Context) (string, error) {
	return func(context.Context) (string, error) {
		n := calls.Add(1)
		return val(n), nil
	}
}

func TestFreshHitDoesNotFetch(t *testing.T) {
	c, _ := newTestCache()
	var calls atomic.Int32
	fetch := counter(&calls, func(n int32) string { return fmt.Sprint("v", n) })
	ctx := context.Background()

	v, stale, err := Get(ctx, c, "k", 30*time.Second, fetch)
	if err != nil || v != "v1" || stale {
		t.Fatalf("miss: %q %v %v", v, stale, err)
	}
	v, stale, _ = Get(ctx, c, "k", 30*time.Second, fetch)
	if v != "v1" || stale || calls.Load() != 1 {
		t.Fatalf("hit: %q stale=%v calls=%d", v, stale, calls.Load())
	}
}

func TestStaleReturnsOldAndRevalidatesOnce(t *testing.T) {
	c, clk := newTestCache()
	var calls atomic.Int32
	release := make(chan struct{})
	fetch := func(context.Context) (string, error) {
		n := calls.Add(1)
		if n > 1 {
			<-release
		}
		return fmt.Sprint("v", n), nil
	}
	ctx := context.Background()
	_, _, _ = Get(ctx, c, "k", 30*time.Second, fetch)
	clk.add(40 * time.Second)

	for range 5 {
		v, stale, err := Get(ctx, c, "k", 30*time.Second, fetch)
		if err != nil || v != "v1" || !stale {
			t.Fatalf("stale read: %q %v %v", v, stale, err)
		}
	}
	close(release)

	select {
	case key := <-c.Revalidated():
		if key != "k" {
			t.Fatalf("revalidated %q", key)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no revalidation notice")
	}
	if n := calls.Load(); n != 2 {
		t.Fatalf("background fetches = %d, want exactly 1 (total 2)", n)
	}
	v, stale, _ := Get(ctx, c, "k", 30*time.Second, fetch)
	if v != "v2" || stale {
		t.Fatalf("after revalidation: %q stale=%v", v, stale)
	}
}

func TestMissDeduplicatesConcurrentFetches(t *testing.T) {
	c, _ := newTestCache()
	var calls atomic.Int32
	fetch := func(context.Context) (string, error) {
		calls.Add(1)
		time.Sleep(50 * time.Millisecond)
		return "v", nil
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _ = Get(context.Background(), c, "k", time.Minute, fetch)
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("calls = %d", calls.Load())
	}
}

func TestErrorNotCached(t *testing.T) {
	c, _ := newTestCache()
	boom := errors.New("boom")
	if _, _, err := Get(context.Background(), c, "k", time.Minute, func(context.Context) (string, error) { return "", boom }); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	v, _, err := Get(context.Background(), c, "k", time.Minute, func(context.Context) (string, error) { return "ok", nil })
	if err != nil || v != "ok" {
		t.Fatalf("after error: %q %v", v, err)
	}
}

func TestInvalidateByPrefix(t *testing.T) {
	c, _ := newTestCache()
	var calls atomic.Int32
	fetch := counter(&calls, func(n int32) string { return fmt.Sprint("v", n) })
	ctx := context.Background()
	for _, k := range []string{"mrs:a/b:opened", "mr:a/b:479", "mrs:c/d:opened"} {
		_, _, _ = Get(ctx, c, k, time.Minute, fetch)
	}
	c.Invalidate("mrs:a/b:")
	calls.Store(0)

	_, _, _ = Get(ctx, c, "mrs:a/b:opened", time.Minute, fetch)
	_, _, _ = Get(ctx, c, "mrs:c/d:opened", time.Minute, fetch)
	_, _, _ = Get(ctx, c, "mr:a/b:479", time.Minute, fetch)
	if calls.Load() != 1 {
		t.Fatalf("only the invalidated key must be refetched, calls=%d", calls.Load())
	}
}

func TestInvalidateDuringFetchDiscardsResult(t *testing.T) {
	c, _ := newTestCache()
	started := make(chan struct{})
	release := make(chan struct{})
	go func() {
		_, _, _ = Get(context.Background(), c, "mr:a/b:1", time.Minute, func(context.Context) (string, error) {
			close(started)
			<-release
			return "old", nil
		})
	}()
	<-started
	c.Invalidate("mr:a/b:")
	close(release)
	time.Sleep(20 * time.Millisecond)

	v, _, _ := Get(context.Background(), c, "mr:a/b:1", time.Minute, func(context.Context) (string, error) { return "new", nil })
	if v != "new" {
		t.Fatalf("fetch started before invalidation must not be stored, got %q", v)
	}
}

func TestZeroTTLBypasses(t *testing.T) {
	c, _ := newTestCache()
	var calls atomic.Int32
	fetch := counter(&calls, func(int32) string { return "v" })
	for range 3 {
		_, _, _ = Get(context.Background(), c, "running", 0, fetch)
	}
	if calls.Load() != 3 {
		t.Fatalf("calls = %d", calls.Load())
	}
}

func TestDiskRoundTripAndAge(t *testing.T) {
	clk := &clock{t: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)}
	d := Disk{Dir: filepath.Join(t.TempDir(), "cache"), now: clk.now}
	if err := d.Save("starred.json", []string{"a/b", "c/d"}); err != nil {
		t.Fatal(err)
	}
	var got []string
	if !d.Load("starred.json", &got, time.Hour) || len(got) != 2 {
		t.Fatalf("load: %v", got)
	}
	clk.add(2 * time.Hour)
	if d.Load("starred.json", &got, time.Hour) {
		t.Fatal("expired snapshot must be a miss")
	}
	if !d.Load("starred.json", &got, 0) {
		t.Fatal("maxAge 0 accepts any age")
	}
	info, _ := os.Stat(filepath.Join(d.Dir, "starred.json"))
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("perm %o", info.Mode().Perm())
	}
	if d.Load("missing.json", &got, 0) {
		t.Fatal("missing file must be a miss")
	}
}

const (
	hostA = "https://gitlab.example.com"
	hostB = "https://gitlab.example.org"
)

func TestStateRecent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "state.json")
	s := LoadState(path)
	for i := range 25 {
		s.Touch(hostA, fmt.Sprintf("g/p%d", i))
	}
	s.Touch(hostA, "g/p20")
	if r := s.Recent(hostA); len(r) != RecentLimit || r[0] != "g/p20" || r[1] != "g/p24" {
		t.Fatalf("recent = %v", r)
	}
	if err := SaveState(path, s); err != nil {
		t.Fatal(err)
	}
	if r := LoadState(path).Recent(hostA); len(r) != RecentLimit || r[0] != "g/p20" {
		t.Fatalf("reload = %v", r)
	}
}

func TestStateRecentPerHost(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	var s State
	s.Touch(hostA, "platform/route-service")
	s.Touch(hostB, "qa/e2e-routes")
	if err := SaveState(path, s); err != nil {
		t.Fatal(err)
	}
	got := LoadState(path)
	if r := got.Recent(hostA); len(r) != 1 || r[0] != "platform/route-service" {
		t.Fatalf("host A = %v", r)
	}
	if r := got.Recent(hostB); len(r) != 1 || r[0] != "qa/e2e-routes" {
		t.Fatalf("host B = %v", r)
	}
	if r := got.Recent("https://gitlab.example.net"); len(r) != 0 {
		t.Fatalf("unknown host = %v", r)
	}
}

func TestStateOldFormatIsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(`{"recent":["platform/route-service","qa/e2e-routes"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s := LoadState(path)
	if r := s.Recent(hostA); len(r) != 0 {
		t.Fatalf("a host-less list is not shown for a host: %v", r)
	}
	s.Touch(hostA, "infra/router-configs")
	if err := SaveState(path, s); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "qa/e2e-routes") {
		t.Fatalf("the old field is dropped on save:\n%s", data)
	}
}

func TestStateWorkspaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	at := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	var s State
	s.Touch(hostA, "qa/e2e-routes")
	s.SetExpanded(hostA, "~/src", "mrs", []string{"platform/route-service", "qa/e2e-routes"}, at)
	s.SetExpanded(hostA, "~/src", "pipelines", []string{"infra/router-configs"}, at)
	s.SetExpanded(hostA, "group: platform", "mrs", []string{"platform/routing-engine"}, at)
	s.SetExpanded(hostB, "~/src", "mrs", []string{"other/project"}, at)
	if err := SaveState(path, s); err != nil {
		t.Fatal(err)
	}
	got := LoadState(path)
	if e := got.Expanded(hostA, "~/src", "mrs"); len(e) != 2 || e[0] != "platform/route-service" {
		t.Fatalf("mrs = %v", e)
	}
	if e := got.Expanded(hostA, "~/src", "pipelines"); len(e) != 1 || e[0] != "infra/router-configs" {
		t.Fatalf("pipelines = %v", e)
	}
	if e := got.Expanded(hostA, "group: platform", "pipelines"); len(e) != 0 {
		t.Fatalf("group pipelines = %v", e)
	}
	if e := got.Expanded(hostB, "~/src", "mrs"); len(e) != 1 || e[0] != "other/project" {
		t.Fatalf("host B = %v", e)
	}
	if r := got.Recent(hostA); len(r) != 1 {
		t.Fatalf("recent survives: %v", r)
	}
	// Collapsing everything keeps the record.
	got.SetExpanded(hostA, "~/src", "mrs", nil, at.Add(time.Hour))
	if e := got.Expanded(hostA, "~/src", "mrs"); len(e) != 0 || !got.HasWorkspace(hostA, "~/src") {
		t.Fatalf("mrs after collapse = %v", e)
	}
	if !got.TouchWorkspace(hostA, "~/src", at.Add(2*time.Hour)) || got.TouchWorkspace(hostA, "~/other", at) {
		t.Fatal("touch marks only known workspaces")
	}
	// A clone does not share the lists.
	c := got.Clone()
	c.Hosts[hostA].Workspaces["~/src"].Expanded["pipelines"][0] = "changed"
	if got.Expanded(hostA, "~/src", "pipelines")[0] != "infra/router-configs" {
		t.Fatal("clone is deep")
	}
}

func TestStateWorkspaceLimit(t *testing.T) {
	at := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	var s State
	for i := range WorkspaceLimit + 5 {
		s.SetExpanded(hostA, fmt.Sprintf("~/ws%d", i), "mrs", []string{"g/p"}, at.Add(time.Duration(i)*time.Minute))
	}
	// ws5 is opened again: it is now the most recent one.
	s.TouchWorkspace(hostA, "~/ws5", at.Add(time.Hour))
	s.SetExpanded(hostA, "~/new", "mrs", []string{"g/p"}, at.Add(2*time.Hour))
	ws := s.Hosts[hostA].Workspaces
	if len(ws) != WorkspaceLimit {
		t.Fatalf("workspaces = %d", len(ws))
	}
	for _, gone := range []string{"~/ws0", "~/ws4", "~/ws6"} {
		if _, ok := ws[gone]; ok {
			t.Fatalf("%s should be dropped", gone)
		}
	}
	for _, kept := range []string{"~/ws5", "~/new", "~/ws54"} {
		if _, ok := ws[kept]; !ok {
			t.Fatalf("%s should be kept", kept)
		}
	}
}

func TestStateCorruptIsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	for _, data := range []string{`{"hosts":`, `{"hosts":{"x":{"workspaces":{"~/src":{"expanded":{"mrs":"not a list"}}}}}}`, `[]`} {
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		s := LoadState(path)
		if len(s.Hosts) != 0 {
			t.Fatalf("%s: state = %+v", data, s)
		}
	}
}
