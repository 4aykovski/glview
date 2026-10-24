package newmr

import (
	"context"
	"slices"
	"sync"

	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/gitlab"
)

// checkState is what is known about the pair of branches: whether the
// source is on the server, the comparison and the open MR for the pair.
type checkState struct {
	key     string // "src\x00dst" the state is for
	started bool   // the request was sent
	done    bool   // the answer arrived

	absent bool // the server answered and the source branch is not there
	cmp    gitlab.Comparison
	cmpErr error
	mr     *gitlab.MergeRequest // the open MR from source into target
	mrErr  error                // the lookup of mr failed
}

func (f *Form) pairKey() string { return f.src.text() + "\x00" + f.dst.text() }

// markPair notes that the pair of branches changed: the old answer is
// dropped and the title follows the new source.
func (f *Form) markPair() {
	f.checkGen.Next()
	f.check = checkState{key: f.pairKey()}
	f.followTitle()
}

// followTitle sets the title of a form whose title was not edited: the
// subject of the only commit, otherwise the branch name made readable.
func (f *Form) followTitle() {
	if !f.titleFollows {
		return
	}
	title := humanize(f.src.text())
	if st := f.check; st.done && st.key == f.pairKey() && st.cmpErr == nil && len(st.cmp.Commits) == 1 && st.cmp.Commits[0].Title != "" {
		title = st.cmp.Commits[0].Title
	}
	f.title.SetValue(title)
	f.title.CursorEnd()
}

// startCheck asks for the state of the pair of branches: the branch on the
// server and, once it is there, the comparison target…source and the open
// MR from the source into the target, in parallel.
func (f *Form) startCheck() tea.Cmd {
	src, dst := f.src.text(), f.dst.text()
	if src == "" || dst == "" || src == dst {
		f.check = checkState{key: f.pairKey()}
		return nil
	}
	f.check = checkState{key: f.pairKey(), started: true}
	key := f.check.key
	tok := f.checkGen.Next()
	client, p := f.env.Client, f.project
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		st := checkState{key: key, started: true, done: true}
		names, srvErr := client.Projects().Branches(ctx, p, "^"+src+"$")
		if srvErr == nil && !slices.Contains(names, src) {
			// Nothing to compare and no MR to look for: push first.
			st.absent = true
			return checkMsg{tok: tok, st: st}
		}
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			st.cmp, st.cmpErr = client.Tags().Compare(ctx, p, dst, src)
		}()
		go func() {
			defer wg.Done()
			page, err := client.MergeRequests().List(ctx, p, gitlab.MRFilter{State: gitlab.MRStateOpened, SourceBranch: src, TargetBranch: dst}, "", 5)
			if err != nil {
				st.mrErr = err
				return
			}
			for i := range page.Items {
				// GitLab forbids one open MR per pair of branches only.
				if page.Items[i].TargetBranch == dst {
					st.mr = &page.Items[i]
					return
				}
			}
		}()
		wg.Wait()
		return checkMsg{tok: tok, st: st}
	}
}

func (f *Form) onCheck(msg checkMsg) tea.Cmd {
	if !f.checkGen.Valid(msg.tok) {
		return nil
	}
	f.check = msg.st
	f.followTitle()
	if f.waiting && f.snap != nil && f.snap.key == msg.st.key {
		f.waiting = false
		return f.proceed()
	}
	return nil
}

// local reports whether branch is the one the local folder sits on.
func (f *Form) local(branch string) bool {
	return f.repo != nil && !f.repo.Detached && f.repo.Branch == branch
}

// problem is the reason the branches of st cannot make an MR, "" when they
// can; st must be done.
func (f *Form) problem(st checkState, src, dst string) string {
	m := f.env.Msg.NewMR
	switch {
	case st.absent && f.local(src):
		return m.NotPushed(src)
	case st.absent, st.cmpErr != nil:
		return m.CompareErr
	case len(st.cmp.Commits) == 0:
		return m.NoChanges(src, dst)
	}
	return ""
}

// unpushed is the number of commits of the local source branch that are not
// on the server, 0 when unknown.
func (f *Form) unpushed(src string) int {
	if f.repo == nil || !f.local(src) {
		return 0
	}
	if ab := f.env.Git.AheadBehind(f.repo.Root); ab.Known {
		return ab.Ahead
	}
	return 0
}
