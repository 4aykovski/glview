// Package linkslist is the L and A keys of the MR lists (project and
// workspace MRs tabs): it loads the details of the selected MR and opens the
// links overlay or the approvers overlay (SPEC 3.5, "MR links", "Оверлей A").
package linkslist

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/mrlinks"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
)

const (
	dataTTL        = 30 * time.Second // as the MR detail screen: the cache is shared
	requestTimeout = 15 * time.Second
)

// LoadedMsg is the answer to Load.
type LoadedMsg struct {
	Tok       common.Token
	Approvers bool // asked by A (the approvers overlay), not L
	Ref       gitlab.ProjectRef
	IID       int64
	MR        gitlab.MergeRequest
	Err       error
}

// Load requests the details of MR iid of project ref through the cache
// the MR detail screen uses (key "mr:<path>:<iid>").
func Load(env *app.Env, tok common.Token, ref gitlab.ProjectRef, iid int64) tea.Cmd {
	return load(env, tok, ref, iid, false)
}

// LoadApprovers is Load for the approvers overlay.
func LoadApprovers(env *app.Env, tok common.Token, ref gitlab.ProjectRef, iid int64) tea.Cmd {
	return load(env, tok, ref, iid, true)
}

func load(env *app.Env, tok common.Token, ref gitlab.ProjectRef, iid int64, approvers bool) tea.Cmd {
	key := fmt.Sprintf("mr:%s:%d", ref.FullPath, iid)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		mr, _, err := common.Fetch(ctx, env.Cache, key, dataTTL, false, func(ctx context.Context) (gitlab.MergeRequest, error) {
			return env.Client.MergeRequests().Get(ctx, ref, iid)
		})
		return LoadedMsg{Tok: tok, Approvers: approvers, Ref: ref, IID: iid, MR: mr, Err: err}
	}
}

// Links is the link list of mr: the HTML links of the details (when mr is
// one), the config rules and bare URLs.
func Links(env *app.Env, mr gitlab.MergeRequest) []gitlab.Link {
	return mrlinks.Collect(mrlinks.Source{
		Title: mr.Title, Description: mr.Description,
		TitleLinks: mr.TitleLinks, DescriptionLinks: mr.DescriptionLinks,
	}, env.Settings.LinkRules)
}

// Open shows the links overlay of mr, or the "no links" toast.
func Open(env *app.Env, ref gitlab.ProjectRef, mr gitlab.MergeRequest) tea.Cmd {
	links := Links(env, mr)
	if len(links) == 0 {
		return app.Toast(common.ToastInfo, env.Msg.Links.NoLinks, "")
	}
	return app.OpenOverlay(app.NewLinksOverlay(env, Key(ref, mr.IID), fmt.Sprintf("!%d", mr.IID), links))
}

// Key identifies the overlay of an MR (see app.LinksRefreshMsg).
func Key(ref gitlab.ProjectRef, iid int64) string {
	return fmt.Sprintf("links:%s:%d", ref.FullPath, iid)
}

// ApproversKey identifies the approvers overlay of an MR (see
// app.ApproversRefreshMsg).
func ApproversKey(ref gitlab.ProjectRef, iid int64) string {
	return fmt.Sprintf("approvers:%s:%d", ref.FullPath, iid)
}

// Done handles an accepted answer: the overlay of the details, or after an
// error the error toast and, for L, the overlay of the list row if it has
// links. A failed A opens nothing.
func Done(env *app.Env, msg LoadedMsg, row gitlab.MergeRequest) tea.Cmd {
	if msg.Approvers {
		if msg.Err != nil {
			return env.Fail(msg.Err)
		}
		return app.OpenApprovers(env, ApproversKey(msg.Ref, msg.IID), fmt.Sprintf("!%d", msg.IID), msg.MR)
	}
	if msg.Err == nil {
		return Open(env, msg.Ref, msg.MR)
	}
	fail := env.Fail(msg.Err)
	if len(Links(env, row)) == 0 {
		return fail
	}
	return tea.Batch(fail, Open(env, msg.Ref, row))
}
