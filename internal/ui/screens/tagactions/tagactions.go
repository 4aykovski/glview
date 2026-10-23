// Package tagactions holds what the project Tags tab and the workspace
// Tags tab share: deleting a tag with a confirm modal, opening the pipeline
// of a tag, the lazy comparison with the default branch, the preview of the
// selected tag and the cache keys of the tag lists (SPEC §3.4, §3.10).
package tagactions

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/semver"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/screens/newtag"
	"github.com/4aykovski/glview/internal/ui/screens/pipeline"
	"github.com/4aykovski/glview/internal/ui/screens/render"
)

const (
	requestTimeout = 15 * time.Second
	// CompareTTL is how long a comparison stays cached.
	CompareTTL = 30 * time.Second
)

// CreatedMsg reports a tag created by the New tag form.
type CreatedMsg = newtag.CreatedMsg

// NewTag opens the New tag form for the project; its ref starts as the
// default branch.
func NewTag(env *app.Env, ref gitlab.ProjectRef, defaultBranch string) tea.Cmd {
	f := newtag.New(env, ref, defaultBranch)
	return tea.Sequence(app.OpenOverlay(f), f.Init())
}

// Created drops the tag caches of the project after a tag was created. The
// caller reloads its list and puts the cursor on the new tag.
func Created(env *app.Env, msg CreatedMsg) { Invalidate(env, msg.Ref) }

// DeletedMsg reports a finished deletion of tag Name.
type DeletedMsg struct {
	Ref       gitlab.ProjectRef
	Name      string
	Protected bool
	Err       error
}

// Delete asks for confirmation and deletes the tag. A protected tag gets
// a warning in the description; GitLab has the last word.
func Delete(env *app.Env, ref gitlab.ProjectRef, t gitlab.Tag) tea.Cmd {
	m := env.Msg.Tag
	body := m.DeleteBody(t.Commit.ShortSHA)
	if t.Protected {
		body += "\n" + m.DeleteProtect
	}
	name, protected, client := t.Name, t.Protected, env.Client
	del := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		err := client.Tags().Delete(ctx, ref, name)
		return DeletedMsg{Ref: ref, Name: name, Protected: protected, Err: err}
	}
	return app.OpenOverlay(app.NewConfirm(env, m.DeleteTitle(name), body, del))
}

// Done drops the tag caches of the project and returns the toast: success,
// "protected" for a 403 on a protected tag, an error otherwise. The caller
// reloads its list on success.
func Done(env *app.Env, msg DeletedMsg) tea.Cmd {
	Invalidate(env, msg.Ref)
	m := env.Msg.Tag
	switch {
	case msg.Err == nil:
		return app.Toast(common.ToastSuccess, m.Deleted(msg.Name), "")
	case msg.Protected && errors.Is(msg.Err, gitlab.ErrForbidden):
		return app.Toast(common.ToastError, m.Protected(msg.Name), "")
	}
	return env.Fail(msg.Err)
}

// Cache keys of the tag data of a project.
func ListKey(path, search, cursor string) string {
	return fmt.Sprintf("tags:%s:%s:%s", path, search, cursor)
}
func ListPrefix(path string) string { return "tags:" + path + ":" }
func PipesKey(path string) string   { return "tagpipes:" + path }
func CompareKey(path, from, to string) string {
	return fmt.Sprintf("compare:%s:%s:%s", path, from, to)
}

// Invalidate drops the cached tag lists, tag pipelines, comparisons and the
// latest tag of the workspace overview of the project; creating or deleting
// a tag changes all of them.
func Invalidate(env *app.Env, ref gitlab.ProjectRef) {
	if env.Cache == nil {
		return
	}
	path := ref.FullPath
	env.Cache.Invalidate(ListPrefix(path))
	env.Cache.Invalidate(PipesKey(path))
	env.Cache.Invalidate("compare:" + path + ":")
	env.Cache.Invalidate("lasttag:" + path)
}

// OpenPipeline opens the Pipeline screen of the tag's pipeline, or tells
// that there is none.
func OpenPipeline(env *app.Env, ref gitlab.ProjectRef, t gitlab.Tag) tea.Cmd {
	if t.Pipeline == nil {
		return app.Toast(common.ToastInfo, env.Msg.Tag.NoPipeline, "")
	}
	return app.Push(pipeline.New(env, ref, t.Pipeline.ID, t.Pipeline))
}

// Copy copies the tag name.
func Copy(env *app.Env, t gitlab.Tag) tea.Cmd {
	return app.Copy(env, t.Name, env.Msg.Tag.WhatName)
}

// TagsURL is the tags page of a project in the web UI, with the search.
func TagsURL(projectURL, search string) string {
	u := projectURL + "/-/tags"
	if search != "" {
		u += "?" + url.Values{"search": {search}}.Encode()
	}
	return u
}

// Suggested is the default name of the version suggestion for the tags.
func Suggested(tags []gitlab.Tag) string {
	names := make([]string, len(tags))
	for i, t := range tags {
		names[i] = t.Name
	}
	s := semver.Suggest(names)
	return s.Options[s.Default].Name
}

// ---- comparison with the default branch ----

// CompareMsg carries the comparison of Tag with Branch. Tok is the token of
// the tab's Comparer.
type CompareMsg struct {
	Tok    common.Token
	Ref    gitlab.ProjectRef
	Tag    string
	Branch string
	Cmp    gitlab.Comparison
	Err    error
}

// Compare requests the comparison tag...branch through the 30-second cache;
// force (r, poll) goes to the network.
func Compare(env *app.Env, tok common.Token, ref gitlab.ProjectRef, tag, branch string, force bool) (tea.Cmd, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	key := CompareKey(ref.FullPath, tag, branch)
	return func() tea.Msg {
		defer cancel()
		cmp, _, err := common.Fetch(ctx, env.Cache, key, CompareTTL, force, func(ctx context.Context) (gitlab.Comparison, error) {
			return env.Client.Tags().Compare(ctx, ref, tag, branch)
		})
		return CompareMsg{Tok: tok, Ref: ref, Tag: tag, Branch: branch, Cmp: cmp, Err: err}
	}, cancel
}

// CompareState is how far the comparison of a tag is.
type CompareState int

const (
	CompareNone    CompareState = iota // not asked (no default branch known)
	CompareLoading                     // in flight
	CompareDone
	CompareFailed
)

// Entry is the comparison of one tag with the default branch.
type Entry struct {
	State CompareState
	Ahead int // commits of the branch after the tag, with CompareDone
}

// Comparer asks the comparison of the selected tag lazily, one request at a
// time: asking for another tag drops the answer still due for the previous
// one.
type Comparer struct {
	gen     common.Gen
	cancel  context.CancelFunc
	entries map[string]Entry
	cur     string // key asked last
}

// NewComparer returns an empty Comparer.
func NewComparer() *Comparer { return &Comparer{gen: common.NewGen(), entries: map[string]Entry{}} }

func entryKey(ref gitlab.ProjectRef, tag string) string { return ref.FullPath + "\x00" + tag }

// Token is the token of the latest request (for tests).
func (c *Comparer) Token() common.Token { return c.gen.Current() }

// Entry returns what is known about the tag of the project.
func (c *Comparer) Entry(ref gitlab.ProjectRef, tag string) (Entry, bool) {
	e, ok := c.entries[entryKey(ref, tag)]
	return e, ok
}

// Reset forgets every answer and drops the one in flight (the tags or the
// branch changed).
func (c *Comparer) Reset() {
	if c.cancel != nil {
		c.cancel()
	}
	c.gen.Next()
	c.entries = map[string]Entry{}
	c.cur = ""
}

// Stop drops the request in flight (the tab is hidden).
func (c *Comparer) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
	c.gen.Next()
	if e, ok := c.entries[c.cur]; ok && e.State == CompareLoading {
		delete(c.entries, c.cur)
	}
}

// Load asks the comparison of tag with branch unless it is known or in
// flight; a failed one is asked again.
func (c *Comparer) Load(env *app.Env, ref gitlab.ProjectRef, tag, branch string) tea.Cmd {
	key := entryKey(ref, tag)
	if e, ok := c.entries[key]; ok && e.State != CompareFailed {
		return nil
	}
	if c.cancel != nil {
		c.cancel()
	}
	if e, ok := c.entries[c.cur]; ok && e.State == CompareLoading {
		delete(c.entries, c.cur) // its answer will be dropped
	}
	c.cur = key
	c.entries[key] = Entry{State: CompareLoading}
	cmd, cancel := Compare(env, c.gen.Next(), ref, tag, branch, false)
	c.cancel = cancel
	return cmd
}

// OnMsg stores the answer unless it is for another request.
func (c *Comparer) OnMsg(msg CompareMsg) {
	key := entryKey(msg.Ref, msg.Tag)
	if !c.gen.Valid(msg.Tok) || key != c.cur {
		return
	}
	if msg.Err != nil {
		c.entries[key] = Entry{State: CompareFailed}
		return
	}
	c.entries[key] = Entry{State: CompareDone, Ahead: len(msg.Cmp.Commits)}
}

// ---- preview ----

// PreviewData is what the preview of a tag shows.
type PreviewData struct {
	Tag     gitlab.Tag
	Branch  string // default branch, "" when unknown
	State   CompareState
	Ahead   int    // commits of Branch after the tag, with CompareDone
	Suggest string // name "n" would suggest, "" for none
}

// PreviewLines is the number of lines of the preview.
const PreviewLines = 3

// Preview renders the three lines under the list: tag, commit and
// protected; the comparison with the default branch and the suggestion;
// the first line of the release notes. Lines are not clipped.
func Preview(env *app.Env, d PreviewData) []string {
	s, m := env.Styles, env.Msg.Tag
	dot := s.Muted.Render(" · ")
	t := d.Tag
	head := s.PreviewTitle.Render(t.Name) + s.Muted.Render(" @ ") + s.Accent.Render(t.Commit.ShortSHA)
	if title := Message(t); title != "" {
		head += dot + title
	}
	if t.Protected {
		head += dot + s.Warning.Render(m.PrevProtected)
	}

	var cmp []string
	if d.Branch != "" {
		switch d.State {
		case CompareLoading:
			cmp = append(cmp, s.Muted.Render(m.PrevComparing))
		case CompareFailed:
			cmp = append(cmp, s.Muted.Render(m.PrevCompareErr))
		case CompareDone:
			if d.Ahead == 0 {
				cmp = append(cmp, s.Success.Render(m.PrevSame(d.Branch)))
			} else {
				cmp = append(cmp, m.PrevAhead(d.Branch, d.Ahead))
			}
		}
	}
	if d.Suggest != "" {
		cmp = append(cmp, s.Muted.Render(m.PrevSuggest(d.Suggest)))
	}

	notes := ""
	if t.Release != nil {
		if l := render.FirstLine(t.Release.Description); l != "" {
			notes = s.Muted.Render(m.PrevRelease) + " " + l
		}
	}
	return []string{head, strings.Join(cmp, dot), notes}
}

// Message is the first line of the tag message; a lightweight tag has none
// and shows the title of its commit.
func Message(t gitlab.Tag) string {
	if l := render.FirstLine(t.Message); l != "" {
		return l
	}
	return t.Commit.Title
}
