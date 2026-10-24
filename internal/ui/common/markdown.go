package common

import (
	"image/color"
	"os"
	"strconv"
	"strings"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
	xansi "github.com/charmbracelet/x/ansi"
)

// markdownStyle builds the glamour style from the theme so that markdown
// follows the terminal palette: only the 16 ANSI colors, never 256-color or
// truecolor codes (SPEC §9.2). Code is not syntax-highlighted for the same
// reason.
func markdownStyle(t Theme) ansi.StyleConfig {
	if os.Getenv("NO_COLOR") != "" {
		return styles.NoTTYStyleConfig
	}
	accent, muted, link := ansiColor(t.Accent), ansiColor(t.Muted), ansiColor(t.Status.Running)
	yes := true
	// muted text: Faint in the Faint theme, the Muted color otherwise.
	var faint *bool
	if t.Faint {
		faint = &yes
	}
	zero := uint(0)
	one := uint(1)
	bar := "│ "

	heading := ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: accent, Bold: &yes, BlockSuffix: "\n"}}
	cfg := ansi.StyleConfig{
		Document:   ansi.StyleBlock{Margin: &zero},
		Paragraph:  ansi.StyleBlock{},
		BlockQuote: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: muted, Faint: faint}, Indent: &one, IndentToken: &bar},
		List:       ansi.StyleList{LevelIndent: 2},
		Heading:    heading,
		H1:         heading,
		H2:         heading,
		H3:         heading,
		H4:         heading,
		H5:         heading,
		H6:         heading,

		Emph:          ansi.StylePrimitive{Italic: &yes},
		Strong:        ansi.StylePrimitive{Bold: &yes},
		Strikethrough: ansi.StylePrimitive{CrossedOut: &yes},
		HorizontalRule: ansi.StylePrimitive{
			Color: muted, Faint: faint, Format: "\n────────\n",
		},
		Item:        ansi.StylePrimitive{BlockPrefix: "• "},
		Enumeration: ansi.StylePrimitive{BlockPrefix: ". "},
		Task:        ansi.StyleTask{Ticked: "[x] ", Unticked: "[ ] "},

		Link:     ansi.StylePrimitive{Color: link, Underline: &yes},
		LinkText: ansi.StylePrimitive{Color: link},

		Code:      ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: muted, Faint: faint}},
		CodeBlock: ansi.StyleCodeBlock{StyleBlock: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: muted, Faint: faint}, Margin: &one}},
	}
	return cfg
}

// ansiColor returns the glamour color string ("0".."15") for one of the 16
// ANSI colors, nil for "terminal default".
func ansiColor(c color.Color) *string {
	var idx int
	switch v := c.(type) {
	case xansi.BasicColor:
		idx = int(v)
	case xansi.IndexedColor:
		idx = int(v)
	default:
		return nil
	}
	if idx < 0 || idx > 15 {
		return nil
	}
	s := strconv.Itoa(idx)
	return &s
}

// Markdown renders md to ANSI text of the given width with the theme's
// style. On a rendering error the source text is returned unchanged.
func Markdown(s *Styles, md string, width int) string {
	r, err := glamour.NewTermRenderer(glamour.WithStyles(s.Markdown), glamour.WithWordWrap(max(width, 20)))
	if err != nil {
		return md
	}
	out, err := r.Render(md)
	if err != nil {
		return md
	}
	return strings.Trim(out, "\n")
}
