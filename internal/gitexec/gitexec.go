// Package gitexec runs the system git for fetch, fast-forward pull and
// ahead/behind counts (SPEC §10.4). git runs without a terminal and without
// stdin, so missing credentials fail fast instead of prompting; the
// user's own git setup (credential helpers, ssh-agent, insteadOf…) applies.
// This is the only package that starts git as a process.
package gitexec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/4aykovski/glview/internal/gitremote"
)

// Op is a git operation.
type Op int

const (
	OpFetch  Op = iota // git fetch --prune <remote>
	OpPullFF           // fetch, then git merge --ff-only @{u}
)

func (o Op) String() string {
	if o == OpPullFF {
		return "pull"
	}
	return "fetch"
}

// Status is the outcome for one repository.
type Status int

const (
	StatusUpdated  Status = iota // fetched, or pulled and moved
	StatusUpToDate               // pull: already on upstream
	StatusSkipped                // prechecks, diverged, local changes
	StatusFailed                 // auth, network, timeout, other
)

// Reason explains a skip or a failure; the UI translates it.
type Reason string

const (
	ReasonNone       Reason = ""
	ReasonAuth       Reason = "auth"
	ReasonNetwork    Reason = "network"
	ReasonDiverged   Reason = "diverged"
	ReasonDirty      Reason = "dirty"
	ReasonTimeout    Reason = "timeout"
	ReasonDetached   Reason = "detached"
	ReasonNoUpstream Reason = "no-upstream"
	ReasonNoRemote   Reason = "no-remote"
	ReasonInProgress Reason = "in-progress"
	ReasonOther      Reason = "other" // Detail holds the last line of git's output
)

// maxLines is how many lines of git output a result keeps.
const maxLines = 20

// Result is the outcome of an operation in one repository.
type Result struct {
	Dir           string
	Op            Op
	Status        Status
	Reason        Reason
	Detail        string // last output line for ReasonOther
	Ahead, Behind int
	HasUpstream   bool // Ahead and Behind are known
	Moved         int  // pull: commits the branch moved by
	Output        []string
}

// ErrNoUpstream means the current branch has no upstream (or HEAD is
// detached): ahead/behind are shown empty.
var ErrNoUpstream = errors.New("no upstream")

// Runner runs git operations; results arrive as repositories finish and
// the channel is closed at the end.
type Runner interface {
	Run(ctx context.Context, op Op, repos []gitremote.Repo) <-chan Result
	AheadBehind(ctx context.Context, repo gitremote.Repo) (ahead, behind int, err error)
}

// Git is the system git.
type Git struct {
	Path     string
	Timeout  time.Duration // per repository
	Parallel int           // repositories at once
	// Env is appended to the environment of every git process (tests).
	Env []string

	mu    sync.Mutex
	procs map[*exec.Cmd]struct{} // running git processes
}

// Kill kills every running git process group at once (quit).
func (g *Git) Kill() {
	g.mu.Lock()
	defer g.mu.Unlock()
	for cmd := range g.procs {
		if cmd.Cancel != nil {
			_ = cmd.Cancel()
		}
	}
}

var _ Runner = (*Git)(nil)

// Defaults of git.timeout and git.parallel (SPEC §7).
const (
	DefaultTimeout  = 60 * time.Second
	DefaultParallel = 6
	// AheadBehindParallel bounds the local ahead/behind counts.
	AheadBehindParallel = 8
)

// Detect finds git. path overrides the PATH lookup; disabled turns the
// integration off. It returns nil when git is unavailable (or on systems
// where it cannot run without a terminal).
func Detect(path string, disabled bool, timeout time.Duration, parallel int) *Git {
	if disabled || !supported {
		return nil
	}
	if path == "" {
		path = "git"
	}
	full, err := exec.LookPath(path)
	if err != nil {
		return nil
	}
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	if parallel <= 0 {
		parallel = DefaultParallel
	}
	return &Git{Path: full, Timeout: timeout, Parallel: parallel}
}

// Precheck tells, from the files of the repository, whether a pull must
// skip it: detached HEAD, no upstream, a merge or rebase in progress.
func Precheck(repo gitremote.Repo) Reason {
	switch {
	case repo.Detached:
		return ReasonDetached
	case repo.InProgress() != gitremote.OpNone:
		return ReasonInProgress
	}
	if _, _, ok := repo.Upstream(); !ok {
		return ReasonNoUpstream
	}
	return ReasonNone
}

// Run runs op in every repository, at most Parallel at once.
func (g *Git) Run(ctx context.Context, op Op, repos []gitremote.Repo) <-chan Result {
	out := make(chan Result)
	go func() {
		defer close(out)
		sem := make(chan struct{}, max(1, g.Parallel))
		var wg sync.WaitGroup
		for _, r := range repos {
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				wg.Wait()
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				res := g.one(ctx, op, r)
				<-sem
				select {
				case out <- res:
				case <-ctx.Done():
				}
			}()
		}
		wg.Wait()
	}()
	return out
}

func (g *Git) one(ctx context.Context, op Op, repo gitremote.Repo) Result {
	res := Result{Dir: repo.Root, Op: op}
	if repo.Remote == "" {
		res.Status, res.Reason = StatusSkipped, ReasonNoRemote
		return res
	}
	if op == OpPullFF {
		if reason := Precheck(repo); reason != ReasonNone {
			res.Status, res.Reason = StatusSkipped, reason
			return res
		}
	}
	ctx, cancel := context.WithTimeout(ctx, g.timeout())
	defer cancel()

	for _, remote := range fetchRemotes(repo) {
		out, err := g.git(ctx, repo.Root, "fetch", "--prune", remote)
		res.Output = lastLines(append(res.Output, out...))
		if err != nil {
			return g.fail(ctx, res, err)
		}
	}
	ahead, behind, abErr := g.AheadBehind(ctx, repo)
	if abErr == nil {
		res.Ahead, res.Behind, res.HasUpstream = ahead, behind, true
	}
	res.Status = StatusUpdated
	if op == OpFetch {
		return res
	}
	switch {
	case abErr != nil:
		return g.fail(ctx, res, abErr)
	case behind == 0:
		res.Status = StatusUpToDate
		return res
	case ahead > 0:
		res.Status, res.Reason = StatusSkipped, ReasonDiverged
		return res
	}
	out, err := g.git(ctx, repo.Root, "merge", "--ff-only", "@{u}")
	res.Output = lastLines(append(res.Output, out...))
	if err != nil {
		return g.fail(ctx, res, err)
	}
	res.Moved = behind
	if a, b, err := g.AheadBehind(ctx, repo); err == nil {
		res.Ahead, res.Behind = a, b
	}
	return res
}

// fetchRemotes returns the remotes a fetch downloads: the GitLab remote
// of the project and, when the current branch tracks another remote, that
// one too, so ahead/behind and the fast-forward see fresh refs. A local
// upstream (".") needs no fetch.
func fetchRemotes(repo gitremote.Repo) []string {
	remotes := []string{repo.Remote}
	if up, _, ok := repo.Upstream(); ok && up != "." && up != repo.Remote {
		remotes = append(remotes, up)
	}
	return remotes
}

// fail classifies a failed git call; diverged branches and local changes
// in the way are skips, not errors.
func (g *Git) fail(ctx context.Context, res Result, err error) Result {
	res.Status = StatusFailed
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		res.Reason = ReasonTimeout
		return res
	}
	res.Reason, res.Detail = Classify(res.Output)
	if res.Reason == ReasonOther && res.Detail == "" {
		res.Detail = err.Error()
	}
	if res.Reason == ReasonDiverged || res.Reason == ReasonDirty {
		res.Status = StatusSkipped
	}
	return res
}

// patterns map known git messages to reasons (SPEC §10.4).
var patterns = []struct {
	sub    string
	reason Reason
}{
	{"Permission denied", ReasonAuth},
	{"could not read Username", ReasonAuth},
	{"could not read Password", ReasonAuth},
	{"Host key verification failed", ReasonAuth},
	{"Authentication failed", ReasonAuth},
	{"terminal prompts disabled", ReasonAuth},
	{"Could not resolve host", ReasonNetwork},
	{"Connection refused", ReasonNetwork},
	{"Connection timed out", ReasonNetwork},
	{"Operation timed out", ReasonNetwork},
	{"Network is unreachable", ReasonNetwork},
	{"Not possible to fast-forward", ReasonDiverged},
	{"diverged", ReasonDiverged},
	{"would be overwritten", ReasonDirty},
}

// Classify returns the reason of a failure from git's output; unknown
// errors are ReasonOther with the last output line.
func Classify(lines []string) (Reason, string) {
	text := strings.Join(lines, "\n")
	for _, p := range patterns {
		if strings.Contains(text, p.sub) {
			return p.reason, ""
		}
	}
	last := ""
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			last = l
			break
		}
	}
	return ReasonOther, last
}

// AheadBehind counts the commits of HEAD not in the upstream (ahead) and
// of the upstream not in HEAD (behind), locally. ErrNoUpstream when the
// branch has none.
func (g *Git) AheadBehind(ctx context.Context, repo gitremote.Repo) (int, int, error) {
	if repo.Detached {
		return 0, 0, ErrNoUpstream
	}
	if _, _, ok := repo.Upstream(); !ok {
		return 0, 0, ErrNoUpstream
	}
	ctx, cancel := context.WithTimeout(ctx, g.timeout())
	defer cancel()
	out, err := g.git(ctx, repo.Root, "rev-list", "--left-right", "--count", "HEAD...@{u}")
	if err != nil {
		if r, _ := Classify(out); r == ReasonOther && strings.Contains(strings.Join(out, " "), "upstream") {
			return 0, 0, ErrNoUpstream
		}
		return 0, 0, fmt.Errorf("rev-list: %w: %s", err, strings.Join(out, " "))
	}
	fields := strings.Fields(strings.Join(out, " "))
	if len(fields) != 2 {
		return 0, 0, fmt.Errorf("rev-list: unexpected output %q", strings.Join(out, " "))
	}
	a, err1 := strconv.Atoi(fields[0])
	b, err2 := strconv.Atoi(fields[1])
	if err1 != nil || err2 != nil {
		return 0, 0, fmt.Errorf("rev-list: unexpected output %q", strings.Join(out, " "))
	}
	return a, b, nil
}

func (g *Git) timeout() time.Duration {
	if g.Timeout > 0 {
		return g.Timeout
	}
	return DefaultTimeout
}

// git runs one git command in dir and returns its output (stdout and
// stderr, last lines).
func (g *Git) git(ctx context.Context, dir string, args ...string) ([]string, error) {
	cmd := exec.CommandContext(ctx, g.Path, args...)
	cmd.Dir = dir
	cmd.Stdin = nil // /dev/null
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0", // never ask for a username or password
		"GCM_INTERACTIVE=never", // Git Credential Manager without UI
		"LC_ALL=C",              // stable messages for Classify
	)
	cmd.Env = append(cmd.Env, g.Env...)
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	detach(cmd)
	cmd.WaitDelay = 2 * time.Second
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	g.mu.Lock()
	if g.procs == nil {
		g.procs = map[*exec.Cmd]struct{}{}
	}
	g.procs[cmd] = struct{}{}
	g.mu.Unlock()
	err := cmd.Wait()
	g.mu.Lock()
	delete(g.procs, cmd)
	g.mu.Unlock()
	return lastLines(strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")), err
}

func lastLines(lines []string) []string {
	var out []string
	for _, l := range lines {
		if l = strings.TrimRight(l, "\r "); l != "" {
			out = append(out, l)
		}
	}
	if len(out) > maxLines {
		out = out[len(out)-maxLines:]
	}
	return out
}
