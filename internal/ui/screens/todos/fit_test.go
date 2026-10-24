package todos

import (
	"testing"

	"github.com/4aykovski/glview/internal/i18n"
)

// TestFitsTerminal is the SPEC §12.3 invariant for the To-Do screen and
// what it opens: at 80×24 and 120×40, in en and ru, no line is wider than
// the terminal and no view is taller.
func TestFitsTerminal(t *testing.T) {
	steps := []struct {
		name string
		keys []string
	}{
		{"list", nil},
		{"mention", []string{"j"}},
		{"help", []string{"?"}},
		{"confirm all", []string{"esc", "X"}},
		{"type filter", []string{"esc", "t"}},
		{"done", []string{"s"}},
		{"empty", []string{"t", "t"}},
		{"reset", []string{"backspace", "j", "j", "j"}},
		{"mr detail", []string{"g", "enter"}},
		{"mr detail help", []string{"?"}},
		{"issue", []string{"esc", "esc", "j", "enter"}},
	}
	for name, lang := range map[string]i18n.Lang{"en": i18n.EN, "ru": i18n.RU} {
		t.Run(name, func(t *testing.T) {
			d := openLang(t, todoFake(), lang).d
			for _, st := range steps {
				d.Key(st.keys...)
				d.RequireFits()
				if t.Failed() {
					t.Fatalf("step %q", st.name)
				}
			}
		})
	}
}
