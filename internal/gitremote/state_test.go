package gitremote

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUpstream(t *testing.T) {
	root := t.TempDir()
	mkRepo(t, root, originCfg, "ref: refs/heads/main")
	r, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	remote, branch, ok := r.Upstream()
	if !ok || remote != "origin" || branch != "main" {
		t.Fatalf("upstream = %q %q %v", remote, branch, ok)
	}
	write(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/feat/local\n")
	_ = r.RefreshHEAD()
	if _, _, ok := r.Upstream(); ok {
		t.Fatal("a branch without branch.<name> section has no upstream")
	}
	write(t, filepath.Join(root, ".git", "HEAD"), "0b7d3e9a1c2b3d4e5f60718293a4b5c6d7e8f901\n")
	_ = r.RefreshHEAD()
	if _, _, ok := r.Upstream(); ok {
		t.Fatal("detached HEAD has no upstream")
	}
}

func TestUpstreamWorktree(t *testing.T) {
	main := t.TempDir()
	mkRepo(t, main, originCfg+"[branch \"feat/wt\"]\n\tremote = origin\n\tmerge = refs/heads/feat/wt\n", "ref: refs/heads/main")
	gitdir := filepath.Join(main, ".git", "worktrees", "wt")
	write(t, filepath.Join(gitdir, "HEAD"), "ref: refs/heads/feat/wt\n")
	write(t, filepath.Join(gitdir, "commondir"), "../..\n")
	wt := t.TempDir()
	write(t, filepath.Join(wt, ".git"), "gitdir: "+gitdir+"\n")
	r, err := Open(wt)
	if err != nil {
		t.Fatal(err)
	}
	if _, b, ok := r.Upstream(); !ok || b != "feat/wt" {
		t.Fatalf("worktree upstream = %q %v", b, ok)
	}
	if r.InProgress() != OpNone {
		t.Fatal("clean worktree")
	}
	write(t, filepath.Join(gitdir, "rebase-merge", "head-name"), "x")
	if r.InProgress() != OpRebase {
		t.Fatal("rebase-merge in the worktree git dir")
	}
}

func TestInProgress(t *testing.T) {
	root := t.TempDir()
	mkRepo(t, root, originCfg, "ref: refs/heads/main")
	r, _ := Open(root)
	if r.InProgress() != OpNone {
		t.Fatal("clean")
	}
	write(t, filepath.Join(root, ".git", "MERGE_HEAD"), "abc\n")
	if r.InProgress() != OpMerge {
		t.Fatal("merge")
	}
	os.Remove(filepath.Join(root, ".git", "MERGE_HEAD"))
	if err := os.MkdirAll(filepath.Join(root, ".git", "rebase-apply"), 0o755); err != nil {
		t.Fatal(err)
	}
	if r.InProgress() != OpRebase {
		t.Fatal("rebase-apply")
	}
}
