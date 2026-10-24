package app

// Stash returns the value kept under key for the session, nil when none.
// Screens keep their drafts here (memory only, never on disk).
func (e *Env) Stash(key string) any { return e.stash[key] }

// SetStash keeps v under key; nil forgets it.
func (e *Env) SetStash(key string, v any) {
	if v == nil {
		delete(e.stash, key)
		return
	}
	if e.stash == nil {
		e.stash = map[string]any{}
	}
	e.stash[key] = v
}

// DefaultYes preselects "Yes": for a question whose usual answer is yes.
func (c *Confirm) DefaultYes() *Confirm {
	c.yes = true
	return c
}
