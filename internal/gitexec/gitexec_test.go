package gitexec

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/4aykovski/glview/internal/gitremote"
)

func TestMain(m *testing.M) {
	// Tests must not depend on the user's git configuration.
	os.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	os.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	os.Exit(m.Run())
}

func gitBin(t *testing.T) *Git {
	t.Helper()
	g := Detect("", false, 20*time.Second, 3)
	if g == nil {
		t.Skip("git is not available")
	}
	return g
}

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=Egor", "-c", "user.email=egor@example.com", "-c", "init.defaultBranch=main"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func commit(t *testing.T, dir, file, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "add", file)
	run(t, dir, "commit", "-q", "-m", "change "+file)
}

// fixture creates a bare remote with one commit and two clones: work (the
// repository under test) and other (pushes new commits).
func fixture(t *testing.T) (work, other string) {
	t.Helper()
	root := t.TempDir()
	bare := filepath.Join(root, "remote.git")
	run(t, root, "init", "-q", "--bare", bare)
	seed := filepath.Join(root, "seed")
	run(t, root, "clone", "-q", "file://"+bare, seed)
	commit(t, seed, "README.md", "one\n")
	run(t, seed, "push", "-q", "origin", "HEAD:main")
	work, other = filepath.Join(root, "work"), filepath.Join(root, "other")
	run(t, root, "clone", "-q", "file://"+bare, work)
	run(t, root, "clone", "-q", "file://"+bare, other)
	return work, other
}

func open(t *testing.T, dir string) gitremote.Repo {
	t.Helper()
	r, err := gitremote.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func one(t *testing.T, g *Git, op Op, r gitremote.Repo) Result {
	t.Helper()
	var out []Result
	for res := range g.Run(context.Background(), op, []gitremote.Repo{r}) {
		out = append(out, res)
	}
	if len(out) != 1 {
		t.Fatalf("results = %+v", out)
	}
	return out[0]
}

func TestFetchAndPullNewCommits(t *testing.T) {
	t.Parallel()
	g := gitBin(t)
	work, other := fixture(t)
	for i := range 3 {
		commit(t, other, "f.txt", strings.Repeat("x", i+1))
	}
	run(t, other, "push", "-q", "origin", "main")
	repo := open(t, work)
	head := run(t, work, "rev-parse", "HEAD")

	res := one(t, g, OpFetch, repo)
	if res.Status != StatusUpdated || res.Behind != 3 || res.Ahead != 0 || !res.HasUpstream {
		t.Fatalf("fetch = %+v", res)
	}
	if run(t, work, "rev-parse", "HEAD") != head {
		t.Fatal("fetch must not move the branch")
	}
	if a, b, err := g.AheadBehind(context.Background(), repo); err != nil || a != 0 || b != 3 {
		t.Fatalf("ahead/behind = %d %d %v", a, b, err)
	}

	res = one(t, g, OpPullFF, repo)
	if res.Status != StatusUpdated || res.Moved != 3 || res.Behind != 0 {
		t.Fatalf("pull = %+v", res)
	}
	if res = one(t, g, OpPullFF, repo); res.Status != StatusUpToDate {
		t.Fatalf("second pull = %+v", res)
	}
}

func TestFetchUpstreamOnAnotherRemote(t *testing.T) {
	t.Parallel()
	g := gitBin(t)
	work, _ := fixture(t)
	root := filepath.Dir(work)
	// A second remote ("upstream") that the branch tracks; origin stays
	// the GitLab remote of the project.
	up := filepath.Join(root, "upstream.git")
	run(t, root, "clone", "-q", "--bare", filepath.Join(root, "remote.git"), up)
	run(t, work, "remote", "add", "upstream", "file://"+up)
	run(t, work, "fetch", "-q", "upstream")
	run(t, work, "branch", "-q", "--set-upstream-to=upstream/main", "main")
	pusher := filepath.Join(root, "pusher")
	run(t, root, "clone", "-q", "file://"+up, pusher)
	commit(t, pusher, "up.txt", "one")
	commit(t, pusher, "up.txt", "two")
	run(t, pusher, "push", "-q", "origin", "main")
	want := run(t, pusher, "rev-parse", "HEAD")

	repo := open(t, work)
	if repo.Remote != "origin" {
		t.Fatalf("remote = %q", repo.Remote)
	}
	if got := fetchRemotes(repo); len(got) != 2 || got[0] != "origin" || got[1] != "upstream" {
		t.Fatalf("fetchRemotes = %v", got)
	}
	res := one(t, g, OpFetch, repo)
	if res.Status != StatusUpdated || res.Behind != 2 || res.Ahead != 0 {
		t.Fatalf("fetch = %+v", res)
	}
	res = one(t, g, OpPullFF, repo)
	if res.Status != StatusUpdated || res.Moved != 2 || res.Behind != 0 {
		t.Fatalf("pull = %+v", res)
	}
	if got := run(t, work, "rev-parse", "HEAD"); got != want {
		t.Fatalf("HEAD = %s, want %s", got, want)
	}
}

func TestFetchLocalUpstream(t *testing.T) {
	t.Parallel()
	g := gitBin(t)
	work, _ := fixture(t)
	run(t, work, "checkout", "-q", "-b", "feat/local")
	run(t, work, "branch", "-q", "--set-upstream-to=main")
	repo := open(t, work)
	if up, _, ok := repo.Upstream(); !ok || up != "." {
		t.Fatalf("upstream = %q %v", up, ok)
	}
	if got := fetchRemotes(repo); len(got) != 1 || got[0] != "origin" {
		t.Fatalf("fetchRemotes = %v", got)
	}
	if res := one(t, g, OpFetch, repo); res.Status != StatusUpdated || !res.HasUpstream {
		t.Fatalf("fetch = %+v", res)
	}
}

func TestPullDiverged(t *testing.T) {
	t.Parallel()
	g := gitBin(t)
	work, other := fixture(t)
	commit(t, other, "a.txt", "theirs")
	run(t, other, "push", "-q", "origin", "main")
	commit(t, work, "b.txt", "mine")
	head := run(t, work, "rev-parse", "HEAD")
	res := one(t, g, OpPullFF, open(t, work))
	if res.Status != StatusSkipped || res.Reason != ReasonDiverged || res.Ahead != 1 || res.Behind != 1 {
		t.Fatalf("pull = %+v", res)
	}
	if run(t, work, "rev-parse", "HEAD") != head {
		t.Fatal("HEAD must not move")
	}
}

func TestPullDirty(t *testing.T) {
	t.Parallel()
	g := gitBin(t)
	work, other := fixture(t)
	commit(t, other, "README.md", "two\n")
	run(t, other, "push", "-q", "origin", "main")
	if err := os.WriteFile(filepath.Join(work, "README.md"), []byte("local edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	head := run(t, work, "rev-parse", "HEAD")
	res := one(t, g, OpPullFF, open(t, work))
	if res.Status != StatusSkipped || res.Reason != ReasonDirty || len(res.Output) == 0 {
		t.Fatalf("pull = %+v", res)
	}
	if run(t, work, "rev-parse", "HEAD") != head {
		t.Fatal("HEAD must not move")
	}
}

func TestPrechecks(t *testing.T) {
	t.Parallel()
	g := gitBin(t)
	work, _ := fixture(t)
	run(t, work, "checkout", "-q", "-b", "feat/local")
	repo := open(t, work)
	if res := one(t, g, OpPullFF, repo); res.Status != StatusSkipped || res.Reason != ReasonNoUpstream {
		t.Fatalf("no upstream: %+v", res)
	}
	if _, _, err := g.AheadBehind(context.Background(), repo); err != ErrNoUpstream {
		t.Fatalf("ahead/behind without upstream: %v", err)
	}
	if res := one(t, g, OpFetch, repo); res.Status != StatusUpdated || res.HasUpstream {
		t.Fatalf("fetch without upstream: %+v", res)
	}
	run(t, work, "checkout", "-q", "--detach")
	repo = open(t, work)
	if res := one(t, g, OpPullFF, repo); res.Reason != ReasonDetached {
		t.Fatalf("detached: %+v", res)
	}
}

func TestUnreachableSSHDoesNotHang(t *testing.T) {
	t.Parallel()
	g := gitBin(t)
	g.Env = []string{"GIT_SSH_COMMAND=ssh -o ConnectTimeout=2 -o BatchMode=yes"}
	dir := t.TempDir()
	run(t, dir, "init", "-q")
	run(t, dir, "remote", "add", "origin", "ssh://git@127.0.0.1:1/x.git")
	start := time.Now()
	res := one(t, g, OpFetch, open(t, dir))
	if res.Status != StatusFailed || res.Reason == ReasonNone {
		t.Fatalf("fetch = %+v", res)
	}
	if time.Since(start) > 15*time.Second {
		t.Fatalf("took %s", time.Since(start))
	}
}

func TestTimeoutKillsProcessGroup(t *testing.T) {
	t.Parallel()
	g := gitBin(t)
	g.Timeout = 500 * time.Millisecond
	g.Env = []string{"GIT_SSH_COMMAND=sleep 30 #"}
	dir := t.TempDir()
	run(t, dir, "init", "-q")
	run(t, dir, "remote", "add", "origin", "ssh://git@gitlab.example.com/x.git")
	start := time.Now()
	res := one(t, g, OpFetch, open(t, dir))
	if res.Status != StatusFailed || res.Reason != ReasonTimeout {
		t.Fatalf("fetch = %+v", res)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("took %s", time.Since(start))
	}
}

func TestCancel(t *testing.T) {
	t.Parallel()
	g := gitBin(t)
	g.Env = []string{"GIT_SSH_COMMAND=sleep 30 #"}
	dir := t.TempDir()
	run(t, dir, "init", "-q")
	run(t, dir, "remote", "add", "origin", "ssh://git@gitlab.example.com/x.git")
	ctx, cancel := context.WithCancel(context.Background())
	ch := g.Run(ctx, OpFetch, []gitremote.Repo{open(t, dir)})
	time.AfterFunc(200*time.Millisecond, cancel)
	start := time.Now()
	for range ch {
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("cancel took %s", time.Since(start))
	}
}

func TestRunManyRepos(t *testing.T) {
	t.Parallel()
	g := gitBin(t)
	g.Parallel = 3
	work, _ := fixture(t)
	repos := make([]gitremote.Repo, 10)
	for i := range repos {
		r := open(t, work)
		repos[i] = r
	}
	n := 0
	for res := range g.Run(context.Background(), OpFetch, repos) {
		if res.Status != StatusUpdated {
			t.Fatalf("res = %+v", res)
		}
		n++
	}
	if n != 10 {
		t.Fatalf("results = %d", n)
	}
}

func TestClassify(t *testing.T) {
	cases := map[string]Reason{
		"git@gitlab.example.com: Permission denied (publickey).":                                     ReasonAuth,
		"fatal: could not read Username for 'https://gitlab.example.com': terminal prompts disabled": ReasonAuth,
		"Host key verification failed.":                                                              ReasonAuth,
		"ssh: Could not resolve hostname x: nodename nor servname provided":                          ReasonNetwork,
		"fatal: unable to access 'https://x/': Could not resolve host: x":                            ReasonNetwork,
		"fatal: Not possible to fast-forward, aborting.":                                             ReasonDiverged,
		"error: Your local changes to the following files would be overwritten by merge:":            ReasonDirty,
	}
	for line, want := range cases {
		if got, _ := Classify([]string{line}); got != want {
			t.Errorf("%q → %s, want %s", line, got, want)
		}
	}
	if r, d := Classify([]string{"fatal: something odd", ""}); r != ReasonOther || d != "fatal: something odd" {
		t.Fatalf("other = %s %q", r, d)
	}
}
