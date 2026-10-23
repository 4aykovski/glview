package i18n

import "fmt"

func fillM1EN(m *Messages) {
	k := &m.Keys
	k.Filter = "filter"
	k.Approve = "approve"
	k.Merge = "merge"
	k.Rebase = "rebase"
	k.Checkout = "checkout"
	k.Retry = "retry"
	k.Cancel = "cancel"
	k.NewPipeline = "new pipeline"
	k.Log = "log"
	k.Play = "play"
	k.Follow = "follow"
	k.Fold = "fold"
	k.FoldAll = "fold all"
	k.Stage = "stage"
	k.Job = "job"
	k.Edit = "edit"
	k.Add = "add"
	k.Delete = "delete"
	k.Submit = "run"
	k.Next = "next"
	k.Details = "details"
	k.Attempts = "attempts"
	k.PrevNextAttempt = "prev/next attempt"
	k.State, k.Status, k.Reset = "state", "status", "reset filters"
	k.BrowserList, k.BrowserOne, k.BrowserJob = "list in browser", "pipeline in browser", "job in browser"
	k.BrowserMR, k.BrowserCommit = "MR in browser", "commit in browser"
	k.Find = "find in log"
	k.Comment = "comment"
	k.Threads = "threads"

	m.Plural.MRs = func(n int) string { return Count(EN, n, PluralForms{"MR", "", "MRs"}) }
	m.Plural.Pipelines = func(n int) string { return Count(EN, n, PluralForms{"pipeline", "", "pipelines"}) }
	m.Plural.Jobs = func(n int) string { return Count(EN, n, PluralForms{"job", "", "jobs"}) }
	m.Plural.Threads = func(n int) string { return Count(EN, n, PluralForms{"thread", "", "threads"}) }
	m.Plural.Replies = func(n int) string { return Count(EN, n, PluralForms{"reply", "", "replies"}) }
	m.Plural.Files = func(n int) string { return Count(EN, n, PluralForms{"file", "", "files"}) }

	p := &m.Project
	p.LoadingMore = "◐ loading more…"
	p.LoadFailed = "✗ could not load, r — retry"
	p.Loading = "◐ loading…"
	p.Counter = func(loaded, total int, atLeast bool) string {
		if total < 0 {
			return fmt.Sprintf("%d loaded", loaded)
		}
		t := groupThousands(total, ' ')
		if atLeast {
			t = "≥" + t
		}
		return fmt.Sprintf("%d of %s", loaded, t)
	}
	p.FilterHint = "key:value … text · tab completes · enter applies · esc closes"
	p.FilterUnknown = func(key, known string) string { return fmt.Sprintf("unknown key %q, known: %s", key, known) }
	p.FilterBad = func(key, value, allowed string) string {
		if allowed == "" {
			return fmt.Sprintf("%s: value %q is not valid", key, value)
		}
		return fmt.Sprintf("%s: %q is not valid, allowed: %s", key, value, allowed)
	}
	p.PreviewHidden = "widen the terminal for the preview"
	p.KeyState, p.KeyScope, p.KeyReviewer, p.KeyDraft, p.KeyLabels = "MR state", "whose MRs", "reviewer", "draft or not", "labels"
	p.KeyRef, p.KeyStatus, p.KeySource, p.KeyUser = "branch or tag", "pipeline status", "what started it", "who started it"
	p.StatusPoll = func(i string) string { return "poll " + i }
	p.StatusLive = func(n int, i string) string { return fmt.Sprintf("live: %d running · poll %s", n, i) }
	p.NoteBranch, p.NoteTag = "branch", "tag"

	pp := &m.Pipe
	pp.EmptyBase, pp.EmptyWhere = "No pipelines", ", where "
	pp.QualRef = func(v string) string { return "ref is " + v }
	pp.QualStatus = func(v string) string { return "status is " + v }
	pp.QualSource = func(v string) string { return "source is " + v }
	pp.QualUser = func(v string) string { return "started by @" + v }
	pp.QualMine = "you started them"
	pp.EmptyHint = "/ changes filters · m toggles \"mine\""
	pp.RetryTitle = func(id int64) string { return fmt.Sprintf("Retry failed jobs of pipeline #%d?", id) }
	pp.CancelTitle = func(id int64) string { return fmt.Sprintf("Cancel pipeline #%d?", id) }
	pp.RetryBody = "Failed jobs of the pipeline will be started again."
	pp.CancelBody = "Running and pending jobs will be canceled."
	pp.ToastRetried = func(id int64) string { return fmt.Sprintf("retrying pipeline #%d", id) }
	pp.ToastCanceled = func(id int64) string { return fmt.Sprintf("pipeline #%d canceled", id) }
	pp.NoLogJobs = "no failed or running jobs"
	pp.PrevOn = func(ref string) string { return "on " + ref }
	pp.PrevStagesLoading, pp.PrevNoStages = "◐ loading stages…", "no stages"

	pp.Crumb = "Pipelines"
	pp.Summary = func(jobs, parts string) string { return jobs + ": " + parts }
	pp.PanelStage, pp.PanelRunner, pp.PanelAttempts, pp.PanelAllowFailure = "stage", "runner", "attempts", "allow_failure"
	pp.PanelNoLog = "(log is empty)"
	pp.BridgeWaiting, pp.BridgeNoAccess = "waiting", "no access"
	pp.Downstream = "downstream"
	pp.YAMLErrors = "Pipeline configuration errors"
	pp.ToastNoAccess = "No access to the downstream project"
	pp.NoJobs = "no jobs"
	pp.JobRetried = func(n string) string { return "retrying " + n }
	pp.JobCanceled = func(n string) string { return n + " canceled" }
	pp.JobPlayed = func(n string) string { return n + " started" }
	pp.CancelJobTitle = func(n string) string { return "Cancel job " + n + "?" }
	pp.CancelJobBody = "The job will be stopped."
	pp.PlayTitle = func(n string) string { return "Run " + n }
	pp.NotManual = "only manual jobs can be played"
	pp.AttemptsTitle = func(n string) string { return "Attempts of " + n }
	pp.AttemptLatest = "latest"
	pp.AttemptsKeys = "j/k choose · enter log · esc close"

	f := &m.Form
	f.ColKey, f.ColValue, f.ColType = "KEY", "VALUE", "type"
	f.KeyPlaceholder, f.ValuePlaceholder = "KEY", "value"
	f.VarsEmpty = "no variables — the job starts as it is"
	f.VarsHint = "a add · d delete · enter edit · space type · tab next · esc cancel"
	f.RunHint = "enter run · tab next · esc cancel"
	f.TypeEnv, f.TypeFile = "env_var", "file"
	f.RunTitle, f.RefLabel, f.VarsLabel = "Run pipeline", "Ref", "Variables"
	f.RefCurrent, f.RefTag = "current in git", "tag"
	f.RefSearching, f.NoRefs, f.RefsFailed = "searching…", "no matching branches or tags", "no refs: request failed"
	f.RefBranches, f.RefTags = "branches", "tags"
	f.HintsLabel, f.HintsLoading, f.HintsNone = "From .gitlab-ci.yml:", "loading hints from .gitlab-ci.yml…", "no variables declared in .gitlab-ci.yml"
	f.HintsUnavailable = "hints unavailable"
	f.WillCreate = func(ref string, n int) string { return fmt.Sprintf("will be created: ref=%s, variables=%d", ref, n) }
	f.RefRequired = "Enter a branch or tag"
	f.PickValue = func(key string) string { return "Value for " + key }
	f.Created = func(id int64) string { return fmt.Sprintf("pipeline #%d created", id) }
	f.RunNoVars = "no variables — a to add"
	f.RunButton = "Run"
	f.HintsAll = func(n int) string { return fmt.Sprintf("s all (%d)", n) }
	f.RefKeys = "↑/↓ ref · pgdn/pgup group · enter pick · tab next · esc cancel"
	f.VarsKeys = "a add · d delete · enter edit · space type · 1-9/s hint · tab next · esc cancel"
	f.ButtonKeys = "enter run · tab next · esc cancel"
	f.EditKeys = "enter save · esc discard input"
	f.PickKeys = "j/k choose · enter take · esc close"
	f.CheckingKeys, f.CreatingKeys = "checking tag… · esc cancel", "creating…"
	f.TagRerunTitle = func(tag string) string { return "Run another pipeline on tag " + tag + "?" }
	f.TagRerunBody = func(id int64, status string) string {
		return fmt.Sprintf("The tag already has pipeline #%d (%s). The new one runs on the same commit: the tag's jobs — build, publish, deploy — will run again.", id, status)
	}
	f.TagCheckFailed = func(err string) string { return "Could not check whether the tag already has a pipeline: " + err }

	lg := &m.Log
	lg.Follow, lg.NoFollow = "● follow", "○ paused"
	lg.Updated = func(age string) string { return "updated " + age }
	lg.Truncated = "… beginning truncated, o — open the full log in the browser"
	lg.Empty = func(status string) string { return "log is empty: job " + status }
	lg.SearchPrompt = "/"
	lg.MatchOf = func(i, n int) string { return fmt.Sprintf("match %d of %d", i, n) }
	lg.NoMatches = "no matches"
	lg.SearchKeys = "n / N next / previous"
	lg.Crumb = "log"
	lg.Attempt = func(i, n int) string { return fmt.Sprintf("attempt %d/%d", i, n) }

	r := &m.MR
	r.ColIID, r.ColTitle, r.ColAuthor, r.ColBranch = "IID", "Title", "Author", "Branch"
	r.ColPipeline, r.ColAppr, r.ColThr, r.ColUpd = "Pipeline", "Appr", "Thr", "Upd"
	r.Draft = "draft"
	r.Conflicts = "⚠ conflicts"
	r.EmptyBase = func(state string) string {
		switch state {
		case "merged":
			return "No merged MRs"
		case "closed":
			return "No closed MRs"
		case "all":
			return "No MRs"
		}
		return "No open MRs"
	}
	r.EmptyWhere = " where "
	r.QualReviewerMe = "you are the reviewer"
	r.QualAuthorMe = "you are the author"
	r.QualAssigneeMe = "you are the assignee"
	r.QualMine = "you are the author, assignee or reviewer"
	r.QualDraft = "they are drafts"
	r.QualNotDraft = "they are not drafts"
	r.QualReviewer = func(u string) string { return "@" + u + " is the reviewer" }
	r.QualLabels = func(l string) string { return "labels are " + l }
	r.QualText = func(t string) string { return fmt.Sprintf("the title has %q", t) }
	r.EmptyHint = "/ changes filters · m toggles \"me\""
	r.PrevApprovals = func(names string) string { return "approved by " + names }
	r.PrevNoApprovals = "no approvals yet"
	r.PrevPipeline = "pipeline"
	r.PrevDiff = func(add, del, files int) string {
		return fmt.Sprintf("+%d −%d in %s", add, del, Count(EN, files, PluralForms{"file", "", "files"}))
	}
	r.PrevNoDesc = "no description"

	r.Crumb = "MRs"
	r.TabOverview, r.TabDiscussions, r.TabPipelines = "Overview", "Discussions", "Pipelines"
	r.Opened = func(age string) string { return "opened " + age + " ago" }
	r.Merged = func(by, age string) string {
		if by == "" {
			return "merged " + age + " ago"
		}
		return "merged by " + by + " " + age + " ago"
	}
	r.Closed = func(age string) string { return "closed " + age + " ago" }
	r.ChipApprovals = func(have, req int) string {
		if req > 0 && have < req {
			return fmt.Sprintf("%d/%d approved", have, req)
		}
		return fmt.Sprintf("%d approved", have)
	}
	r.ChipThreads = func(n int) string {
		return fmt.Sprintf("%s unresolved", Count(EN, n, PluralForms{"thread", "", "threads"}))
	}
	r.SecApprovals, r.SecLabels, r.SecPipeline, r.SecLastThread = "APPROVALS", "LABELS", "PIPELINE", "LAST THREAD"
	r.None, r.ThreadResolved, r.ThreadUnresolved = "none", "resolved", "unresolved"
	r.EmptyDiscussions, r.EmptyPipelines, r.Loading = "No discussions", "No pipelines for this MR", "◐ loading…"
	r.ColStatus, r.ColID, r.ColRef, r.ColSource, r.ColCommit, r.ColUser = "Status", "ID", "Ref", "Source", "Commit", "User"
	r.ColDur, r.ColAgo = "Dur", "Ago"
	r.TabCommits, r.EmptyCommits, r.ColSHA = "Commits", "No commits", "SHA"
	r.NeedApprovals = func(left int) string { return fmt.Sprintf("%d more needed", left) }

	r.ToastApproved = func(iid int64) string { return fmt.Sprintf("approved !%d", iid) }
	r.ToastUnapproved = func(iid int64) string { return fmt.Sprintf("approval removed !%d", iid) }
	r.ToastMerged = func(iid int64) string { return fmt.Sprintf("merged !%d", iid) }
	r.ToastAutoMerge = func(iid int64) string { return fmt.Sprintf("!%d will merge when the pipeline succeeds", iid) }
	r.MergePushed = "Branch was pushed to, refresh (r)"
	r.CannotMerge = func(status string) string { return "Can't merge: " + status }
	r.RebaseTitle = func(iid int64) string { return fmt.Sprintf("Rebase !%d?", iid) }
	r.RebaseBody = "The source branch will be rebased onto the target branch on the server."
	r.RebaseDone, r.RebaseStillRunning = "rebase done", "rebase is still running"
	r.RebaseFailed = func(reason string) string { return "rebase failed: " + reason }
	r.RebaseChip = "◐ rebase…"
	r.MergeTitle = func(iid int64) string { return fmt.Sprintf("Merge !%d", iid) }
	r.MergeNow, r.MergeWhenPipeline = "Merge now", "Merge when the pipeline succeeds"
	r.MergeSquash, r.MergeRemoveBranch = "Squash commits", "Remove source branch"
	r.MergeReason = func(status, label string) string {
		if label == "" {
			return "Can't merge: " + status
		}
		return "Can't merge: " + status + " — " + label
	}
	r.MergeHint = "←/→ method · space toggle · enter merge · esc cancel"
	r.MergeHintBlocked = "esc close"
	r.CheckoutFailed = func(code int) string { return fmt.Sprintf("glab exited with an error (code %d)", code) }
	r.GlabNotLoggedIn = "glab is not logged in"
	r.StatusLabel = func(s string) string {
		switch s {
		case "mergeable":
			return "ready to merge"
		case "conflict":
			return "has conflicts"
		case "need_rebase":
			return "needs rebase"
		case "ci_still_running":
			return "pipeline still running"
		case "ci_must_pass":
			return "pipeline must pass"
		case "not_approved":
			return "needs approvals"
		case "discussions_not_resolved":
			return "unresolved threads"
		case "draft_status":
			return "draft"
		case "blocked_status":
			return "blocked"
		case "checking":
			return "being checked"
		case "unchecked":
			return "not checked yet"
		case "external_status_checks":
			return "waiting for external checks"
		case "requested_changes":
			return "changes requested"
		case "not_open":
			return "not open"
		}
		return ""
	}
}

// groupThousands formats 1482 as "1 482".
func groupThousands(n int, sep rune) string {
	s := fmt.Sprint(n)
	var out []rune
	for i, r := range []rune(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, sep)
		}
		out = append(out, r)
	}
	return string(out)
}
