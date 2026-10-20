package gitremote

import (
	"os"
	"path/filepath"
	"strings"
)

// Upstream returns the upstream of the current branch from .git/config
// (branch.<name>.remote and branch.<name>.merge): the remote name and the
// remote branch ("refs/heads/main" → "main"). ok is false for a detached
// HEAD or a branch without upstream. The config is read anew on every call.
func (r Repo) Upstream() (remote, branch string, ok bool) {
	if r.Detached || r.Branch == "" {
		return "", "", false
	}
	f, err := os.Open(filepath.Join(r.CommonDir, "config"))
	if err != nil {
		return "", "", false
	}
	cfg := parseGitConfig(f)
	f.Close()
	sec := "branch." + r.Branch
	remote, merge := cfg.get(sec, "remote"), cfg.get(sec, "merge")
	if remote == "" || merge == "" {
		return "", "", false
	}
	return remote, strings.TrimPrefix(merge, "refs/heads/"), true
}

// Operation is an unfinished multi-step git operation in the work tree.
type Operation int

const (
	OpNone Operation = iota
	OpMerge
	OpRebase
)

// InProgress reports a merge (MERGE_HEAD) or a rebase (rebase-merge/,
// rebase-apply/) left unfinished in the repository's git dir.
func (r Repo) InProgress() Operation {
	exists := func(name string) bool {
		_, err := os.Stat(filepath.Join(r.GitDir, name))
		return err == nil
	}
	switch {
	case exists("rebase-merge"), exists("rebase-apply"):
		return OpRebase
	case exists("MERGE_HEAD"):
		return OpMerge
	}
	return OpNone
}
