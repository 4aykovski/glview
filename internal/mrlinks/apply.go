package mrlinks

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	xansi "github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/gitlab"
)

// cell is one visible rune of a rendered line: its bytes in the line and
// whether it already lies inside an OSC 8 hyperlink.
type cell struct {
	r          rune
	start, end int
	linked     bool
}

// Apply wraps the occurrences of the links' texts in rendered ANSI text
// into OSC 8 hyperlinks. The visible text does not change. An occurrence
// is a whole token on one line (neighbours are not letters, digits or
// "_"); text already inside an OSC 8 hyperlink is left alone; of
// overlapping occurrences the longer wins, then the earlier link.
func Apply(rendered string, links []gitlab.Link) string {
	if len(links) == 0 || rendered == "" {
		return rendered
	}
	lines := strings.Split(rendered, "\n")
	linked := false // an OSC 8 hyperlink stays open across lines
	for i, line := range lines {
		var cells []cell
		cells, linked = scan(line, linked)
		lines[i] = applyLine(line, cells, links)
	}
	return strings.Join(lines, "\n")
}

// scan returns the visible runes of line; open tells whether a hyperlink
// is open at its start and the second result whether one is open at its end.
func scan(line string, open bool) ([]cell, bool) {
	var cells []cell
	for i := 0; i < len(line); {
		if line[i] == 0x1b {
			n := seqLen(line[i:])
			if seq := line[i : i+n]; strings.HasPrefix(seq, "\x1b]8;") {
				open = hyperlinkURL(seq) != ""
			}
			i += n
			continue
		}
		r, size := utf8.DecodeRuneInString(line[i:])
		cells = append(cells, cell{r: r, start: i, end: i + size, linked: open})
		i += size
	}
	return cells, open
}

// seqLen is the byte length of the escape sequence at the start of s.
func seqLen(s string) int {
	if len(s) < 2 {
		return len(s)
	}
	switch s[1] {
	case '[': // CSI: parameters, then a final byte 0x40..0x7e
		for i := 2; i < len(s); i++ {
			if s[i] >= 0x40 && s[i] <= 0x7e {
				return i + 1
			}
		}
	case ']', 'P', '_', '^', 'X': // OSC and string sequences: BEL or ST
		for i := 2; i < len(s); i++ {
			if s[i] == 0x07 {
				return i + 1
			}
			if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' {
				return i + 2
			}
		}
	default:
		return 2
	}
	return len(s)
}

// hyperlinkURL extracts the URL of an OSC 8 sequence ("" for the closing one).
func hyperlinkURL(seq string) string {
	body := strings.TrimPrefix(seq, "\x1b]8;")
	body = strings.TrimSuffix(strings.TrimSuffix(body, "\x07"), "\x1b\\")
	_, u, _ := strings.Cut(body, ";")
	return u
}

type match struct {
	from, to int // cell indexes, to exclusive
	link     int
}

func applyLine(line string, cells []cell, links []gitlab.Link) string {
	var found []match
	for li, l := range links {
		text := []rune(l.Text)
		if len(text) == 0 {
			continue
		}
		for from := 0; from+len(text) <= len(cells); from++ {
			if !fits(cells, from, text) {
				continue
			}
			found = append(found, match{from, from + len(text), li})
		}
	}
	if len(found) == 0 {
		return line
	}
	sort.SliceStable(found, func(i, j int) bool {
		a, b := found[i], found[j]
		if la, lb := a.to-a.from, b.to-b.from; la != lb {
			return la > lb
		}
		return a.link < b.link
	})
	taken := make([]bool, len(cells))
	var picked []match
	for _, m := range found {
		free := true
		for k := m.from; k < m.to; k++ {
			if taken[k] {
				free = false
				break
			}
		}
		if !free {
			continue
		}
		for k := m.from; k < m.to; k++ {
			taken[k] = true
		}
		picked = append(picked, m)
	}
	sort.Slice(picked, func(i, j int) bool { return picked[i].from < picked[j].from })
	var b strings.Builder
	pos := 0
	for _, m := range picked {
		start, end := cells[m.from].start, cells[m.to-1].end
		b.WriteString(line[pos:start])
		b.WriteString(xansi.SetHyperlink(links[m.link].URL))
		b.WriteString(line[start:end])
		b.WriteString(xansi.ResetHyperlink())
		pos = end
	}
	b.WriteString(line[pos:])
	return b.String()
}

// fits reports whether text sits at cells[from:] as a whole token outside
// existing hyperlinks.
func fits(cells []cell, from int, text []rune) bool {
	for k, r := range text {
		c := cells[from+k]
		if c.r != r || c.linked {
			return false
		}
	}
	if from > 0 && wordRune(cells[from-1].r) {
		return false
	}
	if to := from + len(text); to < len(cells) && wordRune(cells[to].r) {
		return false
	}
	return true
}

func wordRune(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }
