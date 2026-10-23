package issueactions

import "github.com/4aykovski/glview/internal/ui/common"

// RenderCache keeps the markdown renders of the preview of one issue, so
// that View does not run glamour on every frame. It holds the renders of
// the issue it was used for last: another issue clears it.
type RenderCache struct {
	iid     int64
	entries map[renderKey]string
	// Renders counts the real glamour runs (for tests).
	Renders int
}

type renderKey struct {
	s    *common.Styles
	w    int
	text string
}

// Reset forgets every render (the theme changed).
func (c *RenderCache) Reset() {
	if c != nil {
		c.entries = nil
	}
}

// use starts working for issue iid.
func (c *RenderCache) use(iid int64) {
	if c == nil {
		return
	}
	if c.iid != iid || c.entries == nil {
		c.iid, c.entries = iid, map[renderKey]string{}
	}
}

// markdown renders md of width w, from the cache when it is there. A nil
// cache renders every time.
func (c *RenderCache) markdown(s *common.Styles, md string, w int) string {
	if c == nil {
		return common.Markdown(s, md, w)
	}
	k := renderKey{s, w, md}
	if out, ok := c.entries[k]; ok {
		return out
	}
	c.Renders++
	out := common.Markdown(s, md, w)
	c.entries[k] = out
	return out
}
