// Package diff turns the unified diff hunks GitLab returns for a file into
// numbered lines and finds the line a diff thread points at. It has no UI
// dependencies.
package diff

import (
	"strconv"
	"strings"

	"github.com/4aykovski/glview/internal/gitlab"
)

// Kind is the role of a line.
type Kind int

const (
	Hunk      Kind = iota // "@@ -a,b +c,d @@ …" header
	Context               // unchanged line
	Add                   // added line
	Del                   // removed line
	NoNewline             // "\ No newline at end of file"
)

// Line is one line of a file diff.
type Line struct {
	Kind Kind
	// Old and New are the line numbers in the old and the new file; 0 where
	// the line has none (Add has no Old, Del no New, Hunk and NoNewline
	// neither).
	Old, New int
	// Text is the line without its leading +, - or space; for Hunk the whole
	// header, for NoNewline the whole marker.
	Text string
}

// Parse splits hunks (as in DiffFile.Diff: "@@" headers, no ---/+++) into
// lines. Text before the first header and empty input give no lines.
func Parse(hunks string) []Line {
	if hunks == "" {
		return nil
	}
	var out []Line
	var old, nw int
	inHunk := false
	for raw := range strings.SplitSeq(strings.TrimSuffix(hunks, "\n"), "\n") {
		raw = strings.TrimSuffix(raw, "\r")
		if strings.HasPrefix(raw, "@@") {
			o, n, ok := hunkStart(raw)
			if ok {
				old, nw, inHunk = o, n, true
				out = append(out, Line{Kind: Hunk, Text: raw})
				continue
			}
		}
		if !inHunk {
			continue
		}
		switch {
		case strings.HasPrefix(raw, "\\"):
			out = append(out, Line{Kind: NoNewline, Text: raw})
		case strings.HasPrefix(raw, "+"):
			out = append(out, Line{Kind: Add, New: nw, Text: raw[1:]})
			nw++
		case strings.HasPrefix(raw, "-"):
			out = append(out, Line{Kind: Del, Old: old, Text: raw[1:]})
			old++
		default:
			out = append(out, Line{Kind: Context, Old: old, New: nw, Text: strings.TrimPrefix(raw, " ")})
			old++
			nw++
		}
	}
	return out
}

// hunkStart reads the first old and new line numbers of "@@ -a[,b] +c[,d] @@".
func hunkStart(h string) (old, nw int, ok bool) {
	rest := strings.TrimPrefix(h, "@@ -")
	if rest == h {
		return 0, 0, false
	}
	oldS, rest, found := strings.Cut(rest, " +")
	if !found {
		return 0, 0, false
	}
	newS, _, _ := strings.Cut(rest, " @@")
	o, err1 := strconv.Atoi(strings.SplitN(oldS, ",", 2)[0])
	n, err2 := strconv.Atoi(strings.SplitN(newS, ",", 2)[0])
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return o, n, true
}

// Counts returns the number of added and removed lines.
func Counts(lines []Line) (add, del int) {
	for _, l := range lines {
		switch l.Kind {
		case Add:
			add++
		case Del:
			del++
		}
	}
	return add, del
}

// Find returns the index of the line a thread at pos points at: by NewLine
// among added and context lines, by OldLine among removed (and context)
// lines when NewLine is 0. -1 when the diff has no such line.
func Find(lines []Line, pos gitlab.NotePosition) int {
	for i, l := range lines {
		switch {
		case pos.NewLine > 0:
			if (l.Kind == Add || l.Kind == Context) && l.New == pos.NewLine {
				return i
			}
		case pos.OldLine > 0:
			if (l.Kind == Del || l.Kind == Context) && l.Old == pos.OldLine {
				return i
			}
		}
	}
	return -1
}
