// Package issueactions holds what the Issues tab and the issue screen share:
// the cache keys of issue data and closing/reopening an issue (SPEC §3.4,
// §3.15).
package issueactions

import (
	"fmt"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/ui/app"
)

// Cache keys of the issue data of a project.

func ListKey(path string, f gitlab.IssueFilter, cursor string) string {
	return fmt.Sprintf("issues:%s:%s:%s", path, f.Key(), cursor)
}
func ListPrefix(path string) string          { return "issues:" + path + ":" }
func GetKey(path string, iid int64) string   { return fmt.Sprintf("issue:%s:%d", path, iid) }
func NotesKey(path string, iid int64) string { return fmt.Sprintf("inotes:%s:%d", path, iid) }
func DiscsKey(path string, iid int64) string { return fmt.Sprintf("idiscs:%s:%d", path, iid) }
func RelatedKey(path string, iid int64) string {
	return fmt.Sprintf("irelated:%s:%d", path, iid)
}

// Invalidate drops the cached lists of the project and the data of one
// issue (its related MRs stay: a comment or a state change does not touch
// them); a comment or a state change affects all of them.
func Invalidate(env *app.Env, ref gitlab.ProjectRef, iid int64) {
	if env.Cache == nil {
		return
	}
	path := ref.FullPath
	env.Cache.Invalidate(ListPrefix(path))
	env.Cache.Invalidate(GetKey(path, iid))
	env.Cache.Invalidate(NotesKey(path, iid))
	env.Cache.Invalidate(DiscsKey(path, iid))
}
