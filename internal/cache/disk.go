package cache

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Disk stores small JSON snapshots (starred projects, workspace metadata)
// under a directory, usually xdg.CacheDir(). Corrupt or missing files are
// treated as a miss.
type Disk struct {
	Dir string
	now func() time.Time
}

type diskEnvelope struct {
	SavedAt time.Time       `json:"saved_at"`
	Data    json.RawMessage `json:"data"`
}

// Load decodes name into v if the snapshot exists and is younger than
// maxAge (maxAge <= 0 accepts any age). It reports whether v was filled.
func (d Disk) Load(name string, v any, maxAge time.Duration) bool {
	data, err := os.ReadFile(filepath.Join(d.Dir, name))
	if err != nil {
		return false
	}
	var env diskEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return false
	}
	if maxAge > 0 && d.clock().Sub(env.SavedAt) > maxAge {
		return false
	}
	return json.Unmarshal(env.Data, v) == nil
}

// Save writes v as name atomically with 0600 permissions.
func (d Disk) Save(name string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	data, err := json.Marshal(diskEnvelope{SavedAt: d.clock(), Data: raw})
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(d.Dir, name), data)
}

func (d Disk) clock() time.Time {
	if d.now != nil {
		return d.now()
	}
	return time.Now()
}

func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// RecentLimit is how many recently opened projects are remembered.
const RecentLimit = 20

// WorkspaceLimit is how many workspaces per host keep their expanded
// groups; the ones not opened for the longest time are dropped first.
const WorkspaceLimit = 50

// State is persisted user state ($XDG_STATE_HOME/gitlab-tui/state.json),
// kept per host. The old host-less "recent" field is ignored and dropped
// on the next save.
type State struct {
	Hosts map[string]HostState `json:"hosts,omitempty"`
}

// HostState is the state of one GitLab host.
type HostState struct {
	Recent []string `json:"recent"` // project full paths, most recent first
	// Workspaces are the expanded groups of the workspace tabs, by
	// workspace source (a folder or a group, as in the snapshot key).
	Workspaces map[string]WorkspaceState `json:"workspaces,omitempty"`
}

// WorkspaceState is what one workspace remembers.
type WorkspaceState struct {
	At       time.Time           `json:"at"`                 // last opened
	Expanded map[string][]string `json:"expanded,omitempty"` // project paths by tab ("mrs", "pipelines")
	Selected map[string][]string `json:"selected,omitempty"` // projects marked in P, by tab
}

// Recent returns the recent projects of host, most recent first.
func (s State) Recent(host string) []string { return s.Hosts[host].Recent }

// LoadState reads the state file; a missing or corrupt file yields empty state.
func LoadState(path string) State {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) || err != nil {
		return State{}
	}
	var s State
	if json.Unmarshal(data, &s) != nil {
		return State{}
	}
	return s
}

// SaveState writes the state file atomically.
func SaveState(path string, s State) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, data)
}

// host returns the state of host for changing (it is stored back by the
// caller).
func (s *State) host(host string) HostState {
	if s.Hosts == nil {
		s.Hosts = map[string]HostState{}
	}
	return s.Hosts[host]
}

// Touch moves project to the front of the recent list of host, keeping
// RecentLimit entries.
func (s *State) Touch(host, project string) {
	out := []string{project}
	for _, p := range s.Recent(host) {
		if p != project && len(out) < RecentLimit {
			out = append(out, p)
		}
	}
	h := s.host(host)
	h.Recent = out
	s.Hosts[host] = h
}

// Expanded returns the expanded groups of tab in workspace source of host.
func (s State) Expanded(host, source, tab string) []string {
	return s.Hosts[host].Workspaces[source].Expanded[tab]
}

// HasWorkspace reports whether workspace source of host has a record.
func (s State) HasWorkspace(host, source string) bool {
	_, ok := s.Hosts[host].Workspaces[source]
	return ok
}

// SetExpanded stores the expanded groups of tab in workspace source of
// host and marks the workspace opened at at. An empty list removes the
// tab; a workspace without tabs keeps its record (and its time).
func (s *State) SetExpanded(host, source, tab string, paths []string, at time.Time) {
	s.setTab(host, source, at, func(ws *WorkspaceState) { ws.Expanded = withTab(ws.Expanded, tab, paths) })
}

// Selected returns the projects marked in the P filter of tab in workspace
// source of host.
func (s State) Selected(host, source, tab string) []string {
	return s.Hosts[host].Workspaces[source].Selected[tab]
}

// SetSelected stores the projects marked in the P filter of tab in
// workspace source of host, like SetExpanded.
func (s *State) SetSelected(host, source, tab string, paths []string, at time.Time) {
	s.setTab(host, source, at, func(ws *WorkspaceState) { ws.Selected = withTab(ws.Selected, tab, paths) })
}

// setTab changes the record of workspace source of host with set and marks
// it opened at at.
func (s *State) setTab(host, source string, at time.Time, set func(*WorkspaceState)) {
	h := s.host(host)
	ws := h.Workspaces[source]
	set(&ws)
	ws.At = at
	h.Workspaces = withWorkspace(h.Workspaces, source, ws)
	s.Hosts[host] = h
}

// withTab returns a copy of byTab with tab set to paths (removed when
// paths is empty); nil if no tab is left.
func withTab(byTab map[string][]string, tab string, paths []string) map[string][]string {
	out := make(map[string][]string, len(byTab)+1)
	for k, v := range byTab {
		if k != tab {
			out[k] = v
		}
	}
	if len(paths) > 0 {
		out[tab] = append([]string(nil), paths...)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// TouchWorkspace marks workspace source of host opened at at, if it has a
// record. It reports whether it had one.
func (s *State) TouchWorkspace(host, source string, at time.Time) bool {
	if !s.HasWorkspace(host, source) {
		return false
	}
	h := s.host(host)
	ws := h.Workspaces[source]
	ws.At = at
	h.Workspaces = withWorkspace(h.Workspaces, source, ws)
	s.Hosts[host] = h
	return true
}

// withWorkspace returns a copy of all with source set to ws, keeping the
// WorkspaceLimit most recently opened workspaces.
func withWorkspace(all map[string]WorkspaceState, source string, ws WorkspaceState) map[string]WorkspaceState {
	out := make(map[string]WorkspaceState, len(all)+1)
	for k, v := range all {
		out[k] = v
	}
	out[source] = ws
	for len(out) > WorkspaceLimit {
		oldest, found := "", false
		for k, v := range out {
			if k == source {
				continue
			}
			if o := out[oldest]; !found || v.At.Before(o.At) || (v.At.Equal(o.At) && k < oldest) {
				oldest, found = k, true
			}
		}
		delete(out, oldest)
	}
	return out
}

// Clone returns a deep copy (to save it off the UI goroutine).
func (s State) Clone() State {
	out := State{Hosts: make(map[string]HostState, len(s.Hosts))}
	for h, st := range s.Hosts {
		c := HostState{Recent: append([]string(nil), st.Recent...)}
		if st.Workspaces != nil {
			c.Workspaces = make(map[string]WorkspaceState, len(st.Workspaces))
			for src, ws := range st.Workspaces {
				c.Workspaces[src] = WorkspaceState{At: ws.At, Expanded: cloneTabs(ws.Expanded), Selected: cloneTabs(ws.Selected)}
			}
		}
		out.Hosts[h] = c
	}
	return out
}

func cloneTabs(byTab map[string][]string) map[string][]string {
	if byTab == nil {
		return nil
	}
	out := make(map[string][]string, len(byTab))
	for tab, ps := range byTab {
		out[tab] = append([]string(nil), ps...)
	}
	return out
}
