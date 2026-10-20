// Package gittest is a scriptable gitexec.Runner for UI tests.
package gittest

import (
	"context"
	"sync"

	"github.com/4aykovski/glview/internal/gitexec"
	"github.com/4aykovski/glview/internal/gitremote"
)

// Runner returns scripted results per folder. Folders listed in Block never
// finish (until the context is cancelled); folders without a script are
// updated with ahead/behind from AB.
type Runner struct {
	mu      sync.Mutex
	Results map[string]gitexec.Result
	AB      map[string][2]int // dir → {ahead, behind}
	Block   map[string]bool
	// OnRun runs before a folder's result is sent (to change files).
	OnRun func(op gitexec.Op, dir string)

	runs []string // "fetch:/dir"
	abs  int
}

// New returns an empty runner.
func New() *Runner {
	return &Runner{Results: map[string]gitexec.Result{}, AB: map[string][2]int{}, Block: map[string]bool{}}
}

// Runs lists the operations run so far ("fetch:/src/x").
func (r *Runner) Runs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.runs...)
}

// AheadBehindCalls is the number of AheadBehind calls.
func (r *Runner) AheadBehindCalls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.abs
}

func (r *Runner) Run(ctx context.Context, op gitexec.Op, repos []gitremote.Repo) <-chan gitexec.Result {
	out := make(chan gitexec.Result)
	go func() {
		defer close(out)
		for _, repo := range repos {
			r.mu.Lock()
			r.runs = append(r.runs, op.String()+":"+repo.Root)
			block := r.Block[repo.Root]
			res, ok := r.Results[repo.Root]
			ab, hasAB := r.AB[repo.Root]
			hook := r.OnRun
			r.mu.Unlock()
			if block {
				<-ctx.Done()
				return
			}
			if !ok {
				res = gitexec.Result{Status: gitexec.StatusUpdated}
				if hasAB {
					res.Ahead, res.Behind, res.HasUpstream = ab[0], ab[1], true
				}
			}
			res.Dir, res.Op = repo.Root, op
			if hook != nil {
				hook(op, repo.Root)
			}
			select {
			case out <- res:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

func (r *Runner) AheadBehind(_ context.Context, repo gitremote.Repo) (int, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.abs++
	ab, ok := r.AB[repo.Root]
	if !ok {
		return 0, 0, gitexec.ErrNoUpstream
	}
	return ab[0], ab[1], nil
}
