package common

import (
	"context"
	"time"

	"github.com/4aykovski/glview/internal/cache"
)

// Fetch loads a value through the cache.
//
// Without force it is stale-while-revalidate: a cached value (possibly
// stale — then a background refresh starts and the root announces it with
// CacheRevalidatedMsg) is returned at once, a miss fetches. With force —
// refresh with r and poll ticks — the network is always hit and the result
// is stored. A nil cache or ttl <= 0 means no caching.
func Fetch[T any](ctx context.Context, c *cache.Cache, key string, ttl time.Duration, force bool, fn func(context.Context) (T, error)) (v T, stale bool, err error) {
	if c == nil || ttl <= 0 {
		v, err = fn(ctx)
		return v, false, err
	}
	if force {
		v, err = fn(ctx)
		if err == nil {
			c.Set(key, v)
		}
		return v, false, err
	}
	return cache.Get(ctx, c, key, ttl, fn)
}
