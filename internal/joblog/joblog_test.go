package joblog

import (
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func trace(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/trace_sections.txt")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func parseAll(data []byte) *Parser {
	p := &Parser{}
	p.Write(data)
	return p
}

func plains(p *Parser) []string {
	var out []string
	for _, l := range p.Lines() {
		out = append(out, l.Plain)
	}
	return out
}

func TestSectionsNestedCollapsedAndUnclosed(t *testing.T) {
	p := parseAll(trace(t))
	secs := p.Sections()
	if len(secs) != 4 {
		t.Fatalf("sections = %+v", secs)
	}
	byName := map[string]Section{}
	idx := map[string]int{}
	for i, s := range secs {
		byName[s.Name] = s
		idx[s.Name] = i
	}
	prep, step, deps, open := byName["prepare_executor"], byName["step_script"], byName["deps"], byName["never_closed"]
	if prep.Title != `Preparing the "docker" executor` || prep.Parent != -1 || prep.Collapsed || prep.Duration() != 3*time.Second {
		t.Errorf("prepare = %+v", prep)
	}
	if !step.Collapsed || step.Duration() != 7*time.Second {
		t.Errorf("step_script = %+v", step)
	}
	if deps.Parent != idx["step_script"] || deps.Duration() != 5*time.Second || deps.Collapsed {
		t.Errorf("deps must nest in step_script: %+v", deps)
	}
	if !open.Open() || open.LastLine != -1 || open.Duration() != 0 {
		t.Errorf("unclosed = %+v", open)
	}
	lines := p.Lines()
	if !lines[prep.FirstLine].Header || lines[prep.FirstLine].Section != idx["prepare_executor"] {
		t.Errorf("header line = %+v", lines[prep.FirstLine])
	}
	for i := step.FirstLine + 1; i <= step.LastLine; i++ {
		if lines[i].Section != idx["step_script"] && lines[i].Section != idx["deps"] {
			t.Errorf("line %d %q outside step_script", i, lines[i].Plain)
		}
	}
	// A line after both sections closed belongs to no section.
	if l := lines[open.FirstLine]; l.Section != idx["never_closed"] || l.Plain != "Unclosed section" {
		t.Errorf("open header = %+v", l)
	}
}

func TestEndMarkerLinesAreInvisible(t *testing.T) {
	p := parseAll(trace(t))
	for _, l := range p.Lines() {
		if strings.Contains(l.Plain, "section_") {
			t.Errorf("marker leaked: %q", l.Plain)
		}
	}
}

func TestCarriageReturnKeepsLastSegment(t *testing.T) {
	p := parseAll([]byte("10%\r50%\r100%\nwindows\r\nnext\n"))
	got := plains(p)
	if !reflect.DeepEqual(got, []string{"100%", "windows", "next"}) {
		t.Fatalf("lines = %q", got)
	}
}

func TestEscapeHandling(t *testing.T) {
	p := parseAll([]byte("\x1b[31mred\x1b[0m \x1b[2K\x1b[1Aup \x1b]0;title\x07done \x1b(B\x1b[38;5;196mx\x1b[m\n"))
	l := p.Lines()[0]
	if l.Plain != "red up done x" {
		t.Fatalf("plain = %q", l.Plain)
	}
	if !strings.Contains(l.Text, "\x1b[31m") || !strings.Contains(l.Text, "\x1b[38;5;196m") || !strings.Contains(l.Text, "\x1b[m") {
		t.Fatalf("SGR must stay: %q", l.Text)
	}
	for _, bad := range []string{"\x1b[2K", "\x1b[1A", "\x1b]", "\x07", "\x1b("} {
		if strings.Contains(l.Text, bad) {
			t.Fatalf("%q must be dropped: %q", bad, l.Text)
		}
	}
}

func TestPartialLine(t *testing.T) {
	p := &Parser{}
	p.Write([]byte("one\ntw"))
	if pl := p.Partial(); pl == nil || pl.Plain != "tw" || len(p.Lines()) != 1 {
		t.Fatalf("partial = %+v lines=%d", pl, len(p.Lines()))
	}
	p.Write([]byte("o\n"))
	if p.Partial() != nil || p.Lines()[1].Plain != "two" {
		t.Fatalf("lines = %q", plains(p))
	}
}

func equalParsers(t *testing.T, a, b *Parser, what string) {
	t.Helper()
	if !reflect.DeepEqual(a.Lines(), b.Lines()) {
		t.Fatalf("%s: lines differ\n got %q\nwant %q", what, plains(a), plains(b))
	}
	if !reflect.DeepEqual(a.Sections(), b.Sections()) || a.Dropped != b.Dropped {
		t.Fatalf("%s: sections/dropped differ\n got %+v\nwant %+v", what, a.Sections(), b.Sections())
	}
	pa, pb := a.Partial(), b.Partial()
	if !reflect.DeepEqual(pa, pb) {
		t.Fatalf("%s: partial differs: %+v vs %+v", what, pa, pb)
	}
}

func TestChunkedEqualsWholeAtEveryBoundary(t *testing.T) {
	data := trace(t)
	whole := parseAll(data)
	for i := 0; i <= len(data); i++ {
		p := &Parser{}
		p.Write(data[:i])
		p.Write(data[i:])
		equalParsers(t, p, whole, fmt.Sprintf("split at %d", i))
	}
	// Byte by byte and random chunk sizes.
	p := &Parser{}
	for _, b := range data {
		p.Write([]byte{b})
	}
	equalParsers(t, p, whole, "byte by byte")
	rng := rand.New(rand.NewSource(1))
	for round := 0; round < 50; round++ {
		p := &Parser{}
		for rest := data; len(rest) > 0; {
			n := min(len(rest), 1+rng.Intn(40))
			p.Write(rest[:n])
			rest = rest[n:]
		}
		equalParsers(t, p, whole, fmt.Sprintf("random round %d", round))
	}
}

func TestTrimKeepsLastMaxLines(t *testing.T) {
	var b strings.Builder
	b.WriteString("section_start:1:big\r\x1b[0KBig section\n")
	for i := 0; i < 80000; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	b.WriteString("section_end:2:big\r\x1b[0K\n")
	p := parseAll([]byte(b.String()))
	if len(p.Lines()) != MaxLines || p.Dropped != 80001-MaxLines {
		t.Fatalf("lines=%d dropped=%d", len(p.Lines()), p.Dropped)
	}
	if last := p.Lines()[MaxLines-1].Plain; last != "line 79999" {
		t.Fatalf("last = %q", last)
	}
	s := p.Sections()[0]
	if s.LastLine != MaxLines-1 || s.FirstLine >= 0 {
		t.Fatalf("section after trim = %+v", s)
	}
	// Chunked parsing trims to the same result.
	q := &Parser{}
	data := []byte(b.String())
	for len(data) > 0 {
		n := min(len(data), 100000)
		q.Write(data[:n])
		data = data[n:]
	}
	equalParsers(t, q, p, "big chunked")
}

func TestSearch(t *testing.T) {
	p := parseAll(trace(t))
	got := Search(p.Lines(), "TIMEOUT")
	if len(got) != 1 || p.Lines()[got[0].Line].Plain != "timeout while fetching module" {
		t.Fatalf("matches = %+v", got)
	}
	if Search(p.Lines(), "") != nil || len(Search(p.Lines(), "zzz")) != 0 {
		t.Fatal("empty and missing queries")
	}
	// The match is inside the collapsed step_script > deps sections.
	if l := p.Lines()[got[0].Line]; l.Section < 0 {
		t.Fatal("match must be inside a section")
	}
	// Search sees text without colors.
	if len(Search(p.Lines(), "ERROR: Job failed")) != 1 {
		t.Fatal("colored line must be found")
	}
}

func TestResetAndInvalidUTF8(t *testing.T) {
	p := parseAll([]byte("ok\xff\xfeend\n"))
	if !strings.Contains(p.Lines()[0].Plain, "ok") || !strings.Contains(p.Lines()[0].Plain, "end") {
		t.Fatalf("plain = %q", p.Lines()[0].Plain)
	}
	p.Reset()
	if len(p.Lines()) != 0 || p.Dropped != 0 {
		t.Fatal("reset")
	}
}
