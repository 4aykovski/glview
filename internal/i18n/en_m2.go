package i18n

import "fmt"

func fillM2EN(m *Messages) {
	w := &m.WS
	w.ShownOf = func(shown, total int) string { return fmt.Sprintf("showing %d of %d", shown, total) }
	w.ShownOfMany = func(shown int) string { return fmt.Sprintf("showing %d of many", shown) }
	w.ColFolder, w.ColGitLab, w.ColBranch, w.ColAheadBehind = "Folder", "GitLab", "Local branch", "↑↓"
	w.ColMRs, w.ColDefault, w.ColTag, w.ColActivity = "MRs", "Default branch", "Tag", "Act"
	w.NoAccess = "no access"
	w.Archived = "archived"
	w.EmptyHint = "p pick a project · F search GitLab · q quit"
	w.FilterHint = "text in a folder name or project path · enter apply · esc cancel"
	w.Search = "search"
	w.PrevRemote = "remote"
	w.PrevOpenMRs = func(open, review int) string {
		s := Count(EN, open, PluralForms{"open MR", "", "open MRs"})
		if review > 0 {
			s += fmt.Sprintf(" (%d waiting for your review)", review)
		}
		return s
	}
	w.PrevLocal = func(branch string) string { return "on " + branch + " locally" }
	w.PrevNoMR = "no open MR"
	w.PrevLoading = "◐ loading…"
	w.PrevNoPipe = "no pipelines"
	w.GroupCount = func(n int, state string, atLeast bool) string {
		if state == "opened" {
			state = "open"
		}
		if atLeast {
			return fmt.Sprintf("≥%d %s", n, state)
		}
		return fmt.Sprintf("%d %s", n, state)
	}
	w.GroupWaiting = func(n int) string { return fmt.Sprintf("%d waiting for you", n) }
	w.GroupShown = func(n int) string { return fmt.Sprintf("showing %d", n) }
	w.More = func(n int) string { return fmt.Sprintf("↳ %d more · f filter by project", n) }
	w.EmptyMRs = func(state string, mine bool) string {
		if state == "opened" {
			state = "open"
		}
		if mine {
			return "no " + state + " MRs of yours"
		}
		return "no " + state
	}
	w.EmptyPipes = func(mine bool) string {
		if mine {
			return "no pipelines of yours"
		}
		return "no pipelines"
	}
	w.Loading = "loading…"
	w.ColYou = "You"
	w.MeMRs = "me: author/assignee/reviewer"
	w.MePipes = "me: mine"
	w.StatusGroups = "groups by latest activity"
	w.ProjectKey = "project"
	w.ProjectsKey = "projects"
	w.PickTitle = "Projects"
	w.PickMarked = func(n int) string { return fmt.Sprintf("%d marked", n) }
	w.PickHints = "space mark  ctrl+a all  enter apply  esc close"
	w.ShownProjects = func(shown, total int) string {
		return fmt.Sprintf("showing %d of %d %s", shown, total, Plural(EN, total, PluralForms{"project", "", "projects"}))
	}
	w.FoundIn = func(found, total int) string {
		return fmt.Sprintf("found in %d of %d %s", found, total, Plural(EN, total, PluralForms{"project", "", "projects"}))
	}
	w.NothingFound = "nothing found"
	w.KeyGroups, w.KeyCollapse, w.KeyProject, w.KeyOpenProject = "next/prev project", "expand/collapse", "filter by project", "open project"
	w.KeyProjects = "filter projects"

	s := &m.Search
	s.PickerTitle, s.GlobalTitle = "Switch project", "Search GitLab"
	s.SecRemote, s.SecWorkspace, s.SecStarred, s.SecRecent = "FROM GIT REMOTE", "WORKSPACE", "STARRED", "RECENT"
	s.SecProjects, s.SecGroups = "Projects", "Groups"
	s.Current, s.Local = "current", "local"
	s.MRs = func(n int) string { return fmt.Sprintf("%d MR", n) }
	s.NothingFound = "nothing found — F search GitLab"
	s.NothingGlobal = "nothing found"
	s.TypeMore = "type at least 3 characters"
	s.Searching = "◐ searching…"
	s.PickerHints = "↑↓ select  enter open  esc close"
	s.GlobalHints = "↑↓ select  enter open (a group opens as a workspace)  esc close"
	s.Failed = func(err string) string { return "✗ search failed: " + err }
	s.Counter = func(shown, total int) string { return fmt.Sprintf("%d / %d", shown, total) }
	s.KeyPicker, s.KeyGlobal = "pick project", "search GitLab"
	s.HelpTitle, s.KeyMove, s.KeyHalfPage, s.KeyClose = "p and F overlays", "move", "half a page down / up", "close"

	g := &m.Git
	g.Progress = func(op string, done, total int) string { return fmt.Sprintf("⟳ %s %d/%d", op, done, total) }
	g.Busy = func(op string) string { return op + " is already running" }
	g.Summary = func(op string, ok, skipped, failed int, first string) string {
		s := fmt.Sprintf("%s: %d ✓", op, ok)
		if skipped > 0 {
			s += fmt.Sprintf(", %d ○", skipped)
		}
		if failed > 0 {
			s += fmt.Sprintf(", %d ✗", failed)
		}
		if first != "" {
			s += " (" + first + ")"
		}
		return s
	}
	g.ReasonAuth, g.ReasonNetwork, g.ReasonDiverged = "needs authentication", "network", "diverged"
	g.ReasonDirty, g.ReasonTimeout = "local changes in the way", "timeout"
	g.ReasonDetached, g.ReasonNoUpstream, g.ReasonInProgress, g.ReasonNoRemote = "detached", "no upstream", "merge or rebase in progress", "no remote"
	g.AuthHint = func(dir string) string { return "run git fetch in " + dir + " manually (ssh key, known_hosts)" }
	g.UpToDate = "up to date"
	g.Moved = func(n int) string { return fmt.Sprintf("+%d", n) }
	g.PullTitle = "Pull (fast-forward only)?"
	g.PullBody = func(update, skip int, reasons string) string {
		s := "update " + Count(EN, update, PluralForms{"repository", "", "repositories"})
		if skip > 0 {
			s += fmt.Sprintf(", skip %d (%s)", skip, reasons)
		}
		return s
	}
	g.KeyFetch, g.KeyPull = "fetch", "pull ff-only"
}
