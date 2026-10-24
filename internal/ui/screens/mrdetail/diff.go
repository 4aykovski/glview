package mrdetail

import (
	"context"
	"crypto/sha1" //nolint:gosec // GitLab anchors diff files by the SHA1 of the path
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/diff"
	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/screens/comment"
	"github.com/4aykovski/glview/internal/ui/screens/render"
	"github.com/4aykovski/glview/internal/ui/screens/threads"
)

const (
	diffTTL    = 10 * time.Minute
	diffShift  = 8 // columns a press of ←/→ moves the code text
	diffTabLen = 4 // a tab in code text is shown as four spaces
)

type diffsLoadedMsg struct {
	tok   common.Token
	files []gitlab.DiffFile
	err   error
}

// rowKind is the role of a row of the Diff tab.
type rowKind int

const (
	rowBlank  rowKind = iota // between files; the cursor skips it
	rowFile                  // file header
	rowHunk                  // @@ header
	rowCode                  // added, removed or context line
	rowNoNL                  // "\ No newline at end of file"
	rowThread                // one-line summary of a diff thread
	rowNote                  // "diff hidden" / "no text diff" under a file header
)

// Bodies of a rowNote.
const (
	noteHidden = iota
	noteNoText
)

// drow is one visible row. line indexes diffTab.lines[file] for the line
// rows; disc is the discussion ID of a thread row; for a rowNote line is
// noteHidden or noteNoText.
type drow struct {
	kind rowKind
	file int
	line int
	disc string
}

// diffTab is the state of the Diff tab: loaded lazily on the first visit,
// then once per r and when the head SHA differs from reqSHA (like Commits).
// rows is the flat list of visible rows; the cursor indexes it.
type diffTab struct {
	files  []gitlab.DiffFile
	lines  [][]diff.Line
	counts [][2]int // added, removed per file
	numW   int      // width of a line number column

	loaded  bool
	loading bool
	err     bool
	opened  bool
	reqSHA  string
	reqKey  string // cache key of the last request
	refresh bool

	fold map[string]bool // user's choice by file key, over the defaults
	rows []drow
	cur  int
	xoff int // shift of the code text, columns

	threads map[string]gitlab.Discussion // by ID, for thread rows
}

// fileKey identifies a file across reloads.
func fileKey(f gitlab.DiffFile) string {
	if f.NewPath != "" {
		return f.NewPath
	}
	return f.OldPath
}

// folded reports whether file i is shown as its header only: the user's
// choice, else generated files and files the server hid.
func (d *diffTab) folded(i int) bool {
	f := d.files[i]
	if v, ok := d.fold[fileKey(f)]; ok {
		return v
	}
	return f.Generated || f.TooLarge
}

// rowKey identifies a row across rebuilds: the file or the line (by numbers)
// or the thread.
func (d *diffTab) rowKey(r drow) string {
	if r.kind == rowBlank || r.file >= len(d.files) {
		return ""
	}
	fk := fileKey(d.files[r.file])
	switch r.kind {
	case rowFile:
		return "f|" + fk
	case rowThread:
		return "t|" + r.disc
	case rowNote:
		return "n|" + fk
	}
	l := d.lines[r.file][r.line]
	return fmt.Sprintf("l|%s|%d|%d|%d|%s", fk, l.Kind, l.Old, l.New, l.Text)
}

func (d *diffTab) fileOfCursor() (int, bool) {
	if d.cur < 0 || d.cur >= len(d.rows) || d.rows[d.cur].kind == rowBlank {
		return 0, false
	}
	return d.rows[d.cur].file, true
}

// pathMatches reports whether position p lies in file f: removed lines (no
// new line number) are matched by the old path.
func pathMatches(p *gitlab.NotePosition, f gitlab.DiffFile) bool {
	if p.NewLine > 0 {
		return p.NewPath != "" && p.NewPath == f.NewPath
	}
	return p.OldPath != "" && p.OldPath == f.OldPath
}

// rebuildDiff recomputes the rows from the files, folds and discussions and
// keeps the cursor on the same row (else on the header of its file, else at
// the same index).
func (s *Screen) rebuildDiff() {
	d := &s.diff
	var oldKey, oldFile string
	if d.cur >= 0 && d.cur < len(d.rows) {
		oldKey = d.rowKey(d.rows[d.cur])
		if r := d.rows[d.cur]; r.kind != rowBlank && r.file < len(d.files) {
			oldFile = "f|" + fileKey(d.files[r.file])
		}
	}

	type at struct{ file, line int }
	onLine := map[at][]string{}
	d.threads = map[string]gitlab.Discussion{}
	for _, disc := range threads.User(s.discs) {
		p := disc.Position
		if p == nil {
			continue
		}
		for fi, f := range d.files {
			if !pathMatches(p, f) {
				continue
			}
			if li := diff.Find(d.lines[fi], *p); li >= 0 {
				onLine[at{fi, li}] = append(onLine[at{fi, li}], disc.ID)
				d.threads[disc.ID] = disc
			}
			break
		}
	}

	rows := make([]drow, 0, 64)
	for fi := range d.files {
		if fi > 0 {
			rows = append(rows, drow{kind: rowBlank, file: fi})
		}
		rows = append(rows, drow{kind: rowFile, file: fi})
		if d.folded(fi) {
			continue
		}
		if len(d.lines[fi]) == 0 {
			note := noteNoText
			if d.files[fi].TooLarge {
				note = noteHidden
			}
			rows = append(rows, drow{kind: rowNote, file: fi, line: note})
			continue
		}
		for li, l := range d.lines[fi] {
			k := rowCode
			switch l.Kind {
			case diff.Hunk:
				k = rowHunk
			case diff.NoNewline:
				k = rowNoNL
			}
			rows = append(rows, drow{kind: k, file: fi, line: li})
			for _, id := range onLine[at{fi, li}] {
				rows = append(rows, drow{kind: rowThread, file: fi, line: li, disc: id})
			}
		}
	}
	d.rows = rows

	switch {
	case len(rows) == 0:
		d.cur = 0
	default:
		found := -1
		for _, key := range []string{oldKey, oldFile} {
			if key == "" {
				continue
			}
			for i, r := range rows {
				if d.rowKey(r) == key {
					found = i
					break
				}
			}
			if found >= 0 {
				break
			}
		}
		if found < 0 {
			found = min(max(d.cur, 0), len(rows)-1)
		}
		d.cur = found
	}
	s.diffVisible()
}

// setDiffFiles stores a loaded diff.
func (s *Screen) setDiffFiles(files []gitlab.DiffFile) {
	d := &s.diff
	d.files = files
	d.lines = make([][]diff.Line, len(files))
	d.counts = make([][2]int, len(files))
	maxNo := 1
	for i, f := range files {
		d.lines[i] = diff.Parse(f.Diff)
		a, del := diff.Counts(d.lines[i])
		d.counts[i] = [2]int{a, del}
		for _, l := range d.lines[i] {
			maxNo = max(maxNo, l.Old, l.New)
		}
	}
	d.numW = len(fmt.Sprint(maxNo))
	d.loaded = true
	s.rebuildDiff()
}

// ---- loading ----

func (s *Screen) diffCacheKey() string { return s.key("diffs") + ":" + s.mr.SHA }

// loadDiffs requests the MR diff and remembers the head SHA it is requested
// at.
func (s *Screen) loadDiffs(force bool) tea.Cmd {
	tok := s.diffGen.Next()
	d := &s.diff
	d.loading, d.err, d.reqSHA, d.reqKey = true, false, s.mr.SHA, s.diffCacheKey()
	env, ref, iid, key := s.env, s.ref, s.iid, d.reqKey
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		files, _, err := common.Fetch(ctx, env.Cache, key, diffTTL, force, func(ctx context.Context) ([]gitlab.DiffFile, error) {
			return env.Client.MergeRequests().Diffs(ctx, ref, iid)
		})
		return diffsLoadedMsg{tok: tok, files: files, err: err}
	}
}

// reloadDiffs runs on every answer to the MR details: it reloads an opened
// Diff tab after r, or (fresh details) when the head SHA moved past the last
// request. Poll ticks that bring the same SHA do nothing.
func (s *Screen) reloadDiffs(fresh bool) tea.Cmd {
	d := &s.diff
	if !d.opened {
		return nil
	}
	moved := fresh && s.mr.SHA != "" && s.mr.SHA != d.reqSHA
	if !d.refresh && !moved {
		return nil
	}
	d.refresh = false
	return s.loadDiffs(true)
}

// ---- keys ----

// diffKey handles the keys of the Diff tab that tab-independent handling
// does not cover.
func (s *Screen) diffKey(k string) tea.Cmd {
	d := &s.diff
	n := len(d.rows)
	page := max(1, s.bodyHeight()-1)
	switch k {
	case "j", "down":
		d.cur = s.stepRow(d.cur, 1)
	case "k", "up":
		d.cur = s.stepRow(d.cur, -1)
	case "pgdown", "ctrl+d":
		d.cur = s.landRow(min(d.cur+page, n-1), -1)
	case "pgup", "ctrl+u":
		d.cur = s.landRow(max(d.cur-page, 0), 1)
	case "g", "home":
		d.cur = s.landRow(0, 1)
	case "G", "end":
		d.cur = s.landRow(n-1, -1)
	case "J":
		d.cur = s.jumpFile(1)
	case "K":
		d.cur = s.jumpFile(-1)
	case "right":
		d.xoff += diffShift
	case "left":
		d.xoff = max(0, d.xoff-diffShift)
	case "space":
		s.toggleFile()
	case "z":
		s.toggleAll()
	case "enter":
		return s.openThread()
	}
	s.diffVisible()
	return nil
}

// stepRow moves one row, skipping blank separators; at an end it stays.
func (s *Screen) stepRow(from, delta int) int {
	d := &s.diff
	for i := from + delta; i >= 0 && i < len(d.rows); i += delta {
		if d.rows[i].kind != rowBlank {
			return i
		}
	}
	return from
}

// landRow is the nearest non-blank row to i, looking in direction dir first.
func (s *Screen) landRow(i, dir int) int {
	d := &s.diff
	if len(d.rows) == 0 {
		return 0
	}
	i = min(max(i, 0), len(d.rows)-1)
	if d.rows[i].kind != rowBlank {
		return i
	}
	if j := s.stepRow(i, dir); j != i {
		return j
	}
	return s.stepRow(i, -dir)
}

// jumpFile moves to the header of the next or previous file.
func (s *Screen) jumpFile(delta int) int {
	d := &s.diff
	cur, ok := d.fileOfCursor()
	if !ok {
		return d.cur
	}
	target := cur + delta
	if delta < 0 && d.rows[d.cur].kind != rowFile {
		target = cur // K inside a file goes to its own header first
	}
	for i, r := range d.rows {
		if r.kind == rowFile && r.file == target {
			return i
		}
	}
	return d.cur
}

// toggleFile folds or unfolds the file under the cursor; the cursor lands
// on its header.
func (s *Screen) toggleFile() {
	d := &s.diff
	fi, ok := d.fileOfCursor()
	if !ok {
		return
	}
	d.fold[fileKey(d.files[fi])] = !d.folded(fi)
	d.cur = s.headerRow(fi)
	s.rebuildDiff()
	d.cur = s.headerRow(fi)
}

// toggleAll folds every file if any is open, else unfolds them all.
func (s *Screen) toggleAll() {
	d := &s.diff
	if len(d.files) == 0 {
		return
	}
	anyOpen := false
	for i := range d.files {
		if !d.folded(i) {
			anyOpen = true
		}
	}
	fi, _ := d.fileOfCursor()
	for _, f := range d.files {
		d.fold[fileKey(f)] = anyOpen
	}
	s.rebuildDiff()
	d.cur = s.headerRow(fi)
}

func (s *Screen) headerRow(fi int) int {
	for i, r := range s.diff.rows {
		if r.kind == rowFile && r.file == fi {
			return i
		}
	}
	return 0
}

// onThreadRow reports whether the cursor is on a thread row.
func (s *Screen) onThreadRow() bool {
	d := &s.diff
	return d.cur < len(d.rows) && d.rows[d.cur].kind == rowThread
}

// openThread shows the thread under the cursor on the Discussions tab.
func (s *Screen) openThread() tea.Cmd {
	d := &s.diff
	if d.cur >= len(d.rows) || d.rows[d.cur].kind != rowThread {
		return nil
	}
	id := d.rows[d.cur].disc
	cmd := s.setTab(tabDiscussions)
	s.threads.Layout(s.w)
	s.scroll = s.threads.Focus(id)
	return cmd
}

// diffVisible scrolls the window so the cursor row is in it.
func (s *Screen) diffVisible() {
	d := &s.diff
	h := s.bodyHeight()
	if d.cur < s.scroll {
		s.scroll = d.cur
	}
	if d.cur >= s.scroll+h {
		s.scroll = d.cur - h + 1
	}
	s.scroll = max(0, min(s.scroll, max(0, len(d.rows)-h)))
}

// ---- comment, links ----

// linePosition is the position of the code line under the cursor and its
// "path:line" label; false when the cursor is not on a code line or the
// instance did not report the diff refs.
func (s *Screen) linePosition() (gitlab.DiffPosition, string, bool) {
	d := &s.diff
	if s.tab != tabDiff || s.mr.DiffRefs == nil || d.cur >= len(d.rows) {
		return gitlab.DiffPosition{}, "", false
	}
	r := d.rows[d.cur]
	if r.kind != rowCode {
		return gitlab.DiffPosition{}, "", false
	}
	f, l := d.files[r.file], d.lines[r.file][r.line]
	pos := gitlab.DiffPosition{DiffRefs: *s.mr.DiffRefs, OldPath: f.OldPath, NewPath: f.NewPath}
	switch l.Kind {
	case diff.Add:
		pos.NewLine = l.New
	case diff.Del:
		pos.OldLine = l.Old
	default:
		pos.OldLine, pos.NewLine = l.Old, l.New
	}
	shown := l.New
	if shown == 0 {
		shown = l.Old
	}
	return pos, fmt.Sprintf("%s:%d", fileKey(f), shown), true
}

// commentCmd opens the comment form: for the line under the cursor on the
// Diff tab, else for the MR.
func (s *Screen) commentCmd() tea.Cmd {
	if s.archived {
		return nil
	}
	t := comment.Target{Project: s.ref, Kind: comment.KindMR, IID: s.iid, Title: s.mr.Title}
	if pos, where, ok := s.linePosition(); ok {
		t.Position, t.Where = &pos, where
	}
	return comment.Open(s.env, t)
}

// diffLink is the MR diff page anchored at the file under the cursor.
func (s *Screen) diffLink() string {
	base := s.mr.WebURL
	if base == "" {
		return ""
	}
	base += "/diffs"
	if fi, ok := s.diff.fileOfCursor(); ok && s.diff.loaded {
		sum := sha1.Sum([]byte(s.diff.files[fi].NewPath)) //nolint:gosec // see import
		return base + "#" + hex.EncodeToString(sum[:])
	}
	return base
}

// ---- view ----

func (s *Screen) diffTabName() string {
	name := s.env.Msg.Diff.Tab
	switch {
	case s.diff.loaded:
		return fmt.Sprintf("%s %d", name, len(s.diff.files))
	case s.mr.Diff != nil:
		return fmt.Sprintf("%s %d", name, s.mr.Diff.Files)
	}
	return name
}

// diffView draws the visible rows of the tab.
func (s *Screen) diffView() string {
	st, m := s.env.Styles, s.env.Msg
	d := &s.diff
	switch {
	case !d.loaded && d.err:
		return st.Error.Render("  " + m.Project.LoadFailed)
	case !d.loaded:
		return st.Muted.Render(m.Diff.Loading)
	case len(d.files) == 0:
		return "\n  " + m.Diff.Empty
	}
	h := s.bodyHeight()
	s.diffVisible()
	end := min(len(d.rows), s.scroll+h)
	out := make([]string, 0, end-s.scroll)
	for i := s.scroll; i < end; i++ {
		out = append(out, s.diffRow(i))
	}
	return strings.Join(out, "\n")
}

// diffRow draws row i: the marker column, then the body clipped to the
// width. The cursor row is drawn plain so its highlight is not broken by the
// colors inside.
func (s *Screen) diffRow(i int) string {
	d := &s.diff
	r := d.rows[i]
	cw := max(1, s.w-1)
	body := s.diffBody(r, cw)
	if i == d.cur && r.kind != rowBlank {
		plain := ansi.Strip(body)
		pad := max(0, cw-ansi.StringWidth(plain))
		return s.env.Styles.RowSelected.Render("›" + plain + strings.Repeat(" ", pad))
	}
	return " " + body
}

func (s *Screen) diffBody(r drow, cw int) string {
	st, m := s.env.Styles, s.env.Msg.Diff
	d := &s.diff
	if r.kind == rowBlank {
		return ""
	}
	numCols := 2*d.numW + 2 // old, new and their separators
	switch r.kind {
	case rowFile:
		return s.fileHeader(r.file, cw)
	case rowHunk:
		return st.DiffHunk.Render(ansi.Truncate(d.lines[r.file][r.line].Text, cw, "…"))
	case rowNote:
		text := m.NoText
		if r.line == noteHidden {
			text = m.Hidden
		}
		return st.Muted.Render(ansi.Truncate("  "+text, cw, "…"))
	case rowThread:
		return s.threadRow(d.threads[r.disc], numCols, cw)
	}
	l := d.lines[r.file][r.line]
	avail := cw - numCols - 1
	if avail <= 0 {
		return ""
	}
	if l.Kind == diff.NoNewline {
		return st.Muted.Render(strings.Repeat(" ", numCols) + ansi.Truncate(l.Text, cw-numCols, "…"))
	}
	num := func(n int) string {
		if n == 0 {
			return strings.Repeat(" ", d.numW)
		}
		return fmt.Sprintf("%*d", d.numW, n)
	}
	text := ansi.Cut(strings.ReplaceAll(l.Text, "\t", strings.Repeat(" ", diffTabLen)), d.xoff, d.xoff+avail)
	nums := st.Muted.Render(num(l.Old) + " " + num(l.New) + " ")
	switch l.Kind {
	case diff.Add:
		return nums + st.DiffAdd.Render("+"+text)
	case diff.Del:
		return nums + st.DiffDel.Render("-"+text)
	}
	return nums + " " + text
}

// fileHeader is "▾ path" with badges on the left and "+A −D" on the right.
func (s *Screen) fileHeader(fi, cw int) string {
	st, m := s.env.Styles, s.env.Msg.Diff
	d := &s.diff
	f := d.files[fi]
	mark := "▾ "
	if d.folded(fi) {
		mark = "▸ "
	}
	title := fileKey(f)
	if f.RenamedFile && f.OldPath != "" && f.OldPath != f.NewPath {
		title = f.OldPath + " → " + f.NewPath
	}
	var badges []string
	if f.NewFile {
		badges = append(badges, m.New)
	}
	if f.DeletedFile {
		badges = append(badges, m.Deleted)
	}
	if f.Generated {
		badges = append(badges, m.Generated)
	}
	if f.TooLarge {
		badges = append(badges, m.TooLarge)
	}
	right, rw := "", 0
	if c := d.counts[fi]; c[0]+c[1] > 0 {
		add, del := fmt.Sprintf("+%d", c[0]), fmt.Sprintf("−%d", c[1])
		right = st.DiffAdd.Render(add) + " " + st.DiffDel.Render(del)
		rw = len(add) + 1 + ansi.StringWidth(del)
	}
	left := st.Bold.Render(mark + title)
	for _, b := range badges {
		left += "  " + st.Muted.Render(b)
	}
	left = render.Clip(left, max(1, cw-rw-1))
	gap := max(1, cw-ansi.StringWidth(left)-rw)
	return left + strings.Repeat(" ", gap) + right
}

// threadRow is the one-line summary of a diff thread under its line.
func (s *Screen) threadRow(disc gitlab.Discussion, numCols, cw int) string {
	st, env := s.env.Styles, s.env
	if len(disc.Notes) == 0 {
		return ""
	}
	n := disc.Notes[0]
	glyph := st.Warning.Render("●")
	if disc.Resolvable && disc.Resolved {
		glyph = st.Muted.Render("✓")
	}
	head := glyph + " " + n.Author.Username + st.Muted.Render(" · "+render.Ago(env.Lang, env.Now(), n.CreatedAt)+" · ")
	tail := ""
	if r := len(disc.Notes) - 1; r > 0 {
		tail = st.Muted.Render(" · " + env.Msg.Plural.Replies(r))
	}
	first := strings.Join(strings.Fields(render.FirstLine(n.Body)), " ")
	room := cw - numCols - ansi.StringWidth(head) - ansi.StringWidth(tail)
	return strings.Repeat(" ", numCols) + head + ansi.Truncate(first, max(1, room), "…") + tail
}
