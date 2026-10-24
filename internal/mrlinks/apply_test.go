package mrlinks_test

import (
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/mrlinks"
	"github.com/4aykovski/glview/internal/ui/common"
)

var (
	issue12 = gitlab.Link{Text: "#12", URL: "https://gitlab.example.com/platform/route-service/-/issues/12"}
	abc     = gitlab.Link{Text: "ABC-123", URL: "https://tracker.example.com/browse/ABC-123"}
)

func opens(s string) int { return strings.Count(s, "\x1b]8;;http") }
func closes(s string) int {
	return strings.Count(s, "\x1b]8;;\x07") + strings.Count(s, "\x1b]8;;\x1b\\")
}

func TestApplyBasic(t *testing.T) {
	got := mrlinks.Apply("Closes #12 and ABC-123.", []gitlab.Link{issue12, abc})
	if xansi.Strip(got) != "Closes #12 and ABC-123." {
		t.Fatalf("visible text changed: %q", got)
	}
	if opens(got) != 2 || closes(got) != 2 {
		t.Errorf("want 2 balanced links: %q", got)
	}
	if !strings.Contains(got, xansi.SetHyperlink(issue12.URL)+"#12"+xansi.ResetHyperlink()) {
		t.Errorf("#12 not wrapped: %q", got)
	}
}

func TestApplyWholeToken(t *testing.T) {
	one := gitlab.Link{Text: "#1", URL: "https://gitlab.example.com/i/1"}
	got := mrlinks.Apply("see #12, #1 and xABC-123", []gitlab.Link{one, abc})
	if opens(got) != 1 || !strings.Contains(got, xansi.SetHyperlink(one.URL)+"#1"+xansi.ResetHyperlink()+" and") {
		t.Errorf("got %q", got)
	}
}

func TestApplyAcrossStyles(t *testing.T) {
	s := "\x1b[1mABC\x1b[0m\x1b[31m-123\x1b[0m done"
	got := mrlinks.Apply(s, []gitlab.Link{abc})
	if xansi.Strip(got) != "ABC-123 done" || opens(got) != 1 || closes(got) != 1 {
		t.Errorf("got %q", got)
	}
}

func TestApplySkipsExistingHyperlink(t *testing.T) {
	s := xansi.SetHyperlink("https://x.example.com") + "ABC-123" + xansi.ResetHyperlink() + " ABC-123"
	got := mrlinks.Apply(s, []gitlab.Link{abc})
	if opens(got) != 2 || closes(got) != 2 || strings.Count(got, abc.URL) != 1 {
		t.Errorf("got %q", got)
	}
	// An ST-terminated glamour-style link spanning two lines.
	s = "\x1b]8;;https://x.example.com\x1b\\ABC-\n123 ABC-123\x1b]8;;\x1b\\ ABC-123"
	got = mrlinks.Apply(s, []gitlab.Link{abc})
	if strings.Count(got, abc.URL) != 1 {
		t.Errorf("got %q", got)
	}
}

func TestApplyLongerWins(t *testing.T) {
	long := gitlab.Link{Text: "ABC-123 fix", URL: "https://x.example.com/long"}
	got := mrlinks.Apply("ABC-123 fix now", []gitlab.Link{abc, long})
	if strings.Contains(got, abc.URL) || !strings.Contains(got, long.URL) {
		t.Errorf("got %q", got)
	}
}

func TestApplyMultiLine(t *testing.T) {
	got := mrlinks.Apply("#12\nnone\nx #12", []gitlab.Link{issue12})
	if opens(got) != 2 || closes(got) != 2 || xansi.Strip(got) != "#12\nnone\nx #12" {
		t.Errorf("got %q", got)
	}
}

func TestApplyOnGlamourOutput(t *testing.T) {
	st := common.NewStyles(common.TerminalTheme(true))
	md := "# Heading ABC-123\n\n- item #12\n- [link text](https://docs.example.com/x) ABC-123\n\n`ABC-123` code\n\nPlain https://docs.example.com/y text with #12.\n\n> quote ABC-123\n"
	rendered := common.Markdown(st, md, 60)
	links := []gitlab.Link{abc, issue12, {Text: "https://docs.example.com/y", URL: "https://docs.example.com/y"}}
	got := mrlinks.Apply(rendered, links)
	if xansi.Strip(got) != xansi.Strip(rendered) {
		t.Fatalf("visible text changed:\n%s\n---\n%s", xansi.Strip(got), xansi.Strip(rendered))
	}
	if opens(got) < opens(rendered)+4 {
		t.Errorf("too few links added: %d vs %d", opens(got), opens(rendered))
	}
	// Balanced: every sequence that sets a URL is followed by a reset before the next one.
	depth := 0
	for i := 0; i < len(got); i++ {
		if strings.HasPrefix(got[i:], "\x1b]8;") {
			end := strings.IndexAny(got[i+1:], "\x07\x1b")
			seq := got[i : i+1+end]
			if strings.HasSuffix(seq, ";") {
				depth--
			} else {
				depth++
			}
			if depth < 0 || depth > 1 {
				t.Fatalf("unbalanced OSC 8 at %d: %q", i, got)
			}
		}
	}
	if depth != 0 {
		t.Errorf("hyperlink left open")
	}
}
