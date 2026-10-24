package mrlinks

import (
	"reflect"
	"regexp"
	"testing"

	"github.com/4aykovski/glview/internal/gitlab"
)

const origin = "https://gitlab.example.com"

func TestFromHTML(t *testing.T) {
	src := `<p>Fix <a href="https://tracker.example.com/browse/ABC-123">ABC-123</a> and <a href="/anna.k" class="gfm">@anna.k</a>
<a href="#setup" class="anchor"></a><a href="#x">skip</a> <a href="mailto:a@example.com">mail</a>
<a href="https://x.example.com/?a=1&amp;b=2"><code>x &amp;
  y</code></a><a href="/p/uploads/1">  </a></p>`
	got := FromHTML(src, origin)
	want := []gitlab.Link{
		{Text: "ABC-123", URL: "https://tracker.example.com/browse/ABC-123"},
		{Text: "@anna.k", URL: "https://gitlab.example.com/anna.k"},
		{Text: "x & y", URL: "https://x.example.com/?a=1&b=2"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v", got)
	}
}

func rule(t *testing.T, re, u string) Rule {
	t.Helper()
	return Rule{Re: regexp.MustCompile(re), URL: u}
}

func TestRuleExpand(t *testing.T) {
	r := []Rule{
		rule(t, `INC(\d+)`, "https://tracker.example.com/incident/$1"),
		rule(t, `(?P<key>[A-Z]+-\d+)`, "https://tracker.example.com/browse/${key}"),
		rule(t, `[A-Z]+-\d+`, "https://tracker.example.com/browse/$0"),
	}
	got := Collect(Source{Title: "INC42 hotfix ABC-7"}, r)
	want := []gitlab.Link{
		{Text: "INC42", URL: "https://tracker.example.com/incident/42"},
		{Text: "ABC-7", URL: "https://tracker.example.com/browse/ABC-7"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v", got)
	}
}

func TestBareURLTail(t *testing.T) {
	cases := map[string]string{
		"see https://x.example.com/a.":                    "https://x.example.com/a",
		"(https://x.example.com/a)":                       "https://x.example.com/a",
		"(https://x.example.com/a),":                      "https://x.example.com/a",
		"(https://x.example.com/a_(b))":                   "https://x.example.com/a_(b)",
		"https://x.example.com/a?x=1!":                    "https://x.example.com/a?x=1",
		"<https://x.example.com/a> and https://y.example": "https://x.example.com/a",
	}
	for in, want := range cases {
		got := Collect(Source{Title: in}, nil)
		if len(got) == 0 || got[0].URL != want || got[0].Text != want {
			t.Errorf("%q: got %+v, want %q", in, got, want)
		}
	}
}

func TestCollectOrderAndDedup(t *testing.T) {
	r := []Rule{rule(t, `[A-Z]+-\d+`, "https://tracker.example.com/browse/$0")}
	src := Source{
		Title:       "ABC-123: fix !45",
		Description: "Closes #12, see !45 and ABC-123. More https://docs.example.com/x.",
		TitleLinks:  []gitlab.Link{{Text: "!45", URL: "https://gitlab.example.com/platform/route-service/-/merge_requests/45"}},
		DescriptionLinks: []gitlab.Link{
			{Text: "#12", URL: "https://gitlab.example.com/platform/route-service/-/issues/12"},
			{Text: "!45", URL: "https://gitlab.example.com/platform/route-service/-/merge_requests/45"},
			{Text: "@gone", URL: "https://gitlab.example.com/gone"},
		},
	}
	var texts []string
	for _, l := range Collect(src, r) {
		texts = append(texts, l.Text)
	}
	want := []string{"ABC-123", "!45", "#12", "https://docs.example.com/x", "@gone"}
	if !reflect.DeepEqual(texts, want) {
		t.Errorf("got %v, want %v", texts, want)
	}
}

func TestCollectSameTextHTMLWins(t *testing.T) {
	r := []Rule{rule(t, `[A-Z]+-\d+`, "https://rules.example.com/$0")}
	src := Source{Title: "ABC-1", TitleLinks: []gitlab.Link{{Text: "ABC-1", URL: "https://tracker.example.com/ABC-1"}}}
	got := Collect(src, r)
	if len(got) != 2 || got[0].URL != "https://tracker.example.com/ABC-1" {
		t.Errorf("got %+v", got)
	}
}
