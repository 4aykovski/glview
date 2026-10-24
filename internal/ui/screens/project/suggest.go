package project

import (
	"context"
	"strings"
	"time"

	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/common/filterline"
)

// suggestTTL is how long server suggestions stay cached.
const suggestTTL = 5 * time.Minute

type sourceFunc = func(ctx context.Context, prefix string) ([]filterline.Suggestion, error)

func (sh *shared) cached(ctx context.Context, kind, prefix string, fn func(context.Context) ([]filterline.Suggestion, error)) ([]filterline.Suggestion, error) {
	key := "sugg:" + sh.ref.FullPath + ":" + kind + ":" + strings.ToLower(prefix)
	items, _, err := common.Fetch(ctx, sh.env.Cache, key, suggestTTL, false, fn)
	return items, err
}

// membersSource suggests project members by login or name.
func (sh *shared) membersSource() sourceFunc {
	return func(ctx context.Context, prefix string) ([]filterline.Suggestion, error) {
		return sh.cached(ctx, "members", prefix, func(ctx context.Context) ([]filterline.Suggestion, error) {
			users, err := sh.env.Client.Projects().Members(ctx, sh.ref, prefix)
			if err != nil {
				return nil, err
			}
			out := make([]filterline.Suggestion, len(users))
			for i, u := range users {
				out[i] = filterline.Suggestion{Value: u.Username, Note: u.Name}
			}
			return out, nil
		})
	}
}

// labelsSource suggests project labels.
func (sh *shared) labelsSource() sourceFunc {
	return func(ctx context.Context, prefix string) ([]filterline.Suggestion, error) {
		return sh.cached(ctx, "labels", prefix, func(ctx context.Context) ([]filterline.Suggestion, error) {
			names, err := sh.env.Client.Projects().Labels(ctx, sh.ref, prefix)
			if err != nil {
				return nil, err
			}
			out := make([]filterline.Suggestion, len(names))
			for i, n := range names {
				out[i] = filterline.Suggestion{Value: n}
			}
			return out, nil
		})
	}
}

// refsSource suggests branches and tags; a failing half does not hide the other.
func (sh *shared) refsSource() sourceFunc {
	return func(ctx context.Context, prefix string) ([]filterline.Suggestion, error) {
		return sh.cached(ctx, "refs", prefix, func(ctx context.Context) ([]filterline.Suggestion, error) {
			m := sh.env.Msg.Project
			var out []filterline.Suggestion
			branches, berr := sh.env.Client.Projects().Branches(ctx, sh.ref, prefix)
			for _, b := range branches {
				out = append(out, filterline.Suggestion{Value: b, Note: m.NoteBranch})
			}
			tags, terr := sh.env.Client.Tags().List(ctx, sh.ref, prefix, "", 20)
			for _, t := range tags.Items {
				out = append(out, filterline.Suggestion{Value: t.Name, Note: m.NoteTag})
			}
			if berr != nil && terr != nil {
				return nil, berr
			}
			return out, nil
		})
	}
}

// milestonesSource suggests the active milestones of the project (not those
// of its groups).
func (sh *shared) milestonesSource() sourceFunc {
	return func(ctx context.Context, prefix string) ([]filterline.Suggestion, error) {
		return sh.cached(ctx, "milestones", prefix, func(ctx context.Context) ([]filterline.Suggestion, error) {
			titles, err := sh.env.Client.Projects().Milestones(ctx, sh.ref, prefix)
			if err != nil {
				return nil, err
			}
			out := make([]filterline.Suggestion, len(titles))
			for i, t := range titles {
				out[i] = filterline.Suggestion{Value: t}
			}
			return out, nil
		})
	}
}
