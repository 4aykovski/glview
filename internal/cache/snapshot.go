package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/4aykovski/glview/internal/gitlab"
)

// StarredMaxAge is how long the starred snapshot is fresh (SPEC §5.8).
const StarredMaxAge = time.Hour

const starredFile = "starred.json"

// LoadAt is Load without an age limit that also returns when the snapshot
// was saved.
func (d Disk) LoadAt(name string, v any) (time.Time, bool) {
	data, err := os.ReadFile(filepath.Join(d.Dir, name))
	if err != nil {
		return time.Time{}, false
	}
	var env diskEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return time.Time{}, false
	}
	if json.Unmarshal(env.Data, v) != nil {
		return time.Time{}, false
	}
	return env.SavedAt, true
}

// WorkspaceSnapshot is the saved overview of one workspace: the Projects
// tab rows without MRs, with the status of the default branch pipeline
// and the default branch (rootRef) — enough to draw the tab at once.
type WorkspaceSnapshot struct {
	Projects []gitlab.ProjectOverview `json:"projects"`
}

// workspaceName is the snapshot file of a workspace: one per host and
// source ("~/src", "group: platform").
func workspaceName(host, source string) string {
	sum := sha256.Sum256([]byte(host + "\n" + source))
	return filepath.Join("workspace", hex.EncodeToString(sum[:])[:16]+".json")
}

// LoadWorkspace reads the snapshot of a workspace; a missing or corrupt
// file is a miss.
func (d Disk) LoadWorkspace(host, source string) (WorkspaceSnapshot, time.Time, bool) {
	var s WorkspaceSnapshot
	at, ok := d.LoadAt(workspaceName(host, source), &s)
	return s, at, ok
}

// SaveWorkspace writes the snapshot atomically. Only the fields the tab
// needs are kept: the default branch pipeline keeps its ID and status.
func (d Disk) SaveWorkspace(host, source string, rows []gitlab.ProjectOverview) error {
	out := make([]gitlab.ProjectOverview, len(rows))
	for i, r := range rows {
		if p := r.DefaultPipeline; p != nil {
			r.DefaultPipeline = &gitlab.PipelineSummary{Project: p.Project, ID: p.ID, IID: p.IID, Status: p.Status, Ref: p.Ref, WebURL: p.WebURL}
		}
		out[i] = r
	}
	return d.Save(workspaceName(host, source), WorkspaceSnapshot{Projects: out})
}

// LoadStarred returns the starred snapshot and its age.
func (d Disk) LoadStarred() ([]gitlab.Project, time.Time, bool) {
	var ps []gitlab.Project
	at, ok := d.LoadAt(starredFile, &ps)
	return ps, at, ok
}

// SaveStarred writes the starred snapshot.
func (d Disk) SaveStarred(ps []gitlab.Project) error { return d.Save(starredFile, ps) }

// Fresh reports whether a snapshot saved at was younger than maxAge.
func (d Disk) Fresh(at time.Time, maxAge time.Duration) bool {
	return !at.IsZero() && d.clock().Sub(at) < maxAge
}
