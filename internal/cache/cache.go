// Package cache is an in-memory TTL cache with stale-while-revalidate: a
// stale value is returned immediately while exactly one background fetch
// refreshes it. Keys refreshed in the background are announced on
// Revalidated so the UI can re-render.
package cache

import (
	"context"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// BackgroundTimeout bounds background revalidation fetches.
const BackgroundTimeout = 30 * time.Second

type entry struct {
	value any
	at    time.Time
}

// Cache is safe for concurrent use.
type Cache struct {
	mu      sync.Mutex
	entries map[string]entry
	epochs  map[string]uint64 // bumped on invalidation; stale fetches are discarded
	busy    map[string]bool   // keys with a background refresh started and not finished
	sf      singleflight.Group
	updates chan string

	now func() time.Time
}

// New returns an empty cache.
func New() *Cache {
	return &Cache{
		entries: map[string]entry{},
		epochs:  map[string]uint64{},
		busy:    map[string]bool{},
		updates: make(chan string, 64),
		now:     time.Now,
	}
}

// Revalidated yields keys whose value was refreshed in the background.
// Sends are non-blocking: if nobody reads, notifications are dropped.
func (c *Cache) Revalidated() <-chan string { return c.updates }

// Get returns the value for key:
//   - fresh hit → cached value, stale=false;
//   - stale hit → cached value, stale=true, one background refresh starts;
//   - miss → fetches synchronously (concurrent callers share one fetch).
//
// ttl <= 0 disables caching for this call (always fetch, never store).
func Get[T any](ctx context.Context, c *Cache, key string, ttl time.Duration, fetch func(context.Context) (T, error)) (T, bool, error) {
	if ttl <= 0 {
		v, err := fetch(ctx)
		return v, false, err
	}

	c.mu.Lock()
	e, ok := c.entries[key]
	epoch, known := c.epochs[key]
	if !known {
		c.epochs[key] = 0 // register so Invalidate can see in-flight fetches
	}
	fresh := ok && c.now().Sub(e.at) < ttl
	// One background refresh per stale value: a goroutine started late must
	// not fetch again after the first one finished (singleflight only joins
	// calls that overlap).
	refresh := ok && !fresh && !c.busy[key]
	if refresh {
		c.busy[key] = true
	}
	c.mu.Unlock()

	if ok {
		v, _ := e.value.(T)
		if fresh {
			return v, false, nil
		}
		if refresh {
			go c.revalidate(key, epoch, func(ctx context.Context) (any, error) { return fetch(ctx) })
		}
		return v, true, nil
	}

	res, err, _ := c.sf.Do(key, func() (any, error) {
		v, err := fetch(ctx)
		if err != nil {
			return nil, err
		}
		c.store(key, epoch, v)
		return v, nil
	})
	if err != nil {
		var zero T
		return zero, false, err
	}
	v, _ := res.(T)
	return v, false, nil
}

func (c *Cache) revalidate(key string, epoch uint64, fetch func(context.Context) (any, error)) {
	defer func() {
		c.mu.Lock()
		delete(c.busy, key)
		c.mu.Unlock()
	}()
	_, _, _ = c.sf.Do(key, func() (any, error) {
		ctx, cancel := context.WithTimeout(context.Background(), BackgroundTimeout)
		defer cancel()
		v, err := fetch(ctx)
		if err != nil {
			return nil, err
		}
		if c.store(key, epoch, v) {
			select {
			case c.updates <- key:
			default:
			}
		}
		return v, nil
	})
}

// store saves v unless the key was invalidated since the fetch started.
func (c *Cache) store(key string, epoch uint64, v any) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.epochs[key] != epoch {
		return false
	}
	c.entries[key] = entry{value: v, at: c.now()}
	return true
}

// Set stores a value directly (e.g. the result of a mutation).
func (c *Cache) Set(key string, v any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = entry{value: v, at: c.now()}
}

// Invalidate drops every key with the given prefix ("mr:platform/x:479",
// "mrs:platform/x:"). In-flight fetches for those keys will not be stored.
func (c *Cache) Invalidate(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k := range c.entries {
		if strings.HasPrefix(k, prefix) {
			delete(c.entries, k)
		}
	}
	for k := range c.epochs {
		if strings.HasPrefix(k, prefix) {
			c.epochs[k]++
		}
	}
}

// Lookup returns the cached value of key if it is younger than ttl,
// without fetching or revalidating.
func Lookup[T any](c *Cache, key string, ttl time.Duration) (T, bool) {
	var zero T
	if c == nil {
		return zero, false
	}
	c.mu.Lock()
	e, ok := c.entries[key]
	c.mu.Unlock()
	if !ok || c.now().Sub(e.at) >= ttl {
		return zero, false
	}
	v, ok := e.value.(T)
	return v, ok
}
