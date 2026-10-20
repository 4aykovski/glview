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

// State is persisted user state ($XDG_STATE_HOME/gitlab-tui/state.json).
type State struct {
	Recent []string `json:"recent"` // project full paths, most recent first
}

// LoadState reads the state file; a missing or corrupt file yields empty state.
func LoadState(path string) State {
	var s State
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) || err != nil {
		return s
	}
	_ = json.Unmarshal(data, &s)
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

// Touch moves project to the front of the recent list, keeping RecentLimit entries.
func (s *State) Touch(project string) {
	out := []string{project}
	for _, p := range s.Recent {
		if p != project && len(out) < RecentLimit {
			out = append(out, p)
		}
	}
	s.Recent = out
}
