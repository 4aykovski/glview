package common

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

const sampleMD = "# Why\n\nFix the **race** in `slotMap`.\n\n- first item\n- second item\n\n```go\nfunc main() {}\n```\n\n[docs](https://example.com)\n"

func TestMarkdownUsesOnlyAnsi16(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	for _, dark := range []bool{true, false} {
		s := NewStyles(TerminalTheme(dark))
		out := Markdown(s, sampleMD, 60)
		for _, bad := range []string{"38;5;", "38;2;", "48;5;", "48;2;"} {
			if strings.Contains(out, bad) {
				t.Errorf("dark=%v: output contains %q:\n%q", dark, bad, out)
			}
		}
		if !strings.Contains(out, "\x1b[") {
			t.Errorf("dark=%v: expected colored output", dark)
		}
		plain := ansi.Strip(out)
		for _, want := range []string{"Why", "first item", "second item", "func main() {}", "race", "slotMap"} {
			if !strings.Contains(plain, want) {
				t.Errorf("dark=%v: missing %q in\n%s", dark, want, plain)
			}
		}
		for _, raw := range []string{"# ", "`", "**", "```"} {
			if strings.Contains(plain, raw) {
				t.Errorf("dark=%v: raw markdown %q left in\n%s", dark, raw, plain)
			}
		}
		if !strings.Contains(plain, "• first item") {
			t.Errorf("dark=%v: list not rendered as bullets:\n%s", dark, plain)
		}
	}
}

func TestMarkdownNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	out := Markdown(NewStyles(TerminalTheme(true)), sampleMD, 60)
	if strings.Contains(out, "\x1b[3") || strings.Contains(out, "\x1b[9") {
		t.Errorf("NO_COLOR output has color codes: %q", out)
	}
	if !strings.Contains(ansi.Strip(out), "first item") {
		t.Error("text lost")
	}
}

func TestMarkdownWraps(t *testing.T) {
	s := NewStyles(TerminalTheme(true))
	out := ansi.Strip(Markdown(s, strings.Repeat("word ", 40), 30))
	for _, l := range strings.Split(out, "\n") {
		if len([]rune(l)) > 30 {
			t.Errorf("line longer than width: %q", l)
		}
	}
}
