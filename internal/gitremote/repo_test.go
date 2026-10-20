package gitremote

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// mkRepo creates a minimal repository layout at dir.
func mkRepo(t *testing.T, dir, config, head string) {
	t.Helper()
	write(t, filepath.Join(dir, ".git", "config"), config)
	write(t, filepath.Join(dir, ".git", "HEAD"), head+"\n")
}

const originCfg = `[core]
	bare = false
[remote "origin"]
	url = git@gitlab.example.com:platform/route-service.git
	fetch = +refs/heads/*:refs/remotes/origin/*
[branch "main"]
	remote = origin
	merge = refs/heads/main
`

func TestFindWalksUp(t *testing.T) {
	root := t.TempDir()
	mkRepo(t, root, originCfg, "ref: refs/heads/fix/slot-race")
	nested := filepath.Join(root, "internal", "pkg")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	r, err := Find(nested)
	if err != nil {
		t.Fatal(err)
	}
	if r.Root != root {
		t.Fatalf("root = %q, want %q", r.Root, root)
	}
	if r.Remote != "origin" || r.RemoteURL != "git@gitlab.example.com:platform/route-service.git" {
		t.Fatalf("remote = %q %q", r.Remote, r.RemoteURL)
	}
	if r.Branch != "fix/slot-race" || r.Detached {
		t.Fatalf("branch = %q detached=%v", r.Branch, r.Detached)
	}
}

func TestFindNotRepo(t *testing.T) {
	if _, err := Find(t.TempDir()); !errors.Is(err, ErrNotRepo) {
		t.Fatalf("want ErrNotRepo, got %v", err)
	}
}

func TestDetachedHEAD(t *testing.T) {
	root := t.TempDir()
	mkRepo(t, root, originCfg, "0b7d3e9a1c2b3d4e5f60718293a4b5c6d7e8f901")
	r, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Detached || r.Branch != "0b7d3e9a" {
		t.Fatalf("branch = %q detached=%v", r.Branch, r.Detached)
	}
}

func TestRemoteSelection(t *testing.T) {
	cases := []struct {
		name, cfg, wantName, wantURL string
	}{
		{"single non-origin", "[remote \"upstream\"]\n\turl = https://gitlab.com/a/b.git\n", "upstream", "https://gitlab.com/a/b.git"},
		{"several without origin", "[remote \"up\"]\n\turl = x:a/b\n[remote \"fork\"]\n\turl = x:c/d\n", "", ""},
		{"none", "[core]\n\tbare = false\n", "", ""},
		{"origin among many", "[remote \"up\"]\n\turl = x:a/b\n[remote \"origin\"]\n\turl = x:c/d\n", "origin", "x:c/d"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			mkRepo(t, root, c.cfg, "ref: refs/heads/main")
			r, err := Open(root)
			if err != nil {
				t.Fatal(err)
			}
			if r.Remote != c.wantName || r.RemoteURL != c.wantURL {
				t.Fatalf("got %q %q", r.Remote, r.RemoteURL)
			}
		})
	}
}

func TestWorktreeLayout(t *testing.T) {
	base := t.TempDir()
	main := filepath.Join(base, "main")
	mkRepo(t, main, originCfg, "ref: refs/heads/main")
	wtGit := filepath.Join(main, ".git", "worktrees", "wt")
	write(t, filepath.Join(wtGit, "HEAD"), "ref: refs/heads/feat/x\n")
	write(t, filepath.Join(wtGit, "commondir"), "../..\n")
	wt := filepath.Join(base, "wt")
	write(t, filepath.Join(wt, ".git"), "gitdir: "+wtGit+"\n")

	r, err := Find(wt)
	if err != nil {
		t.Fatal(err)
	}
	if r.Branch != "feat/x" {
		t.Fatalf("branch = %q, want worktree branch", r.Branch)
	}
	if r.RemoteURL == "" || r.CommonDir != filepath.Join(main, ".git") {
		t.Fatalf("config must come from common dir: %+v", r)
	}
}

func TestSubmoduleLayout(t *testing.T) {
	parent := t.TempDir()
	mkRepo(t, parent, originCfg, "ref: refs/heads/main")
	modGit := filepath.Join(parent, ".git", "modules", "sub")
	write(t, filepath.Join(modGit, "config"), "[remote \"origin\"]\n\turl = https://gitlab.example.com/libs/sub.git\n")
	write(t, filepath.Join(modGit, "HEAD"), "ref: refs/heads/develop\n")
	write(t, filepath.Join(parent, "sub", ".git"), "gitdir: ../.git/modules/sub\n")

	r, err := Find(filepath.Join(parent, "sub"))
	if err != nil {
		t.Fatal(err)
	}
	if r.RemoteURL != "https://gitlab.example.com/libs/sub.git" || r.Branch != "develop" {
		t.Fatalf("submodule must use its own gitdir: %+v", r)
	}
}

func TestMalformedDotGitFile(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".git"), "garbage\n")
	if _, err := Open(root); !errors.Is(err, ErrNotRepo) {
		t.Fatalf("want ErrNotRepo, got %v", err)
	}
}

func TestRefreshHEAD(t *testing.T) {
	root := t.TempDir()
	mkRepo(t, root, originCfg, "ref: refs/heads/main")
	r, _ := Open(root)
	write(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/other\n")
	if err := r.RefreshHEAD(); err != nil {
		t.Fatal(err)
	}
	if r.Branch != "other" {
		t.Fatalf("branch = %q", r.Branch)
	}
}

// TestRealGit cross-checks the parser against layouts produced by git itself.
func TestRealGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	base := t.TempDir()
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	repo := filepath.Join(base, "repo")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	run(repo, "init", "-q", "-b", "main")
	run(repo, "remote", "add", "origin", "ssh://git@gitlab.example.com:2222/team/app.git")
	run(repo, "commit", "-q", "--allow-empty", "-m", "init")
	run(repo, "worktree", "add", "-q", "-b", "feat/wt", filepath.Join(base, "wt"))

	r, err := Find(filepath.Join(base, "wt"))
	if err != nil {
		t.Fatal(err)
	}
	if r.Branch != "feat/wt" || !strings.Contains(r.RemoteURL, "team/app") {
		t.Fatalf("real worktree: %+v", r)
	}
	main, err := Find(repo)
	if err != nil || main.Branch != "main" {
		t.Fatalf("real repo: %+v %v", main, err)
	}
}
