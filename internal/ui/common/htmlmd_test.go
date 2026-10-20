package common

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestHTMLToMarkdown(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"plain markdown unchanged", "## Title\n\n- a\n- b <https://x.io>\n\n`<div>` stays", "## Title\n\n- a\n- b <https://x.io>\n\n`<div>` stays"},
		{"table with th", "<table><tr><th>Компонент</th><th>Версия</th></tr><tr><td>moby/go-archive</td><td>0.2.0</td></tr></table>",
			"| Компонент | Версия |\n| --- | --- |\n| moby/go-archive | 0.2.0 |"},
		{"table without th", "<table><tr><td>a</td><td>b</td></tr><tr><td>c</td><td>d</td></tr></table>",
			"| a | b |\n| --- | --- |\n| c | d |"},
		{"pipe in cell", "<table><tr><th>x</th></tr><tr><td>a|b</td></tr></table>", "| x |\n| --- |\n| a\\|b |"},
		{"br in cell", "<table><tr><th>v</th></tr><tr><td>1<br/>2<br>3</td></tr></table>", "| v |\n| --- |\n| 1 2 3 |"},
		{"br in text", "one<br>two", "one  \ntwo"},
		{"details", "<details><summary>весь список</summary>0.3.0</details>", "▸ весь список\n\n0.3.0"},
		{"details in cell", "<table><tr><th>v</th></tr><tr><td><details><summary>CVE-1</summary><p>text</p></details></td></tr></table>", "| v |\n| --- |\n| ▸ CVE-1 text |"},
		{"inline markup", "<b>bold</b> <strong>s</strong> <i>it</i> <em>e</em> <code>x := 1</code> <a href=\"https://e.com\">link</a>",
			"**bold** **s** *it* *e* `x := 1` [link](https://e.com)"},
		{"unknown tag stripped keeps text", "<p>a <span style='x'>b</span> <blink>c</blink></p>", "a b c"},
		{"headings and paragraphs", "<h2>Head</h2><p>para</p>", "## Head\n\npara"},
		{"rowspan", "<table><tr><th>a</th><th>b</th></tr><tr><td rowspan=\"2\">x</td><td>1</td></tr><tr><td>2</td></tr></table>",
			"| a | b |\n| --- | --- |\n| x | 1 |\n|  | 2 |"},
		{"entities", "<p>a &amp;&amp; b</p>", "a && b"},
		{"fenced code untouched", "<b>x</b>\n```\n<table>\n```\n", "**x**\n```\n<table>\n```\n"},
		{"list", "<ul><li>a</li><li>b</li></ul>", "- a\n- b"},
		{"zero width removed", "<p>CVE-\u200b2026</p>", "CVE-2026"},
	}
	for _, c := range cases {
		if got := HTMLToMarkdown(c.in); got != c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
	}
}

func TestHTMLToMarkdownSecurityBotReport(t *testing.T) {
	b, err := os.ReadFile("testdata/dependency-report.html")
	if err != nil {
		t.Fatal(err)
	}
	md := HTMLToMarkdown(string(b))
	if strings.Contains(md, "<table") || strings.Contains(md, "<td") || strings.Contains(md, "<details") {
		t.Fatalf("html left:\n%s", md)
	}
	if !strings.Contains(md, "| Компонент | Версия | Безопасные версии |") {
		t.Fatalf("header row missing:\n%s", md)
	}
	if !strings.Contains(md, "[чат поддержки](https://example.com/support)") {
		t.Fatalf("markdown link lost:\n%s", md)
	}
	out := ansi.Strip(Markdown(NewStyles(TerminalTheme(true)), md, 120))
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "Компонент") {
			// Headers sit in separate cells (a narrow column may cut the word with "…").
			cells := strings.Split(l, "│")
			if len(cells) < 6 || !strings.HasPrefix(strings.TrimSpace(cells[1]), "Верс") {
				t.Fatalf("header cells glued or lost: %q", l)
			}
			return
		}
	}
	t.Fatalf("no header line in render:\n%s", out)
}
