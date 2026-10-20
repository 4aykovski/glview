// Package joblog parses GitLab job traces incrementally: sections (nested,
// collapsed, unclosed), carriage-return rewrites, SGR colors kept and every
// other escape sequence dropped. It has no UI dependencies.
package joblog

import (
	"bytes"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// MaxLines is how many lines are kept; older ones are dropped.
const MaxLines = 50000

// Line is one displayed line of the log.
type Line struct {
	Text    string // with SGR sequences
	Plain   string // without any escape sequences (search)
	Section int    // innermost section index, -1 outside sections
	Header  bool   // the title line of Section
}

// Section is a collapsible part of the log.
type Section struct {
	Name      string
	Title     string // plain text of the header line
	Parent    int    // -1 for top level
	Start     time.Time
	End       time.Time // zero while open
	Collapsed bool      // collapsed=true option
	FirstLine int       // index of the header line (may be negative after trimming)
	LastLine  int       // index of the last line inside; -1 while open
}

// Open reports whether the section has not been closed.
func (s Section) Open() bool { return s.End.IsZero() }

// Duration is End−Start, zero for open sections.
func (s Section) Duration() time.Duration {
	if s.End.IsZero() {
		return 0
	}
	return s.End.Sub(s.Start)
}

// Parser accumulates a trace. The zero value is ready to use.
type Parser struct {
	lines    []Line
	sections []Section
	stack    []int // open sections, innermost last
	pending  []byte
	// Dropped is the number of lines trimmed from the beginning.
	Dropped int
}

var markerRe = regexp.MustCompile(`^section_(start|end):(\d+):([A-Za-z0-9_.\-]+)(\[[^\]\r\n]*\])?\r?(?:\x1b\[0K)?`)

// Write feeds the next part of the trace. Only complete lines are parsed;
// the unfinished tail is kept until its newline arrives, so the result does
// not depend on how the input is split.
func (p *Parser) Write(chunk []byte) {
	p.pending = append(p.pending, chunk...)
	for {
		i := bytes.IndexByte(p.pending, '\n')
		if i < 0 {
			break
		}
		raw := string(p.pending[:i])
		p.pending = p.pending[i+1:]
		p.line(strings.TrimSuffix(raw, "\r"))
	}
	if len(p.pending) == 0 {
		p.pending = nil
	}
	p.trim()
}

// Reset clears everything (the log got shorter and is parsed again).
func (p *Parser) Reset() { *p = Parser{} }

// Lines returns the parsed lines. The slice must not be modified.
func (p *Parser) Lines() []Line { return p.lines }

// Sections returns all sections seen so far. LastLine of open sections is -1.
func (p *Parser) Sections() []Section { return p.sections }

// Partial returns the unfinished last line, or nil when the log ends with a
// newline.
func (p *Parser) Partial() *Line {
	if len(p.pending) == 0 {
		return nil
	}
	rest := string(p.pending)
	for {
		m := markerRe.FindStringSubmatchIndex(rest)
		if m == nil {
			break
		}
		rest = rest[m[1]:]
	}
	text := clean(lastRewrite(rest))
	if text == "" {
		return nil
	}
	sec := -1
	if n := len(p.stack); n > 0 {
		sec = p.stack[n-1]
	}
	return &Line{Text: text, Plain: ansi.Strip(text), Section: sec}
}

func (p *Parser) line(raw string) {
	hadMarker, onlyEnds := false, true
	startIdx := -1
	for {
		m := markerRe.FindStringSubmatch(raw)
		if m == nil {
			break
		}
		hadMarker = true
		raw = raw[len(m[0]):]
		ts := time.Time{}
		if sec, ok := atoi(m[2]); ok {
			ts = time.Unix(sec, 0).UTC()
		}
		if m[1] == "start" {
			onlyEnds = false
			startIdx = p.startSection(m[3], m[4], ts)
		} else {
			p.endSection(m[3], ts)
		}
	}
	text := clean(lastRewrite(raw))
	if hadMarker && onlyEnds && text == "" {
		return // a line made of end markers only
	}
	ln := Line{Text: text, Plain: ansi.Strip(text), Section: -1}
	if n := len(p.stack); n > 0 {
		ln.Section = p.stack[n-1]
	}
	if startIdx >= 0 {
		ln.Header = true
		if ln.Plain == "" {
			ln.Plain = p.sections[startIdx].Name
			ln.Text = ln.Plain
		}
		p.sections[startIdx].Title = ln.Plain
		p.sections[startIdx].FirstLine = len(p.lines)
	}
	p.lines = append(p.lines, ln)
}

func (p *Parser) startSection(name, opts string, ts time.Time) int {
	parent := -1
	if n := len(p.stack); n > 0 {
		parent = p.stack[n-1]
	}
	s := Section{Name: name, Parent: parent, Start: ts, FirstLine: len(p.lines), LastLine: -1,
		Collapsed: strings.Contains(opts, "collapsed=true")}
	p.sections = append(p.sections, s)
	idx := len(p.sections) - 1
	p.stack = append(p.stack, idx)
	return idx
}

// endSection closes the innermost section called name (and any unclosed
// children above it).
func (p *Parser) endSection(name string, ts time.Time) {
	for i := len(p.stack) - 1; i >= 0; i-- {
		if p.sections[p.stack[i]].Name != name {
			continue
		}
		for _, idx := range p.stack[i:] {
			s := &p.sections[idx]
			s.End, s.LastLine = ts, len(p.lines)-1
			if s.End.IsZero() {
				s.End = s.Start
			}
		}
		p.stack = p.stack[:i]
		return
	}
}

// trim drops the oldest lines beyond MaxLines and shifts section indexes.
func (p *Parser) trim() {
	over := len(p.lines) - MaxLines
	if over <= 0 {
		return
	}
	p.lines = append([]Line(nil), p.lines[over:]...)
	p.Dropped += over
	for i := range p.sections {
		s := &p.sections[i]
		s.FirstLine -= over
		if s.LastLine >= 0 {
			s.LastLine -= over
		}
	}
}

// lastRewrite keeps the text after the last carriage return (progress bars);
// a trailing empty segment falls back to the last non-empty one.
func lastRewrite(s string) string {
	if !strings.Contains(s, "\r") {
		return s
	}
	parts := strings.Split(s, "\r")
	for i := len(parts) - 1; i >= 0; i-- {
		if cleaned := ansi.Strip(parts[i]); strings.TrimSpace(cleaned) != "" {
			return parts[i]
		}
	}
	return ""
}

// clean keeps SGR sequences and printable text, dropping every other
// escape sequence and control character (tab stays).
func clean(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == 0x1b:
			i = skipEscape(s, i, &b)
		case c == '\t':
			b.WriteByte(c)
			i++
		case c < 0x20 || c == 0x7f:
			i++
		default:
			r, size := utf8.DecodeRuneInString(s[i:])
			if r == utf8.RuneError && size == 1 {
				b.WriteRune('�')
			} else {
				b.WriteString(s[i : i+size])
			}
			i += size
		}
	}
	return b.String()
}

// skipEscape consumes the escape sequence at s[i] and returns the next
// index; SGR sequences are copied to b.
func skipEscape(s string, i int, b *strings.Builder) int {
	if i+1 >= len(s) {
		return len(s)
	}
	switch s[i+1] {
	case '[': // CSI: parameters, intermediates, final byte
		j := i + 2
		for j < len(s) && s[j] >= 0x30 && s[j] <= 0x3f {
			j++
		}
		for j < len(s) && s[j] >= 0x20 && s[j] <= 0x2f {
			j++
		}
		if j >= len(s) {
			return len(s)
		}
		if s[j] == 'm' {
			b.WriteString(s[i : j+1])
		}
		return j + 1
	case ']': // OSC: until BEL or ST
		for j := i + 2; j < len(s); j++ {
			if s[j] == 0x07 {
				return j + 1
			}
			if s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\' {
				return j + 2
			}
		}
		return len(s)
	}
	// ESC, intermediate bytes (charset selection "ESC ( B"), final byte.
	j := i + 1
	for j < len(s) && s[j] >= 0x20 && s[j] <= 0x2f {
		j++
	}
	return min(j+1, len(s))
}

func atoi(s string) (int64, bool) {
	var n int64
	if s == "" {
		return 0, false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int64(c-'0')
	}
	return n, true
}

// Match is a search hit.
type Match struct{ Line int }

// Search finds lines whose plain text contains query, ignoring case.
func Search(lines []Line, query string) []Match {
	if query == "" {
		return nil
	}
	q := strings.ToLower(query)
	var out []Match
	for i, l := range lines {
		if strings.Contains(strings.ToLower(l.Plain), q) {
			out = append(out, Match{Line: i})
		}
	}
	return out
}
