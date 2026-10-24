package diff

import (
	"reflect"
	"testing"

	"github.com/4aykovski/glview/internal/gitlab"
)

func TestParse(t *testing.T) {
	src := "@@ -40,3 +40,4 @@ func alloc() {\n ctx\n-gone\n+new1\n+new2\n"
	want := []Line{
		{Kind: Hunk, Text: "@@ -40,3 +40,4 @@ func alloc() {"},
		{Kind: Context, Old: 40, New: 40, Text: "ctx"},
		{Kind: Del, Old: 41, Text: "gone"},
		{Kind: Add, New: 41, Text: "new1"},
		{Kind: Add, New: 42, Text: "new2"},
	}
	if got := Parse(src); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

func TestParseHeaderWithoutCounts(t *testing.T) {
	got := Parse("@@ -1 +1 @@\n-a\n+b\n")
	if len(got) != 3 || got[1].Old != 1 || got[2].New != 1 {
		t.Fatalf("got %+v", got)
	}
}

func TestParseSeveralHunks(t *testing.T) {
	got := Parse("@@ -1,2 +1,2 @@\n a\n-b\n+c\n@@ -50,2 +60,2 @@\n x\n y\n")
	if len(got) != 7 {
		t.Fatalf("got %+v", got)
	}
	if got[4].Kind != Hunk || got[5].Old != 50 || got[5].New != 60 || got[6].Old != 51 || got[6].New != 61 {
		t.Errorf("second hunk numbering: %+v", got[4:])
	}
}

func TestParseNoNewline(t *testing.T) {
	got := Parse("@@ -1 +1 @@\n-a\n\\ No newline at end of file\n+b\n")
	if got[2].Kind != NoNewline || got[2].Old != 0 || got[2].New != 0 || got[2].Text != "\\ No newline at end of file" {
		t.Errorf("marker = %+v", got[2])
	}
	if got[3].New != 1 {
		t.Errorf("numbering after marker: %+v", got[3])
	}
}

func TestParseEmptyAndJunk(t *testing.T) {
	if got := Parse(""); got != nil {
		t.Errorf("empty = %+v", got)
	}
	if got := Parse("Binary files differ\n"); len(got) != 0 {
		t.Errorf("junk before a header must be skipped: %+v", got)
	}
}

func TestParseEmptyContextLine(t *testing.T) {
	// Some servers strip the single space of an empty context line.
	got := Parse("@@ -1,3 +1,3 @@\n a\n\n c\n")
	if len(got) != 4 || got[2].Kind != Context || got[2].Old != 2 || got[2].New != 2 || got[2].Text != "" {
		t.Errorf("got %+v", got)
	}
}

func TestCounts(t *testing.T) {
	a, d := Counts(Parse("@@ -1,2 +1,3 @@\n a\n-b\n+c\n+d\n"))
	if a != 2 || d != 1 {
		t.Errorf("counts = +%d -%d", a, d)
	}
}

func TestFind(t *testing.T) {
	lines := Parse("@@ -40,3 +40,4 @@\n ctx\n-gone\n+new1\n+new2\n")
	cases := []struct {
		name string
		pos  gitlab.NotePosition
		want int
	}{
		{"added by new line", gitlab.NotePosition{NewLine: 42}, 4},
		{"context by new line", gitlab.NotePosition{NewLine: 40}, 1},
		{"removed by old line", gitlab.NotePosition{OldLine: 41}, 2},
		{"new line wins over old", gitlab.NotePosition{OldLine: 41, NewLine: 41}, 3},
		{"not found", gitlab.NotePosition{NewLine: 300}, -1},
		{"old line of an added number is not a match", gitlab.NotePosition{OldLine: 42}, -1},
		{"no numbers", gitlab.NotePosition{}, -1},
	}
	for _, c := range cases {
		if got := Find(lines, c.pos); got != c.want {
			t.Errorf("%s: Find = %d, want %d", c.name, got, c.want)
		}
	}
}
