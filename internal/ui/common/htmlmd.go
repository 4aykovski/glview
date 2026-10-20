package common

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// htmlTag matches the tags that bots put into GitLab comments. Anything
// else (autolinks like <https://…>, generics in prose) is left alone.
var htmlTag = regexp.MustCompile(`(?i)</?(table|thead|tbody|tfoot|caption|tr|th|td|br|hr|details|summary|b|strong|i|em|code|pre|a|p|div|span|h[1-6]|ul|ol|li|blockquote|del|s|sup|sub|kbd|img)[\s/>]`)

// HTMLToMarkdown converts the HTML found in GitLab comments (tables, <br>,
// <details>, inline markup) into markdown so that glamour can render it.
// Text without such tags, and fenced code blocks, are returned unchanged.
func HTMLToMarkdown(src string) string {
	if !htmlTag.MatchString(src) {
		return src
	}
	var out, run []string
	flush := func() {
		if len(run) == 0 {
			return
		}
		seg := strings.Join(run, "\n")
		if htmlTag.MatchString(seg) {
			seg = convertWithCodeSpans(seg)
		}
		out = append(out, seg)
		run = nil
	}
	fence := false
	for _, l := range strings.Split(src, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			if !fence {
				flush()
			}
			fence = !fence
			out = append(out, l)
			continue
		}
		if fence {
			out = append(out, l)
		} else {
			run = append(run, l)
		}
	}
	flush()
	return strings.Join(out, "\n")
}

var codeSpan = regexp.MustCompile("`[^`\n]+`")

// convertWithCodeSpans hides markdown code spans (they may hold literal
// tags) behind placeholders while the HTML is converted.
func convertWithCodeSpans(seg string) string {
	var spans []string
	hidden := codeSpan.ReplaceAllStringFunc(seg, func(m string) string {
		spans = append(spans, m)
		return "\ue000" + strconv.Itoa(len(spans)-1) + "\ue001"
	})
	if !htmlTag.MatchString(hidden) {
		return seg
	}
	out := convertHTML(hidden)
	for i, m := range spans {
		out = strings.Replace(out, "\ue000"+strconv.Itoa(i)+"\ue001", m, 1)
	}
	return out
}

func convertHTML(src string) string {
	src = strings.ReplaceAll(src, "\u200b", "")
	nodes, err := html.ParseFragment(strings.NewReader(src), &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body})
	if err != nil {
		return src
	}
	c := &mdWriter{}
	for _, n := range nodes {
		c.node(n)
	}
	return strings.TrimSpace(string(c.buf))
}

// mdWriter turns an HTML tree into markdown. In inline mode (table cells)
// every block collapses into a single line.
type mdWriter struct {
	buf    []byte
	inline bool
}

func (c *mdWriter) write(s string) { c.buf = append(c.buf, s...) }

// block starts a new paragraph (a space in inline mode).
func (c *mdWriter) block() {
	if c.inline {
		c.write(" ")
		return
	}
	if len(c.buf) == 0 {
		return
	}
	n := len(c.buf)
	for n > 0 && (c.buf[n-1] == ' ' || c.buf[n-1] == '\t' || c.buf[n-1] == '\n') {
		n--
	}
	c.buf = append(c.buf[:n], '\n', '\n')
}

func (c *mdWriter) children(n *html.Node) {
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		c.node(ch)
	}
}

// sub renders the children of n in a fresh writer.
func (c *mdWriter) sub(n *html.Node, inline bool) string {
	w := &mdWriter{inline: inline}
	w.children(n)
	if inline {
		return strings.Join(strings.Fields(string(w.buf)), " ")
	}
	return strings.TrimSpace(string(w.buf))
}

func (c *mdWriter) wrap(n *html.Node, mark string) {
	in := c.sub(n, true)
	if in == "" {
		return
	}
	c.write(mark + in + mark)
}

func (c *mdWriter) node(n *html.Node) {
	switch n.Type {
	case html.TextNode:
		if c.inline {
			c.write(collapseSpaces(n.Data))
			return
		}
		c.write(n.Data)
		return
	case html.ElementNode:
	default:
		return
	}
	switch n.DataAtom {
	case atom.Script, atom.Style, atom.Head:
	case atom.Br:
		if c.inline {
			c.write(" ")
		} else {
			c.write("  \n")
		}
	case atom.Hr:
		c.block()
		if !c.inline {
			c.write("---")
			c.block()
		}
	case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
		c.block()
		if t := c.sub(n, true); t != "" {
			if !c.inline {
				c.write(strings.Repeat("#", int(n.Data[1]-'0')) + " ")
			}
			c.write(t)
		}
		c.block()
	case atom.P, atom.Div, atom.Caption, atom.Summary:
		c.block()
		c.children(n)
		c.block()
	case atom.Details:
		c.details(n)
	case atom.B, atom.Strong:
		c.wrap(n, "**")
	case atom.I, atom.Em:
		c.wrap(n, "*")
	case atom.Del, atom.S:
		c.wrap(n, "~~")
	case atom.Code, atom.Kbd:
		if t := strings.TrimSpace(textOf(n)); t != "" {
			c.write("`" + strings.Join(strings.Fields(t), " ") + "`")
		}
	case atom.A:
		in := c.sub(n, true)
		href := attr(n, "href")
		switch {
		case in == "":
		case href == "":
			c.write(in)
		default:
			c.write("[" + in + "](" + href + ")")
		}
	case atom.Img:
		c.write(attr(n, "alt"))
	case atom.Pre:
		if c.inline {
			c.write(strings.Join(strings.Fields(textOf(n)), " "))
			return
		}
		c.block()
		c.write("```\n" + strings.Trim(textOf(n), "\n") + "\n```")
		c.block()
	case atom.Ul, atom.Ol:
		c.list(n)
	case atom.Blockquote:
		in := c.sub(n, c.inline)
		if in == "" {
			return
		}
		c.block()
		if c.inline {
			c.write(in)
		} else {
			c.write("> " + strings.ReplaceAll(in, "\n", "\n> "))
		}
		c.block()
	case atom.Table:
		c.table(n)
	default:
		c.children(n)
	}
}

// details renders <summary> as "▸ title" followed by the rest.
func (c *mdWriter) details(n *html.Node) {
	c.block()
	if c.inline {
		c.inlineDetails(n)
		return
	}
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		if ch.Type == html.ElementNode && ch.DataAtom == atom.Summary {
			if t := c.sub(ch, true); t != "" {
				c.write("▸ " + t)
				c.block()
			}
			continue
		}
		c.node(ch)
	}
	c.block()
}

func (c *mdWriter) list(n *html.Node) {
	c.block()
	i := 0
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		if ch.Type != html.ElementNode || ch.DataAtom != atom.Li {
			continue
		}
		i++
		in := c.sub(ch, c.inline)
		marker := "- "
		if n.DataAtom == atom.Ol {
			marker = strconv.Itoa(i) + ". "
		}
		if c.inline {
			c.write(marker + in + " ")
			continue
		}
		c.write(marker + strings.ReplaceAll(in, "\n", "\n"+strings.Repeat(" ", len(marker))) + "\n")
	}
	c.block()
}

// table renders rows as a markdown table: the first row is the header, "|"
// in a cell is escaped, line breaks inside a cell become spaces and a
// rowspan leaves empty cells below.
func (c *mdWriter) table(n *html.Node) {
	var rows [][]string
	var caption []*html.Node
	var walk func(*html.Node)
	span := map[int]int{} // column -> rows still covered by a rowspan
	walk = func(p *html.Node) {
		for ch := p.FirstChild; ch != nil; ch = ch.NextSibling {
			if ch.Type != html.ElementNode {
				continue
			}
			switch ch.DataAtom {
			case atom.Caption:
				caption = append(caption, ch)
			case atom.Thead, atom.Tbody, atom.Tfoot:
				walk(ch)
			case atom.Tr:
				rows = append(rows, c.row(ch, span))
			}
		}
	}
	walk(n)
	for _, cp := range caption {
		c.block()
		c.write(c.sub(cp, true))
		c.block()
	}
	cols := 0
	for _, r := range rows {
		cols = max(cols, len(r))
	}
	if cols == 0 {
		return
	}
	c.block()
	if c.inline {
		for _, r := range rows {
			for _, cell := range r {
				if cell != "" {
					c.write(strings.ReplaceAll(cell, `\|`, "|") + " ")
				}
			}
		}
		c.block()
		return
	}
	for i, r := range rows {
		for len(r) < cols {
			r = append(r, "")
		}
		c.write("| " + strings.Join(r, " | ") + " |\n")
		if i == 0 {
			c.write("|" + strings.Repeat(" --- |", cols) + "\n")
		}
	}
	c.block()
}

func (c *mdWriter) row(tr *html.Node, span map[int]int) []string {
	var cells []string
	col := 0
	fill := func() {
		for span[col] > 0 {
			span[col]--
			cells = append(cells, "")
			col++
		}
	}
	for ch := tr.FirstChild; ch != nil; ch = ch.NextSibling {
		if ch.Type != html.ElementNode || (ch.DataAtom != atom.Td && ch.DataAtom != atom.Th) {
			continue
		}
		fill()
		text := c.sub(ch, true)
		text = strings.ReplaceAll(text, "|", `\|`)
		cells = append(cells, text)
		if rs, _ := strconv.Atoi(attr(ch, "rowspan")); rs > 1 {
			span[col] = rs - 1
		}
		col++
	}
	fill()
	return cells
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return strings.TrimSpace(a.Val)
		}
	}
	return ""
}

func textOf(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var sb strings.Builder
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		sb.WriteString(textOf(ch))
	}
	return sb.String()
}

// collapseSpaces folds every whitespace run into one space, keeping a
// leading and trailing space so neighbours do not stick together.
func collapseSpaces(s string) string {
	words := strings.Fields(s)
	if len(words) == 0 {
		if s == "" {
			return ""
		}
		return " "
	}
	out := strings.Join(words, " ")
	r := []rune(s)
	if unicode.IsSpace(r[0]) {
		out = " " + out
	}
	if unicode.IsSpace(r[len(r)-1]) {
		out += " "
	}
	return out
}

// maxCellDetails caps the body of a <details> inside a table cell: long
// advisories would otherwise stretch one row over the whole screen.
const maxCellDetails = 40

func (c *mdWriter) inlineDetails(n *html.Node) {
	title := ""
	w := &mdWriter{inline: true}
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		if ch.Type == html.ElementNode && ch.DataAtom == atom.Summary {
			title = c.sub(ch, true)
			continue
		}
		w.node(ch)
	}
	body := strings.Join(strings.Fields(string(w.buf)), " ")
	if r := []rune(body); len(r) > maxCellDetails {
		body = strings.TrimSpace(string(r[:maxCellDetails])) + "…"
	}
	c.write("▸ " + strings.TrimSpace(title+" "+body) + " ")
}
