package common

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/4aykovski/glview/internal/cache"
)

func TestFetchForceAndSWR(t *testing.T) {
	c := cache.New()
	ctx := context.Background()
	var n atomic.Int32
	fn := func(context.Context) (int, error) { return int(n.Add(1)), nil }

	v, stale, err := Fetch(ctx, c, "k", time.Hour, false, fn)
	if err != nil || v != 1 || stale {
		t.Fatalf("miss: %d %v %v", v, stale, err)
	}
	if v, _, _ = Fetch(ctx, c, "k", time.Hour, false, fn); v != 1 || n.Load() != 1 {
		t.Fatalf("hit must not fetch: v=%d calls=%d", v, n.Load())
	}
	if v, _, _ = Fetch(ctx, c, "k", time.Hour, true, fn); v != 2 {
		t.Fatalf("force must fetch: %d", v)
	}
	if v, _, _ = Fetch(ctx, c, "k", time.Hour, false, fn); v != 2 || n.Load() != 2 {
		t.Fatalf("forced value must be stored: v=%d calls=%d", v, n.Load())
	}
}

func TestFetchWithoutCache(t *testing.T) {
	var n int
	fn := func(context.Context) (int, error) { n++; return n, nil }
	for want := 1; want <= 2; want++ {
		if v, _, _ := Fetch(context.Background(), nil, "k", time.Hour, false, fn); v != want {
			t.Fatalf("nil cache must always fetch: %d", v)
		}
	}
	c := cache.New()
	Fetch(context.Background(), c, "k", 0, false, fn)
	Fetch(context.Background(), c, "k", 0, false, fn)
	if n != 4 {
		t.Fatalf("ttl 0 must not cache, calls = %d", n)
	}
}

func TestPollInterval(t *testing.T) {
	cases := []struct {
		base    time.Duration
		focused bool
		want    time.Duration
	}{
		{3 * time.Second, true, 3 * time.Second},
		{3 * time.Second, false, 15 * time.Second},
		{30 * time.Second, false, 150 * time.Second},
		{2 * time.Minute, false, 5 * time.Minute},   // capped
		{10 * time.Minute, false, 10 * time.Minute}, // never faster than base
	}
	for _, c := range cases {
		if got := PollInterval(c.base, c.focused); got != c.want {
			t.Errorf("PollInterval(%v, %v) = %v, want %v", c.base, c.focused, got, c.want)
		}
	}
}
