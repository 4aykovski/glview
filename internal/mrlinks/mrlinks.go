// Package mrlinks collects the links of a merge request (GitLab's
// titleHtml/descriptionHtml anchors, the user's config rules and bare URLs)
// and overlays them as OSC 8 hyperlinks on already rendered text.
package mrlinks

import (
	"net/url"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/4aykovski/glview/internal/gitlab"
)

// Rule turns a regexp match in a title or description into a link.
type Rule struct {
	Re  *regexp.Regexp
	URL string // regexp.Expand template: $0, $1, ${name}
}

// Source is the text of one merge request and the links GitLab found in it.
type Source struct {
	Title, Description           string
	TitleLinks, DescriptionLinks []gitlab.Link // from HTML; nil for list rows
}

// FromHTML returns the links of a GitLab-rendered HTML fragment in document
// order. Only href and the visible text of <a> are used. Anchors without
// text, fragment-only hrefs and schemes other than http(s) are skipped; a
// relative href starting with "/" gets origin (scheme://host) prepended.
func FromHTML(src, origin string) []gitlab.Link {
	nodes, err := html.ParseFragment(strings.NewReader(src), &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body})
	if err != nil {
		return nil
	}
	var out []gitlab.Link
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			if l, ok := anchor(n, origin); ok {
				out = append(out, l)
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	for _, n := range nodes {
		walk(n)
	}
	return out
}

func anchor(n *html.Node, origin string) (gitlab.Link, bool) {
	var href string
	for _, a := range n.Attr {
		if a.Key == "href" {
			href = strings.TrimSpace(a.Val)
		}
	}
	var b strings.Builder
	var text func(n *html.Node)
	text = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			text(c)
		}
	}
	text(n)
	label := strings.Join(strings.Fields(b.String()), " ")
	if label == "" || href == "" || strings.HasPrefix(href, "#") {
		return gitlab.Link{}, false
	}
	if strings.HasPrefix(href, "/") && !strings.HasPrefix(href, "//") {
		href = strings.TrimRight(origin, "/") + href
	}
	if !httpURL(href) {
		return gitlab.Link{}, false
	}
	return gitlab.Link{Text: label, URL: href}, true
}

// httpURL reports whether s is an absolute http(s) URL with a host.
func httpURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

var bareRe = regexp.MustCompile("https?://[^\\s<>\"'`]+")

// bareURLs returns the bare http(s) URLs of text with the trailing
// punctuation and unpaired closing brackets cut off.
func bareURLs(text string) [][2]int {
	var out [][2]int
	for _, m := range bareRe.FindAllStringIndex(text, -1) {
		end := trimURLTail(text[m[0]:m[1]]) + m[0]
		if end > m[0] && httpURL(text[m[0]:end]) {
			out = append(out, [2]int{m[0], end})
		}
	}
	return out
}

// trimURLTail returns the length of s without trailing sentence
// punctuation and closing brackets that have no opener inside s.
func trimURLTail(s string) int {
	for len(s) > 0 {
		c := s[len(s)-1]
		switch c {
		case '.', ',', ';', ':', '!', '?':
			s = s[:len(s)-1]
			continue
		case ')', ']', '}', '>':
			open := map[byte]byte{')': '(', ']': '[', '}': '{', '>': '<'}[c]
			if strings.Count(s, string(open)) < strings.Count(s, string(c)) {
				s = s[:len(s)-1]
				continue
			}
		}
		break
	}
	return len(s)
}

type found struct {
	link gitlab.Link
	pos  int // position in the block text; -1 = not found
}

// block collects the links of one text (title or description) in text
// order: HTML links first, then rule matches, then bare URLs; the sort by
// position is stable, so at equal positions that order is kept.
func block(text string, htmlLinks []gitlab.Link, rules []Rule) []gitlab.Link {
	var all []found
	for _, l := range htmlLinks {
		all = append(all, found{l, strings.Index(text, l.Text)})
	}
	for _, r := range rules {
		if r.Re == nil {
			continue
		}
		for _, m := range r.Re.FindAllStringSubmatchIndex(text, -1) {
			if m[1] == m[0] {
				continue
			}
			u := string(r.Re.ExpandString(nil, r.URL, text, m))
			if httpURL(u) {
				all = append(all, found{gitlab.Link{Text: text[m[0]:m[1]], URL: u}, m[0]})
			}
		}
	}
	for _, m := range bareURLs(text) {
		u := text[m[0]:m[1]]
		all = append(all, found{gitlab.Link{Text: u, URL: u}, m[0]})
	}
	sort.SliceStable(all, func(i, j int) bool {
		a, b := all[i].pos, all[j].pos
		if a < 0 || b < 0 {
			return b < 0 && a >= 0
		}
		return a < b
	})
	out := make([]gitlab.Link, len(all))
	for i, f := range all {
		out[i] = f.link
	}
	return out
}

// Collect builds the common link list of a merge request: the title block,
// then the description block; URL duplicates keep their first occurrence.
func Collect(src Source, rules []Rule) []gitlab.Link {
	var out []gitlab.Link
	seen := map[string]bool{}
	for _, l := range append(block(src.Title, src.TitleLinks, rules), block(src.Description, src.DescriptionLinks, rules)...) {
		if !seen[l.URL] {
			seen[l.URL] = true
			out = append(out, l)
		}
	}
	return out
}
