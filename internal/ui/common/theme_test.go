package common

import (
	"image/color"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/gitlab"
)

// TestTerminalThemeIsANSIOnly checks every color role is one of the 16 ANSI
// colors, the terminal default (NoColor) or unset.
func TestTerminalThemeIsANSIOnly(t *testing.T) {
	for _, dark := range []bool{true, false} {
		th := TerminalTheme(dark)
		var bad []string
		walkColors(reflect.ValueOf(th), "Theme", &bad)
		if len(bad) > 0 {
			t.Errorf("dark=%v: non-ANSI colors: %v", dark, bad)
		}
	}
}

var colorType = reflect.TypeFor[color.Color]()

func walkColors(v reflect.Value, path string, bad *[]string) {
	switch {
	case v.Type() == colorType:
		if v.IsNil() {
			return
		}
		switch c := v.Interface().(type) {
		case lipgloss.NoColor:
		case ansi.BasicColor:
			if c > 15 {
				*bad = append(*bad, path)
			}
		default:
			*bad = append(*bad, path)
		}
	case v.Kind() == reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			walkColors(v.Field(i), path+"."+v.Type().Field(i).Name, bad)
		}
	}
}

func TestAccentNotYellowOnLight(t *testing.T) {
	if TerminalTheme(false).Accent == lipgloss.Yellow {
		t.Fatal("accent must not be yellow on light backgrounds")
	}
}

func TestStatusesHaveDistinctGlyphsAndColors(t *testing.T) {
	th := TerminalTheme(true)
	colors := map[color.Color]string{}
	for name, c := range map[string]color.Color{
		"success": th.Status.Success, "failed": th.Status.Failed, "running": th.Status.Running,
		"manual": th.Status.Manual, "canceled": th.Status.Canceled,
	} {
		if prev, dup := colors[c]; dup {
			t.Errorf("%s and %s share a color", name, prev)
		}
		colors[c] = name
	}
	glyphs := map[string]gitlab.PipelineStatus{}
	for _, st := range []gitlab.PipelineStatus{gitlab.StatusSuccess, gitlab.StatusFailed, gitlab.StatusRunning, gitlab.StatusManual, gitlab.StatusCanceled, gitlab.StatusCreated} {
		g := Glyph(st)
		if prev, dup := glyphs[g]; dup {
			t.Errorf("%s and %s share glyph %s", st, prev, g)
		}
		glyphs[g] = st
	}
}

func TestStatusRendersUntranslatedWord(t *testing.T) {
	s := NewStyles(TerminalTheme(true))
	if got := ansi.Strip(s.Status(gitlab.StatusFailed)); got != "✗ failed" {
		t.Fatalf("got %q", got)
	}
}

func TestSelectionUsesReverse(t *testing.T) {
	s := NewStyles(TerminalTheme(true))
	if !s.RowSelected.GetReverse() || !s.TabActive.GetReverse() {
		t.Fatal("selection and active tab must use reverse")
	}
}

func TestNewStylesNilColors(t *testing.T) {
	_ = NewStyles(Theme{}) // must not panic
}

// forbidden matches direct color use in screens (SPEC §9.3).
var forbidden = regexp.MustCompile(`lipgloss\.(Color\(|Black|Red|Green|Yellow|Blue|Magenta|Cyan|White|BrightBlack|BrightRed|BrightGreen|BrightYellow|BrightBlue|BrightMagenta|BrightCyan|BrightWhite)\b|\.(Foreground|Background|BorderForeground)\(`)

func colorViolations(src string) []string {
	var out []string
	for _, line := range strings.Split(src, "\n") {
		if forbidden.MatchString(strings.ReplaceAll(line, "context.Background(", "")) {
			out = append(out, strings.TrimSpace(line))
		}
	}
	return out
}

func TestNoColorsInScreens(t *testing.T) {
	if v := colorViolations(`x := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))`); len(v) == 0 {
		t.Fatal("checker must detect a violation")
	}
	if v := colorViolations(`x := st.Row.Render(title)`); len(v) != 0 {
		t.Fatal("false positive")
	}
	if v := colorViolations(`ctx, cancel := context.WithTimeout(context.Background(), time.Second)`); len(v) != 0 {
		t.Fatal("false positive")
	}

	root := filepath.Join("..", "screens")
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, v := range colorViolations(string(data)) {
			t.Errorf("%s: colors are forbidden in screens, use common.Styles: %s", path, v)
		}
		return nil
	})
}

func TestTerminalFaintThemeIsANSIOnly(t *testing.T) {
	for _, dark := range []bool{true, false} {
		var bad []string
		walkColors(reflect.ValueOf(TerminalFaintTheme(dark)), "Theme", &bad)
		if len(bad) > 0 {
			t.Errorf("dark=%v: non-ANSI colors: %v", dark, bad)
		}
	}
}

func TestFaintThemeMutedIsFaintWithoutColor(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	s := NewStyles(TerminalFaintTheme(true))
	for name, st := range map[string]lipgloss.Style{
		"Muted": s.Muted, "HeaderMuted": s.HeaderMuted, "ColumnHeader": s.ColumnHeader, "RowMuted": s.RowMuted,
		"KeyDesc": s.KeyDesc, "LogLineNo": s.LogLineNo,
		"created": s.StatusStyle(gitlab.StatusCreated), "skipped": s.StatusStyle(gitlab.StatusSkipped),
		"canceled": s.StatusStyle(gitlab.StatusCanceled),
	} {
		if !st.GetFaint() {
			t.Errorf("%s: not faint", name)
		}
		if _, none := st.GetForeground().(lipgloss.NoColor); !none {
			t.Errorf("%s: foreground = %v, want none", name, st.GetForeground())
		}
	}
	if s.Text.GetFaint() {
		t.Error("main text must not be faint")
	}
	if f := s.Markdown.Code.Faint; f == nil || !*f || s.Markdown.BlockQuote.Faint == nil || !*s.Markdown.BlockQuote.Faint {
		t.Error("markdown code and quotes must be faint")
	}
}

func TestFaintThemeBorder(t *testing.T) {
	if got := TerminalFaintTheme(true).Border; got != lipgloss.Black {
		t.Errorf("dark border = %v, want ANSI 0", got)
	}
	if got := TerminalFaintTheme(false).Border; got != TerminalTheme(false).Border {
		t.Errorf("light border = %v, want the terminal theme's", got)
	}
}

func TestTerminalThemeHasNoFaint(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	s := NewStyles(TerminalTheme(true))
	if s.Muted.GetFaint() || s.Muted.GetForeground() != lipgloss.BrightBlack || s.Markdown.Code.Faint != nil {
		t.Fatal("terminal theme must keep ANSI 8 and no Faint")
	}
}

func TestThemeByName(t *testing.T) {
	if ThemeByName("terminal-faint", true).Name != ThemeTerminalFaint || ThemeByName("terminal", true).Name != ThemeTerminal ||
		ThemeByName("nope", false).Name != ThemeTerminal || ThemeByName("", false).IsDark {
		t.Fatal("ThemeByName mismatch")
	}
}
