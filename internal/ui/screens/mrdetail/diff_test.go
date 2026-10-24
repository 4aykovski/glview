package mrdetail

import (
	"crypto/sha1" //nolint:gosec // GitLab anchors diff files by the SHA1 of the path
	"encoding/hex"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitlab/fake"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/uitest"
)

const (
	diffsMethod = "MergeRequests.Diffs"
	allocPath   = "internal/slots/alloc.go"
)

// allocHunk: context 40, removed 41, added 41 and 42, context 43.
const allocHunk = "@@ -40,3 +40,4 @@ func alloc() {\n func alloc() {\n-\tslots = nil\n+\tmu.Lock()\n+\tslots = map[int]int{}\n }\n"

// testDiffs are three files: a changed one, a rename and a new file.
func testDiffs() []gitlab.DiffFile {
	return []gitlab.DiffFile{
		{OldPath: allocPath, NewPath: allocPath, Diff: allocHunk},
		{OldPath: "docs/old.md", NewPath: "docs/metrics.md", RenamedFile: true, Diff: "@@ -1 +1 @@\n-# Old\n+# Metrics\n"},
		{OldPath: "cmd/tool/main.go", NewPath: "cmd/tool/main.go", NewFile: true, Diff: "@@ -0,0 +1,2 @@\n+package main\n+\n"},
	}
}

func diffRefs() *gitlab.DiffRefs {
	return &gitlab.DiffRefs{BaseSHA: "b0b0", StartSHA: "5757", HeadSHA: "a91f2c0d"}
}

// diffSetup opens the MR with the diff registered and the Diff tab shown.
func diffSetup(t *testing.T, mr gitlab.MergeRequest, files []gitlab.DiffFile) (*uitest.Driver, *Screen, *fake.Client) {
	t.Helper()
	d, scr, client := diffSetupOverview(t, mr, files)
	d.Key("tab")
	return d, scr, client
}

// diffSetupOverview is diffSetup staying on Overview.
func diffSetupOverview(t *testing.T, mr gitlab.MergeRequest, files []gitlab.DiffFile) (*uitest.Driver, *Screen, *fake.Client) {
	t.Helper()
	if mr.DiffRefs == nil {
		mr.DiffRefs = diffRefs()
	}
	d, _, scr, client := setup(t, mr)
	client.SetMRDiffs(path, mr.IID, files)
	return d, scr, client
}

// lineWith returns the plain view line containing sub.
func lineWith(t *testing.T, d *uitest.Driver, sub string) string {
	t.Helper()
	for _, l := range strings.Split(ansi.Strip(d.View()), "\n") {
		if strings.Contains(l, sub) {
			return l
		}
	}
	t.Fatalf("no line with %q in:\n%s", sub, d.View())
	return ""
}

// fields splits a line into words, dropping the cursor marker.
func fields(l string) []string {
	return strings.Fields(strings.TrimPrefix(strings.TrimSpace(l), "›"))
}

func equalFields(got, want []string) bool {
	return strings.Join(got, "\x00") == strings.Join(want, "\x00")
}

func cursorRow(s *Screen) drow { return s.diff.rows[s.diff.cur] }

// toLine moves the cursor to the first row whose key contains sub.
func toLine(t *testing.T, d *uitest.Driver, s *Screen, text string) {
	t.Helper()
	d.Key("g")
	for range len(s.diff.rows) + 1 {
		r := cursorRow(s)
		if r.kind == rowCode && s.diff.lines[r.file][r.line].Text == text {
			return
		}
		d.Key("j")
	}
	t.Fatalf("no code line %q", text)
}

func TestDiffTabOrderAndCount(t *testing.T) {
	_, scr, _ := diffSetupOverview(t, baseMR(), testDiffs())
	bar := plainTabBar(scr)
	order := []string{"Overview", "Diff 3", "Discussions 2", "Pipelines 2", "Commits"}
	last := -1
	for _, name := range order {
		i := strings.Index(bar, name)
		if i <= last {
			t.Fatalf("tab order: %q in %q", name, bar)
		}
		last = i
	}
	mr := baseMR()
	mr.Diff = nil
	_, scr, _ = diffSetupOverview(t, mr, testDiffs())
	if bar := plainTabBar(scr); strings.Contains(bar, "Diff 3") || !strings.Contains(bar, "Diff") {
		t.Fatalf("unknown stats: no count before loading: %q", bar)
	}
	d, scr, _ := diffSetupOverview(t, mr, testDiffs()[:2])
	d.Key("tab")
	if bar := plainTabBar(scr); !strings.Contains(bar, "Diff 2") {
		t.Fatalf("the loaded diff wins: %q", bar)
	}
}

func TestDiffLoadsLazily(t *testing.T) {
	d, _, client := diffSetupOverview(t, baseMR(), testDiffs())
	d.Key("r")
	if n := client.CallCount(diffsMethod); n != 0 {
		t.Fatalf("diff requested before the tab was opened: %d", n)
	}
	d.Key("tab")
	if n := client.CallCount(diffsMethod); n != 1 {
		t.Fatalf("first visit: %d requests", n)
	}
	d.Key("tab", "shift+tab")
	if n := client.CallCount(diffsMethod); n != 1 {
		t.Fatalf("second visit must not reload: %d", n)
	}
}

func TestDiffFileView(t *testing.T) {
	d, _, _ := diffSetup(t, baseMR(), testDiffs())
	d.RequireView("▾ "+allocPath, "+2 −1", "@@ -40,3 +40,4 @@ func alloc() {")
	if got := fields(lineWith(t, d, "▾ "+allocPath)); !equalFields(got, []string{"▾", allocPath, "+2", "−1"}) {
		t.Errorf("file header = %v", got)
	}
	// The hunk header also holds "func alloc() {": look for the code line.
	foundContext := false
	for _, l := range strings.Split(ansi.Strip(d.View()), "\n") {
		if f := fields(l); len(f) >= 4 && f[0] == "40" && f[1] == "40" && f[2] == "func" {
			foundContext = true
		}
	}
	if !foundContext {
		t.Errorf("context line must carry both numbers 40 40:\n%s", d.View())
	}
	if got := fields(lineWith(t, d, "slots = nil")); !equalFields(got, []string{"41", "-", "slots", "=", "nil"}) {
		t.Errorf("removed line = %v (old number only, sign -)", got)
	}
	if got := fields(lineWith(t, d, "mu.Lock()")); !equalFields(got, []string{"41", "+", "mu.Lock()"}) {
		t.Errorf("added line = %v (new number only, sign +)", got)
	}
	if got := fields(lineWith(t, d, "slots = map")); got[0] != "42" || got[1] != "+" {
		t.Errorf("second added line = %v", got)
	}
}

func TestDiffRenameAndNewFile(t *testing.T) {
	d, _, _ := diffSetup(t, baseMR(), testDiffs())
	d.RequireView("▾ docs/old.md → docs/metrics.md", "▾ cmd/tool/main.go  new")
}

func TestDiffTabsBecomeSpaces(t *testing.T) {
	d, _, _ := diffSetup(t, baseMR(), testDiffs())
	if strings.Contains(d.View(), "\t") {
		t.Fatal("tabs must be expanded")
	}
	if l := lineWith(t, d, "mu.Lock()"); !strings.Contains(l, "+    mu.Lock()") {
		t.Fatalf("a tab is four spaces: %q", l)
	}
}

func TestDiffHorizontalScroll(t *testing.T) {
	files := []gitlab.DiffFile{{OldPath: "a.go", NewPath: "a.go", Diff: "@@ -1 +1 @@\n+abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789\n"}}
	d, scr, _ := diffSetup(t, baseMR(), files)
	d.Send(tea.WindowSizeMsg{Width: 80, Height: 24})
	d.RequireView("abcdefghijklmnop")
	d.Key("right", "right")
	if scr.diff.xoff != 16 {
		t.Fatalf("xoff = %d", scr.diff.xoff)
	}
	d.RequireView("qrstuvwxyzABC")
	d.RequireNoView("abcdefghijklmnop")
	if got := fields(lineWith(t, d, "qrstuvwxyzABC")); got[0] != "1" || !strings.HasPrefix(got[1], "+") {
		t.Fatalf("the number stays: %v", got)
	}
	d.Key("left", "left", "left")
	if scr.diff.xoff != 0 {
		t.Fatalf("xoff = %d, must stop at 0", scr.diff.xoff)
	}
}

func TestDiffNoNewlineMarker(t *testing.T) {
	files := []gitlab.DiffFile{{OldPath: "a.go", NewPath: "a.go", Diff: "@@ -1 +1 @@\n-a\n\\ No newline at end of file\n+b\n"}}
	d, _, _ := diffSetup(t, baseMR(), files)
	if got := fields(lineWith(t, d, "No newline")); got[0] != "\\" {
		t.Fatalf("marker has no numbers: %v", got)
	}
}

func TestDiffFileJumpFoldAndAll(t *testing.T) {
	d, scr, _ := diffSetup(t, baseMR(), testDiffs())
	toLine(t, d, scr, "\tmu.Lock()")
	d.Key("J")
	if r := cursorRow(scr); r.kind != rowFile || r.file != 1 {
		t.Fatalf("J: cursor on %+v", r)
	}
	d.Key("K", "K")
	if r := cursorRow(scr); r.kind != rowFile || r.file != 0 {
		t.Fatalf("K: cursor on %+v", r)
	}
	d.Key("K")
	if r := cursorRow(scr); r.file != 0 {
		t.Fatalf("K on the first file stays: %+v", r)
	}

	toLine(t, d, scr, "\tmu.Lock()")
	d.Key("space")
	d.RequireView("▸ " + allocPath)
	d.RequireNoView("mu.Lock()")
	if r := cursorRow(scr); r.kind != rowFile || r.file != 0 {
		t.Fatalf("space puts the cursor on the header: %+v", r)
	}
	d.Key("space")
	d.RequireView("▾ "+allocPath, "mu.Lock()")

	d.Key("z")
	d.RequireView("▸ "+allocPath, "▸ docs/old.md → docs/metrics.md", "▸ cmd/tool/main.go")
	d.RequireNoView("mu.Lock()", "# Metrics")
	d.Key("z")
	d.RequireView("▾ "+allocPath, "mu.Lock()", "# Metrics", "package main")
	d.Key("space", "z") // one file folded: z folds the rest
	d.RequireNoView("# Metrics")
}

func TestDiffCursorSkipsBlankLines(t *testing.T) {
	d, scr, _ := diffSetup(t, baseMR(), testDiffs())
	d.Key("J")
	d.Key("k")
	if r := cursorRow(scr); r.kind == rowBlank {
		t.Fatal("the cursor must not stop on the blank separator")
	}
	d.Key("G")
	if r := cursorRow(scr); r.kind == rowBlank || scr.diff.cur != len(scr.diff.rows)-1 {
		t.Fatalf("G: %+v", r)
	}
	d.Key("g")
	if scr.diff.cur != 0 {
		t.Fatalf("g: %d", scr.diff.cur)
	}
}

func TestDiffHiddenByServerAndBinary(t *testing.T) {
	files := []gitlab.DiffFile{
		{OldPath: "big.sql", NewPath: "big.sql", TooLarge: true},
		{OldPath: "logo.png", NewPath: "logo.png"},
		{OldPath: "api/gen.pb.go", NewPath: "api/gen.pb.go", Generated: true, Diff: "@@ -1 +1 @@\n-a\n+b\n"},
	}
	d, scr, _ := diffSetup(t, baseMR(), files)
	d.RequireView("▸ big.sql  too large", "▾ logo.png", "no text diff", "▸ api/gen.pb.go  generated")
	d.RequireNoView("diff hidden")
	d.Key("space") // big.sql: first row is its header
	d.RequireView("▾ big.sql  too large", "diff hidden by GitLab — o opens the file in the browser")
	d.Key("J", "J")
	d.Key("space")
	d.RequireView("▾ api/gen.pb.go  generated", "+b")

	// The user's choice survives a refresh; so does the cursor.
	toLine(t, d, scr, "b")
	d.Key("r")
	d.RequireView("▾ api/gen.pb.go  generated", "+b")
	if r := cursorRow(scr); r.kind != rowCode || scr.diff.lines[r.file][r.line].Text != "b" {
		t.Fatalf("cursor after refresh: %+v", r)
	}
	d.Key("ctrl+l")
	d.RequireView("слишком большой", "нет текстового диффа", "сгенерирован")
}

func TestDiffStatesLoadingEmptyAndError(t *testing.T) {
	d, _, client := diffSetupOverview(t, baseMR(), testDiffs())
	client.SetDelay(diffsMethod, 10*uitest.CmdTimeout)
	d.Key("tab")
	d.RequireView("◐ loading diff…")

	d, _, _ = diffSetup(t, baseMR(), nil)
	d.RequireView("No changes")
	d.Key("ctrl+l")
	d.RequireView("Нет изменений")

	d, _, client = diffSetupOverview(t, baseMR(), testDiffs())
	client.FailNext(diffsMethod, &gitlab.Error{Kind: gitlab.ErrServer, Status: 500})
	d.Key("tab")
	d.RequireView("GitLab server error")
	d.Key("tab", "shift+tab") // re-entering retries
	d.RequireView("▾ " + allocPath)
	if n := client.CallCount(diffsMethod); n != 2 {
		t.Fatalf("requests = %d", n)
	}
}

func TestDiffReloadOnRefreshAndHeadSHAChangeNotOnPoll(t *testing.T) {
	d, scr, client := diffSetupOverview(t, baseMR(), testDiffs())

	// Before the tab is opened a new SHA does not request the diff.
	client.UpdateMR(path, 479, func(m *gitlab.MergeRequest) { m.SHA = "b7c8d9e0" })
	d.Send(common.PollTickMsg{Token: scr.poll.Current()})
	if n := client.CallCount(diffsMethod); n != 0 {
		t.Fatalf("SHA change before opening: %d requests", n)
	}
	d.Key("tab")
	if n := client.CallCount(diffsMethod); n != 1 {
		t.Fatalf("open: %d", n)
	}
	gets := client.CallCount("MergeRequests.Get")
	d.Send(common.PollTickMsg{Token: scr.poll.Current()})
	if client.CallCount("MergeRequests.Get") != gets+1 {
		t.Fatal("tick must reload the MR")
	}
	if n := client.CallCount(diffsMethod); n != 1 {
		t.Fatalf("tick with the same SHA must not reload the diff: %d", n)
	}

	client.UpdateMR(path, 479, func(m *gitlab.MergeRequest) { m.SHA = "c8d9e0f1" })
	d.Send(common.PollTickMsg{Token: scr.poll.Current()})
	if n := client.CallCount(diffsMethod); n != 2 {
		t.Fatalf("a push must reload the diff once: %d", n)
	}
	d.Send(common.PollTickMsg{Token: scr.poll.Current()})
	if n := client.CallCount(diffsMethod); n != 2 {
		t.Fatalf("no reload once the SHA matches: %d", n)
	}

	d.Key("r")
	if n := client.CallCount(diffsMethod); n != 3 {
		t.Fatalf("r must reload the diff once: %d", n)
	}
}

func TestDiffOpenedOnDiffLoadsOnce(t *testing.T) {
	d, _, scr, client := setupTab(t, baseMR(), false, TabDiff, func(c *fake.Client) { c.SetMRDiffs(path, 479, testDiffs()) })
	d.RequireView("▾ "+allocPath, "+2 −1")
	if scr.tab != tabDiff {
		t.Fatalf("tab = %d", scr.tab)
	}
	if n := client.CallCount(diffsMethod); n != 1 {
		t.Fatalf("Diffs calls = %d, want 1", n)
	}
}

// ---- threads on lines ----

func lineThreads() []gitlab.Discussion {
	return []gitlab.Discussion{
		{ID: "t1", Resolvable: true, Position: &gitlab.NotePosition{Path: allocPath, Line: 42, OldPath: allocPath, NewPath: allocPath, NewLine: 42}, Notes: []gitlab.Note{
			{ID: 11, Author: gitlab.User{Username: "anna.k"}, Body: "Why a mutex here?\nsecond line", CreatedAt: now.Add(-2 * time.Hour)},
			{ID: 12, Author: gitlab.User{Username: "egor"}, Body: "Latency", CreatedAt: now.Add(-time.Hour)},
		}},
		{ID: "t2", Resolvable: true, Resolved: true, Position: &gitlab.NotePosition{Path: allocPath, Line: 41, OldPath: allocPath, NewPath: allocPath, OldLine: 41}, Notes: []gitlab.Note{
			{ID: 13, Author: gitlab.User{Username: "bob"}, Body: "Fine to drop", CreatedAt: now.Add(-3 * time.Hour)},
		}},
		{ID: "t3", Resolvable: true, Position: &gitlab.NotePosition{Path: allocPath, Line: 300, OldPath: allocPath, NewPath: allocPath, NewLine: 300}, Notes: []gitlab.Note{
			{ID: 14, Author: gitlab.User{Username: "d.orlov"}, Body: "Stale remark", CreatedAt: now.Add(-4 * time.Hour)},
		}},
	}
}

func threadSetup(t *testing.T) (*uitest.Driver, *Screen, *fake.Client) {
	t.Helper()
	d, scr, client := diffSetupOverview(t, baseMR(), testDiffs())
	client.SetDiscussions(path, 479, lineThreads())
	d.Key("r", "tab")
	return d, scr, client
}

func TestDiffThreadsUnderTheirLines(t *testing.T) {
	d, _, _ := threadSetup(t)
	d.RequireView("● anna.k · 2h · Why a mutex here? · 1 reply", "✓ bob · 3h · Fine to drop")
	d.RequireNoView("Stale remark", "d.orlov")
	v := ansi.Strip(d.View())
	// The thread of line 42 sits right under it, the removed-line thread under 41.
	if i, j := strings.Index(v, "slots = map"), strings.Index(v, "Why a mutex"); i < 0 || j < i {
		t.Fatalf("thread must follow its line:\n%s", v)
	}
	if i, j := strings.Index(v, "slots = nil"), strings.Index(v, "Fine to drop"); i < 0 || j < i {
		t.Fatalf("thread of a removed line must follow it:\n%s", v)
	}
	d.Key("ctrl+l")
	d.RequireView("1 ответ")
}

func TestDiffThreadOfAnotherVersionIsNotAttached(t *testing.T) {
	d, scr, client := diffSetupOverview(t, baseMR(), testDiffs())
	ths := lineThreads()
	ths[0].Position.HeadSHA = "a91f2c0d" // the current version: stays
	ths[1].Position.HeadSHA = "0ldf0rce" // the line number matches, the code does not
	client.SetDiscussions(path, 479, ths)
	d.Key("r", "tab")
	d.RequireView("anna.k · 2h · Why a mutex here?")
	d.RequireNoView("Fine to drop", "bob")
	if _, ok := scr.diff.threads["t2"]; ok {
		t.Fatal("a thread of another head must not be attached to a line")
	}
	if _, ok := scr.threads.Current(); !ok {
		t.Fatal("the thread stays in Discussions")
	}
}

func TestDiffEnterOnThreadOpensDiscussions(t *testing.T) {
	d, scr, _ := threadSetup(t)
	for range len(scr.diff.rows) {
		if cursorRow(scr).kind == rowThread && cursorRow(scr).disc == "t1" {
			break
		}
		d.Key("j")
	}
	d.RequireView("enter thread")
	d.Key("enter")
	if scr.tab != tabDiscussions {
		t.Fatalf("tab = %d", scr.tab)
	}
	if c, ok := scr.threads.Current(); !ok || c.ID != "t1" {
		t.Fatalf("current thread = %+v", c)
	}
	d.RequireView("Why a mutex here?", "Latency")

	// enter elsewhere does nothing
	d.Key("shift+tab")
	d.Key("g", "enter")
	if scr.tab != tabDiff {
		t.Fatalf("enter on a file header must stay: tab %d", scr.tab)
	}
}

// ---- line comments ----

func typeText(d *uitest.Driver, text string) {
	for _, r := range text {
		d.Key(string(r))
	}
}

func TestDiffCommentOnAddedLine(t *testing.T) {
	d, scr, client := diffSetup(t, baseMR(), testDiffs())
	toLine(t, d, scr, "\tmu.Lock()")
	d.Send(tea.WindowSizeMsg{Width: 200, Height: 40})
	d.RequireView("c to line")
	d.Send(tea.WindowSizeMsg{Width: uitest.Width, Height: uitest.Height})
	d.Key("c")
	d.RequireView("Comment · !479 · " + allocPath + ":41")
	typeText(d, "Hold the lock longer")
	d.Key("tab", "enter")
	calls := client.CallsOf("MergeRequests.CommentLine")
	if len(calls) != 1 {
		t.Fatalf("CommentLine calls = %v", calls)
	}
	pos, ok := calls[0][3].(gitlab.DiffPosition)
	if !ok || calls[0][2] != "Hold the lock longer" {
		t.Fatalf("call = %v", calls[0])
	}
	want := gitlab.DiffPosition{DiffRefs: *diffRefs(), OldPath: allocPath, NewPath: allocPath, NewLine: 41}
	if pos != want {
		t.Fatalf("position = %+v, want %+v", pos, want)
	}
	if client.CallCount("MergeRequests.Comment") != 0 {
		t.Fatal("a line comment is not a plain comment")
	}
	// Discussions are fetched again and the new thread hangs under the line.
	d.RequireView("Comment added", "● egor")
	if v := ansi.Strip(d.View()); strings.Index(v, "● egor") < strings.Index(v, "mu.Lock()") {
		t.Fatalf("the new thread must follow line 41:\n%s", v)
	}
}

func TestDiffCommentOnRemovedAndContextLines(t *testing.T) {
	d, scr, client := diffSetup(t, baseMR(), testDiffs())
	toLine(t, d, scr, "\tslots = nil")
	d.Key("c")
	d.RequireView("Comment · !479 · " + allocPath + ":41")
	typeText(d, "gone")
	d.Key("tab", "enter")
	pos := client.CallsOf("MergeRequests.CommentLine")[0][3].(gitlab.DiffPosition)
	if pos.OldLine != 41 || pos.NewLine != 0 {
		t.Fatalf("removed line: %+v", pos)
	}

	toLine(t, d, scr, "func alloc() {")
	d.Key("c")
	typeText(d, "ctx")
	d.Key("tab", "enter")
	pos = client.CallsOf("MergeRequests.CommentLine")[1][3].(gitlab.DiffPosition)
	if pos.OldLine != 40 || pos.NewLine != 40 {
		t.Fatalf("context line: %+v", pos)
	}
}

func TestDiffCommentOnHeaderIsPlain(t *testing.T) {
	d, _, client := diffSetup(t, baseMR(), testDiffs())
	d.Send(tea.WindowSizeMsg{Width: 200, Height: 40})
	d.RequireView("c comment")
	d.Send(tea.WindowSizeMsg{Width: uitest.Width, Height: uitest.Height})
	d.Key("c")
	d.RequireView("Comment · !479 Fix race in courier slot allocator")
	typeText(d, "General remark")
	d.Key("tab", "enter")
	if client.CallCount("MergeRequests.Comment") != 1 || client.CallCount("MergeRequests.CommentLine") != 0 {
		t.Fatal("a header comment is a plain comment")
	}
}

func TestDiffCommentWithoutDiffRefsIsPlain(t *testing.T) {
	d, _, scr, client := setup(t, baseMR()) // DiffRefs unknown
	client.SetMRDiffs(path, 479, testDiffs())
	d.Key("tab")
	r := scr.diff.rows
	var cur int
	for i, row := range r {
		if row.kind == rowCode {
			cur = i
			break
		}
	}
	scr.diff.cur = cur
	d.Key("c")
	d.RequireView("Comment · !479 Fix race")
}

func TestDiffCommentRejectedKeepsTheForm(t *testing.T) {
	d, scr, client := diffSetup(t, baseMR(), testDiffs())
	toLine(t, d, scr, "\tmu.Lock()")
	client.FailNext("MergeRequests.CommentLine", &gitlab.Error{Kind: gitlab.ErrNotPossible, Status: 400, Message: "line_code is invalid"})
	d.Key("c")
	typeText(d, "stale")
	d.Key("tab", "enter")
	d.RequireView("line_code is invalid", "refresh (r)", "Comment · !479 · "+allocPath, "stale")
	d.Key("esc", "ctrl+l", "c")
	d.RequireView("Комментарий · !479 · " + allocPath)
}

func TestDiffLineDraftsAreKeptPerLine(t *testing.T) {
	d, scr, _ := diffSetup(t, baseMR(), testDiffs())
	toLine(t, d, scr, "\tmu.Lock()")
	d.Key("c")
	typeText(d, "draft one")
	d.Key("esc")
	d.Key("j", "c")
	d.RequireView(allocPath + ":42")
	d.RequireNoView("draft one")
	d.Key("esc", "k", "c")
	d.RequireView(allocPath+":41", "draft one")
}

func TestDiffArchivedProjectHasNoLineComments(t *testing.T) {
	d, _, scr, client := setupIn(t, baseMR(), true)
	client.SetMRDiffs(path, 479, testDiffs())
	d.Key("tab", "c")
	if scr.tab != tabDiff {
		t.Fatal("tab")
	}
	d.RequireNoView("Comment ·")
}

// ---- links ----

func TestDiffLinks(t *testing.T) {
	d, scr, _ := diffSetup(t, baseMR(), testDiffs())
	var opened []string
	scr.env.Browser = func(u string) error { opened = append(opened, u); return nil }
	mrURL := baseMR().WebURL
	sum := sha1.Sum([]byte(allocPath)) //nolint:gosec // see import
	want := mrURL + "/diffs#" + hex.EncodeToString(sum[:])
	toLine(t, d, scr, "\tmu.Lock()")
	d.Key("o")
	if len(opened) != 1 || opened[0] != want {
		t.Fatalf("opened = %q, want %q", opened, want)
	}
	d.Key("y")
	d.RequireView("copied: …/merge_requests/479/diffs#")

	d.Key("J")
	sum = sha1.Sum([]byte("docs/metrics.md")) //nolint:gosec // see import
	d.Key("o")
	if opened[1] != mrURL+"/diffs#"+hex.EncodeToString(sum[:]) {
		t.Fatalf("the renamed file is anchored by its new path: %q", opened[1])
	}
}

func TestDiffLinkBeforeLoadAndOnEmptyDiff(t *testing.T) {
	d, scr, client := diffSetupOverview(t, baseMR(), nil)
	client.SetDelay(diffsMethod, 10*uitest.CmdTimeout)
	var opened []string
	scr.env.Browser = func(u string) error { opened = append(opened, u); return nil }
	d.Key("tab", "o")
	if len(opened) != 1 || opened[0] != baseMR().WebURL+"/diffs" {
		t.Fatalf("before load: %q", opened)
	}
}

// ---- footer, fit, golden ----

func TestDiffFooter(t *testing.T) {
	d, _, _ := diffSetup(t, baseMR(), testDiffs())
	d.Send(tea.WindowSizeMsg{Width: 200, Height: 40})
	d.RequireView("j/k line", "J/K file", "space fold", "←/→ shift", "o file in browser")
	d.Key("ctrl+l")
	d.RequireView("j/k строка", "J/K файл", "space свернуть", "←/→ сдвиг", "o файл в браузере")
}

func TestDiffFitsTerminal(t *testing.T) {
	files := append(testDiffs(), gitlab.DiffFile{OldPath: "x.go", NewPath: "x.go", Diff: "@@ -1 +1 @@\n+" + strings.Repeat("long ", 60) + "\n"})
	d, _, client := diffSetupOverview(t, baseMR(), files)
	client.SetDiscussions(path, 479, lineThreads())
	d.Key("r", "tab")
	d.RequireFits()
	for _, keys := range [][]string{{"j", "j", "j"}, {"right", "right"}, {"G"}, {"z"}, {"z"}, {"c"}, {"esc", "?"}} {
		d.Key(keys...)
		d.RequireFits()
	}
}

func goldenDiffs() []gitlab.DiffFile {
	files := testDiffs()
	return append(files,
		gitlab.DiffFile{OldPath: "api/gen.pb.go", NewPath: "api/gen.pb.go", Generated: true, Diff: "@@ -1 +1 @@\n-a\n+b\n"},
		gitlab.DiffFile{OldPath: "big.sql", NewPath: "big.sql", TooLarge: true},
	)
}

func goldenSetup(t *testing.T) *uitest.Driver {
	t.Helper()
	d, scr, client := diffSetupOverview(t, baseMR(), goldenDiffs())
	client.SetDiscussions(path, 479, lineThreads())
	d.Key("r", "tab")
	toLine(t, d, scr, "\tmu.Lock()")
	return d
}

func TestGoldenDiff(t *testing.T) {
	d := goldenSetup(t)
	d.RequireGoldenSizes("mrdetail-diff")
	d.Key("ctrl+l")
	d.RequireGoldenSizes("mrdetail-diff-ru")
}

func TestGoldenDiffFolded(t *testing.T) {
	d := goldenSetup(t)
	d.Key("z")
	d.RequireGoldenSizes("mrdetail-diff-folded")
}
